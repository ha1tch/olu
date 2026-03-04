// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package graph

// graph_intensive_test.go
//
// Deep scrutiny of the graph engine: UpdateFromEntity, edge consistency under
// mutations, complex topologies (diamonds, fan-out, deep chains), persistence
// round-trips, and heavy concurrent pressure.
//
// These complement graph_test.go which covers basic CRUD and simple paths.
//
// Author: ha1tch <h@ual.fi>

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ============================================================================
// 1. UpdateFromEntity — the critical path for Shelf AMS
// ============================================================================

// TestUpdateFromEntity_BasicRef tests that a single REF field creates proper
// node + edge in the graph.
func TestUpdateFromEntity_BasicRef(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Simulate: POST /api/v1/assets with asset_type REF
	// First the target must exist in the graph (normally created when asset_type was POSTed)
	must(t, g.AddNode("asset_types:1", "asset_types"))

	err := g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"code":   "FE-001",
		"status": "active",
		"asset_type": map[string]interface{}{
			"type":   "REF",
			"entity": "asset_types",
			"id":     1,
		},
	})
	if err != nil {
		t.Fatalf("UpdateFromEntity failed: %v", err)
	}

	// Node should exist
	if !g.NodeExists("assets:1") {
		t.Error("assets:1 node should exist")
	}

	// Outgoing edge: assets:1 -> asset_types:1
	neighbors, err := g.GetNeighbors("assets:1")
	if err != nil {
		t.Fatal(err)
	}
	if neighbors["asset_types:1"] != "asset_type" {
		t.Errorf("expected edge with relationship 'asset_type', got %v", neighbors)
	}

	// Reverse edge
	incoming, err := g.GetIncomingEdges("asset_types:1")
	if err != nil {
		t.Fatal(err)
	}
	if incoming["assets:1"] != "asset_type" {
		t.Errorf("expected incoming edge from assets:1, got %v", incoming)
	}

	// Counts
	if g.NodeCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 1 {
		t.Errorf("expected 1 edge, got %d", g.EdgeCount())
	}
}

// TestUpdateFromEntity_MultipleRefs tests an entity with several REF fields.
func TestUpdateFromEntity_MultipleRefs(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("assets:1", "assets"))
	must(t, g.AddNode("sensors:1", "sensors"))

	// Event referencing both an asset and a sensor
	err := g.UpdateFromEntity("events", 1, map[string]interface{}{
		"event_type": "reading",
		"asset": map[string]interface{}{
			"type": "REF", "entity": "assets", "id": 1,
		},
		"sensor": map[string]interface{}{
			"type": "REF", "entity": "sensors", "id": 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	neighbors, _ := g.GetNeighbors("events:1")
	if len(neighbors) != 2 {
		t.Errorf("expected 2 outgoing edges, got %d: %v", len(neighbors), neighbors)
	}
	if neighbors["assets:1"] != "asset" {
		t.Errorf("expected 'asset' relationship to assets:1, got %v", neighbors["assets:1"])
	}
	if neighbors["sensors:1"] != "sensor" {
		t.Errorf("expected 'sensor' relationship to sensors:1, got %v", neighbors["sensors:1"])
	}
}

// TestUpdateFromEntity_RefChange tests that changing a REF field removes the
// old edge and creates a new one.
func TestUpdateFromEntity_RefChange(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("asset_types:1", "asset_types"))
	must(t, g.AddNode("asset_types:2", "asset_types"))

	// Initial state: asset points to type 1
	must(t, g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"asset_type": map[string]interface{}{
			"type": "REF", "entity": "asset_types", "id": 1,
		},
	}))

	neighbors, _ := g.GetNeighbors("assets:1")
	if neighbors["asset_types:1"] != "asset_type" {
		t.Fatal("initial edge not created")
	}

	// Change REF: asset now points to type 2
	must(t, g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"asset_type": map[string]interface{}{
			"type": "REF", "entity": "asset_types", "id": 2,
		},
	}))

	neighbors, _ = g.GetNeighbors("assets:1")
	if _, stillExists := neighbors["asset_types:1"]; stillExists {
		t.Error("old edge to asset_types:1 should have been removed")
	}
	if neighbors["asset_types:2"] != "asset_type" {
		t.Error("new edge to asset_types:2 should exist")
	}

	// Only 1 edge total from this node
	if len(neighbors) != 1 {
		t.Errorf("expected exactly 1 outgoing edge, got %d", len(neighbors))
	}

	// Verify reverse edges are clean
	inc1, _ := g.GetIncomingEdges("asset_types:1")
	if _, has := inc1["assets:1"]; has {
		t.Error("reverse edge to old target should be removed")
	}
	inc2, _ := g.GetIncomingEdges("asset_types:2")
	if inc2["assets:1"] != "asset_type" {
		t.Error("reverse edge to new target should exist")
	}
}

