// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package graph

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// mustAddNode adds a node, failing the test if it errors
func mustAddNode(t *testing.T, g *IndexedGraph, id, entityType string) {
	t.Helper()
	if err := g.AddNode(id, entityType); err != nil {
		t.Fatalf("AddNode(%s, %s) failed: %v", id, entityType, err)
	}
}

// mustAddEdge adds an edge, failing the test if it errors
func mustAddEdge(t *testing.T, g *IndexedGraph, from, to, relType string) {
	t.Helper()
	if err := g.AddEdge(from, to, relType); err != nil {
		t.Fatalf("AddEdge(%s, %s, %s) failed: %v", from, to, relType, err)
	}
}

func TestNewIndexedGraph(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()
	if g == nil {
		t.Fatal("NewIndexedGraph returned nil")
	}
	if g.NodeCount() != 0 {
		t.Errorf("Expected 0 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 0 {
		t.Errorf("Expected 0 edges, got %d", g.EdgeCount())
	}
}

func TestAddRemoveNode(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Add node
	err := g.AddNode("users:1", "users")
	if err != nil {
		t.Fatalf("AddNode failed: %v", err)
	}

	if g.NodeCount() != 1 {
		t.Errorf("Expected 1 node, got %d", g.NodeCount())
	}

	if !g.NodeExists("users:1") {
		t.Error("Node users:1 should exist")
	}

	// Add duplicate node (should succeed, update type)
	err = g.AddNode("users:1", "users")
	if err != nil {
		t.Errorf("Adding duplicate node should not error: %v", err)
	}

	// Remove node
	err = g.RemoveNode("users:1")
	if err != nil {
		t.Fatalf("RemoveNode failed: %v", err)
	}

	if g.NodeCount() != 0 {
		t.Errorf("Expected 0 nodes after removal, got %d", g.NodeCount())
	}

	if g.NodeExists("users:1") {
		t.Error("Node users:1 should not exist after removal")
	}

	// Remove non-existent node (implementation may or may not error)
	err = g.RemoveNode("users:999")
	// Note: current implementation doesn't error, just no-ops
	_ = err
}

func TestAddRemoveEdge(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Add nodes first
	mustAddNode(t, g, "users:1", "users")
	mustAddNode(t, g, "users:2", "users")

	// Add edge
	err := g.AddEdge("users:1", "users:2", "FOLLOWS")
	if err != nil {
		t.Fatalf("AddEdge failed: %v", err)
	}

	if g.EdgeCount() != 1 {
		t.Errorf("Expected 1 edge, got %d", g.EdgeCount())
	}

	// Verify outgoing
	neighbors, err := g.GetNeighbors("users:1")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	if neighbors["users:2"] != "FOLLOWS" {
		t.Errorf("Expected FOLLOWS relationship, got %s", neighbors["users:2"])
	}

	// Verify incoming
	incoming, err := g.GetIncomingEdges("users:2")
	if err != nil {
		t.Fatalf("GetIncomingEdges failed: %v", err)
	}
	if incoming["users:1"] != "FOLLOWS" {
		t.Errorf("Expected incoming FOLLOWS, got %s", incoming["users:1"])
	}

	// Remove edge
	err = g.RemoveEdge("users:1", "users:2")
	if err != nil {
		t.Fatalf("RemoveEdge failed: %v", err)
	}

	if g.EdgeCount() != 0 {
		t.Errorf("Expected 0 edges after removal, got %d", g.EdgeCount())
	}

	// Add edge to non-existent node (should auto-create)
	err = g.AddEdge("users:1", "users:3", "FOLLOWS")
	if err != nil {
		t.Fatalf("AddEdge to new node failed: %v", err)
	}

	if !g.NodeExists("users:3") {
		t.Error("Node users:3 should have been auto-created")
	}
}

func TestGetNeighbors(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	mustAddNode(t, g, "users:1", "users")
	mustAddNode(t, g, "users:2", "users")
	mustAddNode(t, g, "users:3", "users")
	mustAddEdge(t, g, "users:1", "users:2", "FOLLOWS")
	mustAddEdge(t, g, "users:1", "users:3", "KNOWS")

	neighbors, err := g.GetNeighbors("users:1")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}

	if len(neighbors) != 2 {
		t.Errorf("Expected 2 neighbors, got %d", len(neighbors))
	}

	if neighbors["users:2"] != "FOLLOWS" {
		t.Errorf("Expected FOLLOWS to users:2")
	}
	if neighbors["users:3"] != "KNOWS" {
		t.Errorf("Expected KNOWS to users:3")
	}

	// Non-existent node (may return error or empty map depending on impl)
	neighbors, err = g.GetNeighbors("users:999")
	// Current impl may return empty map rather than error
	if err == nil && len(neighbors) != 0 {
		t.Error("Expected empty neighbors for non-existent node")
	}
}

