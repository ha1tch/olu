// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

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
	// ErrConflict is returned when an optimistic concurrency check fails
	ErrConflict = errors.New("version conflict")
)

// StoreConfig is the canonical configuration for all store backends.
// A store is constructed with a StoreConfig and scoped to that config
// for its entire lifetime. TenantID 0 means no tenant scoping.
type StoreConfig struct {
	Type            string // "sqlite", "jsonfile"
	DBPath          string // SQLite database file path
	BaseDir         string // JSONFile base directory
	Schema          string // JSONFile schema subdirectory
	FullTextEnabled bool   // controls FTS indexing in backend
	GraphEnabled    bool   // controls graph edge table maintenance
	TenantID        uint16 // 0 = no tenant scoping

	// Performance tuning (SQLite-specific; zero = use defaults)
	SQLiteCacheSize           int // Page cache size in KB
	SQLiteBusyTimeout         int // Milliseconds to wait on locked database
	SQLiteMaxOpenConns        int // Max open database connections
	SQLiteMaxIdleConns        int // Max idle database connections
	SQLiteReadPoolSize        int // Max open read connections (0 = auto)
	SQLiteContentionThreshold int // Adaptive lock threshold 0-100
}

// Store defines the core interface for entity storage backends
type Store interface {
	// Config returns the store's configuration.
	Config() StoreConfig

	// Entity CRUD operations
	Create(ctx context.Context, entity string, data map[string]interface{}) (int, error)
	Get(ctx context.Context, entity string, id int) (map[string]interface{}, error)
	Update(ctx context.Context, entity string, id int, data map[string]interface{}) error
	Patch(ctx context.Context, entity string, id int, data map[string]interface{}) error
	// PatchValidated is like Patch but runs a validation function against the
	// merged data inside the transaction. If the validator returns an error,
	// the transaction is rolled back and the error is returned to the caller.
	// This avoids TOCTOU races where a Get-merge-Update sequence can observe
	// stale data between the Get and the Update.
	PatchValidated(ctx context.Context, entity string, id int, data map[string]interface{}, validate func(merged map[string]interface{}) error) error
	Delete(ctx context.Context, entity string, id int) error
	Save(ctx context.Context, entity string, id int, data map[string]interface{}) error
	
	// Query operations
	List(ctx context.Context, entity string) ([]map[string]interface{}, error)
	Exists(ctx context.Context, entity string, id int) bool
	Search(ctx context.Context, entity string, field string, query string, matchType string) ([]map[string]interface{}, error)
	
	// Full-text search (optional - may return empty if not supported)
	FullTextSearch(ctx context.Context, query string, entity string) ([]map[string]interface{}, error)
	
	// Ping verifies that the storage backend is reachable. Returns nil on
	// success. Used by health and readiness probes.
	Ping(ctx context.Context) error

	// Lifecycle
	Close() error
}

// IDGenerator defines interface for ID generation strategies
type IDGenerator interface {
	NextID(ctx context.Context, entity string) (int, error)
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

// EntityLister defines optional entity type listing support
type EntityLister interface {
	ListEntities(ctx context.Context) ([]string, error)
}

// PagedResult holds a page of results plus the total count.
type PagedResult struct {
	Data       []map[string]interface{}
	TotalItems int
}

// PagedLister is an optional interface for storage backends that support
// server-side pagination. Backends that implement this avoid loading every
// record into memory for paginated list requests.
type PagedLister interface {
	// ListPaged returns a single page of entities, plus the total count.
	// limit and offset are applied at the storage layer (SQL LIMIT/OFFSET).
	ListPaged(ctx context.Context, entity string, limit, offset int) (*PagedResult, error)
}