// TestUpdateFromEntity_RefRemoval tests that removing a REF field (entity
// updated without it) cleans up the edge.
func TestUpdateFromEntity_RefRemoval(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("sensors:1", "sensors"))

	// Create with REF
	must(t, g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"status": "active",
		"sensor": map[string]interface{}{
			"type": "REF", "entity": "sensors", "id": 1,
		},
	}))
	if g.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", g.EdgeCount())
	}

	// Update WITHOUT REF — edge should be removed
	must(t, g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"status": "maintenance",
	}))

	if g.EdgeCount() != 0 {
		t.Errorf("expected 0 edges after ref removal, got %d", g.EdgeCount())
	}

	neighbors, _ := g.GetNeighbors("assets:1")
	if len(neighbors) != 0 {
		t.Errorf("expected no outgoing edges, got %v", neighbors)
	}
}

// TestUpdateFromEntity_NoRefs tests that an entity with no REF fields just
// creates a node with no edges.
func TestUpdateFromEntity_NoRefs(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.UpdateFromEntity("users", 1, map[string]interface{}{
		"username": "alice",
		"email":    "alice@test.com",
	}))

	if !g.NodeExists("users:1") {
		t.Error("node should exist")
	}
	if g.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", g.EdgeCount())
	}
}

// TestUpdateFromEntity_NonRefMapField ensures that map fields that aren't
// REFs don't create edges.
func TestUpdateFromEntity_NonRefMapField(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.UpdateFromEntity("assets", 1, map[string]interface{}{
		"metadata": map[string]interface{}{
			"colour": "red",
			"weight": 42,
		},
		"nested_obj": map[string]interface{}{
			"type":   "NOT_A_REF",
			"entity": "fakes",
			"id":     99,
		},
	}))

	if g.EdgeCount() != 0 {
		t.Errorf("non-REF maps should not create edges, got %d edges", g.EdgeCount())
	}
}

// TestUpdateFromEntity_Idempotent tests that calling UpdateFromEntity twice
// with the same data doesn't duplicate edges.
func TestUpdateFromEntity_Idempotent(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("asset_types:1", "asset_types"))

	data := map[string]interface{}{
		"asset_type": map[string]interface{}{
			"type": "REF", "entity": "asset_types", "id": 1,
		},
	}

	must(t, g.UpdateFromEntity("assets", 1, data))
	must(t, g.UpdateFromEntity("assets", 1, data))
	must(t, g.UpdateFromEntity("assets", 1, data))

	if g.EdgeCount() != 1 {
		t.Errorf("expected 1 edge after idempotent updates, got %d", g.EdgeCount())
	}
}

// ============================================================================
// 2. Complex topologies
// ============================================================================

