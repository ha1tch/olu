package storage_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/ha1tch/olu/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSQLiteTest(t *testing.T) (storage.Store, func()) {
	t.Helper()
	
	// Create temp database file
	tmpFile, err := os.CreateTemp("", "olu-test-*.db")
	require.NoError(t, err)
	tmpFile.Close()
	
	dbPath := tmpFile.Name()
	
	config := map[string]interface{}{
		"db_path": dbPath,
	}
	
	store, err := storage.NewStore("sqlite", config)
	require.NoError(t, err)
	require.NotNil(t, store)
	
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
		"name":  name,
		"email": fmt.Sprintf("%s@example.com", name),
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
	require.NoError(t, err)
	assert.Equal(t, 1, id)
	
	// Verify data was stored
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	assert.Equal(t, "alice@example.com", retrieved["email"])
	assert.Equal(t, float64(30), retrieved["age"])
	assert.Equal(t, float64(1), retrieved["id"])
}

func TestSQLiteStore_CreateMultiple(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create multiple entities
	id1, err := store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	require.NoError(t, err)
	assert.Equal(t, 1, id1)
	
	id2, err := store.Create(ctx, "users", map[string]interface{}{"name": "Bob"})
	require.NoError(t, err)
	assert.Equal(t, 2, id2)
	
	id3, err := store.Create(ctx, "users", map[string]interface{}{"name": "Charlie"})
	require.NoError(t, err)
	assert.Equal(t, 3, id3)
	
	// IDs should be unique and sequential
	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id2, id3)
}

func TestSQLiteStore_CreateDifferentEntityTypes(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create entities of different types - IDs should be independent
	userId, err := store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	require.NoError(t, err)
	assert.Equal(t, 1, userId)
	
	postId, err := store.Create(ctx, "posts", map[string]interface{}{"title": "Post 1"})
	require.NoError(t, err)
	assert.Equal(t, 1, postId)
	
	// Both should have ID 1 since they're different entity types
	assert.Equal(t, userId, postId)
}

func TestSQLiteStore_Get(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create entity
	data := testUserData("Alice")
	id, err := store.Create(ctx, "users", data)
	require.NoError(t, err)
	
	// Get entity
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	assert.Equal(t, "Alice@example.com", retrieved["email"])
	assert.Equal(t, true, retrieved["active"])
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	_, err := store.Get(ctx, "users", 999)
	assert.ErrorIs(t, err, storage.ErrNotFound)
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
	require.NoError(t, err)
	
	// Update entity
	err = store.Update(ctx, "users", id, map[string]interface{}{
		"name": "Alice Smith",
		"age":  31,
	})
	require.NoError(t, err)
	
	// Verify update
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice Smith", retrieved["name"])
	assert.Equal(t, float64(31), retrieved["age"])
}

func TestSQLiteStore_UpdateNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	err := store.Update(ctx, "users", 999, map[string]interface{}{"name": "Nobody"})
	assert.ErrorIs(t, err, storage.ErrNotFound)
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
	require.NoError(t, err)
	
	// Patch only age
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"age": 31,
	})
	require.NoError(t, err)
	
	// Verify only age changed
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	assert.Equal(t, "alice@example.com", retrieved["email"])
	assert.Equal(t, float64(31), retrieved["age"])
}

func TestSQLiteStore_PatchAddField(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
	})
	require.NoError(t, err)
	
	// Add new field
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"email": "alice@example.com",
	})
	require.NoError(t, err)
	
	// Verify field was added
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	assert.Equal(t, "alice@example.com", retrieved["email"])
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
	require.NoError(t, err)
	
	// Remove field by setting to nil
	err = store.Patch(ctx, "users", id, map[string]interface{}{
		"email": nil,
	})
	require.NoError(t, err)
	
	// Verify field was removed
	retrieved, err := store.Get(ctx, "users", id)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	_, hasEmail := retrieved["email"]
	assert.False(t, hasEmail)
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
	})
	require.NoError(t, err)
	
	// Verify exists
	assert.True(t, store.Exists(ctx, "users", id))
	
	// Delete entity
	err = store.Delete(ctx, "users", id)
	require.NoError(t, err)
	
	// Verify deleted
	assert.False(t, store.Exists(ctx, "users", id))
	_, err = store.Get(ctx, "users", id)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestSQLiteStore_DeleteNotFound(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	err := store.Delete(ctx, "users", 999)
	assert.ErrorIs(t, err, storage.ErrNotFound)
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
	require.NoError(t, err)
	assert.Len(t, results, 3)
	
	// Verify names
	names := []string{}
	for _, result := range results {
		names = append(names, result["name"].(string))
	}
	assert.Contains(t, names, "Alice")
	assert.Contains(t, names, "Bob")
	assert.Contains(t, names, "Charlie")
}