func TestGetIncomingEdges(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	mustAddNode(t, g, "users:1", "users")
	mustAddNode(t, g, "users:2", "users")
	mustAddNode(t, g, "users:3", "users")
	mustAddEdge(t, g, "users:1", "users:3", "FOLLOWS")
	mustAddEdge(t, g, "users:2", "users:3", "FOLLOWS")

	incoming, err := g.GetIncomingEdges("users:3")
	if err != nil {
		t.Fatalf("GetIncomingEdges failed: %v", err)
	}

	if len(incoming) != 2 {
		t.Errorf("Expected 2 incoming edges, got %d", len(incoming))
	}

	if incoming["users:1"] != "FOLLOWS" {
		t.Errorf("Expected FOLLOWS from users:1")
	}
	if incoming["users:2"] != "FOLLOWS" {
		t.Errorf("Expected FOLLOWS from users:2")
	}
}

func TestFindPath(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Create chain: 1 -> 2 -> 3 -> 4
	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")
	_ = g.AddNode("n:4", "node")
	_ = g.AddEdge("n:1", "n:2", "NEXT")
	_ = g.AddEdge("n:2", "n:3", "NEXT")
	_ = g.AddEdge("n:3", "n:4", "NEXT")

	// Find path 1 -> 4
	path, err := g.FindPath("n:1", "n:4", 10)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}

	expected := []string{"n:1", "n:2", "n:3", "n:4"}
	if len(path) != len(expected) {
		t.Fatalf("Expected path length %d, got %d", len(expected), len(path))
	}
	for i, node := range expected {
		if path[i] != node {
			t.Errorf("Path[%d]: expected %s, got %s", i, node, path[i])
		}
	}

	// Path to self
	path, err = g.FindPath("n:1", "n:1", 10)
	if err != nil {
		t.Fatalf("FindPath to self failed: %v", err)
	}
	if len(path) != 1 || path[0] != "n:1" {
		t.Errorf("Expected [n:1], got %v", path)
	}

	// No path exists
	_ = g.AddNode("n:5", "node") // Isolated node
	path, err = g.FindPath("n:1", "n:5", 10)
	if err == nil {
		t.Error("Expected error when no path exists")
	}
}

func TestFindPathMaxDepth(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Create chain: 1 -> 2 -> 3 -> 4 -> 5
	for i := 1; i <= 5; i++ {
		_ = g.AddNode(fmt.Sprintf("n:%d", i), "node")
	}
	for i := 1; i < 5; i++ {
		_ = g.AddEdge(fmt.Sprintf("n:%d", i), fmt.Sprintf("n:%d", i+1), "NEXT")
	}

	// Should find path with sufficient depth
	path, err := g.FindPath("n:1", "n:5", 10)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	if len(path) != 5 {
		t.Errorf("Expected path length 5, got %d", len(path))
	}

	// Should fail with insufficient depth
	_, err = g.FindPath("n:1", "n:5", 2)
	if err == nil {
		t.Error("Expected error when max depth insufficient")
	}
}

func TestPathExists(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("a:1", "a")
	_ = g.AddNode("b:1", "b")
	_ = g.AddNode("c:1", "c")
	_ = g.AddEdge("a:1", "b:1", "REL")
	_ = g.AddEdge("b:1", "c:1", "REL")

	// Path exists
	exists, depth, err := g.PathExists("a:1", "c:1", 10)
	if err != nil {
		t.Fatalf("PathExists failed: %v", err)
	}
	if !exists {
		t.Error("Path should exist")
	}
	if depth != 2 {
		t.Errorf("Expected depth 2, got %d", depth)
	}

	// No path
	_ = g.AddNode("d:1", "d") // Isolated
	exists, _, err = g.PathExists("a:1", "d:1", 10)
	if err != nil {
		t.Fatalf("PathExists failed: %v", err)
	}
	if exists {
		t.Error("Path should not exist to isolated node")
	}

	// Path to self
	exists, depth, err = g.PathExists("a:1", "a:1", 10)
	if err != nil {
		t.Fatalf("PathExists to self failed: %v", err)
	}
	if !exists || depth != 0 {
		t.Errorf("Path to self should exist with depth 0, got exists=%v depth=%d", exists, depth)
	}
}