// TestTopology_Diamond tests diamond-shaped graph: A -> B, A -> C, B -> D, C -> D
func TestTopology_Diamond(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	for _, n := range []string{"a:1", "b:1", "c:1", "d:1"} {
		must(t, g.AddNode(n, strings.Split(n, ":")[0]))
	}
	must(t, g.AddEdge("a:1", "b:1", "LEFT"))
	must(t, g.AddEdge("a:1", "c:1", "RIGHT"))
	must(t, g.AddEdge("b:1", "d:1", "MERGE"))
	must(t, g.AddEdge("c:1", "d:1", "MERGE"))

	// Path A → D exists via both branches
	path, err := g.FindPath("a:1", "d:1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 {
		t.Errorf("expected shortest path length 3 (A→B→D or A→C→D), got %d: %v", len(path), path)
	}

	// D has 2 incoming edges
	incoming, _ := g.GetIncomingEdges("d:1")
	if len(incoming) != 2 {
		t.Errorf("d:1 should have 2 incoming, got %d", len(incoming))
	}

	// Common neighbors of B and C should include D (outgoing) and A (incoming)
	common, _ := g.CommonNeighbors("b:1", "c:1")
	sort.Strings(common)
	if len(common) != 2 {
		t.Errorf("expected 2 common neighbors (a:1, d:1), got %d: %v", len(common), common)
	}

	// PathExists with exact depth check
	exists, length, _ := g.PathExists("a:1", "d:1", 10)
	if !exists || length != 2 {
		t.Errorf("expected path length 2, got exists=%v length=%d", exists, length)
	}
}

// TestTopology_FanOut tests high fan-out: one node with many edges.
func TestTopology_FanOut(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("hub:1", "hub"))
	const fanOut = 100
	for i := 0; i < fanOut; i++ {
		nodeID := fmt.Sprintf("leaf:%d", i)
		must(t, g.AddNode(nodeID, "leaf"))
		must(t, g.AddEdge("hub:1", nodeID, "CONNECTS"))
	}

	if g.EdgeCount() != fanOut {
		t.Errorf("expected %d edges, got %d", fanOut, g.EdgeCount())
	}

	degree, _ := g.GetDegree("hub:1")
	if degree.Out != fanOut {
		t.Errorf("expected out-degree %d, got %d", fanOut, degree.Out)
	}
	if degree.In != 0 {
		t.Errorf("expected in-degree 0, got %d", degree.In)
	}

	// Verify all leaves have exactly 1 incoming
	for i := 0; i < fanOut; i++ {
		nodeID := fmt.Sprintf("leaf:%d", i)
		deg, _ := g.GetDegree(nodeID)
		if deg.In != 1 || deg.Out != 0 {
			t.Errorf("leaf %s: expected in=1 out=0, got in=%d out=%d", nodeID, deg.In, deg.Out)
			break
		}
	}

	nodes := g.GetNodesByType("leaf")
	if len(nodes) != fanOut {
		t.Errorf("GetNodesByType expected %d leaves, got %d", fanOut, len(nodes))
	}
}

// TestTopology_FanIn tests high fan-in: many nodes pointing to one.
func TestTopology_FanIn(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("sink:1", "sink"))
	const fanIn = 100
	for i := 0; i < fanIn; i++ {
		nodeID := fmt.Sprintf("source:%d", i)
		must(t, g.AddNode(nodeID, "source"))
		must(t, g.AddEdge(nodeID, "sink:1", "FEEDS"))
	}

	degree, _ := g.GetDegree("sink:1")
	if degree.In != fanIn {
		t.Errorf("expected in-degree %d, got %d", fanIn, degree.In)
	}

	incoming, _ := g.GetIncomingEdges("sink:1")
	if len(incoming) != fanIn {
		t.Errorf("expected %d incoming edges, got %d", fanIn, len(incoming))
	}
}

