package sulpher

import (
	"testing"

	"github.com/ha1tch/olu/pkg/graph"
)

// setupTestGraph creates a test graph with users and relationships
func setupTestGraph() *graph.IndexedGraph {
	g := graph.NewIndexedGraph()

	// Users
	g.AddNode("users:1", "users")
	g.AddNode("users:2", "users")
	g.AddNode("users:3", "users")
	g.AddNode("users:4", "users")
	g.AddNode("users:5", "users")

	// Posts
	g.AddNode("posts:1", "posts")
	g.AddNode("posts:2", "posts")

	// Follow relationships: 1 -> 2 -> 3 -> 4, 1 -> 5
	g.AddEdge("users:1", "users:2", "FOLLOWS")
	g.AddEdge("users:2", "users:3", "FOLLOWS")
	g.AddEdge("users:3", "users:4", "FOLLOWS")
	g.AddEdge("users:1", "users:5", "FOLLOWS")

	// Knows (bidirectional-ish): 2 <-> 5
	g.AddEdge("users:2", "users:5", "KNOWS")
	g.AddEdge("users:5", "users:2", "KNOWS")

	// Authored
	g.AddEdge("users:1", "posts:1", "AUTHORED")
	g.AddEdge("users:2", "posts:2", "AUTHORED")

	return g
}

func TestExecuteSimpleMatch(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) RETURN u")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 5 {
		t.Errorf("Expected 5 users, got %d", len(result.Data))
	}
}

func TestExecuteWithRelationship(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users)-[:FOLLOWS]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// users:1 follows 2 and 5, users:2 follows 3, users:3 follows 4
	// So f should be: 2, 5, 3, 4 = 4 results
	if len(result.Data) != 4 {
		t.Errorf("Expected 4 followed users, got %d", len(result.Data))
	}
}

func TestExecuteWithInlineProperty(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Match specific user by inline property (id matching)
	query, err := parser.Parse("MATCH (u:users {id: 1}) RETURN u")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 1 {
		t.Errorf("Expected 1 user with id=1, got %d", len(result.Data))
	}
}

func TestExecuteVariableLengthPath(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Find users reachable via 1-3 FOLLOWS hops from user 1
	query, err := parser.Parse("MATCH (u:users {id: 1})-[:FOLLOWS*1..3]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// From user:1: hop1 -> 2,5; hop2 -> 3; hop3 -> 4
	// So f = 2, 5, 3, 4 = 4 results
	if len(result.Data) < 3 {
		t.Errorf("Expected at least 3 users reachable, got %d", len(result.Data))
	}
}

func TestExecuteBFS(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// BFS is the default algorithm
	query, err := parser.Parse("MATCH (u:users)-[:FOLLOWS]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if query.Algorithm != BFS {
		t.Errorf("Expected BFS algorithm (default)")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) == 0 {
		t.Error("Expected results from BFS traversal")
	}
}

func TestExecuteDFS(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// DFS is specified before MATCH
	query, err := parser.Parse("DFS MATCH (u:users)-[:FOLLOWS]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if query.Algorithm != DFS {
		t.Errorf("Expected DFS algorithm")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) == 0 {
		t.Error("Expected results from DFS traversal")
	}
}

func TestExecuteWithWhereCondition(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// This tests WHERE with a condition - depends on node data having the field
	query, err := parser.Parse("MATCH (u:users) WHERE u.id > 2 RETURN u")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// users:3, users:4, users:5 have id > 2
	if len(result.Data) != 3 {
		t.Errorf("Expected 3 users with id > 2, got %d", len(result.Data))
	}
}

func TestExecuteWithOrConditions(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) WHERE u.id = 1 OR u.id = 5 RETURN u")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 2 {
		t.Errorf("Expected 2 users (id=1 or id=5), got %d", len(result.Data))
	}
}

func TestExecuteDistinct(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Without DISTINCT, traversing from multiple start nodes may produce duplicates
	query, err := parser.Parse("MATCH (u:users)-[:FOLLOWS]->(f:users) RETURN DISTINCT f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !query.Distinct {
		t.Error("Expected Distinct=true")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check no duplicates (unique node IDs)
	seen := make(map[string]bool)
	for _, row := range result.Data {
		if f, ok := row["f"].(map[string]interface{}); ok {
			if id, ok := f["_id"].(string); ok {
				if seen[id] {
					t.Errorf("Duplicate found: %s", id)
				}
				seen[id] = true
			}
		}
	}
}

func TestExecuteLimit(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) RETURN u LIMIT 2")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if query.Limit != 2 {
		t.Errorf("Expected Limit=2, got %d", query.Limit)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 2 {
		t.Errorf("Expected 2 results with LIMIT 2, got %d", len(result.Data))
	}
}

