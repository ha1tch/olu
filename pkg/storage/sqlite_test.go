package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/ha1tch/olu/pkg/storage"
)

func setupSQLiteTest(t *testing.T) (storage.Store, func()) {
	t.Helper()

	// Create temp database file
	tmpFile, err := os.CreateTemp("", "olu-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	dbPath := tmpFile.Name()

	config := map[string]interface{}{
		"db_path": dbPath,
	}

	store, err := storage.NewStore("sqlite", config)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("Failed to create store: %v", err)
	}
	if store == nil {
		os.Remove(dbPath)
		t.Fatal("NewStore returned nil")
	}

	cleanup := func() {
		if store != nil {
			store.Close()
		}
		os.Remove(dbPath)
	}

	return store, cleanup
}

// Helper to create test user data
func testUserData(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":   name,
		"email":  fmt.Sprintf("%s@example.com", name),
		"active": true,
	}
}

// =============================================================================
// Basic CRUD Tests
// =============================================================================

func TestSQLiteStore_Create(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	data := map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
		"age":   30,
	}

	id, err := store.Create(ctx, "users", data)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id != 1 {
		t.Errorf("Expected id 1, got %d", id)
	}

	// Verify data was stored
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", retrieved["name"])
	}
	if retrieved["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", retrieved["email"])
	}
	if retrieved["age"] != float64(30) {
		t.Errorf("Expected age 30, got %v", retrieved["age"])
	}
	if retrieved["id"] != float64(1) {
		t.Errorf("Expected id 1, got %v", retrieved["id"])
	}
}

