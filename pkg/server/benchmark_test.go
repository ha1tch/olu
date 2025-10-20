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

// setupBenchServer creates a server for benchmarking
func setupBenchServer(b *testing.B) (*httptest.Server, *config.Config) {
	b.Helper()

	tmpDir, err := os.MkdirTemp("", "olu-bench-*")
	if err != nil {
		b.Fatal(err)
	}

	cfg := &config.Config{
		BaseDir:            tmpDir,
		Schema:             "bench_schema",
		CacheType:          "memory",
		CacheTTL:           300,
		GraphEnabled:       true,
		GraphMode:          "indexed",
		FullTextEnabled:    false,
		CascadingDelete:    false,
		RefEmbedDepth:      3,
		MaxEmbedDepth:      10,
		GraphDataFile:      filepath.Join(tmpDir, "graph.data"),
		GraphIndexFile:     filepath.Join(tmpDir, "graph.index"),
		MaxCascadeDeletions: 100,
	}

	storeConfig := map[string]interface{}{
		"base_dir": cfg.BaseDir,
		"schema":   cfg.Schema,
	}

	store, _ := storage.NewStore("jsonfile", storeConfig)
	memCache := cache.NewMemoryCache(1000, time.Duration(cfg.CacheTTL)*time.Second)
	g := graph.NewIndexedGraph()
	schemaDir := filepath.Join(cfg.BaseDir, cfg.Schema, "_schemas")
	validator := validation.NewJSONSchemaValidator(schemaDir)
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)

	srv := server.New(cfg, store, memCache, g, validator, logger)
	ts := httptest.NewServer(srv.Handler())

	b.Cleanup(func() {
		ts.Close()
		os.RemoveAll(tmpDir)
	})

	return ts, cfg
}

