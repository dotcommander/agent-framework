// Package tools provides tool registration and integration.
package tools

import (
	"context"
	"fmt"
	"slices"

	"github.com/dotcommander/agent/internal/conv"
)

// InputValidationError represents a validation failure for tool input.
type InputValidationError struct {
	Field   string
	Message string
}

func (e *InputValidationError) Error() string {
	return fmt.Sprintf("validation failed for field %q: %s", e.Field, e.Message)
}

// Handler is a function that handles tool invocations.
type Handler func(ctx context.Context, input map[string]any) (any, error)

// Tool represents a tool available to the AI.
type Tool struct {
	// Name is the tool name.
	Name string

	// Description describes what the tool does.
	Description string

	// InputSchema is the JSON schema for the tool's input.
	InputSchema map[string]any

	// Handler is the function that executes the tool.
	Handler Handler
}

// NewTool creates a new tool.
func NewTool(name, description string, schema map[string]any, handler Handler) *Tool {
	return &Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
		Handler:     handler,
	}
}

// ValidateInput validates input against the tool's InputSchema.
// Returns nil if validation passes, or an error describing the validation failure.
func (t *Tool) ValidateInput(input map[string]any) error {
	if t.InputSchema == nil {
		return nil
	}

	// Extract properties from schema (both are optional - missing means empty)
	var properties map[string]any
	if p, ok := t.InputSchema["properties"].(map[string]any); ok {
		properties = p
	}

	var required []any
	if r, ok := t.InputSchema["required"].([]any); ok {
		required = r
	}

	// Build set of required fields
	requiredSet := make(map[string]bool)
	for _, r := range required {
		if fieldName, ok := r.(string); ok {
			requiredSet[fieldName] = true
		}
	}

	// Check required fields are present
	for fieldName := range requiredSet {
		if _, exists := input[fieldName]; !exists {
			return &InputValidationError{
				Field:   fieldName,
				Message: "required field is missing",
			}
		}
	}

	// Validate each provided field against its schema
	for fieldName, value := range input {
		propSchema, exists := properties[fieldName]
		if !exists {
			// Unknown field - could be strict or lenient depending on additionalProperties
			additionalProps, hasAdditional := t.InputSchema["additionalProperties"]
			if hasAdditional {
				if allowed, ok := additionalProps.(bool); ok && !allowed {
					return &InputValidationError{
						Field:   fieldName,
						Message: "unknown field not allowed",
					}
				}
			}
			continue
		}

		if err := validateFieldValue(fieldName, value, propSchema); err != nil {
			return err
		}
	}

	return nil
}

// validateFieldValue validates a single field value against its schema definition.
func validateFieldValue(fieldName string, value any, schema any) error {
	propDef, ok := schema.(map[string]any)
	if !ok {
		return nil // Cannot validate without schema definition
	}

	expectedType, _ := propDef["type"].(string)
	if expectedType == "" {
		return nil // No type constraint
	}

	// Check type matches
	if !isValidType(value, expectedType) {
		return &InputValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("expected type %s", expectedType),
		}
	}

	// Validate enum constraints
	if enum, hasEnum := propDef["enum"].([]any); hasEnum {
		if !slices.Contains(enum, value) {
			return &InputValidationError{
				Field:   fieldName,
				Message: "value not in allowed enum values",
			}
		}
	}

	// Validate string constraints
	if expectedType == "string" {
		s, _ := value.(string)

		if minLen, ok := propDef["minLength"].(float64); ok {
			if len(s) < int(minLen) {
				return &InputValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("string length must be at least %d", int(minLen)),
				}
			}
		}
		if maxLen, ok := propDef["maxLength"].(float64); ok {
			if len(s) > int(maxLen) {
				return &InputValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("string length must be at most %d", int(maxLen)),
				}
			}
		}
	}

	// Validate numeric constraints
	if expectedType == "number" || expectedType == "integer" {
		n, _ := conv.ToFloat64(value)

		if min, ok := propDef["minimum"].(float64); ok {
			if n < min {
				return &InputValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("value must be at least %v", min),
				}
			}
		}
		if max, ok := propDef["maximum"].(float64); ok {
			if n > max {
				return &InputValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("value must be at most %v", max),
				}
			}
		}
	}

	// Validate array constraints
	if expectedType == "array" {
		arr, ok := value.([]any)
		if ok {
			if minItems, ok := propDef["minItems"].(float64); ok {
				if len(arr) < int(minItems) {
					return &InputValidationError{
						Field:   fieldName,
						Message: fmt.Sprintf("array must have at least %d items", int(minItems)),
					}
				}
			}
			if maxItems, ok := propDef["maxItems"].(float64); ok {
				if len(arr) > int(maxItems) {
					return &InputValidationError{
						Field:   fieldName,
						Message: fmt.Sprintf("array must have at most %d items", int(maxItems)),
					}
				}
			}
		}
	}

	return nil
}

// isValidType checks if a value matches the expected JSON Schema type.
func isValidType(value any, expectedType string) bool {
	if value == nil {
		return false // null is not valid for non-nullable types
	}

	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32:
			return true
		}
		return false
	case "integer":
		switch v := value.(type) {
		case int, int64, int32:
			return true
		case float64:
			return v == float64(int64(v))
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return true // Unknown type, allow
}

// Invoke invokes the tool with the given input after validation.
func (t *Tool) Invoke(ctx context.Context, input map[string]any) (any, error) {
	if err := t.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}
	return t.Handler(ctx, input)
}