func TestSQLiteStore_CreateMultiple(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple entities
	id1, err := store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id1 != 1 {
		t.Errorf("Expected id1=1, got %d", id1)
	}

	id2, err := store.Create(ctx, "users", map[string]interface{}{"name": "Bob"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id2 != 2 {
		t.Errorf("Expected id2=2, got %d", id2)
	}

	id3, err := store.Create(ctx, "users", map[string]interface{}{"name": "Charlie"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id3 != 3 {
		t.Errorf("Expected id3=3, got %d", id3)
	}

	// IDs should be unique and sequential
	if id1 == id2 || id2 == id3 {
		t.Error("IDs should be unique")
	}
}

func TestSQLiteStore_CreateDifferentEntityTypes(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities of different types - IDs should be independent
	userId, err := store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if userId != 1 {
		t.Errorf("Expected userId=1, got %d", userId)
	}

	postId, err := store.Create(ctx, "posts", map[string]interface{}{"title": "Post 1"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if postId != 1 {
		t.Errorf("Expected postId=1, got %d", postId)
	}

	// Both should have ID 1 since they're different entity types
	if userId != postId {
		t.Errorf("Expected userId == postId for different entity types")
	}
}

func TestSQLiteStore_Get(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	data := testUserData("Alice")
	id, err := store.Create(ctx, "users", data)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get entity
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", retrieved["name"])
	}
	if retrieved["email"] != "Alice@example.com" {
		t.Errorf("Expected email Alice@example.com, got %v", retrieved["email"])
	}
	if retrieved["active"] != true {
		t.Errorf("Expected active true, got %v", retrieved["active"])
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.Get(ctx, "users", 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update entity
	err = store.Update(ctx, "users", id, map[string]interface{}{
		"name": "Alice Smith",
		"age":  31,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice Smith" {
		t.Errorf("Expected name 'Alice Smith', got %v", retrieved["name"])
	}
	if retrieved["age"] != float64(31) {
		t.Errorf("Expected age 31, got %v", retrieved["age"])
	}
}

func TestSQLiteStore_UpdateNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	err := store.Update(ctx, "users", 999, map[string]interface{}{"name": "Nobody"})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_Patch(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
		"age":   30,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Patch only age
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"age": 31,
	})
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Verify only age changed
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", retrieved["name"])
	}
	if retrieved["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", retrieved["email"])
	}
	if retrieved["age"] != float64(31) {
		t.Errorf("Expected age 31, got %v", retrieved["age"])
	}
}

func TestSQLiteStore_PatchAddField(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Add new field
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Verify field was added
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", retrieved["name"])
	}
	if retrieved["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", retrieved["email"])
	}
}

func TestSQLiteStore_PatchRemoveField(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Remove field by setting to nil
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"email": nil,
	})
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Verify field was removed
	retrieved, err := store.Get(ctx, "users", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", retrieved["name"])
	}
	if _, hasEmail := retrieved["email"]; hasEmail {
		t.Error("Expected email field to be removed")
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify exists
	if !store.Exists(ctx, "users", id) {
		t.Error("Entity should exist before delete")
	}

	// Delete entity
	err = store.Delete(ctx, "users", id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	if store.Exists(ctx, "users", id) {
		t.Error("Entity should not exist after delete")
	}
	_, err = store.Get(ctx, "users", id)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLiteStore_DeleteNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	err := store.Delete(ctx, "users", 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_List(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple entities
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Charlie"})

	// List all
	results, err := store.List(ctx, "users")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Verify names
	names := make(map[string]bool)
	for _, result := range results {
		names[result["name"].(string)] = true
	}
	for _, expected := range []string{"Alice", "Bob", "Charlie"} {
		if !names[expected] {
			t.Errorf("Expected %s in results", expected)
		}
	}
}

func TestSQLiteStore_ListEmpty(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	results, err := store.List(ctx, "users")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty list, got %d results", len(results))
	}
}

func TestSQLiteStore_Exists(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Non-existent
	if store.Exists(ctx, "users", 1) {
		t.Error("Should not exist before creation")
	}

	// Create
	id, err := store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Exists
	if !store.Exists(ctx, "users", id) {
		t.Error("Should exist after creation")
	}

	// Delete
	store.Delete(ctx, "users", id)

	// No longer exists
	if store.Exists(ctx, "users", id) {
		t.Error("Should not exist after deletion")
	}
}

func TestSQLiteStore_Save(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Save creates a new entity with a specific ID
	err := store.Save(ctx, "users", 42, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify entity was created with specified ID
	retrieved, err := store.Get(ctx, "users", 42)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got %v", retrieved["name"])
	}
	if retrieved["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", retrieved["email"])
	}

	// Save on existing ID should fail
	err = store.Save(ctx, "users", 42, map[string]interface{}{
		"name": "Bob",
	})
	if err == nil {
		t.Error("Expected error when saving to existing ID")
	}
}

// =============================================================================
// Data Type Tests
// =============================================================================

func TestSQLiteStore_DataTypes(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	data := map[string]interface{}{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"array":  []interface{}{1, 2, 3},
		"object": map[string]interface{}{"nested": "value"},
	}

	id, err := store.Create(ctx, "test", data)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := store.Get(ctx, "test", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved["string"] != "hello" {
		t.Errorf("String mismatch: %v", retrieved["string"])
	}
	if retrieved["float"] != 3.14 {
		t.Errorf("Float mismatch: %v", retrieved["float"])
	}
	if retrieved["bool"] != true {
		t.Errorf("Bool mismatch: %v", retrieved["bool"])
	}

	// Array should be preserved
	arr, ok := retrieved["array"].([]interface{})
	if !ok {
		t.Errorf("Array type mismatch: %T", retrieved["array"])
	} else if len(arr) != 3 {
		t.Errorf("Array length mismatch: %d", len(arr))
	}

	// Object should be preserved
	obj, ok := retrieved["object"].(map[string]interface{})
	if !ok {
		t.Errorf("Object type mismatch: %T", retrieved["object"])
	} else if obj["nested"] != "value" {
		t.Errorf("Nested value mismatch: %v", obj["nested"])
	}
}

// =============================================================================
// Search Tests
// =============================================================================

func TestSQLiteStore_Search(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice", "age": 30})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob", "age": 25})
	store.Create(ctx, "users", map[string]interface{}{"name": "Charlie", "age": 35})
	store.Create(ctx, "users", map[string]interface{}{"name": "Alicia", "age": 28})

	searcher, ok := store.(storage.Searcher)
	if !ok {
		t.Skip("Store does not implement Searcher interface")
	}

	// Exact match
	results, err := searcher.Search(ctx, "users", "name", "Alice", "exact")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for exact match, got %d", len(results))
	}

	// Contains
	results, err = searcher.Search(ctx, "users", "name", "li", "contains")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results for contains 'li' (Alice, Alicia, Charlie), got %d", len(results))
	}
}

func TestSQLiteStore_SearchStarts(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Alfred"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob"})

	searcher := store.(storage.Searcher)

	results, err := searcher.Search(ctx, "users", "name", "Al", "starts")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for starts 'Al', got %d", len(results))
	}
}

