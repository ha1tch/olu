package graph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ha1tch/olu/pkg/models"
)

// Graph interface defines graph operations
type Graph interface {
	AddNode(nodeID string, nodeType string) error
	RemoveNode(nodeID string) error
	AddEdge(from, to, relationship string) error
	RemoveEdge(from, to string) error
	GetNeighbors(nodeID string) (map[string]string, error)
	GetIncomingEdges(nodeID string) (map[string]string, error)
	FindPath(from, to string, maxDepth int) ([]string, error)
	HasCycle() bool
	Save(filename string) error
	Load(filename string) error
	Clear() error
	UpdateFromEntity(entity string, id int, data map[string]interface{}) error
}

// IndexedGraph implements an indexed graph with adjacency lists
type IndexedGraph struct {
	adjacency map[string]map[string]string // node -> {neighbor -> relationship}
	reverse   map[string]map[string]string // reverse edges for incoming queries
	index     map[string][]string          // type/property index
	mu        sync.RWMutex
}

// NewIndexedGraph creates a new indexed graph
func NewIndexedGraph() *IndexedGraph {
	return &IndexedGraph{
		adjacency: make(map[string]map[string]string),
		reverse:   make(map[string]map[string]string),
		index:     make(map[string][]string),
	}
}

// AddNode adds a node to the graph
func (g *IndexedGraph) AddNode(nodeID string, nodeType string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if _, exists := g.adjacency[nodeID]; !exists {
		g.adjacency[nodeID] = make(map[string]string)
		g.reverse[nodeID] = make(map[string]string)
	}
	
	// Index by type
	if nodeType != "" {
		g.index[nodeType] = append(g.index[nodeType], nodeID)
	}
	
	return nil
}

// RemoveNode removes a node and all its edges
func (g *IndexedGraph) RemoveNode(nodeID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Remove outgoing edges
	delete(g.adjacency, nodeID)
	
	// Remove incoming edges
	delete(g.reverse, nodeID)
	
	// Remove from all neighbors' adjacency lists
	for node := range g.adjacency {
		delete(g.adjacency[node], nodeID)
	}
	
	// Remove from all neighbors' reverse lists
	for node := range g.reverse {
		delete(g.reverse[node], nodeID)
	}
	
	// Remove from index
	for key, nodes := range g.index {
		filtered := make([]string, 0)
		for _, n := range nodes {
			if n != nodeID {
				filtered = append(filtered, n)
			}
		}
		if len(filtered) > 0 {
			g.index[key] = filtered
		} else {
			delete(g.index, key)
		}
	}
	
	return nil
}

// AddEdge adds a directed edge between nodes
func (g *IndexedGraph) AddEdge(from, to, relationship string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Ensure nodes exist
	if _, exists := g.adjacency[from]; !exists {
		g.adjacency[from] = make(map[string]string)
		g.reverse[from] = make(map[string]string)
	}
	if _, exists := g.adjacency[to]; !exists {
		g.adjacency[to] = make(map[string]string)
		g.reverse[to] = make(map[string]string)
	}
	
	g.adjacency[from][to] = relationship
	g.reverse[to][from] = relationship
	
	// Index by relationship type
	relKey := fmt.Sprintf("relationship:%s", relationship)
	g.index[relKey] = append(g.index[relKey], from)
	
	return nil
}

// RemoveEdge removes an edge between nodes
func (g *IndexedGraph) RemoveEdge(from, to string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if adj, exists := g.adjacency[from]; exists {
		delete(adj, to)
	}
	if rev, exists := g.reverse[to]; exists {
		delete(rev, from)
	}
	
	return nil
}

// GetNeighbors returns all outgoing neighbors of a node
func (g *IndexedGraph) GetNeighbors(nodeID string) (map[string]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	neighbors, exists := g.adjacency[nodeID]
	if !exists {
		return make(map[string]string), nil
	}
	
	// Return a copy to avoid concurrent modification
	result := make(map[string]string)
	for k, v := range neighbors {
		result[k] = v
	}
	
	return result, nil
}

// GetIncomingEdges returns all incoming edges to a node
func (g *IndexedGraph) GetIncomingEdges(nodeID string) (map[string]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	incoming, exists := g.reverse[nodeID]
	if !exists {
		return make(map[string]string), nil
	}
	
	// Return a copy
	result := make(map[string]string)
	for k, v := range incoming {
		result[k] = v
	}
	
	return result, nil
}