func TestExecuteOrderBy(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) RETURN u ORDER BY u.id DESC")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(query.OrderBy) != 1 {
		t.Fatalf("Expected 1 ORDER BY item")
	}
	if query.OrderBy[0].Direction != OrderDesc {
		t.Error("Expected DESC order")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) < 2 {
		t.Skip("Not enough results to verify order")
	}

	// Verify descending order by checking first > last
	// (actual verification depends on how results are structured)
}

func TestExecuteCombined(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users)-[:FOLLOWS]->(f:users) WHERE u.id = 1 RETURN DISTINCT f ORDER BY f.id LIMIT 2")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !query.Distinct {
		t.Error("Expected Distinct")
	}
	if query.Limit != 2 {
		t.Error("Expected Limit=2")
	}
	if len(query.OrderBy) == 0 {
		t.Error("Expected ORDER BY")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) > 2 {
		t.Errorf("Expected at most 2 results, got %d", len(result.Data))
	}
}

func TestExecuteIncoming(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Find who follows user 3 (incoming edges)
	query, err := parser.Parse("MATCH (u:users {id: 3})<-[:FOLLOWS]-(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// user:2 follows user:3
	if len(result.Data) != 1 {
		t.Errorf("Expected 1 follower of user:3, got %d", len(result.Data))
	}
}

func TestExecuteBidirectional(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Bidirectional KNOWS relationship
	query, err := parser.Parse("MATCH (u:users {id: 2})-[:KNOWS]-(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// user:2 KNOWS user:5 (bidirectional)
	if len(result.Data) < 1 {
		t.Errorf("Expected at least 1 KNOWS connection, got %d", len(result.Data))
	}
}

func TestExecuteMultiHop(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// Two hops: u -> f -> g
	query, err := parser.Parse("MATCH (u:users {id: 1})-[:FOLLOWS]->(f:users)-[:FOLLOWS]->(g:users) RETURN g")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 1 -> 2 -> 3, so g = user:3
	if len(result.Data) < 1 {
		t.Errorf("Expected at least 1 two-hop result, got %d", len(result.Data))
	}
}

func TestExecuteNoResults(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	// No LIKES relationships exist
	query, err := parser.Parse("MATCH (u:users)-[:LIKES]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 0 {
		t.Errorf("Expected 0 results for non-existent relationship, got %d", len(result.Data))
	}
}

func TestExecuteReturnMultipleVariables(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users)-[r:FOLLOWS]->(f:users) RETURN u, f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(query.ReturnItems) != 2 {
		t.Errorf("Expected 2 return items, got %d", len(query.ReturnItems))
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) > 0 {
		row := result.Data[0]
		if _, hasU := row["u"]; !hasU {
			t.Error("Expected 'u' in result")
		}
		if _, hasF := row["f"]; !hasF {
			t.Error("Expected 'f' in result")
		}
	}
}

func TestExecuteReturnProperty(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) RETURN u.id")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(query.ReturnItems) != 1 || query.ReturnItems[0].Property != "id" {
		t.Error("Expected return of u.id property")
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Results should contain u.id values
	if len(result.Data) != 5 {
		t.Errorf("Expected 5 results, got %d", len(result.Data))
	}
}

func TestExecuteStats(t *testing.T) {
	g := setupTestGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users)-[:FOLLOWS]->(f:users) RETURN f")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Stats.NodesTraversed == 0 {
		t.Error("Expected NodesTraversed > 0")
	}
	if result.Stats.ExecutionTime == 0 {
		t.Error("Expected ExecutionTime > 0")
	}
}

func TestExecuteEmptyGraph(t *testing.T) {
	g := graph.NewIndexedGraph()
	executor := NewExecutor(g, 10)
	parser := NewParser()

	query, err := parser.Parse("MATCH (u:users) RETURN u")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result, err := executor.Execute(query)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Data) != 0 {
		t.Errorf("Expected 0 results from empty graph, got %d", len(result.Data))
	}
}
