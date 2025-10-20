package models

import (
	"encoding/json"
	"time"
)

// Entity represents a stored entity with its data
type Entity struct {
	ID   int                    `json:"id"`
	Type string                 `json:"type,omitempty"`
	Data map[string]interface{} `json:"-"`
}

// UnmarshalJSON implements custom unmarshaling for Entity
func (e *Entity) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	
	if id, ok := raw["id"].(float64); ok {
		e.ID = int(id)
	}
	if t, ok := raw["type"].(string); ok {
		e.Type = t
	}
	e.Data = raw
	return nil
}

// MarshalJSON implements custom marshaling for Entity
func (e *Entity) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Data)
}

// Reference represents a reference to another entity
type Reference struct {
	Type   string `json:"type"`
	Entity string `json:"entity"`
	ID     int    `json:"id"`
}

// IsReference checks if a value is a reference
func IsReference(v interface{}) (*Reference, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	
	typeVal, hasType := m["type"].(string)
	entityVal, hasEntity := m["entity"].(string)
	idVal, hasID := m["id"]
	
	if hasType && typeVal == "REF" && hasEntity && hasID {
		var id int
		switch v := idVal.(type) {
		case float64:
			id = int(v)
		case int:
			id = v
		default:
			return nil, false
		}
		return &Reference{
			Type:   typeVal,
			Entity: entityVal,
			ID:     id,
		}, true
	}
	return nil, false
}

// QueryStats tracks query execution statistics
type QueryStats struct {
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time,omitempty"`
	Duration  float64                `json:"duration,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Query represents a stored query with its execution state
type Query struct {
	ID           string                   `json:"id"`
	QueryString  string                   `json:"query"`
	Status       string                   `json:"status"` // pending, running, completed, failed
	Result       interface{}              `json:"result,omitempty"`
	Error        string                   `json:"error,omitempty"`
	Stats        QueryStats               `json:"stats"`
	ParsedQuery  map[string]interface{}   `json:"parsed_query,omitempty"`
}

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// SortParam represents a sort parameter
type SortParam struct {
	Field string `json:"field"`
	Order string `json:"order"` // asc or desc
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"error"`
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ResourceResponse represents a resource-based response
type ResourceResponse struct {
	Type  string      `json:"type"`
	ID    string      `json:"id,omitempty"`
	Data  interface{} `json:"data"`
	Links interface{} `json:"links,omitempty"`
	Meta  interface{} `json:"meta,omitempty"`
}

// PagedResponse represents a paginated response
type PagedResponse struct {
	Data       interface{} `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalItems int `json:"total_items"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
	Links map[string]string `json:"links,omitempty"`
}

// GraphNode represents a node in the graph
type GraphNode struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// GraphEdge represents an edge in the graph
type GraphEdge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Relationship string `json:"relationship"`
}

// PathInfo represents a path between two nodes
type PathInfo struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Length int           `json:"length"`
	Path   []interface{} `json:"path"`
}
