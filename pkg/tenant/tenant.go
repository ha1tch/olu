// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package tenant provides tenant-scoping helpers for the middle and handler tiers.
//
// The storage tier enforces isolation by construction (see storage.StoreConfig).
// This package provides the supporting utilities that other tiers need:
//
//   - NodeID / CacheKey: construct globally-unique identifiers when a shared
//     in-memory structure (graph, cache) spans multiple tenants.
//   - Registry: maps human-readable tenant names to uint16 identifiers.
//     When a Persister is attached, mappings are durable across restarts.
package tenant

import (
	"context"
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// Node ID and cache key helpers
// ---------------------------------------------------------------------------

// NodeID returns a graph node identifier scoped to a tenant.
// For tenant 0 (unscoped): "entity:id"
// For non-zero tenants:    "XXXX/entity:id"
func NodeID(tenantID uint16, entity string, id int) string {
	if tenantID == 0 {
		return fmt.Sprintf("%s:%d", entity, id)
	}
	return fmt.Sprintf("%04X/%s:%d", tenantID, entity, id)
}

// CacheKey returns a cache key scoped to a tenant.
// For tenant 0 (unscoped): "entity:id"
// For non-zero tenants:    "XXXX:entity:id"
func CacheKey(tenantID uint16, entity string, id int) string {
	if tenantID == 0 {
		return fmt.Sprintf("%s:%d", entity, id)
	}
	return fmt.Sprintf("%04X:%s:%d", tenantID, entity, id)
}

// CachePattern returns a pattern for cache invalidation scoped to a tenant.
// For tenant 0 (unscoped): "entity:*"
// For non-zero tenants:    "XXXX:entity:*"
func CachePattern(tenantID uint16, entity string) string {
	if tenantID == 0 {
		return fmt.Sprintf("%s:*", entity)
	}
	return fmt.Sprintf("%04X:%s:*", tenantID, entity)
}

// CacheTenantPattern returns a pattern matching all keys for a tenant.
// For tenant 0: "*" (everything)
// For non-zero: "XXXX:*"
func CacheTenantPattern(tenantID uint16) string {
	if tenantID == 0 {
		return "*"
	}
	return fmt.Sprintf("%04X:*", tenantID)
}

// CacheListPattern returns a pattern matching only list cache keys for an
// entity type, scoped to a tenant. This leaves individual GET cache entries
// intact, improving cache hit rate when a single entity is modified.
// For tenant 0 (unscoped): "entity:list:*"
// For non-zero tenants:    "XXXX:entity:list:*"
func CacheListPattern(tenantID uint16, entity string) string {
	if tenantID == 0 {
		return fmt.Sprintf("%s:list:*", entity)
	}
	return fmt.Sprintf("%04X:%s:list:*", tenantID, entity)
}

// ---------------------------------------------------------------------------
// Persistence interface
// ---------------------------------------------------------------------------

// Persister stores and retrieves tenant name-to-ID mappings durably.
// Implementations must be safe for concurrent use.
type Persister interface {
	// LoadAll returns all persisted tenant mappings.
	LoadAll(ctx context.Context) (map[string]uint16, error)
	// Save persists a single tenant mapping. It must be idempotent:
	// saving an already-persisted (name, id) pair is not an error.
	Save(ctx context.Context, name string, id uint16) error
}

// ---------------------------------------------------------------------------
// Tenant registry
// ---------------------------------------------------------------------------

// Registry maps human-readable tenant names (e.g. "acme") to uint16 IDs.
// It is safe for concurrent use. When a Persister is attached, all mutations
// are durably stored, ensuring stable name-to-ID mappings across restarts.
type Registry struct {
	mu        sync.RWMutex
	byName    map[string]uint16
	byID      map[uint16]string
	nextAuto  uint16 // for auto-assignment; starts at 1
	persister Persister
}

// NewRegistry creates an empty tenant registry with no persistence.
// Mappings will be lost on restart. Use SetPersister or LoadFrom to
// attach durable storage.
func NewRegistry() *Registry {
	return &Registry{
		byName:   make(map[string]uint16),
		byID:     make(map[uint16]string),
		nextAuto: 1,
	}
}

// SetPersister attaches a persistence backend to the registry.
// Must be called before any Register/GetOrRegister calls.
func (r *Registry) SetPersister(p Persister) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persister = p
}