// TestTopology_DeepChain tests a long linear chain and path finding at the
// maxDepth boundary.
func TestTopology_DeepChain(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	const chainLen = 50
	for i := 0; i < chainLen; i++ {
		must(t, g.AddNode(fmt.Sprintf("n:%d", i), "chain"))
	}
	for i := 0; i < chainLen-1; i++ {
		must(t, g.AddEdge(fmt.Sprintf("n:%d", i), fmt.Sprintf("n:%d", i+1), "NEXT"))
	}

	// Path at exact depth
	path, err := g.FindPath("n:0", fmt.Sprintf("n:%d", chainLen-1), chainLen)
	if err != nil {
		t.Fatalf("FindPath with sufficient depth failed: %v", err)
	}
	if len(path) != chainLen {
		t.Errorf("expected path length %d, got %d", chainLen, len(path))
	}

	// PathExists at exact boundary
	exists, length, _ := g.PathExists("n:0", fmt.Sprintf("n:%d", chainLen-1), chainLen)
	if !exists {
		t.Error("path should exist at exact maxDepth")
	}
	if length != chainLen-1 {
		t.Errorf("expected length %d, got %d", chainLen-1, length)
	}

	// Just below depth limit — should NOT find the path
	_, err = g.FindPath("n:0", fmt.Sprintf("n:%d", chainLen-1), chainLen-2)
	if err == nil {
		t.Error("expected error when maxDepth is insufficient")
	}

	exists, _, _ = g.PathExists("n:0", fmt.Sprintf("n:%d", chainLen-1), chainLen-2)
	if exists {
		t.Error("PathExists should return false when maxDepth is insufficient")
	}
}

// TestTopology_DisconnectedComponents tests operations across separate
// subgraphs.
func TestTopology_DisconnectedComponents(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Component 1: A -> B -> C
	must(t, g.AddNode("a:1", "comp1"))
	must(t, g.AddNode("b:1", "comp1"))
	must(t, g.AddNode("c:1", "comp1"))
	must(t, g.AddEdge("a:1", "b:1", "NEXT"))
	must(t, g.AddEdge("b:1", "c:1", "NEXT"))

	// Component 2: X -> Y
	must(t, g.AddNode("x:1", "comp2"))
	must(t, g.AddNode("y:1", "comp2"))
	must(t, g.AddEdge("x:1", "y:1", "LINK"))

	// No path between components
	_, err := g.FindPath("a:1", "x:1", 10)
	if err == nil {
		t.Error("expected error for path between disconnected components")
	}

	exists, _, _ := g.PathExists("a:1", "y:1", 10)
	if exists {
		t.Error("no path should exist between components")
	}

	// CommonNeighbors across components returns empty
	common, _ := g.CommonNeighbors("a:1", "x:1")
	if len(common) != 0 {
		t.Errorf("expected no common neighbors across components, got %v", common)
	}

	// Remove component 2 entirely
	must(t, g.RemoveNode("x:1"))
	must(t, g.RemoveNode("y:1"))
	if g.NodeCount() != 3 {
		t.Errorf("expected 3 nodes after removing component 2, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 2 {
		t.Errorf("expected 2 edges, got %d", g.EdgeCount())
	}
}

// ============================================================================
// 3. Edge consistency and GetNodeInfo accuracy
// ============================================================================

// TestGetNodeInfo_Detailed verifies every field of NodeInfo with known topology.
func TestGetNodeInfo_Detailed(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	must(t, g.AddNode("assets:42", "assets"))
	must(t, g.AddNode("asset_types:7", "asset_types"))
	must(t, g.AddNode("sensors:3", "sensors"))
	must(t, g.AddNode("events:99", "events"))

	must(t, g.AddEdge("assets:42", "asset_types:7", "asset_type"))
	must(t, g.AddEdge("sensors:3", "assets:42", "asset"))
	must(t, g.AddEdge("events:99", "assets:42", "asset"))

	info, err := g.GetNodeInfo("assets:42")
	if err != nil {
		t.Fatal(err)
	}

	if info.ID != "assets:42" {
		t.Errorf("ID: expected assets:42, got %s", info.ID)
	}
	if info.Entity != "assets" {
		t.Errorf("Entity: expected assets, got %s", info.Entity)
	}
	if info.EntityID != 42 {
		t.Errorf("EntityID: expected 42, got %d", info.EntityID)
	}

	// Outgoing: assets:42 -> asset_types:7
	if len(info.Outgoing) != 1 {
		t.Errorf("expected 1 outgoing, got %d: %v", len(info.Outgoing), info.Outgoing)
	}
	if info.Outgoing["asset_types:7"] != "asset_type" {
		t.Errorf("outgoing to asset_types:7 expected 'asset_type', got %q", info.Outgoing["asset_types:7"])
	}

	// Incoming: sensors:3 and events:99
	if len(info.Incoming) != 2 {
		t.Errorf("expected 2 incoming, got %d: %v", len(info.Incoming), info.Incoming)
	}
	if info.Incoming["sensors:3"] != "asset" {
		t.Errorf("incoming from sensors:3 expected 'asset', got %q", info.Incoming["sensors:3"])
	}
	if info.Incoming["events:99"] != "asset" {
		t.Errorf("incoming from events:99 expected 'asset', got %q", info.Incoming["events:99"])
	}

	// Degree
	if info.Degree.Out != 1 {
		t.Errorf("out-degree: expected 1, got %d", info.Degree.Out)
	}
	if info.Degree.In != 2 {
		t.Errorf("in-degree: expected 2, got %d", info.Degree.In)
	}
	if info.Degree.Total != 3 {
		t.Errorf("total degree: expected 3, got %d", info.Degree.Total)
	}
}

// TestEdgeConsistency_RemoveAndVerify verifies that after node removal,
// no dangling edges remain anywhere in the graph.
func TestEdgeConsistency_RemoveAndVerify(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Build: A -> B -> C, D -> B
	must(t, g.AddNode("a:1", "a"))
	must(t, g.AddNode("b:1", "b"))
	must(t, g.AddNode("c:1", "c"))
	must(t, g.AddNode("d:1", "d"))
	must(t, g.AddEdge("a:1", "b:1", "R1"))
	must(t, g.AddEdge("b:1", "c:1", "R2"))
	must(t, g.AddEdge("d:1", "b:1", "R3"))

	// Remove B — should remove all 3 edges
	must(t, g.RemoveNode("b:1"))

	// Verify no references to b:1 anywhere
	for _, nodeID := range []string{"a:1", "c:1", "d:1"} {
		out, _ := g.GetNeighbors(nodeID)
		for target := range out {
			if target == "b:1" {
				t.Errorf("dangling outgoing edge from %s to b:1", nodeID)
			}
		}
		in, _ := g.GetIncomingEdges(nodeID)
		for source := range in {
			if source == "b:1" {
				t.Errorf("dangling incoming edge to %s from b:1", nodeID)
			}
		}
	}

	if g.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", g.EdgeCount())
	}
}