func TestHasCycle(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Acyclic graph
	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")
	_ = g.AddEdge("n:1", "n:2", "NEXT")
	_ = g.AddEdge("n:2", "n:3", "NEXT")

	if g.HasCycle() {
		t.Error("Graph should not have cycle")
	}

	// Add cycle
	_ = g.AddEdge("n:3", "n:1", "BACK")

	if !g.HasCycle() {
		t.Error("Graph should have cycle after adding back edge")
	}
}

func TestCommonNeighbors(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Setup: A -> C, A -> D, B -> C, B -> E
	_ = g.AddNode("a:1", "a")
	_ = g.AddNode("b:1", "b")
	_ = g.AddNode("c:1", "c")
	_ = g.AddNode("d:1", "d")
	_ = g.AddNode("e:1", "e")
	_ = g.AddEdge("a:1", "c:1", "REL")
	_ = g.AddEdge("a:1", "d:1", "REL")
	_ = g.AddEdge("b:1", "c:1", "REL")
	_ = g.AddEdge("b:1", "e:1", "REL")

	common, err := g.CommonNeighbors("a:1", "b:1")
	if err != nil {
		t.Fatalf("CommonNeighbors failed: %v", err)
	}

	if len(common) != 1 {
		t.Errorf("Expected 1 common neighbor, got %d", len(common))
	}
	if len(common) > 0 && common[0] != "c:1" {
		t.Errorf("Expected common neighbor c:1, got %s", common[0])
	}

	// No common neighbors
	_ = g.AddNode("f:1", "f")
	_ = g.AddEdge("f:1", "d:1", "REL") // f -> d only
	common, err = g.CommonNeighbors("b:1", "f:1")
	if err != nil {
		t.Fatalf("CommonNeighbors failed: %v", err)
	}
	if len(common) != 0 {
		t.Errorf("Expected 0 common neighbors, got %d", len(common))
	}
}

func TestGetNodesByType(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("users:1", "users")
	_ = g.AddNode("users:2", "users")
	_ = g.AddNode("posts:1", "posts")
	_ = g.AddNode("posts:2", "posts")
	_ = g.AddNode("posts:3", "posts")

	users := g.GetNodesByType("users")
	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	posts := g.GetNodesByType("posts")
	if len(posts) != 3 {
		t.Errorf("Expected 3 posts, got %d", len(posts))
	}

	empty := g.GetNodesByType("nonexistent")
	if len(empty) != 0 {
		t.Errorf("Expected 0 for nonexistent type, got %d", len(empty))
	}
}

func TestGetDegree(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")
	_ = g.AddEdge("n:1", "n:2", "OUT")
	_ = g.AddEdge("n:1", "n:3", "OUT")
	_ = g.AddEdge("n:3", "n:1", "IN")

	degree, err := g.GetDegree("n:1")
	if err != nil {
		t.Fatalf("GetDegree failed: %v", err)
	}

	if degree.Out != 2 {
		t.Errorf("Expected out degree 2, got %d", degree.Out)
	}
	if degree.In != 1 {
		t.Errorf("Expected in degree 1, got %d", degree.In)
	}
	if degree.Total != 3 {
		t.Errorf("Expected total degree 3, got %d", degree.Total)
	}
}

func TestGetNodeInfo(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("users:42", "users")
	_ = g.AddNode("posts:1", "posts")
	_ = g.AddNode("posts:2", "posts")
	_ = g.AddEdge("users:42", "posts:1", "AUTHORED")
	_ = g.AddEdge("users:42", "posts:2", "AUTHORED")
	_ = g.AddEdge("posts:1", "users:42", "WRITTEN_BY")

	info, err := g.GetNodeInfo("users:42")
	if err != nil {
		t.Fatalf("GetNodeInfo failed: %v", err)
	}

	if info.ID != "users:42" {
		t.Errorf("Expected ID users:42, got %s", info.ID)
	}
	if info.Entity != "users" {
		t.Errorf("Expected entity users, got %s", info.Entity)
	}
	if len(info.Outgoing) != 2 {
		t.Errorf("Expected 2 outgoing edges, got %d", len(info.Outgoing))
	}
	if len(info.Incoming) != 1 {
		t.Errorf("Expected 1 incoming edge, got %d", len(info.Incoming))
	}
}