// FindPath finds a path between two nodes using BFS
func (g *IndexedGraph) FindPath(from, to string, maxDepth int) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	if _, exists := g.adjacency[from]; !exists {
		return nil, fmt.Errorf("node %s not found", from)
	}
	if _, exists := g.adjacency[to]; !exists {
		return nil, fmt.Errorf("node %s not found", to)
	}
	
	queue := [][]string{{from}}
	visited := make(map[string]bool)
	visited[from] = true
	
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		
		if len(path) > maxDepth {
			continue
		}
		
		current := path[len(path)-1]
		
		if current == to {
			return path, nil
		}
		
		for neighbor := range g.adjacency[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				newPath := make([]string, len(path))
				copy(newPath, path)
				newPath = append(newPath, neighbor)
				queue = append(queue, newPath)
			}
		}
	}
	
	return nil, fmt.Errorf("no path found")
}

// HasCycle checks if the graph has a cycle using DFS
func (g *IndexedGraph) HasCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	
	var hasCycleFrom func(string) bool
	hasCycleFrom = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		
		for neighbor := range g.adjacency[node] {
			if !visited[neighbor] {
				if hasCycleFrom(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				return true
			}
		}
		
		recStack[node] = false
		return false
	}
	
	for node := range g.adjacency {
		if !visited[node] {
			if hasCycleFrom(node) {
				return true
			}
		}
	}
	
	return false
}

// Save saves the graph to a file
func (g *IndexedGraph) Save(filename string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	tempFile := filename + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	writer := bufio.NewWriter(file)
	for nodeID, neighbors := range g.adjacency {
		var parts []string
		for neighbor, relType := range neighbors {
			parts = append(parts, fmt.Sprintf("%s:%s", neighbor, relType))
		}
		line := fmt.Sprintf("%s:%s\n", nodeID, strings.Join(parts, " "))
		if _, err := writer.WriteString(line); err != nil {
			return err
		}
	}
	
	if err := writer.Flush(); err != nil {
		return err
	}
	
	return os.Rename(tempFile, filename)
}

// Load loads the graph from a file
func (g *IndexedGraph) Load(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}
	defer file.Close()
	
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.adjacency = make(map[string]map[string]string)
	g.reverse = make(map[string]map[string]string)
	g.index = make(map[string][]string)
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		
		nodeID := parts[0]
		if _, exists := g.adjacency[nodeID]; !exists {
			g.adjacency[nodeID] = make(map[string]string)
			g.reverse[nodeID] = make(map[string]string)
		}
		
		if parts[1] == "" {
			continue
		}
		
		neighbors := strings.Split(parts[1], " ")
		for _, neighbor := range neighbors {
			if neighbor == "" {
				continue
			}
			neighborParts := strings.SplitN(neighbor, ":", 2)
			if len(neighborParts) == 2 {
				neighborID := neighborParts[0]
				relType := neighborParts[1]
				
				if _, exists := g.adjacency[neighborID]; !exists {
					g.adjacency[neighborID] = make(map[string]string)
					g.reverse[neighborID] = make(map[string]string)
				}
				
				g.adjacency[nodeID][neighborID] = relType
				g.reverse[neighborID][nodeID] = relType
			}
		}
	}
	
	return scanner.Err()
}

// Clear removes all nodes and edges
func (g *IndexedGraph) Clear() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.adjacency = make(map[string]map[string]string)
	g.reverse = make(map[string]map[string]string)
	g.index = make(map[string][]string)
	
	return nil
}

// SaveIndex saves the graph index to a file
func (g *IndexedGraph) SaveIndex(filename string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	tempFile := filename + ".tmp"
	data, err := json.MarshalIndent(g.index, "", "  ")
	if err != nil {
		return err
	}
	
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tempFile, filename)
}

// LoadIndex loads the graph index from a file
func (g *IndexedGraph) LoadIndex(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	g.mu.Lock()
	defer g.mu.Unlock()
	
	return json.Unmarshal(data, &g.index)
}

// UpdateFromEntity updates the graph based on entity data
func (g *IndexedGraph) UpdateFromEntity(entity string, id int, data map[string]interface{}) error {
	nodeID := fmt.Sprintf("%s:%d", entity, id)
	
	// Add node
	nodeType, _ := data["type"].(string)
	if err := g.AddNode(nodeID, nodeType); err != nil {
		return err
	}
	
	// Process references
	for key, value := range data {
		if ref, isRef := models.IsReference(value); isRef {
			targetNodeID := fmt.Sprintf("%s:%d", ref.Entity, ref.ID)
			if err := g.AddEdge(nodeID, targetNodeID, key); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// NodeCount returns the number of nodes in the graph
func (g *IndexedGraph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.adjacency)
}

// EdgeCount returns the number of edges in the graph
func (g *IndexedGraph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	count := 0
	for _, neighbors := range g.adjacency {
		count += len(neighbors)
	}
	return count
}