func TestSQLiteStore_SearchEnds(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, "users", map[string]interface{}{"email": "alice@example.com"})
	store.Create(ctx, "users", map[string]interface{}{"email": "bob@example.com"})
	store.Create(ctx, "users", map[string]interface{}{"email": "charlie@other.com"})

	searcher := store.(storage.Searcher)

	results, err := searcher.Search(ctx, "users", "email", "example.com", "ends")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for ends 'example.com', got %d", len(results))
	}
}

// =============================================================================
// Graph/Reference Tests
// =============================================================================

func TestSQLiteStore_References(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create manager
	managerId, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Manager",
		"role": "manager",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create employee with REF to manager
	employeeId, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"role": "employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get employee and verify REF is stored
	employee, err := store.Get(ctx, "users", employeeId)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	managerRef, ok := employee["manager"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected manager to be a map, got %T", employee["manager"])
	}
	if managerRef["type"] != "REF" {
		t.Errorf("Expected REF type, got %v", managerRef["type"])
	}
	if managerRef["entity"] != "users" {
		t.Errorf("Expected entity 'users', got %v", managerRef["entity"])
	}
}

func TestSQLiteStore_GetNeighbors(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities with references
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee1",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})
	store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee2",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})

	graphStore, ok := store.(storage.GraphNeighbors)
	if !ok {
		t.Skip("Store does not implement GraphNeighbors interface")
	}

	// Get incoming edges (employees who report to this manager)
	neighbors, err := graphStore.GetNeighbors(ctx, "users", managerId, "in")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	if len(neighbors) != 2 {
		t.Errorf("Expected 2 incoming neighbors, got %d", len(neighbors))
	}

	// Verify direction
	for _, n := range neighbors {
		if n["_direction"] != "in" {
			t.Errorf("Expected direction 'in', got %v", n["_direction"])
		}
	}
}

// =============================================================================
// Graph Integrity Tests
// =============================================================================

func TestSQLiteStore_VerifyGraphIntegrity(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities with REFs
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})

	// Test GraphIntegrity interface
	integrityStore, ok := store.(storage.GraphIntegrity)
	if !ok {
		t.Skip("Store does not implement GraphIntegrity interface")
	}

	// Verify integrity
	err := integrityStore.VerifyGraphIntegrity(ctx)
	if err != nil {
		t.Errorf("VerifyGraphIntegrity failed: %v", err)
	}
}

