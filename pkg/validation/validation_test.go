package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewJSONSchemaValidator(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")
	if v == nil {
		t.Fatal("NewJSONSchemaValidator returned nil")
	}
	if v.schemas == nil {
		t.Error("schemas map not initialized")
	}
}

func TestLoadSchema(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"name"},
	}

	err := v.LoadSchema("users", schema)
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	if !v.HasSchema("users") {
		t.Error("HasSchema should return true after LoadSchema")
	}
}

func TestHasSchema(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	if v.HasSchema("nonexistent") {
		t.Error("HasSchema should return false for non-existent schema")
	}

	_ = v.LoadSchema("test", map[string]interface{}{"type": "object"})

	if !v.HasSchema("test") {
		t.Error("HasSchema should return true for loaded schema")
	}
}

func TestGetSchema(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	// Non-existent schema
	_, err := v.GetSchema("nonexistent")
	if err == nil {
		t.Error("GetSchema should error for non-existent schema")
	}

	// Load and retrieve
	schema := map[string]interface{}{"type": "object"}
	_ = v.LoadSchema("test", schema)

	retrieved, err := v.GetSchema("test")
	if err != nil {
		t.Fatalf("GetSchema failed: %v", err)
	}
	if retrieved["type"] != "object" {
		t.Error("Retrieved schema doesn't match loaded schema")
	}
}

func TestValidateNoSchema(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	// No schema means validation passes
	valid, errors := v.Validate("unknown", map[string]interface{}{"anything": "goes"})
	if !valid {
		t.Errorf("Validation should pass without schema, errors: %v", errors)
	}
}

func TestValidateRequired(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":  map[string]interface{}{"type": "string"},
			"email": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"name", "email"},
	}
	_ = v.LoadSchema("users", schema)

	// Missing required field
	valid, errors := v.Validate("users", map[string]interface{}{"name": "Alice"})
	if valid {
		t.Error("Should fail when missing required field 'email'")
	}
	if len(errors) == 0 {
		t.Error("Should have error message for missing field")
	}

	// All required fields present
	valid, errors = v.Validate("users", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if !valid {
		t.Errorf("Should pass with all required fields, errors: %v", errors)
	}
}

func TestValidateTypes(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":   map[string]interface{}{"type": "string"},
			"age":    map[string]interface{}{"type": "number"},
			"active": map[string]interface{}{"type": "boolean"},
		},
	}
	_ = v.LoadSchema("users", schema)

	// Correct types
	valid, errors := v.Validate("users", map[string]interface{}{
		"name":   "Alice",
		"age":    float64(30),
		"active": true,
	})
	if !valid {
		t.Errorf("Should pass with correct types, errors: %v", errors)
	}

	// Wrong type for name (number instead of string)
	valid, errors = v.Validate("users", map[string]interface{}{
		"name": float64(123),
	})
	if valid {
		t.Error("Should fail with wrong type for 'name'")
	}
}

func TestValidateStringConstraints(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"username": map[string]interface{}{
				"type":      "string",
				"minLength": float64(3),
				"maxLength": float64(20),
			},
		},
	}
	_ = v.LoadSchema("users", schema)

	// Too short
	valid, errors := v.Validate("users", map[string]interface{}{"username": "ab"})
	if valid {
		t.Error("Should fail for string shorter than minLength")
	}
	found := false
	for _, e := range errors {
		if e == "field username: string too short (min 3)" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected minLength error, got: %v", errors)
	}

	// Too long
	valid, errors = v.Validate("users", map[string]interface{}{
		"username": "thisusernameiswaytoolong",
	})
	if valid {
		t.Error("Should fail for string longer than maxLength")
	}

	// Just right
	valid, errors = v.Validate("users", map[string]interface{}{"username": "alice"})
	if !valid {
		t.Errorf("Should pass for valid string length, errors: %v", errors)
	}
}

func TestValidateNumberConstraints(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"age": map[string]interface{}{
				"type":    "number",
				"minimum": float64(0),
				"maximum": float64(150),
			},
		},
	}
	_ = v.LoadSchema("people", schema)

	// Below minimum
	valid, errors := v.Validate("people", map[string]interface{}{"age": float64(-5)})
	if valid {
		t.Error("Should fail for value below minimum")
	}

	// Above maximum
	valid, errors = v.Validate("people", map[string]interface{}{"age": float64(200)})
	if valid {
		t.Error("Should fail for value above maximum")
	}

	// Valid range
	valid, errors = v.Validate("people", map[string]interface{}{"age": float64(25)})
	if !valid {
		t.Errorf("Should pass for valid number, errors: %v", errors)
	}
}

