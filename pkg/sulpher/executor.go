package sulpher

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ha1tch/olu/pkg/graph"
)

// QueryResult represents the result of a query execution
type QueryResult struct {
	Data  []map[string]interface{} `json:"data"`
	Stats QueryStats               `json:"stats"`
}

// QueryStats contains execution statistics
type QueryStats struct {
	NodesTraversed int           `json:"nodes_traversed"`
	PathsFound     int           `json:"paths_found"`
	ExecutionTime  time.Duration `json:"execution_time_ms"`
}

// Executor executes Sulpher queries against a graph
type Executor struct {
	graph    *graph.IndexedGraph
	maxDepth int
	mu       sync.RWMutex
}

// NewExecutor creates a new query executor
func NewExecutor(g *graph.IndexedGraph, maxDepth int) *Executor {
	return &Executor{
		graph:    g,
		maxDepth: maxDepth,
	}
}

// Execute runs a parsed query and returns results
func (e *Executor) Execute(query *Query) (*QueryResult, error) {
	startTime := time.Now()
	stats := QueryStats{}

	// Take a snapshot of the graph for consistent reads
	e.mu.RLock()
	snapshot := e.takeSnapshot()
	e.mu.RUnlock()

	// Find matching start nodes
	startPattern := query.Path[0].Node
	startNodes := e.findMatchingNodes(snapshot, startPattern)

	var allPaths [][]pathNode
	for _, startNode := range startNodes {
		var paths [][]pathNode
		if query.Algorithm == DFS {
			paths = e.dfsTraverse(snapshot, startNode, query.Path, &stats)
		} else {
			paths = e.bfsTraverse(snapshot, startNode, query.Path, &stats)
		}
		allPaths = append(allPaths, paths...)
	}

	// Apply WHERE conditions (with OR support)
	if len(query.ConditionGroups) > 0 {
		allPaths = e.applyConditionGroups(allPaths, query.ConditionGroups, query.Path)
	} else if len(query.Conditions) > 0 {
		allPaths = e.applyConditions(allPaths, query.Conditions, query.Path)
	}

	stats.PathsFound = len(allPaths)

	// Apply RETURN projection
	results := e.applyReturn(allPaths, query.ReturnItems, query.Path)

	// Apply DISTINCT
	if query.Distinct {
		results = e.applyDistinct(results)
	}

	// Apply ORDER BY
	if len(query.OrderBy) > 0 {
		results = e.applyOrderBy(results, query.OrderBy)
	}

	// Apply LIMIT
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	stats.ExecutionTime = time.Since(startTime)

	return &QueryResult{
		Data:  results,
		Stats: stats,
	}, nil
}

// pathNode represents a node in a traversal path
type pathNode struct {
	NodeID string
	Data   map[string]interface{}
}

// graphSnapshot is an in-memory copy of the graph for consistent reads
type graphSnapshot struct {
	adjacency    map[string]map[string]string // node -> {neighbor -> relationship} (outgoing)
	revAdjacency map[string]map[string]string // node -> {neighbor -> relationship} (incoming)
	nodeData     map[string]map[string]interface{}
}

// takeSnapshot creates a consistent snapshot of the graph
func (e *Executor) takeSnapshot() *graphSnapshot {
	snapshot := &graphSnapshot{
		adjacency:    make(map[string]map[string]string),
		revAdjacency: make(map[string]map[string]string),
		nodeData:     make(map[string]map[string]interface{}),
	}

	// Get all nodes
	nodes := e.graph.GetAllNodes()
	for _, nodeID := range nodes {
		// Copy outgoing adjacency
		neighbors, _ := e.graph.GetNeighbors(nodeID)
		snapshot.adjacency[nodeID] = neighbors

		// Build reverse adjacency (incoming edges)
		for neighbor, relType := range neighbors {
			if snapshot.revAdjacency[neighbor] == nil {
				snapshot.revAdjacency[neighbor] = make(map[string]string)
			}
			snapshot.revAdjacency[neighbor][nodeID] = relType
		}

		// Parse node data from ID (entity:id format)
		parts := strings.SplitN(nodeID, ":", 2)
		if len(parts) == 2 {
			snapshot.nodeData[nodeID] = map[string]interface{}{
				"type": parts[0],
				"id":   nodeID,
			}
		}
	}

	return snapshot
}