func TestSQLiteStore_RebuildGraph(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities with REFs
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	employeeId, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})

	integrityStore, ok := store.(storage.GraphIntegrity)
	if !ok {
		t.Skip("Store does not implement GraphIntegrity interface")
	}
	graphStore := store.(storage.GraphNeighbors)

	// Verify graph works before rebuild
	neighbors, _ := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	if len(neighbors) != 1 {
		t.Errorf("Expected 1 outgoing neighbor before rebuild, got %d", len(neighbors))
	}

	// Rebuild graph
	err := integrityStore.RebuildGraph(ctx)
	if err != nil {
		t.Fatalf("RebuildGraph failed: %v", err)
	}

	// Verify graph still works after rebuild
	neighbors, err = graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	if len(neighbors) != 1 {
		t.Errorf("Expected 1 outgoing neighbor after rebuild, got %d", len(neighbors))
	}
	if neighbors[0]["name"] != "Manager" {
		t.Errorf("Expected neighbor name 'Manager', got %v", neighbors[0]["name"])
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestSQLiteStore_ConcurrentCreates(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities concurrently
	count := 20
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	ids := make(chan int, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id, err := store.Create(ctx, "users", map[string]interface{}{
				"name": fmt.Sprintf("User%d", n),
			})
			if err != nil {
				errCh <- err
			} else {
				ids <- id
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	close(ids)

	// Check for errors
	for err := range errCh {
		t.Errorf("Concurrent create error: %v", err)
	}

	// Verify all created
	results, err := store.List(ctx, "users")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != count {
		t.Errorf("Expected %d users, got %d", count, len(results))
	}

	// Verify IDs are unique
	idSet := make(map[int]bool)
	for id := range ids {
		if idSet[id] {
			t.Errorf("Duplicate ID: %d", id)
		}
		idSet[id] = true
	}
}

func TestSQLiteStore_ConcurrentReadWrite(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial entities
	for i := 1; i <= 10; i++ {
		store.Create(ctx, "users", map[string]interface{}{"name": fmt.Sprintf("User%d", i)})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 1; j <= 5; j++ {
				_, err := store.Get(ctx, "users", j)
				if err != nil {
					errCh <- err
				}
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := n + 1
			err := store.Update(ctx, "users", id, map[string]interface{}{
				"name": fmt.Sprintf("UpdatedUser%d", n),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

// =============================================================================
// Info Tests
// =============================================================================

func TestSQLiteStore_Info(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	infoProvider, ok := store.(storage.InfoProvider)
	if !ok {
		t.Skip("Store does not implement InfoProvider interface")
	}

	info := infoProvider.Info()
	if info.Type != "sqlite" {
		t.Errorf("Expected type 'sqlite', got %s", info.Type)
	}
	if info.Version == "" {
		t.Error("Expected non-empty version")
	}
	if !info.SupportsSearch {
		t.Error("Expected SupportsSearch to be true")
	}
	if !info.SupportsTransaction {
		t.Error("Expected SupportsTransaction to be true")
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkSQLiteStore_Create(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "olu-bench-*.db")
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, _ := storage.NewStore("sqlite", map[string]interface{}{"db_path": dbPath})
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Create(ctx, "users", map[string]interface{}{
			"name":  "User",
			"email": "user@example.com",
		})
	}
}

func BenchmarkSQLiteStore_Get(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "olu-bench-*.db")
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, _ := storage.NewStore("sqlite", map[string]interface{}{"db_path": dbPath})
	defer store.Close()

	ctx := context.Background()

	// Create test data
	id, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "User",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get(ctx, "users", id)
	}
}

func BenchmarkSQLiteStore_Update(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "olu-bench-*.db")
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, _ := storage.NewStore("sqlite", map[string]interface{}{"db_path": dbPath})
	defer store.Close()

	ctx := context.Background()

	// Create test data
	id, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "User",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Update(ctx, "users", id, map[string]interface{}{
			"name": "Updated",
		})
	}
}

func BenchmarkSQLiteStore_Search(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "olu-bench-*.db")
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, _ := storage.NewStore("sqlite", map[string]interface{}{"db_path": dbPath})
	defer store.Close()

	ctx := context.Background()

	// Create test data
	for i := 0; i < 100; i++ {
		store.Create(ctx, "users", map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		})
	}

	searcher := store.(storage.Searcher)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searcher.Search(ctx, "users", "name", "User", "contains")
	}
}

func BenchmarkSQLiteStore_List(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "olu-bench-*.db")
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, _ := storage.NewStore("sqlite", map[string]interface{}{"db_path": dbPath})
	defer store.Close()

	ctx := context.Background()

	// Create test data
	for i := 0; i < 100; i++ {
		store.Create(ctx, "users", map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.List(ctx, "users")
	}
}

// ============================================================================
// Full-Text Search Tests
// ============================================================================

func setupSQLiteTestWithFTS(t *testing.T) (storage.Store, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "olu-fts-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	dbPath := tmpFile.Name()

	config := map[string]interface{}{
		"db_path":           dbPath,
		"full_text_enabled": true,
	}

	store, err := storage.NewStore("sqlite", config)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		if store != nil {
			store.Close()
		}
		os.Remove(dbPath)
	}

	return store, cleanup
}

func TestSQLiteStore_FullTextSearch_Basic(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create test entities
	store.Create(ctx, "users", map[string]interface{}{
		"name":  "Alice Smith",
		"email": "alice@example.com",
		"bio":   "Software engineer who loves coding",
	})
	store.Create(ctx, "users", map[string]interface{}{
		"name":  "Bob Johnson",
		"email": "bob@example.com",
		"bio":   "Product manager with engineering background",
	})
	store.Create(ctx, "users", map[string]interface{}{
		"name":  "Charlie Brown",
		"email": "charlie@example.com",
		"bio":   "Designer focused on user experience",
	})

	// Search for "engineer"
	results, err := store.FullTextSearch(ctx, "engineer", "")
	if err != nil {
		t.Fatalf("FullTextSearch failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'engineer', got %d", len(results))
	}
}

