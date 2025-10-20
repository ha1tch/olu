package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// SQLiteStore implements Store interface using SQLite database
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
	config SQLiteConfig
}

// SQLiteConfig holds SQLite-specific configuration
type SQLiteConfig struct {
	DBPath           string
	EnableWAL        bool // Write-Ahead Logging for better concurrency
	EnableForeignKeys bool
	CacheSize        int  // Page cache size in KB
	BusyTimeout      int  // Milliseconds to wait on locked database
}

// NewSQLiteStore creates a new SQLite-based storage
func NewSQLiteStore(dbPath string, config SQLiteConfig) (*SQLiteStore, error) {
	if dbPath == "" {
		dbPath = "olu.db"
	}
	
	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	
	store := &SQLiteStore{
		db:     db,
		dbPath: dbPath,
		config: config,
	}
	
	// Initialize database schema
	if err := store.initialize(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	
	return store, nil
}

// initialize creates the necessary tables and triggers
func (s *SQLiteStore) initialize(ctx context.Context) error {
	// Apply pragmas for performance and consistency
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA cache_size = -%d", s.config.CacheSize),
		fmt.Sprintf("PRAGMA busy_timeout = %d", s.config.BusyTimeout),
	}
	
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("failed to set pragma: %w", err)
		}
	}
	
	// Create schema
	schema := `
		-- Main entities table (JSON blob approach)
		CREATE TABLE IF NOT EXISTS entities (
			entity_type TEXT NOT NULL,
			id INTEGER NOT NULL,
			data TEXT NOT NULL, -- JSON stored as TEXT
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (entity_type, id)
		);
		
		CREATE INDEX IF NOT EXISTS idx_entity_type ON entities(entity_type);
		CREATE INDEX IF NOT EXISTS idx_updated_at ON entities(updated_at);
		
		-- Graph relationships table (auto-synced via triggers)
		CREATE TABLE IF NOT EXISTS graph_edges (
			source_entity TEXT NOT NULL,
			source_id INTEGER NOT NULL,
			target_entity TEXT NOT NULL,
			target_id INTEGER NOT NULL,
			relationship_name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (source_entity, source_id, target_entity, target_id, relationship_name)
		);
		
		CREATE INDEX IF NOT EXISTS idx_graph_source ON graph_edges(source_entity, source_id);
		CREATE INDEX IF NOT EXISTS idx_graph_target ON graph_edges(target_entity, target_id);
		CREATE INDEX IF NOT EXISTS idx_graph_relationship ON graph_edges(relationship_name);
		
		-- ID sequences table (replaces _next_id.json files)
		CREATE TABLE IF NOT EXISTS entity_sequences (
			entity_type TEXT PRIMARY KEY,
			next_id INTEGER NOT NULL DEFAULT 1
		);
		
		-- Schema metadata table (optional schema storage)
		CREATE TABLE IF NOT EXISTS schemas (
			entity_type TEXT PRIMARY KEY,
			schema TEXT NOT NULL, -- JSON schema
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		
		-- Version tracking for migrations
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`
	
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	
	// Create triggers for automatic graph synchronization
	if err := s.createGraphTriggers(ctx); err != nil {
		return fmt.Errorf("failed to create triggers: %w", err)
	}
	
	// Mark current schema version
	if _, err := s.db.ExecContext(ctx, 
		"INSERT OR IGNORE INTO schema_version (version) VALUES (1)"); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}
	
	return nil
}

