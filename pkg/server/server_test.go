package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ha1tch/olu/pkg/cache"
	"github.com/ha1tch/olu/pkg/config"
	"github.com/ha1tch/olu/pkg/graph"
	"github.com/ha1tch/olu/pkg/server"
	"github.com/ha1tch/olu/pkg/storage"
	"github.com/ha1tch/olu/pkg/validation"
	"github.com/rs/zerolog"
)

// TestServer holds test server instance and helpers
type TestServer struct {
	server *server.Server
	ts     *httptest.Server
	cfg    *config.Config
	t      *testing.T
}

// setupTestServer creates a test server with temporary storage
func setupTestServer(t *testing.T) *TestServer {
	// Create temporary directory for test data
	tmpDir, err := os.MkdirTemp("", "olu-test-*")
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Host:               "localhost",
		Port:               0, // Let httptest choose port
		BaseDir:            tmpDir,
		Schema:             "test_schema",
		CacheType:          "memory",
		CacheTTL:           300,
		GraphEnabled:       true,
		GraphMode:          "indexed",
		FullTextEnabled:    false,
		CascadingDelete:    false,
		RefEmbedDepth:      3,
		MaxEmbedDepth:      10,
		MaxEntitySize:      1048576,
		PatchNullBehavior:  "store",
		GraphDataFile:      filepath.Join(tmpDir, "graph.data"),
		GraphIndexFile:     filepath.Join(tmpDir, "graph.index"),
		MaxCascadeDeletions: 100,
	}

	// Initialize components
	storeConfig := map[string]interface{}{
		"base_dir": cfg.BaseDir,
		"schema":   cfg.Schema,
	}

	store, err := storage.NewStore("jsonfile", storeConfig)
	if err != nil {
		t.Fatal(err)
	}

	memCache := cache.NewMemoryCache(1000, time.Duration(cfg.CacheTTL)*time.Second)
	g := graph.NewIndexedGraph()
	schemaDir := filepath.Join(cfg.BaseDir, cfg.Schema, "_schemas")
	validator := validation.NewJSONSchemaValidator(schemaDir)
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)

	srv := server.New(cfg, store, memCache, g, validator, logger)
	ts := httptest.NewServer(srv.Handler())

	return &TestServer{
		server: srv,
		ts:     ts,
		cfg:    cfg,
		t:      t,
	}
}

// cleanup removes temporary test data
func (ts *TestServer) cleanup() {
	ts.ts.Close()
	os.RemoveAll(ts.cfg.BaseDir)
}