// LoadFrom loads all tenant mappings from the attached persister into
// the in-memory registry. Existing in-memory mappings are preserved;
// conflicts (same name with different ID) return an error.
// This should be called once at startup, after SetPersister.
func (r *Registry) LoadFrom(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.persister == nil {
		return nil // no persister, nothing to load
	}

	mappings, err := r.persister.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load tenant registry: %w", err)
	}

	for name, id := range mappings {
		if id == 0 {
			continue // skip reserved ID
		}
		if existing, ok := r.byName[name]; ok && existing != id {
			return fmt.Errorf("tenant %q: persisted ID %d conflicts with in-memory ID %d", name, id, existing)
		}
		if existing, ok := r.byID[id]; ok && existing != name {
			return fmt.Errorf("tenant ID %d: persisted name %q conflicts with in-memory name %q", id, name, existing)
		}
		r.byName[name] = id
		r.byID[id] = name
		if id >= r.nextAuto {
			r.nextAuto = id + 1
		}
	}

	return nil
}

// Register adds a tenant with an explicit ID.
// Returns an error if the name or ID is already registered.
// If a persister is attached, the mapping is durably stored.
func (r *Registry) Register(ctx context.Context, name string, id uint16) error {
	if id == 0 {
		return fmt.Errorf("tenant ID 0 is reserved for unscoped operation")
	}
	if name == "" {
		return fmt.Errorf("tenant name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byName[name]; ok {
		if existing == id {
			return nil // idempotent: same mapping already exists
		}
		return fmt.Errorf("tenant name %q already registered with ID %d", name, existing)
	}
	if existing, ok := r.byID[id]; ok {
		return fmt.Errorf("tenant ID %d already registered as %q", id, existing)
	}

	// Persist before committing to memory
	if r.persister != nil {
		if err := r.persister.Save(ctx, name, id); err != nil {
			return fmt.Errorf("persist tenant %q (ID %d): %w", name, id, err)
		}
	}

	r.byName[name] = id
	r.byID[id] = name

	// Keep nextAuto above the highest registered ID
	if id >= r.nextAuto {
		r.nextAuto = id + 1
	}

	return nil
}

// Lookup returns the tenant ID for a name, or 0 and false if not found.
func (r *Registry) Lookup(name string) (uint16, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byName[name]
	return id, ok
}

// GetOrRegister returns the tenant ID for a name, auto-registering with
// the next available ID if the name is not yet known. This is intended for
// non-strict tenant modes where tenants are created on first access.
// If a persister is attached, new mappings are durably stored.
func (r *Registry) GetOrRegister(ctx context.Context, name string) (uint16, error) {
	// Fast path: read lock
	r.mu.RLock()
	if id, ok := r.byName[name]; ok {
		r.mu.RUnlock()
		return id, nil
	}
	r.mu.RUnlock()

	// Slow path: write lock, double-check, register
	r.mu.Lock()
	defer r.mu.Unlock()

	if id, ok := r.byName[name]; ok {
		return id, nil
	}

	if name == "" {
		return 0, fmt.Errorf("tenant name must not be empty")
	}

	id := r.nextAuto
	if id == 0 {
		id = 1 // skip reserved 0
	}

	// Persist before committing to memory
	if r.persister != nil {
		if err := r.persister.Save(ctx, name, id); err != nil {
			return 0, fmt.Errorf("persist tenant %q (ID %d): %w", name, id, err)
		}
	}

	r.byName[name] = id
	r.byID[id] = name
	r.nextAuto = id + 1

	return id, nil
}

// Name returns the tenant name for an ID, or "" and false if not found.
func (r *Registry) Name(id uint16) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.byID[id]
	return name, ok
}

// List returns all registered tenant name-ID pairs.
func (r *Registry) List() map[string]uint16 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]uint16, len(r.byName))
	for k, v := range r.byName {
		result[k] = v
	}
	return result
}

// Count returns the number of registered tenants.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byName)
}
