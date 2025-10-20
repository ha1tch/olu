package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Validator interface defines validation operations
type Validator interface {
	Validate(entity string, data map[string]interface{}) (bool, []string)
	LoadSchema(entity string, schemaData map[string]interface{}) error
	HasSchema(entity string) bool
	GetSchema(entity string) (map[string]interface{}, error)
}

// JSONSchemaValidator implements JSON schema validation
type JSONSchemaValidator struct {
	schemas    map[string]map[string]interface{}
	schemaDir  string
	mu         sync.RWMutex
}

// NewJSONSchemaValidator creates a new JSON schema validator
func NewJSONSchemaValidator(schemaDir string) *JSONSchemaValidator {
	return &JSONSchemaValidator{
		schemas:   make(map[string]map[string]interface{}),
		schemaDir: schemaDir,
	}
}

// LoadSchema loads a schema for an entity
func (v *JSONSchemaValidator) LoadSchema(entity string, schemaData map[string]interface{}) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.schemas[entity] = schemaData
	return nil
}

// LoadSchemaFromFile loads a schema from a file
func (v *JSONSchemaValidator) LoadSchemaFromFile(entity string) error {
	schemaFile := filepath.Join(v.schemaDir, entity+".json")
	
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No schema file, validation passes
		}
		return err
	}
	
	var schemaData map[string]interface{}
	if err := json.Unmarshal(data, &schemaData); err != nil {
		return err
	}
	
	return v.LoadSchema(entity, schemaData)
}

// HasSchema checks if a schema exists for an entity
func (v *JSONSchemaValidator) HasSchema(entity string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	_, exists := v.schemas[entity]
	return exists
}

// GetSchema retrieves a schema for an entity
func (v *JSONSchemaValidator) GetSchema(entity string) (map[string]interface{}, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	schema, exists := v.schemas[entity]
	if !exists {
		return nil, fmt.Errorf("schema not found for entity: %s", entity)
	}
	return schema, nil
}

// Validate validates data against a schema
func (v *JSONSchemaValidator) Validate(entity string, data map[string]interface{}) (bool, []string) {
	v.mu.RLock()
	schema, exists := v.schemas[entity]
	v.mu.RUnlock()
	
	if !exists {
		// No schema means validation passes
		return true, nil
	}
	
	errors := []string{}
	
	// Check required fields
	if required, ok := schema["required"].([]interface{}); ok {
		for _, reqField := range required {
			if field, ok := reqField.(string); ok {
				if _, exists := data[field]; !exists {
					errors = append(errors, fmt.Sprintf("missing required field: %s", field))
				}
			}
		}
	}
	
	// Check properties
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for key, value := range data {
			if key == "id" {
				continue // ID is auto-generated
			}
			
			propSchema, hasProp := properties[key]
			if !hasProp {
				// Allow additional properties by default
				continue
			}
			
			propMap, ok := propSchema.(map[string]interface{})
			if !ok {
				continue
			}
			
			// Validate type
			if expectedType, ok := propMap["type"].(string); ok {
				actualType := getJSONType(value)
				if actualType != expectedType {
					// Special case: allow int for number type
					if !(expectedType == "number" && actualType == "integer") {
						errors = append(errors, 
							fmt.Sprintf("field %s: expected type %s, got %s", 
								key, expectedType, actualType))
					}
				}
			}
			
			// Validate string constraints
			if strVal, ok := value.(string); ok {
				if minLen, ok := propMap["minLength"].(float64); ok {
					if len(strVal) < int(minLen) {
						errors = append(errors, 
							fmt.Sprintf("field %s: string too short (min %d)", key, int(minLen)))
					}
				}
				if maxLen, ok := propMap["maxLength"].(float64); ok {
					if len(strVal) > int(maxLen) {
						errors = append(errors, 
							fmt.Sprintf("field %s: string too long (max %d)", key, int(maxLen)))
					}
				}
			}
			
			// Validate number constraints
			if numVal, ok := value.(float64); ok {
				if min, ok := propMap["minimum"].(float64); ok {
					if numVal < min {
						errors = append(errors, 
							fmt.Sprintf("field %s: value too small (min %v)", key, min))
					}
				}
				if max, ok := propMap["maximum"].(float64); ok {
					if numVal > max {
						errors = append(errors, 
							fmt.Sprintf("field %s: value too large (max %v)", key, max))
					}
				}
			}
			
			// Validate enum
			if enum, ok := propMap["enum"].([]interface{}); ok {
				found := false
				for _, enumVal := range enum {
					if value == enumVal {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, 
						fmt.Sprintf("field %s: value not in allowed enum values", key))
				}
			}
		}
	}
	
	return len(errors) == 0, errors
}

// getJSONType returns the JSON type name for a value
func getJSONType(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		// Check if it's actually an integer
		if f, ok := v.(float64); ok {
			if f == float64(int64(f)) {
				return "integer"
			}
		}
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

// LoadAllSchemas loads all schemas from the schema directory
func (v *JSONSchemaValidator) LoadAllSchemas() error {
	if _, err := os.Stat(v.schemaDir); os.IsNotExist(err) {
		return nil // Schema directory doesn't exist yet
	}
	
	files, err := os.ReadDir(v.schemaDir)
	if err != nil {
		return err
	}
	
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		
		entity := file.Name()[:len(file.Name())-5] // Remove .json extension
		if err := v.LoadSchemaFromFile(entity); err != nil {
			return fmt.Errorf("failed to load schema for %s: %w", entity, err)
		}
	}
	
	return nil
}

// SaveSchema saves a schema to a file
func (v *JSONSchemaValidator) SaveSchema(entity string, schemaData map[string]interface{}) error {
	if err := os.MkdirAll(v.schemaDir, 0755); err != nil {
		return err
	}
	
	schemaFile := filepath.Join(v.schemaDir, entity+".json")
	data, err := json.MarshalIndent(schemaData, "", "  ")
	if err != nil {
		return err
	}
	
	if err := os.WriteFile(schemaFile, data, 0644); err != nil {
		return err
	}
	
	return v.LoadSchema(entity, schemaData)
}

// NoOpValidator is a validator that always passes
type NoOpValidator struct{}

// NewNoOpValidator creates a no-op validator
func NewNoOpValidator() *NoOpValidator {
	return &NoOpValidator{}
}

// Validate always returns true
func (n *NoOpValidator) Validate(entity string, data map[string]interface{}) (bool, []string) {
	return true, nil
}

// LoadSchema is a no-op
func (n *NoOpValidator) LoadSchema(entity string, schemaData map[string]interface{}) error {
	return nil
}

// HasSchema always returns false
func (n *NoOpValidator) HasSchema(entity string) bool {
	return false
}

// GetSchema always returns error
func (n *NoOpValidator) GetSchema(entity string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no-op validator has no schemas")
}