// createGraphTriggers creates triggers to automatically sync graph_edges with REF fields in JSON
func (s *SQLiteStore) createGraphTriggers(ctx context.Context) error {
	// NOTE: Graph synchronization strategy
	// =====================================
	// We use MANUAL graph synchronization instead of triggers for the following reasons:
	//
	// 1. Reliability: json_each() in triggers can cause "malformed JSON" errors in some
	//    SQLite builds, particularly with the pure-Go modernc.org/sqlite driver.
	//
	// 2. Integrity is maintained through transactions:
	//    - All CRUD operations (Create/Update/Patch/Delete/Save) use transactions
	//    - Graph sync happens within the SAME transaction as the document operation
	//    - If either operation fails, the entire transaction rolls back
	//    - This provides ACID guarantees equivalent to triggers
	//
	// 3. Explicit control: Manual sync makes the graph update logic visible and debuggable,
	//    and allows for easier testing and modification.
	//
	// The syncGraphEdges() method is called within every transaction that modifies documents,
	// ensuring document-graph consistency is always maintained atomically.
	
	return nil
}

// Info returns store information
func (s *SQLiteStore) Info() StoreInfo {
	return StoreInfo{
		Type:                "sqlite",
		Version:             "1.0.0",
		SupportsSearch:      true,
		SupportsBatch:       true,
		SupportsTransaction: true,
	}
}