// TestEdgeConsistency_ReverseIndex verifies that forward and reverse adjacency
// lists are always in sync after various mutations.
func TestEdgeConsistency_ReverseIndex(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	ops := []struct {
		op   string
		from string
		to   string
		rel  string
	}{
		{"add_node", "a:1", "", "a"},
		{"add_node", "b:1", "", "b"},
		{"add_node", "c:1", "", "c"},
		{"add_node", "d:1", "", "d"},
		{"add_edge", "a:1", "b:1", "R1"},
		{"add_edge", "a:1", "c:1", "R2"},
		{"add_edge", "b:1", "d:1", "R3"},
		{"add_edge", "c:1", "d:1", "R4"},
		{"remove_edge", "a:1", "c:1", ""},
		{"add_edge", "d:1", "a:1", "BACK"},
		{"remove_node", "b:1", "", ""},
	}

	for _, op := range ops {
		switch op.op {
		case "add_node":
			g.AddNode(op.from, op.rel)
		case "add_edge":
			g.AddEdge(op.from, op.to, op.rel)
		case "remove_edge":
			g.RemoveEdge(op.from, op.to)
		case "remove_node":
			g.RemoveNode(op.from)
		}
	}

	// For every forward edge A->B, verify B's reverse contains A
	allNodes := g.GetAllNodes()
	for _, nodeID := range allNodes {
		out, _ := g.GetNeighbors(nodeID)
		for target, rel := range out {
			in, _ := g.GetIncomingEdges(target)
			if in[nodeID] != rel {
				t.Errorf("forward edge %s->%s (%s) not mirrored in reverse index of %s: %v",
					nodeID, target, rel, target, in)
			}
		}
	}

	// For every reverse edge B<-A, verify A's forward contains B
	for _, nodeID := range allNodes {
		in, _ := g.GetIncomingEdges(nodeID)
		for source, rel := range in {
			out, _ := g.GetNeighbors(source)
			if out[nodeID] != rel {
				t.Errorf("reverse edge %s<-%s (%s) not mirrored in forward index of %s: %v",
					nodeID, source, rel, source, out)
			}
		}
	}
}