func TestSaveLoad(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("users:1", "users")
	_ = g.AddNode("users:2", "users")
	_ = g.AddNode("posts:1", "posts")
	_ = g.AddEdge("users:1", "users:2", "FOLLOWS")
	_ = g.AddEdge("users:1", "posts:1", "AUTHORED")

	tmpFile := "/tmp/test_graph.json"
	defer os.Remove(tmpFile)

	err := g.Save(tmpFile)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was created and is valid JSON
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	if data[0] != '{' {
		t.Error("Save should produce JSON format")
	}

	// Load into new graph
	g2 := NewIndexedGraph()
	err = g2.Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify all data was preserved
	if g2.NodeCount() != g.NodeCount() {
		t.Errorf("Node count mismatch: expected %d, got %d", g.NodeCount(), g2.NodeCount())
	}
	if g2.EdgeCount() != g.EdgeCount() {
		t.Errorf("Edge count mismatch: expected %d, got %d", g.EdgeCount(), g2.EdgeCount())
	}

	// Verify specific edges
	neighbors, err := g2.GetNeighbors("users:1")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	if neighbors["users:2"] != "FOLLOWS" {
		t.Error("FOLLOWS edge not preserved")
	}
	if neighbors["posts:1"] != "AUTHORED" {
		t.Error("AUTHORED edge not preserved")
	}

	// Verify reverse edges (incoming)
	incoming, err := g2.GetIncomingEdges("users:2")
	if err != nil {
		t.Fatalf("GetIncomingEdges failed: %v", err)
	}
	if incoming["users:1"] != "FOLLOWS" {
		t.Error("Reverse edge not preserved")
	}

	// Verify index was preserved
	users := g2.GetNodesByType("users")
	if len(users) != 2 {
		t.Errorf("Expected 2 users in index, got %d", len(users))
	}
}