func TestSQLiteStore_ListEmpty(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	results, err := store.List(ctx, "users")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSQLiteStore_Save(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Save with specific ID
	err := store.Save(ctx, "users", 100, map[string]interface{}{
		"name": "Alice",
	})
	require.NoError(t, err)
	
	// Verify saved
	retrieved, err := store.Get(ctx, "users", 100)
	require.NoError(t, err)
	assert.Equal(t, "Alice", retrieved["name"])
	assert.Equal(t, float64(100), retrieved["id"])
	
	// Try to save again (should fail)
	err = store.Save(ctx, "users", 100, map[string]interface{}{
		"name": "Bob",
	})
	assert.ErrorIs(t, err, storage.ErrAlreadyExists)
}

func TestSQLiteStore_SaveUpdatesSequence(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Save with high ID
	err := store.Save(ctx, "users", 100, map[string]interface{}{"name": "Alice"})
	require.NoError(t, err)
	
	// Next Create should have ID > 100
	id, err := store.Create(ctx, "users", map[string]interface{}{"name": "Bob"})
	require.NoError(t, err)
	assert.Greater(t, id, 100)
}

func TestSQLiteStore_Exists(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create entity
	id, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Alice",
	})
	require.NoError(t, err)
	
	// Check exists
	assert.True(t, store.Exists(ctx, "users", id))
	assert.False(t, store.Exists(ctx, "users", 999))
	assert.False(t, store.Exists(ctx, "nonexistent", 1))
}

// =============================================================================
// Search Tests
// =============================================================================

func TestSQLiteStore_SearchContains(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create test data
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Smith"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob Jones"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Brown"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Charlie"})
	
	// Test Search interface
	searcher, ok := store.(storage.Searcher)
	require.True(t, ok, "SQLiteStore should implement Searcher interface")
	
	// Search contains
	results, err := searcher.Search(ctx, "users", "name", "Alice", "contains")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// Verify both Alice results
	names := []string{results[0]["name"].(string), results[1]["name"].(string)}
	assert.Contains(t, names, "Alice Smith")
	assert.Contains(t, names, "Alice Brown")
}

func TestSQLiteStore_SearchExact(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Smith"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob Jones"})
	
	searcher := store.(storage.Searcher)
	
	// Search exact
	results, err := searcher.Search(ctx, "users", "name", "Bob Jones", "exact")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Bob Jones", results[0]["name"])
}

func TestSQLiteStore_SearchStarts(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Smith"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Brown"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob Alice"})
	
	searcher := store.(storage.Searcher)
	
	// Search starts
	results, err := searcher.Search(ctx, "users", "name", "Alice", "starts")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSQLiteStore_SearchEnds(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice Smith"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Bob Smith"})
	store.Create(ctx, "users", map[string]interface{}{"name": "Charlie"})
	
	searcher := store.(storage.Searcher)
	
	// Search ends
	results, err := searcher.Search(ctx, "users", "name", "Smith", "ends")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSQLiteStore_SearchNoResults(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	store.Create(ctx, "users", map[string]interface{}{"name": "Alice"})
	
	searcher := store.(storage.Searcher)
	
	results, err := searcher.Search(ctx, "users", "name", "NonExistent", "contains")
	require.NoError(t, err)
	assert.Empty(t, results)
}

// =============================================================================
// Graph Synchronization Tests
// =============================================================================

func TestSQLiteStore_GraphSyncOnCreate(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create manager
	managerId, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Manager",
	})
	require.NoError(t, err)
	
	// Create employee with REF to manager
	employeeId, err := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})
	require.NoError(t, err)
	
	// Test GraphNeighbors interface
	graphStore, ok := store.(storage.GraphNeighbors)
	require.True(t, ok, "SQLiteStore should implement GraphNeighbors interface")
	
	// Get neighbors (should find manager)
	neighbors, err := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	require.NoError(t, err)
	assert.Len(t, neighbors, 1)
	assert.Equal(t, "Manager", neighbors[0]["name"])
	assert.Equal(t, "manager", neighbors[0]["_relationship"])
	assert.Equal(t, "out", neighbors[0]["_direction"])
}

func TestSQLiteStore_GraphSyncOnUpdate(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create two managers
	manager1Id, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager1"})
	manager2Id, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager2"})
	
	// Create employee with manager1
	employeeId, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     manager1Id,
		},
	})
	
	graphStore := store.(storage.GraphNeighbors)
	
	// Verify manager1 is neighbor
	neighbors, _ := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	assert.Len(t, neighbors, 1)
	assert.Equal(t, "Manager1", neighbors[0]["name"])
	
	// Update to manager2
	store.Update(ctx, "users", employeeId, map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     manager2Id,
		},
	})
	
	// Verify manager2 is now neighbor (old edge should be gone)
	neighbors, _ = graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	assert.Len(t, neighbors, 1)
	assert.Equal(t, "Manager2", neighbors[0]["name"])
}