// ============================================================================
// 4. Persistence round-trips
// ============================================================================

// TestSaveLoad_ComplexGraph verifies that a graph with mixed topologies
// survives a save/load cycle with all data intact.
func TestSaveLoad_ComplexGraph(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Build a realistic Shelf-like graph
	types := []string{"asset_types:1", "asset_types:2"}
	assets := []string{"assets:1", "assets:2", "assets:3"}
	sensors := []string{"sensors:1", "sensors:2"}
	events := []string{"events:1", "events:2", "events:3", "events:4"}

	for _, n := range types {
		must(t, g.AddNode(n, "asset_types"))
	}
	for _, n := range assets {
		must(t, g.AddNode(n, "assets"))
	}
	for _, n := range sensors {
		must(t, g.AddNode(n, "sensors"))
	}
	for _, n := range events {
		must(t, g.AddNode(n, "events"))
	}

	// assets -> asset_types
	must(t, g.AddEdge("assets:1", "asset_types:1", "asset_type"))
	must(t, g.AddEdge("assets:2", "asset_types:1", "asset_type"))
	must(t, g.AddEdge("assets:3", "asset_types:2", "asset_type"))
	// sensors -> assets
	must(t, g.AddEdge("sensors:1", "assets:1", "asset"))
	must(t, g.AddEdge("sensors:2", "assets:2", "asset"))
	// events -> assets + sensors
	must(t, g.AddEdge("events:1", "assets:1", "asset"))
	must(t, g.AddEdge("events:1", "sensors:1", "sensor"))
	must(t, g.AddEdge("events:2", "assets:1", "asset"))
	must(t, g.AddEdge("events:3", "assets:2", "asset"))
	must(t, g.AddEdge("events:4", "assets:3", "asset"))

	origNodeCount := g.NodeCount()
	origEdgeCount := g.EdgeCount()

	// Save
	tmpFile := t.TempDir() + "/graph.data"
	idxFile := t.TempDir() + "/graph.index"

	if err := g.Save(tmpFile); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveIndex(idxFile); err != nil {
		t.Fatal(err)
	}

	// Load into fresh graph
	g2 := NewIndexedGraph()
	if err := g2.Load(tmpFile); err != nil {
		t.Fatal(err)
	}
	if err := g2.LoadIndex(idxFile); err != nil {
		t.Fatal(err)
	}

	// Verify counts
	if g2.NodeCount() != origNodeCount {
		t.Errorf("node count: expected %d, got %d", origNodeCount, g2.NodeCount())
	}
	if g2.EdgeCount() != origEdgeCount {
		t.Errorf("edge count: expected %d, got %d", origEdgeCount, g2.EdgeCount())
	}

	// Verify specific edges
	neighbors, _ := g2.GetNeighbors("events:1")
	if neighbors["assets:1"] != "asset" {
		t.Error("events:1 -> assets:1 edge lost after save/load")
	}
	if neighbors["sensors:1"] != "sensor" {
		t.Error("events:1 -> sensors:1 edge lost after save/load")
	}

	// Verify reverse edges
	incoming, _ := g2.GetIncomingEdges("assets:1")
	expectedIncoming := map[string]string{
		"sensors:1": "asset",
		"events:1":  "asset",
		"events:2":  "asset",
	}
	for source, rel := range expectedIncoming {
		if incoming[source] != rel {
			t.Errorf("reverse edge %s -> assets:1 (%s) lost after save/load", source, rel)
		}
	}

	// Verify path still works
	path, err := g2.FindPath("events:1", "asset_types:1", 5)
	if err != nil {
		t.Errorf("path events:1 -> asset_types:1 should exist after load: %v", err)
	}
	if len(path) < 3 {
		t.Errorf("expected path length >= 3, got %d", len(path))
	}

	// Verify GetNodesByType works after load
	assetNodes := g2.GetNodesByType("assets")
	if len(assetNodes) != 3 {
		t.Errorf("expected 3 asset nodes after load, got %d", len(assetNodes))
	}
}