// Create inserts a new entity with auto-generated ID
func (s *SQLiteStore) Create(ctx context.Context, entity string, data map[string]interface{}) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	
	// Get next ID
	var nextID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO entity_sequences (entity_type, next_id) 
		VALUES (?, 1)
		ON CONFLICT(entity_type) DO UPDATE SET next_id = next_id + 1
		RETURNING next_id
	`, entity).Scan(&nextID)
	if err != nil {
		return 0, fmt.Errorf("failed to get next ID: %w", err)
	}
	
	// Create a copy of data to avoid mutating input
	dataCopy := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		dataCopy[k] = v
	}
	dataCopy["id"] = nextID
	
	// Marshal to JSON
	jsonData, err := json.Marshal(dataCopy)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal data: %w", err)
	}
	
	// Insert entity
	_, err = tx.ExecContext(ctx, `
		INSERT INTO entities (entity_type, id, data) 
		VALUES (?, ?, ?)
	`, entity, nextID, string(jsonData))
	if err != nil {
		return 0, fmt.Errorf("failed to insert entity: %w", err)
	}
	
	// Manually sync graph edges
	if err := s.syncGraphEdges(ctx, tx, entity, nextID, dataCopy); err != nil {
		return 0, fmt.Errorf("failed to sync graph: %w", err)
	}
	
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}
	
	return nextID, nil
}

// syncGraphEdges extracts REF fields and creates graph edges
func (s *SQLiteStore) syncGraphEdges(ctx context.Context, tx *sql.Tx, sourceEntity string, sourceID int, data map[string]interface{}) error {
	// First, delete old edges from this entity
	_, err := tx.ExecContext(ctx, `
		DELETE FROM graph_edges 
		WHERE source_entity = ? AND source_id = ?
	`, sourceEntity, sourceID)
	if err != nil {
		return err
	}
	
	// Extract and insert REF fields
	for key, value := range data {
		if key == "id" {
			continue
		}
		
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		
		refType, _ := valueMap["type"].(string)
		if refType != "REF" {
			continue
		}
		
		targetEntity, _ := valueMap["entity"].(string)
		if targetEntity == "" {
			continue
		}
		
		// Handle both float64 (from JSON unmarshal) and int
		var targetID int
		switch v := valueMap["id"].(type) {
		case float64:
			targetID = int(v)
		case int:
			targetID = v
		default:
			continue
		}
		
		if targetID == 0 {
			continue
		}
		
		// Insert edge
		_, err := tx.ExecContext(ctx, `
			INSERT INTO graph_edges (source_entity, source_id, target_entity, target_id, relationship_name)
			VALUES (?, ?, ?, ?, ?)
		`, sourceEntity, sourceID, targetEntity, targetID, key)
		if err != nil {
			return err
		}
	}
	
	return nil
}

// Get retrieves an entity by ID
func (s *SQLiteStore) Get(ctx context.Context, entity string, id int) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var jsonData string
	err := s.db.QueryRowContext(ctx, `
		SELECT data FROM entities 
		WHERE entity_type = ? AND id = ?
	`, entity, id).Scan(&jsonData)
	
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query entity: %w", err)
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	
	return result, nil
}

// Update replaces an entity completely
func (s *SQLiteStore) Update(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Create a copy to avoid mutating input
	dataCopy := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		dataCopy[k] = v
	}
	dataCopy["id"] = id
	
	// Marshal to JSON
	jsonData, err := json.Marshal(dataCopy)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	
	// Update entity
	result, err := tx.ExecContext(ctx, `
		UPDATE entities 
		SET data = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE entity_type = ? AND id = ?
	`, string(jsonData), entity, id)
	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	
	// Manually sync graph edges
	if err := s.syncGraphEdges(ctx, tx, entity, id, dataCopy); err != nil {
		return fmt.Errorf("failed to sync graph: %w", err)
	}
	
	return tx.Commit()
}

// Patch partially updates an entity
func (s *SQLiteStore) Patch(ctx context.Context, entity string, id int, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Get existing data directly (we already hold the lock)
	var jsonData string
	err = tx.QueryRowContext(ctx, `
		SELECT data FROM entities 
		WHERE entity_type = ? AND id = ?
	`, entity, id).Scan(&jsonData)
	
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to query entity: %w", err)
	}
	
	var existing map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &existing); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}
	
	// Merge updates into existing data
	for key, value := range updates {
		if key != "id" {
			if value == nil {
				delete(existing, key)
			} else {
				existing[key] = value
			}
		}
	}
	
	// Ensure ID is set
	existing["id"] = id
	
	// Marshal back to JSON
	updatedJSON, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	
	// Update directly (we already hold the lock)
	result, err := tx.ExecContext(ctx, `
		UPDATE entities 
		SET data = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE entity_type = ? AND id = ?
	`, string(updatedJSON), entity, id)
	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	
	// Manually sync graph edges
	if err := s.syncGraphEdges(ctx, tx, entity, id, existing); err != nil {
		return fmt.Errorf("failed to sync graph: %w", err)
	}
	
	return tx.Commit()
}

// Delete removes an entity
func (s *SQLiteStore) Delete(ctx context.Context, entity string, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Delete entity
	result, err := tx.ExecContext(ctx, `
		DELETE FROM entities 
		WHERE entity_type = ? AND id = ?
	`, entity, id)
	if err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	
	// Clean up graph edges
	_, err = tx.ExecContext(ctx, `
		DELETE FROM graph_edges 
		WHERE (source_entity = ? AND source_id = ?)
		   OR (target_entity = ? AND target_id = ?)
	`, entity, id, entity, id)
	if err != nil {
		return fmt.Errorf("failed to delete graph edges: %w", err)
	}
	
	return tx.Commit()
}

// Save creates an entity with a specific ID (fails if exists)
func (s *SQLiteStore) Save(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if exists directly (we already hold the lock)
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM entities WHERE entity_type = ? AND id = ?)
	`, entity, id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check existence: %w", err)
	}
	
	if exists {
		return ErrAlreadyExists
	}
	
	// Create a copy to avoid mutating input
	dataCopy := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		dataCopy[k] = v
	}
	dataCopy["id"] = id
	
	// Marshal to JSON
	jsonData, err := json.Marshal(dataCopy)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Update sequence if needed
	_, err = tx.ExecContext(ctx, `
		INSERT INTO entity_sequences (entity_type, next_id) 
		VALUES (?, ?)
		ON CONFLICT(entity_type) DO UPDATE 
		SET next_id = MAX(next_id, excluded.next_id + 1)
	`, entity, id+1)
	if err != nil {
		return fmt.Errorf("failed to update sequence: %w", err)
	}
	
	// Insert entity
	_, err = tx.ExecContext(ctx, `
		INSERT INTO entities (entity_type, id, data) 
		VALUES (?, ?, ?)
	`, entity, id, string(jsonData))
	if err != nil {
		return fmt.Errorf("failed to save entity: %w", err)
	}
	
	// Manually sync graph edges
	if err := s.syncGraphEdges(ctx, tx, entity, id, dataCopy); err != nil {
		return fmt.Errorf("failed to sync graph: %w", err)
	}
	
	return tx.Commit()
}