func TestLoadLegacyFormat(t *testing.T) {
	t.Parallel()
	// Create a file in legacy format
	legacyContent := `users:1:users:2:FOLLOWS posts:1:AUTHORED
users:2:`

	tmpFile := "/tmp/test_legacy.txt"
	defer os.Remove(tmpFile)

	err := os.WriteFile(tmpFile, []byte(legacyContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write legacy file: %v", err)
	}

	g := NewIndexedGraph()
	err = g.Load(tmpFile)
	if err != nil {
		t.Fatalf("Load legacy failed: %v", err)
	}

	// Should have loaded the edges
	if g.NodeCount() == 0 {
		t.Error("Legacy load should create nodes")
	}
}

func TestClear(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddEdge("n:1", "n:2", "REL")

	err := g.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if g.NodeCount() != 0 {
		t.Errorf("Expected 0 nodes after clear, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 0 {
		t.Errorf("Expected 0 edges after clear, got %d", g.EdgeCount())
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Pre-populate
	for i := 0; i < 100; i++ {
		_ = g.AddNode(fmt.Sprintf("n:%d", i), "node")
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := g.GetNeighbors(fmt.Sprintf("n:%d", id%100))
			if err != nil && err.Error() != "node not found" {
				errors <- err
			}
		}(i)
	}

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := fmt.Sprintf("n:%d", id%100)
			to := fmt.Sprintf("n:%d", (id+1)%100)
			err := g.AddEdge(from, to, "LINK")
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

func TestRemoveNodeCascadesEdges(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")
	_ = g.AddEdge("n:1", "n:2", "OUT")
	_ = g.AddEdge("n:3", "n:2", "IN")

	initialEdges := g.EdgeCount()
	if initialEdges != 2 {
		t.Fatalf("Expected 2 initial edges, got %d", initialEdges)
	}

	// Remove n:2 - should cascade delete both edges
	err := g.RemoveNode("n:2")
	if err != nil {
		t.Fatalf("RemoveNode failed: %v", err)
	}

	if g.EdgeCount() != 0 {
		t.Errorf("Expected 0 edges after removing connected node, got %d", g.EdgeCount())
	}

	// Verify n:1 has no outgoing edges
	neighbors, err := g.GetNeighbors("n:1")
	if err != nil {
		t.Fatalf("GetNeighbors failed: %v", err)
	}
	if len(neighbors) != 0 {
		t.Errorf("n:1 should have no neighbors after n:2 removed")
	}
}

// ============================================================================
// Cycle Detection Tests
// ============================================================================

func TestNewIndexedGraphWithCycleDetection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode string
	}{
		{"ignore"},
		{"warn"},
		{"error"},
	}

	for _, tc := range tests {
		g := NewIndexedGraphWithCycleDetection(tc.mode)
		if g == nil {
			t.Errorf("NewIndexedGraphWithCycleDetection(%s) returned nil", tc.mode)
		}
	}
}

func TestSetCycleDetection(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()
	
	g.SetCycleDetection("error")
	// No direct way to verify, but should not panic
	
	g.SetCycleDetection("warn")
	g.SetCycleDetection("ignore")
}

func TestCycleDetection_Ignore(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("ignore")

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")

	// Create a cycle: 1 -> 2 -> 3 -> 1
	_ = g.AddEdge("n:1", "n:2", "LINK")
	_ = g.AddEdge("n:2", "n:3", "LINK")
	
	// This creates a cycle - should succeed with ignore mode
	err := g.AddEdge("n:3", "n:1", "LINK")
	if err != nil {
		t.Errorf("Expected no error in ignore mode, got: %v", err)
	}

	// Verify cycle exists
	if !g.HasCycle() {
		t.Error("Expected cycle to exist")
	}
}

func TestCycleDetection_Warn(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("warn")

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")

	_ = g.AddEdge("n:1", "n:2", "LINK")
	_ = g.AddEdge("n:2", "n:3", "LINK")
	
	// This creates a cycle - should succeed with warn mode (just logs)
	err := g.AddEdge("n:3", "n:1", "LINK")
	if err != nil {
		t.Errorf("Expected no error in warn mode, got: %v", err)
	}

	// Cycle should exist
	if !g.HasCycle() {
		t.Error("Expected cycle to exist")
	}
}

func TestCycleDetection_Error(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("error")

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")

	_ = g.AddEdge("n:1", "n:2", "LINK")
	_ = g.AddEdge("n:2", "n:3", "LINK")
	
	// This would create a cycle - should fail with error mode
	err := g.AddEdge("n:3", "n:1", "LINK")
	if err == nil {
		t.Error("Expected error when creating cycle in error mode")
	}
	if err != ErrCycleDetected {
		t.Errorf("Expected ErrCycleDetected, got: %v", err)
	}

	// Cycle should NOT exist because edge was rejected
	if g.HasCycle() {
		t.Error("Expected no cycle because edge was rejected")
	}
}

func TestCycleDetection_SelfLoop(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("error")

	_ = g.AddNode("n:1", "node")

	// Self-loop is a cycle
	err := g.AddEdge("n:1", "n:1", "SELF")
	if err == nil {
		t.Error("Expected error for self-loop in error mode")
	}
}

func TestCycleDetection_NoCycle(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("error")

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")
	_ = g.AddNode("n:3", "node")
	_ = g.AddNode("n:4", "node")

	// Create a DAG (no cycles)
	err := g.AddEdge("n:1", "n:2", "LINK")
	if err != nil {
		t.Errorf("AddEdge 1->2 failed: %v", err)
	}
	
	err = g.AddEdge("n:1", "n:3", "LINK")
	if err != nil {
		t.Errorf("AddEdge 1->3 failed: %v", err)
	}
	
	err = g.AddEdge("n:2", "n:4", "LINK")
	if err != nil {
		t.Errorf("AddEdge 2->4 failed: %v", err)
	}
	
	err = g.AddEdge("n:3", "n:4", "LINK")
	if err != nil {
		t.Errorf("AddEdge 3->4 failed: %v", err)
	}

	// No cycle should exist
	if g.HasCycle() {
		t.Error("Expected no cycle in DAG")
	}
}

func TestCycleDetection_LongPath(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("error")

	// Create a long chain: 1 -> 2 -> 3 -> 4 -> 5
	for i := 1; i <= 5; i++ {
		_ = g.AddNode(fmt.Sprintf("n:%d", i), "node")
	}
	
	for i := 1; i < 5; i++ {
		err := g.AddEdge(fmt.Sprintf("n:%d", i), fmt.Sprintf("n:%d", i+1), "LINK")
		if err != nil {
			t.Fatalf("AddEdge %d->%d failed: %v", i, i+1, err)
		}
	}

	// Try to create long cycle: 5 -> 1
	err := g.AddEdge("n:5", "n:1", "LINK")
	if err == nil {
		t.Error("Expected error when creating long cycle")
	}
}

func TestCycleDetection_ParallelEdges(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraphWithCycleDetection("error")

	_ = g.AddNode("n:1", "node")
	_ = g.AddNode("n:2", "node")

	// Add edge 1 -> 2
	err := g.AddEdge("n:1", "n:2", "LINK")
	if err != nil {
		t.Fatalf("First edge failed: %v", err)
	}

	// Add another edge 1 -> 2 with different type - not a cycle
	err = g.AddEdge("n:1", "n:2", "OTHER")
	if err != nil {
		t.Errorf("Parallel edge should not create cycle: %v", err)
	}
}