// getNeighborsByDirection returns neighbors based on relationship direction
func (e *Executor) getNeighborsByDirection(snapshot *graphSnapshot, node string, direction RelDirection) map[string]string {
	result := make(map[string]string)

	switch direction {
	case RelOutgoing:
		for k, v := range snapshot.adjacency[node] {
			result[k] = v
		}
	case RelIncoming:
		for k, v := range snapshot.revAdjacency[node] {
			result[k] = v
		}
	case RelBidirectional:
		// Both outgoing and incoming
		for k, v := range snapshot.adjacency[node] {
			result[k] = v
		}
		for k, v := range snapshot.revAdjacency[node] {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

// findMatchingNodes finds nodes matching a pattern
func (e *Executor) findMatchingNodes(snapshot *graphSnapshot, pattern NodePattern) []string {
	var matches []string

	for nodeID := range snapshot.adjacency {
		if e.matchesNodePattern(nodeID, snapshot.nodeData[nodeID], pattern) {
			matches = append(matches, nodeID)
		}
	}

	return matches
}

// matchesNodePattern checks if a node matches a pattern
func (e *Executor) matchesNodePattern(nodeID string, nodeData map[string]interface{}, pattern NodePattern) bool {
	// Check type if specified
	if pattern.Type != "" {
		// Node ID format is entity:id, so entity is the type
		parts := strings.SplitN(nodeID, ":", 2)
		if len(parts) < 2 || parts[0] != pattern.Type {
			return false
		}
	}

	// Check inline properties
	for key, expected := range pattern.Properties {
		// Special case for "id" - check against the numeric ID
		if key == "id" {
			parts := strings.SplitN(nodeID, ":", 2)
			if len(parts) == 2 {
				// Compare as string or int
				switch v := expected.(type) {
				case int:
					if parts[1] != fmt.Sprintf("%d", v) {
						return false
					}
				case string:
					if parts[1] != v {
						return false
					}
				}
				continue
			}
		}

		actual, exists := nodeData[key]
		if !exists || !valuesEqual(actual, expected) {
			return false
		}
	}

	return true
}

// bfsTraverse performs BFS traversal following the path pattern
func (e *Executor) bfsTraverse(snapshot *graphSnapshot, startNode string, pathPattern []PathElement, stats *QueryStats) [][]pathNode {
	var results [][]pathNode

	type queueItem struct {
		node            string
		patternIndex    int
		path            []pathNode
		varLengthHops   int  // Current hop count in variable-length segment
		inVarLength     bool // Currently traversing a variable-length segment
	}

	queue := []queueItem{{
		node:         startNode,
		patternIndex: 0,
		path:         nil,
	}}

	visited := make(map[string]bool)
	maxIterations := 10000
	iterations := 0

	for len(queue) > 0 && iterations < maxIterations {
		iterations++
		current := queue[0]
		queue = queue[1:]

		if len(current.path) > e.maxDepth {
			continue
		}

		// For variable-length, we use a different visit key that includes hop count
		var visitKey string
		if current.inVarLength {
			visitKey = fmt.Sprintf("%s:%d:%d", current.node, current.patternIndex, current.varLengthHops)
		} else {
			visitKey = fmt.Sprintf("%s:%d", current.node, current.patternIndex)
		}
		if visited[visitKey] {
			continue
		}
		visited[visitKey] = true
		stats.NodesTraversed++

		// Add current node to path
		newPath := make([]pathNode, len(current.path)+1)
		copy(newPath, current.path)
		newPath[len(current.path)] = pathNode{
			NodeID: current.node,
			Data:   snapshot.nodeData[current.node],
		}

		// Check if we've completed the pattern
		if current.patternIndex >= len(pathPattern)-1 {
			results = append(results, newPath)
			continue
		}

		// Get relationship pattern for current position
		relPattern := pathPattern[current.patternIndex].Relationship
		nextNodePattern := pathPattern[current.patternIndex+1].Node

		// Handle variable-length relationships
		if relPattern != nil && relPattern.IsVariable {
			maxHops := relPattern.MaxHops
			if maxHops == 0 {
				maxHops = e.maxDepth
			}

			// If we've reached minimum hops, we can accept this as a valid endpoint
			// and also continue traversing
			if current.varLengthHops >= relPattern.MinHops {
				// Check if current node matches the next pattern
				if e.matchesNodePattern(current.node, snapshot.nodeData[current.node], nextNodePattern) {
					// This is a valid path endpoint
					if current.patternIndex+1 >= len(pathPattern)-1 {
						// Final node in pattern
						results = append(results, newPath)
					} else {
						// Continue to next pattern segment
						queue = append(queue, queueItem{
							node:         current.node,
							patternIndex: current.patternIndex + 1,
							path:         current.path, // Don't duplicate the node
						})
					}
				}
			}

			// Continue variable-length traversal if under max
			if current.varLengthHops < maxHops {
				direction := RelOutgoing
				if relPattern != nil {
					direction = relPattern.Direction
				}
				for neighbor, edgeType := range e.getNeighborsByDirection(snapshot, current.node, direction) {
					if relPattern.Type != "" && edgeType != relPattern.Type {
						continue
					}
					queue = append(queue, queueItem{
						node:          neighbor,
						patternIndex:  current.patternIndex,
						path:          newPath,
						varLengthHops: current.varLengthHops + 1,
						inVarLength:   true,
					})
				}
			}
		} else {
			// Regular single-hop relationship
			direction := RelOutgoing
			if relPattern != nil {
				direction = relPattern.Direction
			}
			for neighbor, edgeType := range e.getNeighborsByDirection(snapshot, current.node, direction) {
				if relPattern != nil && relPattern.Type != "" && edgeType != relPattern.Type {
					continue
				}
				if !e.matchesNodePattern(neighbor, snapshot.nodeData[neighbor], nextNodePattern) {
					continue
				}
				queue = append(queue, queueItem{
					node:         neighbor,
					patternIndex: current.patternIndex + 1,
					path:         newPath,
				})
			}
		}
	}

	return results
}

// dfsTraverse performs DFS traversal following the path pattern
func (e *Executor) dfsTraverse(snapshot *graphSnapshot, startNode string, pathPattern []PathElement, stats *QueryStats) [][]pathNode {
	var results [][]pathNode
	visited := make(map[string]bool)

	e.dfsRecursive(snapshot, startNode, 0, 0, false, nil, pathPattern, visited, &results, stats)

	return results
}

func (e *Executor) dfsRecursive(
	snapshot *graphSnapshot,
	node string,
	patternIndex int,
	varLengthHops int,
	inVarLength bool,
	currentPath []pathNode,
	pathPattern []PathElement,
	visited map[string]bool,
	results *[][]pathNode,
	stats *QueryStats,
) {
	if len(currentPath) > e.maxDepth {
		return
	}

	// Create visit key based on whether we're in variable-length mode
	var visitKey string
	if inVarLength {
		visitKey = fmt.Sprintf("%s:%d:%d", node, patternIndex, varLengthHops)
	} else {
		visitKey = fmt.Sprintf("%s:%d", node, patternIndex)
	}
	if visited[visitKey] {
		return
	}

	// Make a copy of visited for this branch
	visitedCopy := make(map[string]bool)
	for k, v := range visited {
		visitedCopy[k] = v
	}
	visitedCopy[visitKey] = true
	stats.NodesTraversed++

	// Add current node to path
	newPath := make([]pathNode, len(currentPath)+1)
	copy(newPath, currentPath)
	newPath[len(currentPath)] = pathNode{
		NodeID: node,
		Data:   snapshot.nodeData[node],
	}

	// Check if we've completed the pattern
	if patternIndex >= len(pathPattern)-1 {
		*results = append(*results, newPath)
		return
	}

	// Get relationship pattern for current position
	relPattern := pathPattern[patternIndex].Relationship
	nextNodePattern := pathPattern[patternIndex+1].Node

	// Handle variable-length relationships
	if relPattern != nil && relPattern.IsVariable {
		maxHops := relPattern.MaxHops
		if maxHops == 0 {
			maxHops = e.maxDepth
		}

		// If we've reached minimum hops, we can accept this as a valid endpoint
		if varLengthHops >= relPattern.MinHops {
			// Check if current node matches the next pattern
			if e.matchesNodePattern(node, snapshot.nodeData[node], nextNodePattern) {
				if patternIndex+1 >= len(pathPattern)-1 {
					// Final node in pattern
					*results = append(*results, newPath)
				} else {
					// Continue to next pattern segment
					e.dfsRecursive(snapshot, node, patternIndex+1, 0, false,
						currentPath, pathPattern, visitedCopy, results, stats)
				}
			}
		}

		// Continue variable-length traversal if under max
		if varLengthHops < maxHops {
			direction := RelOutgoing
			if relPattern != nil {
				direction = relPattern.Direction
			}
			for neighbor, edgeType := range e.getNeighborsByDirection(snapshot, node, direction) {
				if relPattern.Type != "" && edgeType != relPattern.Type {
					continue
				}
				e.dfsRecursive(snapshot, neighbor, patternIndex, varLengthHops+1, true,
					newPath, pathPattern, visitedCopy, results, stats)
			}
		}
	} else {
		// Regular single-hop relationship
		direction := RelOutgoing
		if relPattern != nil {
			direction = relPattern.Direction
		}
		for neighbor, edgeType := range e.getNeighborsByDirection(snapshot, node, direction) {
			if relPattern != nil && relPattern.Type != "" && edgeType != relPattern.Type {
				continue
			}
			if !e.matchesNodePattern(neighbor, snapshot.nodeData[neighbor], nextNodePattern) {
				continue
			}
			e.dfsRecursive(snapshot, neighbor, patternIndex+1, 0, false,
				newPath, pathPattern, visitedCopy, results, stats)
		}
	}
}

// applyConditions filters paths by WHERE conditions
func (e *Executor) applyConditions(paths [][]pathNode, conditions []Condition, pathPattern []PathElement) [][]pathNode {
	var filtered [][]pathNode

	for _, path := range paths {
		match := true
		for _, cond := range conditions {
			if !e.evaluateCondition(path, cond, pathPattern) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, path)
		}
	}

	return filtered
}

// applyConditionGroups filters paths by OR-joined condition groups
func (e *Executor) applyConditionGroups(paths [][]pathNode, groups []ConditionGroup, pathPattern []PathElement) [][]pathNode {
	var filtered [][]pathNode

	for _, path := range paths {
		// Path matches if ANY group matches (OR logic)
		for _, group := range groups {
			// Within a group, ALL conditions must match (AND logic)
			groupMatch := true
			for _, cond := range group.Conditions {
				if !e.evaluateCondition(path, cond, pathPattern) {
					groupMatch = false
					break
				}
			}
			if groupMatch {
				filtered = append(filtered, path)
				break // Found a matching group, no need to check others
			}
		}
	}

	return filtered
}

// applyDistinct removes duplicate results based on JSON serialization
func (e *Executor) applyDistinct(results []map[string]interface{}) []map[string]interface{} {
	if len(results) == 0 {
		return results
	}

	seen := make(map[string]bool)
	var unique []map[string]interface{}

	for _, result := range results {
		// Serialize to JSON for comparison
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			// If serialization fails, include the result
			unique = append(unique, result)
			continue
		}

		key := string(jsonBytes)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, result)
		}
	}

	return unique
}