// TestSaveLoad_EmptyGraph verifies that an empty graph can be saved and loaded.
func TestSaveLoad_EmptyGraph(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()
	tmpFile := t.TempDir() + "/empty.data"

	if err := g.Save(tmpFile); err != nil {
		t.Fatal(err)
	}

	g2 := NewIndexedGraph()
	if err := g2.Load(tmpFile); err != nil {
		t.Fatal(err)
	}

	if g2.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", g2.NodeCount())
	}
}

// ============================================================================
// 5. Heavy concurrent pressure
// ============================================================================

// TestConcurrent_MixedOperations exercises reads, writes, and deletes
// simultaneously with more workers than the basic ConcurrentAccess test.
func TestConcurrent_MixedOperations(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	const numNodes = 200
	for i := 0; i < numNodes; i++ {
		must(t, g.AddNode(fmt.Sprintf("n:%d", i), "node"))
	}

	var wg sync.WaitGroup
	var errCount int64

	// Writers: add edges
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				from := fmt.Sprintf("n:%d", (worker*50+j)%numNodes)
				to := fmt.Sprintf("n:%d", (worker*50+j+1)%numNodes)
				if err := g.AddEdge(from, to, "LINK"); err != nil {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(i)
	}

	// Readers: query neighbors, degrees, paths
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				nodeID := fmt.Sprintf("n:%d", (worker*13+j)%numNodes)
				g.GetNeighbors(nodeID)
				g.GetIncomingEdges(nodeID)
				g.GetDegree(nodeID)
				g.GetNodeInfo(nodeID)
				g.NodeCount()
				g.EdgeCount()
			}
		}(i)
	}

	// Deleters: remove some edges
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				from := fmt.Sprintf("n:%d", (worker*20+j)%numNodes)
				to := fmt.Sprintf("n:%d", (worker*20+j+1)%numNodes)
				g.RemoveEdge(from, to) // May or may not exist, that's fine
			}
		}(i)
	}

	// Path finders
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				from := fmt.Sprintf("n:%d", (worker*10+j)%numNodes)
				to := fmt.Sprintf("n:%d", (worker*10+j+50)%numNodes)
				g.FindPath(from, to, 5) // May or may not find path
				g.PathExists(from, to, 5)
			}
		}(i)
	}

	wg.Wait()

	if errCount > 0 {
		t.Errorf("%d errors during concurrent operations", errCount)
	}

	// Verify graph is still consistent after all the chaos
	allNodes := g.GetAllNodes()
	for _, nodeID := range allNodes {
		out, err := g.GetNeighbors(nodeID)
		if err != nil {
			t.Errorf("GetNeighbors(%s) failed after concurrent ops: %v", nodeID, err)
			continue
		}
		for target, rel := range out {
			in, _ := g.GetIncomingEdges(target)
			if in[nodeID] != rel {
				t.Errorf("inconsistency: %s->%s (%s) not in reverse index", nodeID, target, rel)
			}
		}
	}
}

