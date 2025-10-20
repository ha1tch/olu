package storage

import (
	"context"
	"errors"
)

var (
	// ErrNotFound is returned when an entity is not found
	ErrNotFound = errors.New("entity not found")
	// ErrAlreadyExists is returned when an entity already exists
	ErrAlreadyExists = errors.New("entity already exists")
	// ErrInvalidEntity is returned when entity name is invalid
	ErrInvalidEntity = errors.New("invalid entity name")
	// ErrInvalidID is returned when ID is invalid
	ErrInvalidID = errors.New("invalid ID")
)

// Store defines the core interface for entity storage backends
type Store interface {
	// Entity CRUD operations
	Create(ctx context.Context, entity string, data map[string]interface{}) (int, error)
	Get(ctx context.Context, entity string, id int) (map[string]interface{}, error)
	Update(ctx context.Context, entity string, id int, data map[string]interface{}) error
	Patch(ctx context.Context, entity string, id int, data map[string]interface{}) error
	Delete(ctx context.Context, entity string, id int) error
	Save(ctx context.Context, entity string, id int, data map[string]interface{}) error
	
	// Query operations
	List(ctx context.Context, entity string) ([]map[string]interface{}, error)
	Exists(ctx context.Context, entity string, id int) bool
	
	// Lifecycle
	Close() error
}

// IDGenerator defines interface for ID generation strategies
type IDGenerator interface {
	NextID(ctx context.Context, entity string) (int, error)
}

// Transactional defines optional transaction support
// Stores that support transactions should implement this interface
type Transactional interface {
	Store
	Begin(ctx context.Context) (Transaction, error)
}

// Transaction represents a storage transaction
type Transaction interface {
	Store
	Commit() error
	Rollback() error
}

// Migrator defines optional schema migration support
// Useful for database backends
type Migrator interface {
	Migrate(ctx context.Context) error
	Version(ctx context.Context) (int, error)
}

// Searcher defines optional search capabilities
type Searcher interface {
	Search(ctx context.Context, entity string, field string, query string, matchType string) ([]map[string]interface{}, error)
}

// Batcher defines optional batch operation support
type Batcher interface {
	BatchCreate(ctx context.Context, entity string, items []map[string]interface{}) ([]int, error)
	BatchDelete(ctx context.Context, entity string, ids []int) error
}

// GraphNeighbors defines optional graph neighbor queries
type GraphNeighbors interface {
	GetNeighbors(ctx context.Context, entity string, id int, direction string) ([]map[string]interface{}, error)
}

// GraphIntegrity defines optional graph integrity checking
type GraphIntegrity interface {
	VerifyGraphIntegrity(ctx context.Context) error
	RebuildGraph(ctx context.Context) error
}

// StoreInfo provides metadata about the store implementation
type StoreInfo struct {
	Type                string // "jsonfile", "sqlite", "postgres", etc.
	Version             string
	SupportsSearch      bool
	SupportsBatch       bool
	SupportsTransaction bool
}

// InfoProvider allows stores to provide metadata about their capabilities
type InfoProvider interface {
	Info() StoreInfo
}
