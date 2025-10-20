package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// JSONFileStore implements Store interface using JSON files
type JSONFileStore struct {
	baseDir   string
	schema    string
	idLocks   map[string]*sync.Mutex
	idMutex   sync.RWMutex
	entityMux sync.RWMutex
}

// NewJSONFileStore creates a new JSON file-based storage
func NewJSONFileStore(baseDir, schema string) (*JSONFileStore, error) {
	schemaPath := filepath.Join(baseDir, schema)
	if err := os.MkdirAll(schemaPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create schema directory: %w", err)
	}
	
	return &JSONFileStore{
		baseDir: baseDir,
		schema:  schema,
		idLocks: make(map[string]*sync.Mutex),
	}, nil
}

// Info returns store information
func (s *JSONFileStore) Info() StoreInfo {
	return StoreInfo{
		Type:                "jsonfile",
		Version:             "1.0.0",
		SupportsSearch:      true,
		SupportsBatch:       false,
		SupportsTransaction: false,
	}
}

// getIDLock gets or creates a mutex for an entity's ID generation
func (s *JSONFileStore) getIDLock(entity string) *sync.Mutex {
	s.idMutex.Lock()
	defer s.idMutex.Unlock()
	
	if lock, exists := s.idLocks[entity]; exists {
		return lock
	}
	
	lock := &sync.Mutex{}
	s.idLocks[entity] = lock
	return lock
}

// GetEntityDir returns the directory path for an entity
func (s *JSONFileStore) GetEntityDir(entity string) string {
	return filepath.Join(s.baseDir, s.schema, entity)
}

// getEntityFile returns the file path for a specific entity instance
func (s *JSONFileStore) getEntityFile(entity string, id int) string {
	return filepath.Join(s.GetEntityDir(entity), fmt.Sprintf("%d.json", id))
}

// getNextIDFile returns the file path for storing the next ID
func (s *JSONFileStore) getNextIDFile(entity string) string {
	return filepath.Join(s.GetEntityDir(entity), "_next_id.json")
}

// NextID gets the next available ID for an entity
func (s *JSONFileStore) NextID(ctx context.Context, entity string) (int, error) {
	lock := s.getIDLock(entity)
	lock.Lock()
	defer lock.Unlock()
	
	entityDir := s.GetEntityDir(entity)
	if err := os.MkdirAll(entityDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create entity directory: %w", err)
	}
	
	nextIDFile := s.getNextIDFile(entity)
	
	// Read current next ID
	var nextID int = 1
	if data, err := os.ReadFile(nextIDFile); err == nil {
		var idData struct {
			NextID int `json:"next_id"`
		}
		if err := json.Unmarshal(data, &idData); err == nil {
			nextID = idData.NextID
		}
	}
	
	// Write incremented ID
	idData := struct {
		NextID int `json:"next_id"`
	}{NextID: nextID + 1}
	
	data, err := json.Marshal(idData)
	if err != nil {
		return 0, err
	}
	
	if err := os.WriteFile(nextIDFile, data, 0644); err != nil {
		return 0, err
	}
	
	return nextID, nil
}

// Create creates a new entity with auto-generated ID
func (s *JSONFileStore) Create(ctx context.Context, entity string, data map[string]interface{}) (int, error) {
	id, err := s.NextID(ctx, entity)
	if err != nil {
		return 0, err
	}
	
	data["id"] = id
	
	filePath := s.getEntityFile(entity, id)
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return 0, err
	}
	
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return 0, err
	}
	
	return id, nil
}

// Get retrieves an entity by ID
func (s *JSONFileStore) Get(ctx context.Context, entity string, id int) (map[string]interface{}, error) {
	filePath := s.getEntityFile(entity, id)
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s with id %d", ErrNotFound, entity, id)
		}
		return nil, err
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	
	return result, nil
}

// Update replaces an entity completely
func (s *JSONFileStore) Update(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	filePath := s.getEntityFile(entity, id)
	
	if !s.Exists(ctx, entity, id) {
		return fmt.Errorf("%w: %s with id %d", ErrNotFound, entity, id)
	}
	
	data["id"] = id
	
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, jsonData, 0644)
}