// List returns all entities of a given type
func (s *SQLiteStore) List(ctx context.Context, entity string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	rows, err := s.db.QueryContext(ctx, `
		SELECT data FROM entities 
		WHERE entity_type = ?
		ORDER BY id
	`, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to list entities: %w", err)
	}
	defer rows.Close()
	
	var results []map[string]interface{}
	for rows.Next() {
		var jsonData string
		if err := rows.Scan(&jsonData); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal data: %w", err)
		}
		
		results = append(results, data)
	}
	
	return results, rows.Err()
}

// Exists checks if an entity exists
func (s *SQLiteStore) Exists(ctx context.Context, entity string, id int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM entities WHERE entity_type = ? AND id = ?)
	`, entity, id).Scan(&exists)
	
	return err == nil && exists
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Search implements field-based search using JSON extraction
func (s *SQLiteStore) Search(ctx context.Context, entity string, field string, query string, matchType string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var sqlQuery string
	var args []interface{}
	
	switch matchType {
	case "exact":
		sqlQuery = `
			SELECT data FROM entities 
			WHERE entity_type = ? 
			  AND json_extract(data, '$.' || ?) = ?
			ORDER BY id
		`
		args = []interface{}{entity, field, query}
		
	case "contains":
		sqlQuery = `
			SELECT data FROM entities 
			WHERE entity_type = ? 
			  AND json_extract(data, '$.' || ?) LIKE ?
			ORDER BY id
		`
		args = []interface{}{entity, field, "%" + query + "%"}
		
	case "starts":
		sqlQuery = `
			SELECT data FROM entities 
			WHERE entity_type = ? 
			  AND json_extract(data, '$.' || ?) LIKE ?
			ORDER BY id
		`
		args = []interface{}{entity, field, query + "%"}
		
	case "ends":
		sqlQuery = `
			SELECT data FROM entities 
			WHERE entity_type = ? 
			  AND json_extract(data, '$.' || ?) LIKE ?
			ORDER BY id
		`
		args = []interface{}{entity, field, "%" + query}
		
	default:
		return nil, fmt.Errorf("invalid match type: %s", matchType)
	}
	
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()
	
	var results []map[string]interface{}
	for rows.Next() {
		var jsonData string
		if err := rows.Scan(&jsonData); err != nil {
			return nil, err
		}
		
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			return nil, err
		}
		
		results = append(results, data)
	}
	
	return results, rows.Err()
}

// GetNeighbors returns graph neighbors for an entity
func (s *SQLiteStore) GetNeighbors(ctx context.Context, entity string, id int, direction string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var query string
	if direction == "out" {
		query = `
			SELECT e.entity_type, e.id, e.data, g.relationship_name
			FROM graph_edges g
			JOIN entities e ON e.entity_type = g.target_entity AND e.id = g.target_id
			WHERE g.source_entity = ? AND g.source_id = ?
		`
	} else if direction == "in" {
		query = `
			SELECT e.entity_type, e.id, e.data, g.relationship_name
			FROM graph_edges g
			JOIN entities e ON e.entity_type = g.source_entity AND e.id = g.source_id
			WHERE g.target_entity = ? AND g.target_id = ?
		`
	} else {
		return nil, fmt.Errorf("invalid direction: %s (must be 'in' or 'out')", direction)
	}
	
	rows, err := s.db.QueryContext(ctx, query, entity, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get neighbors: %w", err)
	}
	defer rows.Close()
	
	var results []map[string]interface{}
	for rows.Next() {
		var entityType string
		var entityID int
		var jsonData string
		var relationship string
		
		if err := rows.Scan(&entityType, &entityID, &jsonData, &relationship); err != nil {
			return nil, err
		}
		
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			return nil, err
		}
		
		// Add metadata
		data["_neighbor_type"] = entityType
		data["_relationship"] = relationship
		data["_direction"] = direction
		
		results = append(results, data)
	}
	
	return results, rows.Err()
}

// VerifyGraphIntegrity checks if graph_edges matches JSON REF fields
func (s *SQLiteStore) VerifyGraphIntegrity(ctx context.Context) error {
	// This is a health check function
	// Compare expected edges (from JSON) vs actual edges (in table)
	
	// Get all entities
	rows, err := s.db.QueryContext(ctx, "SELECT entity_type, id, data FROM entities")
	if err != nil {
		return err
	}
	defer rows.Close()
	
	expectedEdges := make(map[string]bool)
	for rows.Next() {
		var entity string
		var id int
		var jsonData string
		
		if err := rows.Scan(&entity, &id, &jsonData); err != nil {
			return err
		}
		
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			continue
		}
		
		// Extract REF fields
		for key, value := range data {
			if valueMap, ok := value.(map[string]interface{}); ok {
				if valueMap["type"] == "REF" {
					targetEntity, _ := valueMap["entity"].(string)
					targetID, _ := valueMap["id"].(float64)
					if targetEntity != "" && targetID > 0 {
						edgeKey := fmt.Sprintf("%s:%d:%s:%d:%s", 
							entity, id, targetEntity, int(targetID), key)
						expectedEdges[edgeKey] = true
					}
				}
			}
		}
	}
	
	// Get actual edges
	actualRows, err := s.db.QueryContext(ctx, 
		"SELECT source_entity, source_id, target_entity, target_id, relationship_name FROM graph_edges")
	if err != nil {
		return err
	}
	defer actualRows.Close()
	
	actualEdges := make(map[string]bool)
	for actualRows.Next() {
		var source, target, rel string
		var sourceID, targetID int
		
		if err := actualRows.Scan(&source, &sourceID, &target, &targetID, &rel); err != nil {
			return err
		}
		
		edgeKey := fmt.Sprintf("%s:%d:%s:%d:%s", source, sourceID, target, targetID, rel)
		actualEdges[edgeKey] = true
	}
	
	// Compare
	for edge := range expectedEdges {
		if !actualEdges[edge] {
			return fmt.Errorf("graph integrity error: missing edge: %s", edge)
		}
	}
	
	for edge := range actualEdges {
		if !expectedEdges[edge] {
			return fmt.Errorf("graph integrity error: unexpected edge: %s", edge)
		}
	}
	
	return nil
}

// RebuildGraph rebuilds the graph_edges table from JSON data
func (s *SQLiteStore) RebuildGraph(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Clear graph_edges
	if _, err := tx.ExecContext(ctx, "DELETE FROM graph_edges"); err != nil {
		return err
	}
	
	// Rebuild from entities
	rows, err := tx.QueryContext(ctx, "SELECT entity_type, id, data FROM entities")
	if err != nil {
		return err
	}
	defer rows.Close()
	
	for rows.Next() {
		var entity string
		var id int
		var jsonData string
		
		if err := rows.Scan(&entity, &id, &jsonData); err != nil {
			return err
		}
		
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			continue
		}
		
		// Extract and insert REF fields
		for key, value := range data {
			if valueMap, ok := value.(map[string]interface{}); ok {
				if valueMap["type"] == "REF" {
					targetEntity, _ := valueMap["entity"].(string)
					targetIDFloat, _ := valueMap["id"].(float64)
					targetID := int(targetIDFloat)
					
					if targetEntity != "" && targetID > 0 {
						_, err := tx.ExecContext(ctx, `
							INSERT INTO graph_edges 
							(source_entity, source_id, target_entity, target_id, relationship_name)
							VALUES (?, ?, ?, ?, ?)
						`, entity, id, targetEntity, targetID, key)
						
						if err != nil {
							return fmt.Errorf("failed to insert edge: %w", err)
						}
					}
				}
			}
		}
	}
	
	return tx.Commit()
}