// applyOrderBy sorts results by the specified fields
func (e *Executor) applyOrderBy(results []map[string]interface{}, orderBy []OrderByItem) []map[string]interface{} {
	if len(results) == 0 || len(orderBy) == 0 {
		return results
	}

	sort.SliceStable(results, func(i, j int) bool {
		for _, ob := range orderBy {
			vi := getNestedValue(results[i], ob.VarPath)
			vj := getNestedValue(results[j], ob.VarPath)

			cmp := compareForSort(vi, vj)
			if cmp == 0 {
				continue // Equal, check next field
			}

			if ob.Direction == OrderDesc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false // All fields equal
	})

	return results
}

// getNestedValue gets a value from a map using dot notation
func getNestedValue(m map[string]interface{}, path string) interface{} {
	// First try direct key
	if v, ok := m[path]; ok {
		return v
	}

	// Try nested access
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 2 {
		if nested, ok := m[parts[0]].(map[string]interface{}); ok {
			return nested[parts[1]]
		}
	}

	return nil
}

// compareForSort compares two values for sorting, returns -1, 0, or 1
func compareForSort(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try numeric comparison
	aFloat, aIsNum := toFloat64(a)
	bFloat, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

// toFloat64 attempts to convert a value to float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case string:
		var f float64
		_, err := fmt.Sscanf(val, "%f", &f)
		return f, err == nil
	}
	return 0, false
}