func TestValidateEnum(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"active", "inactive", "pending"},
			},
		},
	}
	_ = v.LoadSchema("users", schema)

	// Valid enum value
	valid, errors := v.Validate("users", map[string]interface{}{"status": "active"})
	if !valid {
		t.Errorf("Should pass for valid enum value, errors: %v", errors)
	}

	// Invalid enum value
	valid, errors = v.Validate("users", map[string]interface{}{"status": "deleted"})
	if valid {
		t.Error("Should fail for value not in enum")
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":      "string",
				"minLength": float64(3),
			},
			"age": map[string]interface{}{
				"type":    "number",
				"minimum": float64(0),
			},
		},
		"required": []interface{}{"email"},
	}
	_ = v.LoadSchema("users", schema)

	// Multiple validation errors
	valid, errors := v.Validate("users", map[string]interface{}{
		"name": "ab",          // too short
		"age":  float64(-10),  // below minimum
		// email missing (required)
	})

	if valid {
		t.Error("Should fail with multiple errors")
	}
	if len(errors) < 2 {
		t.Errorf("Expected multiple errors, got %d: %v", len(errors), errors)
	}
}

func TestLoadSchemaFromFile(t *testing.T) {
	// Create temp directory and schema file
	tmpDir, err := os.MkdirTemp("", "olu-validation-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaContent := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	schemaFile := filepath.Join(tmpDir, "users.json")
	if err := os.WriteFile(schemaFile, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema file: %v", err)
	}

	v := NewJSONSchemaValidator(tmpDir)
	err = v.LoadSchemaFromFile("users")
	if err != nil {
		t.Fatalf("LoadSchemaFromFile failed: %v", err)
	}

	if !v.HasSchema("users") {
		t.Error("Schema should be loaded from file")
	}

	// Validate against loaded schema
	valid, _ := v.Validate("users", map[string]interface{}{"name": "Alice"})
	if !valid {
		t.Error("Validation should pass with loaded schema")
	}
}

func TestLoadSchemaFromFile_NotExists(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/nonexistent")

	// Should not error when file doesn't exist
	err := v.LoadSchemaFromFile("missing")
	if err != nil {
		t.Errorf("Should not error for missing schema file: %v", err)
	}
}

func TestNoOpValidator(t *testing.T) {
	v := NewNoOpValidator()

	// Always passes
	valid, errors := v.Validate("anything", map[string]interface{}{"any": "data"})
	if !valid {
		t.Error("NoOpValidator should always pass")
	}
	if len(errors) != 0 {
		t.Error("NoOpValidator should return no errors")
	}

	// LoadSchema does nothing but returns nil
	err := v.LoadSchema("test", map[string]interface{}{})
	if err != nil {
		t.Errorf("NoOpValidator.LoadSchema should return nil: %v", err)
	}

	// HasSchema always returns false
	if v.HasSchema("test") {
		t.Error("NoOpValidator.HasSchema should return false")
	}

	// GetSchema returns error for NoOp
	schema, err := v.GetSchema("test")
	if schema != nil {
		t.Error("NoOpValidator.GetSchema should return nil schema")
	}
	if err == nil {
		t.Error("NoOpValidator.GetSchema should return error")
	}
}

func TestValidateAllowsExtraProperties(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
	}
	_ = v.LoadSchema("users", schema)

	// Extra properties should be allowed by default
	valid, errors := v.Validate("users", map[string]interface{}{
		"name":  "Alice",
		"extra": "field",
		"more":  123,
	})
	if !valid {
		t.Errorf("Should allow extra properties, errors: %v", errors)
	}
}

func TestValidateIntegerForNumber(t *testing.T) {
	v := NewJSONSchemaValidator("/tmp/schemas")

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"count": map[string]interface{}{"type": "number"},
		},
	}
	_ = v.LoadSchema("items", schema)

	// Integer should be accepted for number type
	// Note: JSON unmarshals to float64, but we test the int->number coercion
	valid, errors := v.Validate("items", map[string]interface{}{
		"count": float64(42), // This is how JSON numbers come through
	})
	if !valid {
		t.Errorf("Should accept integer for number type, errors: %v", errors)
	}
}