func TestSQLiteStore_FullTextSearch_EntityFilter(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create entities in different types
	store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice Developer",
	})
	store.Create(ctx, "posts", map[string]interface{}{
		"title":   "Developer Guide",
		"content": "How to become a developer",
	})

	// Search across all entities
	allResults, err := store.FullTextSearch(ctx, "developer", "")
	if err != nil {
		t.Fatalf("FullTextSearch failed: %v", err)
	}
	if len(allResults) != 2 {
		t.Errorf("Expected 2 results across all entities, got %d", len(allResults))
	}

	// Search only in users
	userResults, err := store.FullTextSearch(ctx, "developer", "users")
	if err != nil {
		t.Fatalf("FullTextSearch with entity filter failed: %v", err)
	}
	if len(userResults) != 1 {
		t.Errorf("Expected 1 result in users, got %d", len(userResults))
	}
}

func TestSQLiteStore_FullTextSearch_UpdateReindex(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Original Name",
		"bio":  "Original bio content",
	})

	// Verify initial search works
	results, _ := store.FullTextSearch(ctx, "Original", "")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'Original', got %d", len(results))
	}

	// Update entity
	store.Update(ctx, "users", id, map[string]interface{}{
		"name": "Updated Name",
		"bio":  "Completely different content",
	})

	// Old content should not be found
	oldResults, _ := store.FullTextSearch(ctx, "Original", "")
	if len(oldResults) != 0 {
		t.Errorf("Expected 0 results for 'Original' after update, got %d", len(oldResults))
	}

	// New content should be found
	newResults, _ := store.FullTextSearch(ctx, "Updated", "")
	if len(newResults) != 1 {
		t.Errorf("Expected 1 result for 'Updated', got %d", len(newResults))
	}
}

func TestSQLiteStore_FullTextSearch_DeleteRemovesIndex(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "DeleteMe User",
	})

	// Verify search works
	results, _ := store.FullTextSearch(ctx, "DeleteMe", "")
	if len(results) != 1 {
		t.Errorf("Expected 1 result before delete, got %d", len(results))
	}

	// Delete entity
	store.Delete(ctx, "users", id)

	// Should not be found after delete
	afterResults, _ := store.FullTextSearch(ctx, "DeleteMe", "")
	if len(afterResults) != 0 {
		t.Errorf("Expected 0 results after delete, got %d", len(afterResults))
	}
}

func TestSQLiteStore_FullTextSearch_EmptyQuery(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	results, err := store.FullTextSearch(ctx, "", "")
	if err != nil {
		t.Fatalf("FullTextSearch with empty query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}

func TestSQLiteStore_FullTextSearch_NestedContent(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity with nested content
	store.Create(ctx, "articles", map[string]interface{}{
		"title": "Main Title",
		"metadata": map[string]interface{}{
			"author":   "Nested Author Name",
			"category": "Technology",
		},
		"tags": []interface{}{"golang", "programming", "tutorial"},
	})

	// Search for nested content
	authorResults, _ := store.FullTextSearch(ctx, "Nested", "")
	if len(authorResults) != 1 {
		t.Errorf("Expected 1 result for nested 'Nested', got %d", len(authorResults))
	}

	// Search for array content
	tagResults, _ := store.FullTextSearch(ctx, "golang", "")
	if len(tagResults) != 1 {
		t.Errorf("Expected 1 result for tag 'golang', got %d", len(tagResults))
	}
}

func TestSQLiteStore_FullTextSearch_DisabledByDefault(t *testing.T) {
	// Use regular setup (FTS disabled)
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	store.Create(ctx, "users", map[string]interface{}{
		"name": "Test User",
	})

	// Search should return empty (FTS not enabled)
	results, err := store.FullTextSearch(ctx, "Test", "")
	if err != nil {
		t.Fatalf("FullTextSearch failed: %v", err)
	}
	// With FTS disabled, no content is indexed
	if len(results) != 0 {
		t.Errorf("Expected 0 results with FTS disabled, got %d", len(results))
	}
}

func TestSQLiteStore_FullTextSearch_PatchReindex(t *testing.T) {
	store, cleanup := setupSQLiteTestWithFTS(t)
	defer cleanup()

	ctx := context.Background()

	// Create entity
	id, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Original",
		"bio":  "Original bio",
	})

	// Patch only the bio
	store.Patch(ctx, "users", id, map[string]interface{}{
		"bio": "Patched content here",
	})

	// Should find patched content
	results, _ := store.FullTextSearch(ctx, "Patched", "")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'Patched', got %d", len(results))
	}

	// Original name should still be findable
	nameResults, _ := store.FullTextSearch(ctx, "Original", "")
	if len(nameResults) != 1 {
		t.Errorf("Expected 1 result for 'Original' (name unchanged), got %d", len(nameResults))
	}
}