// BenchmarkCreate benchmarks entity creation
func BenchmarkCreate(b *testing.B) {
	ts, _ := setupBenchServer(b)

	data := map[string]interface{}{
		"name":  "Benchmark User",
		"email": "bench@example.com",
		"age":   25,
	}

	bodyBytes, _ := json.Marshal(data)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkGet benchmarks entity retrieval
func BenchmarkGet(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create test entity
	data := map[string]interface{}{
		"name":  "Benchmark User",
		"email": "bench@example.com",
	}
	bodyBytes, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	
	id := int(result["id"].(float64))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%d", ts.URL, id), nil)
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkUpdate benchmarks entity updates
func BenchmarkUpdate(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create test entity
	data := map[string]interface{}{
		"name":  "Benchmark User",
		"email": "bench@example.com",
	}
	bodyBytes, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	
	id := int(result["id"].(float64))

	updateData := map[string]interface{}{
		"name":  "Updated User",
		"email": "updated@example.com",
	}
	updateBytes, _ := json.Marshal(updateData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/users/%d", ts.URL, id), bytes.NewBuffer(updateBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}

// BenchmarkPatch benchmarks partial updates
func BenchmarkPatch(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create test entity
	data := map[string]interface{}{
		"name":  "Benchmark User",
		"email": "bench@example.com",
		"age":   25,
	}
	bodyBytes, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	
	id := int(result["id"].(float64))

	patchData := map[string]interface{}{
		"age": 26,
	}
	patchBytes, _ := json.Marshal(patchData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/users/%d", ts.URL, id), bytes.NewBuffer(patchBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}

// BenchmarkList benchmarks entity listing
func BenchmarkList(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create 100 test entities
	for i := 0; i < 100; i++ {
		data := map[string]interface{}{
			"name":  fmt.Sprintf("User%d", i),
			"email": fmt.Sprintf("user%d@example.com", i),
		}
		bodyBytes, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("GET", ts.URL+"/api/v1/users", nil)
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkListPaginated benchmarks paginated listing
func BenchmarkListPaginated(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create 100 test entities
	for i := 0; i < 100; i++ {
		data := map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		}
		bodyBytes, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("GET", ts.URL+"/api/v1/users?page=1&per_page=20", nil)
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkDelete benchmarks entity deletion
func BenchmarkDelete(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Pre-create entities to delete
	ids := make([]int, b.N)
	for i := 0; i < b.N; i++ {
		data := map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		}
		bodyBytes, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		ids[i] = int(result["id"].(float64))
		resp.Body.Close()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/users/%d", ts.URL, ids[i]), nil)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}

// BenchmarkGraphPath benchmarks path finding
func BenchmarkGraphPath(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create chain of users
	var prevID float64
	for i := 0; i < 10; i++ {
		data := map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		}
		if i > 0 {
			data["friend"] = map[string]interface{}{
				"type":   "REF",
				"entity": "users",
				"id":     prevID,
			}
		}
		bodyBytes, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		prevID = result["id"].(float64)
		resp.Body.Close()
	}

	pathData := map[string]interface{}{
		"from":      fmt.Sprintf("users:%d", int(prevID)),
		"to":        "users:1",
		"max_depth": 20,
	}
	pathBytes, _ := json.Marshal(pathData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/graph/path", bytes.NewBuffer(pathBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}

// BenchmarkGraphNeighbors benchmarks neighbor queries
func BenchmarkGraphNeighbors(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create user with multiple friends
	mainUser := map[string]interface{}{"name": "MainUser"}
	bodyBytes, _ := json.Marshal(mainUser)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	mainID := result["id"].(float64)
	resp.Body.Close()

	// Create friends
	for i := 0; i < 5; i++ {
		friend := map[string]interface{}{
			"name": fmt.Sprintf("Friend%d", i),
			"friendOf": map[string]interface{}{
				"type":   "REF",
				"entity": "users",
				"id":     mainID,
			},
		}
		bodyBytes, _ := json.Marshal(friend)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	neighborsData := map[string]interface{}{
		"node_id": fmt.Sprintf("users:%d", int(mainID)),
	}
	neighborsBytes, _ := json.Marshal(neighborsData)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("POST", ts.URL+"/api/v1/graph/neighbors", bytes.NewBuffer(neighborsBytes))
			req.Header.Set("Content-Type", "application/json")
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkHealthCheck benchmarks health endpoint
func BenchmarkHealthCheck(b *testing.B) {
	ts, _ := setupBenchServer(b)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("GET", ts.URL+"/health", nil)
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()
		}
	})
}

// BenchmarkConcurrentOperations benchmarks mixed concurrent operations
func BenchmarkConcurrentOperations(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Pre-create some entities
	for i := 0; i < 10; i++ {
		data := map[string]interface{}{
			"name": fmt.Sprintf("User%d", i),
		}
		bodyBytes, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0: // Create
				data := map[string]interface{}{"name": "New User"}
				bodyBytes, _ := json.Marshal(data)
				req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				resp, _ := http.DefaultClient.Do(req)
				resp.Body.Close()
			case 1: // Get
				req, _ := http.NewRequest("GET", ts.URL+"/api/v1/users/1", nil)
				resp, _ := http.DefaultClient.Do(req)
				resp.Body.Close()
			case 2: // List
				req, _ := http.NewRequest("GET", ts.URL+"/api/v1/users", nil)
				resp, _ := http.DefaultClient.Do(req)
				resp.Body.Close()
			case 3: // Update
				data := map[string]interface{}{"name": "Updated"}
				bodyBytes, _ := json.Marshal(data)
				req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/users/1", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				resp, _ := http.DefaultClient.Do(req)
				resp.Body.Close()
			}
			i++
		}
	})
}

// BenchmarkCreateWithReferences benchmarks creating entities with references
func BenchmarkCreateWithReferences(b *testing.B) {
	ts, _ := setupBenchServer(b)

	// Create a reference entity
	refData := map[string]interface{}{"name": "Reference"}
	bodyBytes, _ := json.Marshal(refData)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	refID := result["id"].(float64)
	resp.Body.Close()

	data := map[string]interface{}{
		"name": "User with Ref",
		"manager": map[string]interface{}{
			"type":   "REF",
			"entity": "users",
			"id":     refID,
		},
	}
	dataBytes, _ := json.Marshal(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/users", bytes.NewBuffer(dataBytes))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}
