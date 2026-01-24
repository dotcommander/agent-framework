package agent

import (
	"reflect"
	"strings"
)

// SchemaFor generates a JSON Schema from a Go type using reflection.
// Supports structs, slices, maps, and primitive types.
//
// Example:
//
//	type Response struct {
//	    Name  string   `json:"name"`
//	    Count int      `json:"count"`
//	    Tags  []string `json:"tags"`
//	}
//	schema := agent.SchemaFor[Response]()
func SchemaFor[T any]() map[string]any {
	var zero T
	return generateSchema(reflect.TypeOf(zero))
}

// generateSchema recursively generates JSON Schema from a reflect.Type.
func generateSchema(t reflect.Type) map[string]any {
	// Handle nil type
	if t == nil {
		return map[string]any{"type": "object"}
	}

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "minimum": 0}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": generateSchema(t.Elem()),
		}

	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": generateSchema(t.Elem()),
		}

	case reflect.Struct:
		return generateStructSchema(t)

	case reflect.Interface:
		return map[string]any{"type": "object"}

	default:
		return map[string]any{"type": "object"}
	}
}

// generateStructSchema generates JSON Schema for a struct type.
func generateStructSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			name = field.Name
		}

		// Generate schema for field
		fieldSchema := generateSchema(field.Type)

		// Add description from struct tag if present
		if desc := field.Tag.Get("desc"); desc != "" {
			fieldSchema["description"] = desc
		}

		// Check for enum values
		if enum := field.Tag.Get("enum"); enum != "" {
			fieldSchema["enum"] = strings.Split(enum, ",")
		}

		// Check for validation constraints
		if min := field.Tag.Get("min"); min != "" {
			fieldSchema["minimum"] = parseNumber(min)
		}
		if max := field.Tag.Get("max"); max != "" {
			fieldSchema["maximum"] = parseNumber(max)
		}
		if minLen := field.Tag.Get("minLength"); minLen != "" {
			fieldSchema["minLength"] = parseNumber(minLen)
		}
		if maxLen := field.Tag.Get("maxLength"); maxLen != "" {
			fieldSchema["maxLength"] = parseNumber(maxLen)
		}

		properties[name] = fieldSchema

		// Determine if required
		// Required if: non-pointer, no omitempty, or explicit required tag
		isRequired := field.Type.Kind() != reflect.Ptr && !opts.omitempty
		if reqTag := field.Tag.Get("required"); reqTag == "true" {
			isRequired = true
		}
		if isRequired {
			required = append(required, name)
		}
	}

	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		result["required"] = required
	}

	return result
}

// jsonTagOpts holds parsed JSON tag options.
type jsonTagOpts struct {
	omitempty bool
	string    bool
}

// parseJSONTag parses a JSON struct tag.
func parseJSONTag(tag string) (name string, opts jsonTagOpts) {
	if tag == "" {
		return "", jsonTagOpts{}
	}

	parts := strings.Split(tag, ",")
	name = parts[0]

	for _, opt := range parts[1:] {
		switch opt {
		case "omitempty":
			opts.omitempty = true
		case "string":
			opts.string = true
		}
	}

	return name, opts
}

// parseNumber parses a string as a number for schema constraints.
func parseNumber(s string) any {
	// Try integer first
	var intVal int
	isInt := true
	for _, b := range s {
		if b == '.' {
			isInt = false
			break
		}
		if b < '0' || b > '9' {
			return s // Not a number, return as string
		}
	}

	if isInt {
		for _, b := range s {
			intVal = intVal*10 + int(b-'0')
		}
		return intVal
	}

	// Parse as float
	var floatVal float64
	var decimal float64 = 1
	pastDecimal := false
	for _, b := range s {
		if b == '.' {
			pastDecimal = true
			continue
		}
		if b < '0' || b > '9' {
			return s
		}
		if pastDecimal {
			decimal /= 10
			floatVal += float64(b-'0') * decimal
		} else {
			floatVal = floatVal*10 + float64(b-'0')
		}
	}
	return floatVal
}