// doRequest makes HTTP request and returns response
func (ts *TestServer) doRequest(method, path string, body interface{}) (*http.Response, []byte) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			ts.t.Fatal(err)
		}
	}

	req, err := http.NewRequest(method, ts.ts.URL+path, bytes.NewBuffer(bodyBytes))
	if err != nil {
		ts.t.Fatal(err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ts.t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody := &bytes.Buffer{}
	respBody.ReadFrom(resp.Body)

	return resp, respBody.Bytes()
}

// TestHealthEndpoints tests health and version endpoints
func TestHealthEndpoints(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	t.Run("GET /health", func(t *testing.T) {
		resp, body := ts.doRequest("GET", "/health", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		if result["status"] != "ok" {
			t.Errorf("Expected status ok, got %v", result["status"])
		}
	})

	t.Run("GET /version", func(t *testing.T) {
		resp, body := ts.doRequest("GET", "/version", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		if result["version"] == nil {
			t.Error("Expected version field")
		}
	})
}

// TestEntityCRUD tests complete entity CRUD operations
func TestEntityCRUD(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	entity := "users"
	var createdID float64

	t.Run("POST /api/v1/{entity} - Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Alice Smith",
			"email": "alice@example.com",
			"age":   30,
		}

		resp, body := ts.doRequest("POST", "/api/v1/"+entity, data)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected 201, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		if result["id"] == nil {
			t.Fatal("Expected id in response")
		}
		createdID = result["id"].(float64)
	})

	t.Run("GET /api/v1/{entity}/{id} - Get", func(t *testing.T) {
		resp, body := ts.doRequest("GET", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		if result["name"] != "Alice Smith" {
			t.Errorf("Expected name 'Alice Smith', got %v", result["name"])
		}
		if result["email"] != "alice@example.com" {
			t.Errorf("Expected email 'alice@example.com', got %v", result["email"])
		}
	})

	t.Run("PUT /api/v1/{entity}/{id} - Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Alice Johnson",
			"email": "alice.johnson@example.com",
			"age":   31,
		}

		resp, body := ts.doRequest("PUT", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), data)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify update
		resp, body = ts.doRequest("GET", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), nil)
		var result map[string]interface{}
		json.Unmarshal(body, &result)

		if result["name"] != "Alice Johnson" {
			t.Errorf("Expected updated name, got %v", result["name"])
		}
	})

	t.Run("PATCH /api/v1/{entity}/{id} - Patch", func(t *testing.T) {
		data := map[string]interface{}{
			"age": 32,
		}

		resp, body := ts.doRequest("PATCH", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), data)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify patch
		resp, body = ts.doRequest("GET", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), nil)
		var result map[string]interface{}
		json.Unmarshal(body, &result)

		if result["age"].(float64) != 32 {
			t.Errorf("Expected age 32, got %v", result["age"])
		}
		// Name should still be from update
		if result["name"] != "Alice Johnson" {
			t.Errorf("Expected name unchanged, got %v", result["name"])
		}
	})

	t.Run("GET /api/v1/{entity} - List", func(t *testing.T) {
		// Create another entity
		data := map[string]interface{}{
			"name":  "Bob Smith",
			"email": "bob@example.com",
			"age":   25,
		}
		ts.doRequest("POST", "/api/v1/"+entity, data)

		resp, body := ts.doRequest("GET", "/api/v1/"+entity, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		dataArray, ok := result["data"].([]interface{})
		if !ok {
			t.Fatal("Expected data array")
		}

		if len(dataArray) < 2 {
			t.Errorf("Expected at least 2 entities, got %d", len(dataArray))
		}
	})

	t.Run("DELETE /api/v1/{entity}/{id} - Delete", func(t *testing.T) {
		resp, body := ts.doRequest("DELETE", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify deletion
		resp, _ = ts.doRequest("GET", fmt.Sprintf("/api/v1/%s/%d", entity, int(createdID)), nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 after delete, got %d", resp.StatusCode)
		}
	})
}

// TestEntitySave tests save with specific ID
func TestEntitySave(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	t.Run("POST /api/v1/{entity}/save/{id}", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Charlie",
			"email": "charlie@example.com",
		}

		resp, body := ts.doRequest("POST", "/api/v1/users/save/100", data)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected 201, got %d: %s", resp.StatusCode, string(body))
		}

		// Verify entity exists with ID 100
		resp, body = ts.doRequest("GET", "/api/v1/users/100", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if result["id"].(float64) != 100 {
			t.Errorf("Expected id 100, got %v", result["id"])
		}
	})

	t.Run("POST /api/v1/{entity}/save/{id} - Duplicate", func(t *testing.T) {
		data := map[string]interface{}{
			"name": "Duplicate",
		}

		resp, _ := ts.doRequest("POST", "/api/v1/users/save/100", data)
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected 409 for duplicate, got %d", resp.StatusCode)
		}
	})
}