func TestSQLiteStore_GraphSyncOnDelete(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create manager and employee
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	employeeId, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
	})
	
	graphStore := store.(storage.GraphNeighbors)
	
	// Verify neighbor exists
	neighbors, _ := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	assert.Len(t, neighbors, 1)
	
	// Delete employee
	store.Delete(ctx, "users", employeeId)
	
	// Verify edges cleaned up
	neighbors, err := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	require.NoError(t, err)
	assert.Len(t, neighbors, 0)
}

func TestSQLiteStore_GraphMultipleRefs(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create manager and mentor
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	mentorId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Mentor"})
	
	// Create employee with multiple REFs
	employeeId, _ := store.Create(ctx, "users", map[string]interface{}{
		"name": "Employee",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     managerId,
		},
		"mentor": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     mentorId,
		},
	})
	
	graphStore := store.(storage.GraphNeighbors)
	
	// Should have 2 outgoing edges
	neighbors, err := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	require.NoError(t, err)
	assert.Len(t, neighbors, 2)
	
	// Verify both relationships
	relationships := make(map[string]string)
	for _, n := range neighbors {
		relationships[n["_relationship"].(string)] = n["name"].(string)
	}
	assert.Equal(t, "Manager", relationships["manager"])
	assert.Equal(t, "Mentor", relationships["mentor"])
}

func TestSQLiteStore_GraphIncomingEdges(t *testing.T) {
	store, cleanup := setupSQLiteTest(t)
	defer cleanup()
	
	ctx := context.Background()
	
	// Create manager
	managerId, _ := store.Create(ctx, "users", map[string]interface{}{"name": "Manager"})
	
	// Create two employees reporting to same manager
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
	
	graphStore := store.(storage.GraphNeighbors)
	
	// Get incoming edges (employees who report to this manager)
	neighbors, err := graphStore.GetNeighbors(ctx, "users", managerId, "in")
	require.NoError(t, err)
	assert.Len(t, neighbors, 2)
	
	// Verify direction
	for _, n := range neighbors {
		assert.Equal(t, "in", n["_direction"])
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
	require.True(t, ok, "SQLiteStore should implement GraphIntegrity interface")
	
	// Verify integrity
	err := integrityStore.VerifyGraphIntegrity(ctx)
	assert.NoError(t, err)
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
	
	integrityStore := store.(storage.GraphIntegrity)
	graphStore := store.(storage.GraphNeighbors)
	
	// Verify graph works before rebuild
	neighbors, _ := graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	assert.Len(t, neighbors, 1)
	
	// Rebuild graph
	err := integrityStore.RebuildGraph(ctx)
	require.NoError(t, err)
	
	// Verify graph still works after rebuild
	neighbors, err = graphStore.GetNeighbors(ctx, "users", employeeId, "out")
	require.NoError(t, err)
	assert.Len(t, neighbors, 1)
	assert.Equal(t, "Manager", neighbors[0]["name"])
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
	errors := make(chan error, count)
	ids := make(chan int, count)
	
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id, err := store.Create(ctx, "users", map[string]interface{}{
				"name": fmt.Sprintf("User%d", n),
			})
			if err != nil {
				errors <- err
			} else {
				ids <- id
			}
		}(i)
	}
	
	wg.Wait()
	close(errors)
	close(ids)
	
	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent create error: %v", err)
	}
	
	// Verify all created
	results, err := store.List(ctx, "users")
	require.NoError(t, err)
	assert.Len(t, results, count)
	
	// Verify IDs are unique
	idSet := make(map[int]bool)
	for id := range ids {
		assert.False(t, idSet[id], "Duplicate ID: %d", id)
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
	errors := make(chan error, 100)
	
	// Concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 1; j <= 5; j++ {
				_, err := store.Get(ctx, "users", j)
				if err != nil {
					errors <- err
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
				errors <- err
			}
		}(i)
	}
	
	wg.Wait()
	close(errors)
	
	// Check for errors
	for err := range errors {
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
	require.True(t, ok, "SQLiteStore should implement InfoProvider interface")
	
	info := infoProvider.Info()
	assert.Equal(t, "sqlite", info.Type)
	assert.NotEmpty(t, info.Version)
	assert.True(t, info.SupportsSearch)
	assert.True(t, info.SupportsTransaction)
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