// TestConcurrent_UpdateFromEntity tests concurrent UpdateFromEntity calls
// which is what happens when Shelf processes multiple API requests.
func TestConcurrent_UpdateFromEntity(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	// Pre-create target nodes
	for i := 0; i < 10; i++ {
		must(t, g.AddNode(fmt.Sprintf("asset_types:%d", i), "asset_types"))
	}

	var wg sync.WaitGroup
	var errCount int64

	// Concurrent entity updates
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			targetType := id % 10
			err := g.UpdateFromEntity("assets", id, map[string]interface{}{
				"code": fmt.Sprintf("A-%d", id),
				"asset_type": map[string]interface{}{
					"type": "REF", "entity": "asset_types", "id": targetType,
				},
			})
			if err != nil {
				atomic.AddInt64(&errCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if errCount > 0 {
		t.Errorf("%d errors during concurrent UpdateFromEntity", errCount)
	}

	// Verify: should have 10 types + 50 assets = 60 nodes, 50 edges
	if g.NodeCount() != 60 {
		t.Errorf("expected 60 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 50 {
		t.Errorf("expected 50 edges, got %d", g.EdgeCount())
	}
}

// ============================================================================
// 6. Edge cases and error paths
// ============================================================================

// TestError_PathToNonexistentNode tests error handling for queries with
// nonexistent nodes.
func TestError_PathToNonexistentNode(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()
	must(t, g.AddNode("a:1", "a"))

	// PathExists with nonexistent target
	_, _, err := g.PathExists("a:1", "ghost:1", 10)
	if err == nil {
		t.Error("PathExists to nonexistent node should error")
	}

	// PathExists with nonexistent source
	_, _, err = g.PathExists("ghost:1", "a:1", 10)
	if err == nil {
		t.Error("PathExists from nonexistent node should error")
	}

	// GetNodeInfo on nonexistent
	_, err = g.GetNodeInfo("ghost:1")
	if err == nil {
		t.Error("GetNodeInfo on nonexistent should error")
	}

	// GetDegree on nonexistent
	_, err = g.GetDegree("ghost:1")
	if err == nil {
		t.Error("GetDegree on nonexistent should error")
	}

	// CommonNeighbors with nonexistent
	_, err = g.CommonNeighbors("a:1", "ghost:1")
	if err == nil {
		t.Error("CommonNeighbors with nonexistent should error")
	}
}

// TestError_LoadCorruptFile tests loading from a corrupt file.
func TestError_LoadCorruptFile(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	tmpFile := t.TempDir() + "/corrupt.data"
	os.WriteFile(tmpFile, []byte("this is not valid graph data\n{broken json\n"), 0644)

	err := g.Load(tmpFile)
	// The load should either error or gracefully handle corrupt data
	// It should NOT panic
	_ = err
}

// TestError_LoadNonexistentFile tests that loading from a nonexistent file
// returns nil (by design: fresh startup with no prior graph data).
func TestError_LoadNonexistentFile(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()

	err := g.Load("/nonexistent/path/graph.data")
	if err != nil {
		t.Errorf("Load from nonexistent file should return nil (first-start), got: %v", err)
	}
	// Graph should remain empty
	if g.NodeCount() != 0 {
		t.Errorf("expected 0 nodes after loading nonexistent file, got %d", g.NodeCount())
	}
}

// TestGetAllNodes_IsSorted tests that GetAllNodes returns a consistent result
// (not affected by map ordering randomness).
func TestGetAllNodes_Consistency(t *testing.T) {
	t.Parallel()
	g := NewIndexedGraph()
	for i := 0; i < 20; i++ {
		must(t, g.AddNode(fmt.Sprintf("n:%d", i), "node"))
	}

	// Call multiple times, results should have same elements
	r1 := g.GetAllNodes()
	r2 := g.GetAllNodes()

	sort.Strings(r1)
	sort.Strings(r2)

	if len(r1) != len(r2) {
		t.Fatalf("inconsistent lengths: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Errorf("mismatch at %d: %s vs %s", i, r1[i], r2[i])
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