// TestEntityReferences tests entity references and graph updates
func TestEntityReferences(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	var managerID, employeeID float64

	t.Run("Create entities with references", func(t *testing.T) {
		// Create manager
		manager := map[string]interface{}{
			"name": "Manager Bob",
			"role": "manager",
		}
		resp, body := ts.doRequest("POST", "/api/v1/users", manager)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to create manager: %s", string(body))
		}
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		managerID = result["id"].(float64)

		// Create employee with reference to manager
		employee := map[string]interface{}{
			"name": "Employee Alice",
			"role": "employee",
			"manager": map[string]interface{}{
				"type":   "REF",
				"entity": "users",
				"id":     managerID,
			},
		}
		resp, body = ts.doRequest("POST", "/api/v1/users", employee)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to create employee: %s", string(body))
		}
		json.Unmarshal(body, &result)
		employeeID = result["id"].(float64)
	})

	t.Run("Get with embedded references", func(t *testing.T) {
		resp, body := ts.doRequest("GET", fmt.Sprintf("/api/v1/users/%d?embed_depth=1", int(employeeID)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get employee: %s", string(body))
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		manager, ok := result["manager"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected manager to be embedded")
		}

		if manager["name"] != "Manager Bob" {
			t.Errorf("Expected embedded manager name, got %v", manager["name"])
		}
	})
}

// TestGraphOperations tests graph endpoints
func TestGraphOperations(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create test data with relationships
	var user1ID, user2ID, user3ID float64

	// Setup: Create users with relationships
	user1 := map[string]interface{}{"name": "User1"}
	_, body := ts.doRequest("POST", "/api/v1/users", user1)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	user1ID = result["id"].(float64)

	user2 := map[string]interface{}{
		"name": "User2",
		"friend": map[string]interface{}{
			"type": "REF", "entity": "users", "id": user1ID,
		},
	}
	_, body = ts.doRequest("POST", "/api/v1/users", user2)
	json.Unmarshal(body, &result)
	user2ID = result["id"].(float64)

	user3 := map[string]interface{}{
		"name": "User3",
		"friend": map[string]interface{}{
			"type": "REF", "entity": "users", "id": user2ID,
		},
	}
	_, body = ts.doRequest("POST", "/api/v1/users", user3)
	json.Unmarshal(body, &result)
	user3ID = result["id"].(float64)

	t.Run("GET /api/v1/graph/stats", func(t *testing.T) {
		resp, body := ts.doRequest("GET", "/api/v1/graph/stats", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		if result["node_count"] == nil {
			t.Error("Expected node_count in response")
		}
		if result["edge_count"] == nil {
			t.Error("Expected edge_count in response")
		}
	})

	t.Run("POST /api/v1/graph/path", func(t *testing.T) {
		data := map[string]interface{}{
			"from":      fmt.Sprintf("users:%d", int(user3ID)),
			"to":        fmt.Sprintf("users:%d", int(user1ID)),
			"max_depth": 10,
		}

		resp, body := ts.doRequest("POST", "/api/v1/graph/path", data)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		path, ok := result["path"].([]interface{})
		if !ok {
			t.Fatal("Expected path in response")
		}

		if len(path) < 2 {
			t.Errorf("Expected path with at least 2 nodes, got %d", len(path))
		}
	})

	t.Run("POST /api/v1/graph/neighbors", func(t *testing.T) {
		data := map[string]interface{}{
			"node_id": fmt.Sprintf("users:%d", int(user2ID)),
		}

		resp, body := ts.doRequest("POST", "/api/v1/graph/neighbors", data)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		neighbors, ok := result["neighbors"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected neighbors in response")
		}

		if len(neighbors) == 0 {
			t.Error("Expected at least one neighbor")
		}
	})
}

// TestSchemaOperations tests schema endpoints
func TestSchemaOperations(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	entity := "products"

	t.Run("POST /api/v1/schema/{entity}", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"required": []string{"name", "price"},
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
				"price": map[string]interface{}{
					"type": "number",
				},
			},
		}

		resp, body := ts.doRequest("POST", "/api/v1/schema/"+entity, schema)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected 201, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("GET /api/v1/schema/{entity}", func(t *testing.T) {
		resp, body := ts.doRequest("GET", "/api/v1/schema/"+entity, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		if result["type"] != "object" {
			t.Error("Expected schema type object")
		}
	})
}

// TestPagination tests list pagination
func TestPagination(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Create 10 test entities
	for i := 0; i < 10; i++ {
		data := map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
			"age":  20 + i,
		}
		ts.doRequest("POST", "/api/v1/users", data)
	}

	t.Run("Pagination parameters", func(t *testing.T) {
		resp, body := ts.doRequest("GET", "/api/v1/users?page=1&per_page=5", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		pagination, ok := result["pagination"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected pagination in response")
		}

		if pagination["page"].(float64) != 1 {
			t.Errorf("Expected page 1, got %v", pagination["page"])
		}
		if pagination["per_page"].(float64) != 5 {
			t.Errorf("Expected per_page 5, got %v", pagination["per_page"])
		}

		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatal("Expected data array")
		}
		if len(data) != 5 {
			t.Errorf("Expected 5 items, got %d", len(data))
		}
	})
}

// TestErrorHandling tests error responses
func TestErrorHandling(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	t.Run("GET non-existent entity", func(t *testing.T) {
		resp, _ := ts.doRequest("GET", "/api/v1/users/99999", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid ID format", func(t *testing.T) {
		resp, _ := ts.doRequest("GET", "/api/v1/users/invalid", nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid JSON body", func(t *testing.T) {
		req, _ := http.NewRequest("POST", ts.ts.URL+"/api/v1/users", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})
}