// evaluateCondition evaluates a single condition against a path
func (e *Executor) evaluateCondition(path []pathNode, cond Condition, pathPattern []PathElement) bool {
	// Parse varPath: "u.name" -> variable "u", property "name"
	parts := strings.SplitN(cond.VarPath, ".", 2)
	if len(parts) != 2 {
		return false
	}

	varName := parts[0]
	propName := parts[1]

	// Find the node with this variable
	for i, elem := range pathPattern {
		if elem.Node.Variable == varName && i < len(path) {
			nodeData := path[i].Data
			if nodeData == nil {
				return false
			}

			// Special case for "id"
			var value interface{}
			if propName == "id" {
				// Extract numeric ID from node ID
				nodeParts := strings.SplitN(path[i].NodeID, ":", 2)
				if len(nodeParts) == 2 {
					value = nodeParts[1]
				}
			} else {
				value = nodeData[propName]
			}

			return compareValues(value, cond.Operator, cond.Value)
		}
	}

	return false
}

// applyReturn projects the requested fields from paths
func (e *Executor) applyReturn(paths [][]pathNode, returnItems []ReturnItem, pathPattern []PathElement) []map[string]interface{} {
	var results []map[string]interface{}

	for _, path := range paths {
		result := make(map[string]interface{})

		for _, item := range returnItems {
			// Find the node with this variable
			for i, elem := range pathPattern {
				if elem.Node.Variable == item.Variable && i < len(path) {
					if item.Property != "" {
						// Return specific property
						key := item.Variable + "." + item.Property
						if item.Property == "id" {
							// Extract ID from node ID
							parts := strings.SplitN(path[i].NodeID, ":", 2)
							if len(parts) == 2 {
								result[key] = parts[1]
							}
						} else if path[i].Data != nil {
							result[key] = path[i].Data[item.Property]
						}
					} else {
						// Return whole node
						nodeResult := make(map[string]interface{})
						nodeResult["_id"] = path[i].NodeID
						for k, v := range path[i].Data {
							nodeResult[k] = v
						}
						result[item.Variable] = nodeResult
					}
					break
				}
			}
		}

		results = append(results, result)
	}

	return results
}

