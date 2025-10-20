package storage

import (
	"context"
	"fmt"
	"sync"
)

// StoreFactory is a function that creates a new Store instance
type StoreFactory func(config map[string]interface{}) (Store, error)

var (
	storeMu       sync.RWMutex
	storeRegistry = make(map[string]StoreFactory)
)

// RegisterStore registers a new store implementation
func RegisterStore(name string, factory StoreFactory) {
	storeMu.Lock()
	defer storeMu.Unlock()
	storeRegistry[name] = factory
}

// NewStore creates a new store instance by name
func NewStore(name string, config map[string]interface{}) (Store, error) {
	storeMu.RLock()
	factory, exists := storeRegistry[name]
	storeMu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("unknown store type: %s", name)
	}
	
	return factory(config)
}

// ListStores returns all registered store types
func ListStores() []string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	
	stores := make([]string, 0, len(storeRegistry))
	for name := range storeRegistry {
		stores = append(stores, name)
	}
	return stores
}

// init registers built-in stores
func init() {
	// Register JSONFileStore
	RegisterStore("jsonfile", func(config map[string]interface{}) (Store, error) {
		baseDir, ok := config["base_dir"].(string)
		if !ok {
			baseDir = "data"
		}
		
		schema, ok := config["schema"].(string)
		if !ok {
			schema = "default"
		}
		
		return NewJSONFileStore(baseDir, schema)
	})
	
	// Register SQLiteStore
	RegisterStore("sqlite", func(config map[string]interface{}) (Store, error) {
		dbPath, ok := config["db_path"].(string)
		if !ok {
			dbPath = "olu.db"
		}
		
		sqliteConfig := SQLiteConfig{
			DBPath:           dbPath,
			EnableWAL:        true,
			EnableForeignKeys: true,
			CacheSize:        2000, // 2MB
			BusyTimeout:      5000, // 5 seconds
		}
		
		// Allow overriding config options
		if wal, ok := config["enable_wal"].(bool); ok {
			sqliteConfig.EnableWAL = wal
		}
		if fk, ok := config["enable_foreign_keys"].(bool); ok {
			sqliteConfig.EnableForeignKeys = fk
		}
		if cache, ok := config["cache_size"].(int); ok {
			sqliteConfig.CacheSize = cache
		}
		if timeout, ok := config["busy_timeout"].(int); ok {
			sqliteConfig.BusyTimeout = timeout
		}
		
		return NewSQLiteStore(dbPath, sqliteConfig)
	})
}

// Helper functions for common operations

// WithTransaction executes a function within a transaction if the store supports it
func WithTransaction(ctx context.Context, store Store, fn func(Transaction) error) error {
	if ts, ok := store.(Transactional); ok {
		tx, err := ts.Begin(ctx)
		if err != nil {
			return err
		}
		
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				panic(r)
			}
		}()
		
		if err := fn(tx); err != nil {
			tx.Rollback()
			return err
		}
		
		return tx.Commit()
	}
	
	// If store doesn't support transactions, execute directly
	// Note: This creates a pseudo-transaction that can't rollback
	pseudoTx := &pseudoTransaction{store: store}
	return fn(pseudoTx)
}

// pseudoTransaction wraps a non-transactional store
type pseudoTransaction struct {
	store Store
}

func (pt *pseudoTransaction) Create(ctx context.Context, entity string, data map[string]interface{}) (int, error) {
	return pt.store.Create(ctx, entity, data)
}

func (pt *pseudoTransaction) Get(ctx context.Context, entity string, id int) (map[string]interface{}, error) {
	return pt.store.Get(ctx, entity, id)
}

func (pt *pseudoTransaction) Update(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	return pt.store.Update(ctx, entity, id, data)
}

func (pt *pseudoTransaction) Patch(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	return pt.store.Patch(ctx, entity, id, data)
}

func (pt *pseudoTransaction) Delete(ctx context.Context, entity string, id int) error {
	return pt.store.Delete(ctx, entity, id)
}

func (pt *pseudoTransaction) Save(ctx context.Context, entity string, id int, data map[string]interface{}) error {
	return pt.store.Save(ctx, entity, id, data)
}

func (pt *pseudoTransaction) List(ctx context.Context, entity string) ([]map[string]interface{}, error) {
	return pt.store.List(ctx, entity)
}

func (pt *pseudoTransaction) Exists(ctx context.Context, entity string, id int) bool {
	return pt.store.Exists(ctx, entity, id)
}

func (pt *pseudoTransaction) Close() error {
	return nil
}

func (pt *pseudoTransaction) Commit() error {
	return nil
}

func (pt *pseudoTransaction) Rollback() error {
	return nil
}