// Patch partially updates an entity
func (s *JSONFileStore) Patch(ctx context.Context, entity string, id int, patchData map[string]interface{}) error {
	existing, err := s.Get(ctx, entity, id)
	if err != nil {
		return err
	}
	
	// Merge patch data into existing data
	for k, v := range patchData {
		if k != "id" {
			existing[k] = v
		}
	}
	
	return s.Update(ctx, entity, id, existing)
}

// Delete removes an entity
func (s *JSONFileStore) Delete(ctx context.Context, entity string, id int) error {
	filePath := s.getEntityFile(entity, id)
	
	if !s.Exists(ctx, entity, id) {
		return fmt.Errorf("%w: %s with id %d", ErrNotFound, entity, id)
	}
	
	return os.Remove(filePath)
}

// Save saves an entity with a specific ID (creates if doesn't exist)
func (s *JSONFileStore) Save(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	if s.Exists(ctx, entity, id) {
		return fmt.Errorf("%w: %s with id %d", ErrAlreadyExists, entity, id)
	}
	
	entityDir := s.GetEntityDir(entity)
	if err := os.MkdirAll(entityDir, 0755); err != nil {
		return fmt.Errorf("failed to create entity directory: %w", err)
	}
	
	data["id"] = id
	filePath := s.getEntityFile(entity, id)
	
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, jsonData, 0644)
}

// List returns all entities of a given type
func (s *JSONFileStore) List(ctx context.Context, entity string) ([]map[string]interface{}, error) {
	entityDir := s.GetEntityDir(entity)
	
	if _, err := os.Stat(entityDir); os.IsNotExist(err) {
		return []map[string]interface{}{}, nil
	}
	
	files, err := os.ReadDir(entityDir)
	if err != nil {
		return nil, err
	}
	
	var results []map[string]interface{}
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" || file.Name() == "_next_id.json" {
			continue
		}
		
		data, err := os.ReadFile(filepath.Join(entityDir, file.Name()))
		if err != nil {
			continue
		}
		
		var entity map[string]interface{}
		if err := json.Unmarshal(data, &entity); err != nil {
			continue
		}
		
		results = append(results, entity)
	}
	
	return results, nil
}

// Exists checks if an entity exists
func (s *JSONFileStore) Exists(ctx context.Context, entity string, id int) bool {
	filePath := s.getEntityFile(entity, id)
	_, err := os.Stat(filePath)
	return err == nil
}

// Close closes the storage (cleanup if needed)
func (s *JSONFileStore) Close() error {
	return nil
}

// ListEntities returns all entity types in the schema
func (s *JSONFileStore) ListEntities(ctx context.Context) ([]string, error) {
	schemaPath := filepath.Join(s.baseDir, s.schema)
	
	entries, err := os.ReadDir(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	
	var entities []string
	for _, entry := range entries {
		if entry.IsDir() {
			entities = append(entities, entry.Name())
		}
	}
	
	return entities, nil
}

// Search implements field-based search
func (s *JSONFileStore) Search(ctx context.Context, entity string, field string, query string, matchType string) ([]map[string]interface{}, error) {
	all, err := s.List(ctx, entity)
	if err != nil {
		return nil, err
	}
	
	var results []map[string]interface{}
	query = strings.ToLower(query)
	
	for _, item := range all {
		if value, ok := item[field]; ok {
			valueStr := strings.ToLower(fmt.Sprintf("%v", value))
			
			matched := false
			switch matchType {
			case "contains":
				matched = strings.Contains(valueStr, query)
			case "starts":
				matched = strings.HasPrefix(valueStr, query)
			case "ends":
				matched = strings.HasSuffix(valueStr, query)
			case "exact":
				matched = valueStr == query
			default:
				matched = strings.Contains(valueStr, query)
			}
			
			if matched {
				results = append(results, item)
			}
		}
	}
	
	return results, nil
}