// Helper functions

func valuesEqual(a, b interface{}) bool {
	// Convert to comparable types
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

func compareValues(value interface{}, op Operator, expected interface{}) bool {
	if value == nil {
		return false
	}

	switch op {
	case OpEq:
		return valuesEqual(value, expected)
	case OpNe:
		return !valuesEqual(value, expected)
	case OpLt, OpGt, OpLte, OpGte:
		return compareNumeric(value, op, expected)
	}

	return false
}

func compareNumeric(value interface{}, op Operator, expected interface{}) bool {
	var vFloat, eFloat float64

	switch v := value.(type) {
	case int:
		vFloat = float64(v)
	case int64:
		vFloat = float64(v)
	case float64:
		vFloat = v
	case string:
		if f, err := parseNumeric(v); err == nil {
			vFloat = f
		} else {
			return false
		}
	default:
		return false
	}

	switch e := expected.(type) {
	case int:
		eFloat = float64(e)
	case int64:
		eFloat = float64(e)
	case float64:
		eFloat = e
	case string:
		if f, err := parseNumeric(e); err == nil {
			eFloat = f
		} else {
			return false
		}
	default:
		return false
	}

	switch op {
	case OpLt:
		return vFloat < eFloat
	case OpGt:
		return vFloat > eFloat
	case OpLte:
		return vFloat <= eFloat
	case OpGte:
		return vFloat >= eFloat
	}

	return false
}

func parseNumeric(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
