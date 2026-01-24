package validation

import (
	"testing"

	"github.com/dotcommander/agent/internal/conv"
	"github.com/stretchr/testify/assert"
)

// --- ValidationError Tests ---

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *ValidationError
		want string
	}{
		{
			name: "with field",
			err: &ValidationError{
				Rule:    "required",
				Field:   "username",
				Message: "field is required",
			},
			want: "required: field is required (field: username)",
		},
		{
			name: "without field",
			err: &ValidationError{
				Rule:    "custom",
				Message: "validation failed",
			},
			want: "custom: validation failed",
		},
		{
			name: "empty field",
			err: &ValidationError{
				Rule:    "regex",
				Field:   "",
				Message: "pattern mismatch",
			},
			want: "regex: pattern mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- RequiredRule Tests ---

func TestRequiredRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   any
		wantErr bool
	}{
		{"nil value", "username", map[string]any{"username": nil}, true},
		{"empty string", "username", map[string]any{"username": ""}, true},
		{"empty slice", "tags", map[string]any{"tags": []any{}}, true},
		{"empty map", "meta", map[string]any{"meta": map[string]any{}}, true},
		{"valid string", "username", map[string]any{"username": "alice"}, false},
		{"valid number", "count", map[string]any{"count": 42}, false},
		{"zero value", "count", map[string]any{"count": 0}, false},
		{"missing field", "username", map[string]any{}, true},
		{"empty field name (whole value nil)", "", nil, true},
		{"empty field name (whole value string)", "", "valid", false},
		{"empty field name (whole value empty)", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Required(tt.field)
			assert.Equal(t, "required", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "required", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Equal(t, "field is required", err.Message)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- RegexRule Tests ---

func TestRegexRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		pattern string
		message string
		value   any
		wantErr bool
	}{
		{
			name:    "matching pattern",
			field:   "email",
			pattern: `^[a-z]+@[a-z]+\.[a-z]+$`,
			message: "invalid email",
			value:   map[string]any{"email": "alice@example.com"},
			wantErr: false,
		},
		{
			name:    "non-matching pattern",
			field:   "email",
			pattern: `^[a-z]+@[a-z]+\.[a-z]+$`,
			message: "invalid email",
			value:   map[string]any{"email": "not-an-email"},
			wantErr: true,
		},
		{
			name:    "empty string",
			field:   "email",
			pattern: `^[a-z]+@[a-z]+\.[a-z]+$`,
			message: "invalid email",
			value:   map[string]any{"email": ""},
			wantErr: true,
		},
		{
			name:    "non-string value (skip)",
			field:   "count",
			pattern: `^\d+$`,
			message: "must be digits",
			value:   map[string]any{"count": 123},
			wantErr: false,
		},
		{
			name:    "nil value (skip)",
			field:   "email",
			pattern: `^.+$`,
			message: "required",
			value:   map[string]any{"email": nil},
			wantErr: false,
		},
		{
			name:    "missing field (skip)",
			field:   "email",
			pattern: `^.+$`,
			message: "required",
			value:   map[string]any{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Regex(tt.field, tt.pattern, tt.message)
			assert.Equal(t, "regex", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "regex", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Equal(t, tt.message, err.Message)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- EnumRule Tests ---

func TestEnumRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		allowed []string
		value   any
		wantErr bool
	}{
		{
			name:    "valid option first",
			field:   "status",
			allowed: []string{"active", "inactive", "pending"},
			value:   map[string]any{"status": "active"},
			wantErr: false,
		},
		{
			name:    "valid option middle",
			field:   "status",
			allowed: []string{"active", "inactive", "pending"},
			value:   map[string]any{"status": "inactive"},
			wantErr: false,
		},
		{
			name:    "valid option last",
			field:   "status",
			allowed: []string{"active", "inactive", "pending"},
			value:   map[string]any{"status": "pending"},
			wantErr: false,
		},
		{
			name:    "invalid option",
			field:   "status",
			allowed: []string{"active", "inactive"},
			value:   map[string]any{"status": "deleted"},
			wantErr: true,
		},
		{
			name:    "empty string",
			field:   "status",
			allowed: []string{"active", "inactive"},
			value:   map[string]any{"status": ""},
			wantErr: true,
		},
		{
			name:    "non-string value (skip)",
			field:   "status",
			allowed: []string{"1", "2"},
			value:   map[string]any{"status": 1},
			wantErr: false,
		},
		{
			name:    "nil value (skip)",
			field:   "status",
			allowed: []string{"active"},
			value:   map[string]any{"status": nil},
			wantErr: false,
		},
		{
			name:    "empty allowed list",
			field:   "status",
			allowed: []string{},
			value:   map[string]any{"status": "anything"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Enum(tt.field, tt.allowed...)
			assert.Equal(t, "enum", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "enum", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Contains(t, err.Message, "must be one of:")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- RangeRule Tests ---

func TestRangeRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		min     float64
		max     float64
		value   any
		wantErr bool
	}{
		{"below min", "age", 0, 100, map[string]any{"age": -1}, true},
		{"above max", "age", 0, 100, map[string]any{"age": 101}, true},
		{"at min boundary", "age", 0, 100, map[string]any{"age": 0}, false},
		{"at max boundary", "age", 0, 100, map[string]any{"age": 100}, false},
		{"within range", "age", 0, 100, map[string]any{"age": 50}, false},
		{"float64 value", "price", 0.0, 99.99, map[string]any{"price": 49.99}, false},
		{"float32 value", "price", 0.0, 99.99, map[string]any{"price": float32(49.99)}, false},
		{"int value", "count", 1.0, 10.0, map[string]any{"count": 5}, false},
		{"int64 value", "count", 1.0, 10.0, map[string]any{"count": int64(5)}, false},
		{"int32 value", "count", 1.0, 10.0, map[string]any{"count": int32(5)}, false},
		{"non-numeric value (skip)", "age", 0, 100, map[string]any{"age": "50"}, false},
		{"nil value (skip)", "age", 0, 100, map[string]any{"age": nil}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Range(tt.field, tt.min, tt.max)
			assert.Equal(t, "range", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "range", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Contains(t, err.Message, "must be between")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- LengthRule Tests ---

func TestLengthRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		min     int
		max     int
		value   any
		wantErr bool
	}{
		{"string too short", "name", 3, 10, map[string]any{"name": "ab"}, true},
		{"string too long", "name", 3, 10, map[string]any{"name": "abcdefghijk"}, true},
		{"string at min boundary", "name", 3, 10, map[string]any{"name": "abc"}, false},
		{"string at max boundary", "name", 3, 10, map[string]any{"name": "abcdefghij"}, false},
		{"string within range", "name", 3, 10, map[string]any{"name": "alice"}, false},
		{"empty string (too short)", "name", 1, 10, map[string]any{"name": ""}, true},
		{"slice too short", "tags", 2, 5, map[string]any{"tags": []any{"a"}}, true},
		{"slice too long", "tags", 2, 5, map[string]any{"tags": []any{"a", "b", "c", "d", "e", "f"}}, true},
		{"slice within range", "tags", 2, 5, map[string]any{"tags": []any{"a", "b", "c"}}, false},
		{"string slice", "items", 1, 3, map[string]any{"items": []string{"a", "b"}}, false},
		{"non-length-able value (skip)", "count", 1, 10, map[string]any{"count": 42}, false},
		{"nil value (skip)", "name", 1, 10, map[string]any{"name": nil}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Length(tt.field, tt.min, tt.max)
			assert.Equal(t, "length", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "length", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Contains(t, err.Message, "length must be between")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- CustomRule Tests ---

func TestCustomRule(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		checkFn func(value any) (bool, string)
		value   any
		wantErr bool
		wantMsg string
	}{
		{
			name:  "passes validation",
			field: "username",
			checkFn: func(value any) (bool, string) {
				s, ok := value.(string)
				if !ok || len(s) < 3 {
					return false, "username must be at least 3 characters"
				}
				return true, ""
			},
			value:   map[string]any{"username": "alice"},
			wantErr: false,
		},
		{
			name:  "fails validation",
			field: "username",
			checkFn: func(value any) (bool, string) {
				s, ok := value.(string)
				if !ok || len(s) < 3 {
					return false, "username must be at least 3 characters"
				}
				return true, ""
			},
			value:   map[string]any{"username": "ab"},
			wantErr: true,
			wantMsg: "username must be at least 3 characters",
		},
		{
			name:  "custom error message",
			field: "password",
			checkFn: func(value any) (bool, string) {
				return false, "custom error message"
			},
			value:   map[string]any{"password": "weak"},
			wantErr: true,
			wantMsg: "custom error message",
		},
		{
			name:  "nil value handled by custom logic",
			field: "optional",
			checkFn: func(value any) (bool, string) {
				if value == nil {
					return false, "unexpected nil"
				}
				return true, ""
			},
			value:   map[string]any{"optional": nil},
			wantErr: true,
			wantMsg: "unexpected nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Custom("custom", tt.field, tt.checkFn)
			assert.Equal(t, "custom", rule.Name())

			err := rule.Validate(tt.value)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, "custom", err.Rule)
				assert.Equal(t, tt.field, err.Field)
				assert.Equal(t, tt.wantMsg, err.Message)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// --- RuleSet Tests ---

func TestRuleSet_Validate(t *testing.T) {
	t.Run("all rules pass", func(t *testing.T) {
		rs := NewRuleSet("test",
			Required("username"),
			Length("username", 3, 10),
		)

		value := map[string]any{"username": "alice"}
		errors := rs.Validate(value)
		assert.Empty(t, errors)
	})

	t.Run("some rules fail", func(t *testing.T) {
		rs := NewRuleSet("test",
			Required("username"),
			Length("username", 10, 20),
			Regex("username", `^[A-Z]`, "must start with uppercase"),
		)

		value := map[string]any{"username": "alice"}
		errors := rs.Validate(value)
		assert.Len(t, errors, 2) // Length and Regex fail

		// Check that both errors are collected
		rules := make([]string, len(errors))
		for i, err := range errors {
			rules[i] = err.Rule
		}
		assert.Contains(t, rules, "length")
		assert.Contains(t, rules, "regex")
	})

	t.Run("all rules fail", func(t *testing.T) {
		rs := NewRuleSet("test",
			Required("username"),
			Required("email"),
		)

		value := map[string]any{}
		errors := rs.Validate(value)
		assert.Len(t, errors, 2)
	})

	t.Run("empty ruleset", func(t *testing.T) {
		rs := NewRuleSet("test")
		value := map[string]any{"anything": "value"}
		errors := rs.Validate(value)
		assert.Empty(t, errors)
	})
}

func TestRuleSet_ValidateFirst(t *testing.T) {
	t.Run("short-circuits on first error", func(t *testing.T) {
		callCount := 0
		countingRule := Custom("counter", "field", func(value any) (bool, string) {
			callCount++
			return false, "error"
		})

		rs := NewRuleSet("test",
			countingRule,
			countingRule, // Would be called if not short-circuiting
			countingRule,
		)

		value := map[string]any{"field": "test"}
		err := rs.ValidateFirst(value)
		assert.NotNil(t, err)
		assert.Equal(t, 1, callCount, "should stop after first failure")
	})

	t.Run("returns nil when all pass", func(t *testing.T) {
		rs := NewRuleSet("test",
			Required("username"),
			Length("username", 1, 10),
		)

		value := map[string]any{"username": "alice"}
		err := rs.ValidateFirst(value)
		assert.Nil(t, err)
	})

	t.Run("empty ruleset returns nil", func(t *testing.T) {
		rs := NewRuleSet("test")
		value := map[string]any{}
		err := rs.ValidateFirst(value)
		assert.Nil(t, err)
	})
}

func TestRuleSet_Add(t *testing.T) {
	rs := NewRuleSet("test")
	assert.Equal(t, "test", rs.Name())

	rs.Add(Required("username"))
	rs.Add(Length("username", 3, 10), Regex("username", `^[a-z]+$`, "lowercase only"))

	value := map[string]any{"username": "alice"}
	errors := rs.Validate(value)
	assert.Empty(t, errors)

	// Test that Add returns the RuleSet (fluent)
	rs2 := rs.Add(Required("email"))
	assert.Equal(t, rs, rs2)
}

// --- Validator Tests ---

func TestValidator_Validate(t *testing.T) {
	t.Run("valid input", func(t *testing.T) {
		rules := NewRuleSet("rules",
			Required("username"),
			Length("username", 3, 10),
		)
		validator := NewValidator(rules)

		value := map[string]any{"username": "alice"}
		result := validator.Validate(value)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("invalid input", func(t *testing.T) {
		rules := NewRuleSet("rules",
			Required("username"),
			Length("username", 10, 20),
		)
		validator := NewValidator(rules)

		value := map[string]any{"username": "alice"}
		result := validator.Validate(value)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("with warnings", func(t *testing.T) {
		rules := NewRuleSet("rules",
			Required("username"),
		)
		warnings := NewRuleSet("warnings",
			Length("username", 5, 10),
		)
		validator := NewValidator(rules).WithWarnings(warnings)

		value := map[string]any{"username": "bob"} // Valid but triggers warning
		result := validator.Validate(value)
		assert.True(t, result.Valid) // Still valid despite warnings
		assert.Empty(t, result.Errors)
		assert.NotEmpty(t, result.Warnings)
		assert.Equal(t, "length", result.Warnings[0].Rule)
	})

	t.Run("errors and warnings", func(t *testing.T) {
		rules := NewRuleSet("rules",
			Required("username"),
		)
		warnings := NewRuleSet("warnings",
			Regex("username", `^[A-Z]`, "should start with uppercase"),
		)
		validator := NewValidator(rules).WithWarnings(warnings)

		value := map[string]any{} // Missing username
		result := validator.Validate(value)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
		// Warnings may or may not run on invalid data, but should not affect Valid
	})

	t.Run("nil rules", func(t *testing.T) {
		validator := NewValidator(nil)

		value := map[string]any{"anything": "value"}
		result := validator.Validate(value)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})
}

// --- Helper Function Tests ---

func TestExtractField(t *testing.T) {
	tests := []struct {
		name  string
		value any
		field string
		want  any
	}{
		{
			name:  "map[string]any with existing field",
			value: map[string]any{"username": "alice"},
			field: "username",
			want:  "alice",
		},
		{
			name:  "map[string]any with missing field",
			value: map[string]any{"username": "alice"},
			field: "email",
			want:  nil,
		},
		{
			name:  "map[string]string with existing field",
			value: map[string]string{"username": "alice"},
			field: "username",
			want:  "alice",
		},
		{
			name:  "map[string]string with missing field",
			value: map[string]string{"username": "alice"},
			field: "email",
			want:  "",
		},
		{
			name:  "empty field name returns whole value",
			value: "direct-value",
			field: "",
			want:  "direct-value",
		},
		{
			name:  "non-map type returns nil",
			value: "not-a-map",
			field: "username",
			want:  nil,
		},
		{
			name:  "nil value with field",
			value: nil,
			field: "username",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractField(tt.value, tt.field)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "value", false},
		{"empty slice", []any{}, true},
		{"non-empty slice", []any{"item"}, false},
		{"empty map", map[string]any{}, true},
		{"non-empty map", map[string]any{"key": "value"}, false},
		{"zero int", 0, false},
		{"non-zero int", 42, false},
		{"false bool", false, false},
		{"true bool", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmpty(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    float64
		wantOk  bool
	}{
		{"float64", float64(42.5), 42.5, true},
		{"float32", float32(42.5), 42.5, true},
		{"int", int(42), 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"int32", int32(42), 42.0, true},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := conv.ToFloat64(tt.value)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.InDelta(t, tt.want, got, 0.001)
			}
		})
	}
}

func TestGetLength(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{"string", "hello", 5},
		{"empty string", "", 0},
		{"slice any", []any{"a", "b", "c"}, 3},
		{"empty slice any", []any{}, 0},
		{"slice string", []string{"a", "b"}, 2},
		{"empty slice string", []string{}, 0},
		{"int", 42, -1},
		{"bool", true, -1},
		{"nil", nil, -1},
		{"map", map[string]any{"key": "value"}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getLength(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Integration Tests ---

func TestValidation_Integration(t *testing.T) {
	t.Run("user registration validation", func(t *testing.T) {
		rules := NewRuleSet("registration",
			Required("username"),
			Length("username", 3, 20),
			Regex("username", `^[a-zA-Z0-9_]+$`, "username can only contain letters, numbers, and underscores"),
			Required("email"),
			Regex("email", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, "invalid email format"),
			Required("age"),
			Range("age", 13, 120),
		)

		validator := NewValidator(rules)

		// Valid user
		validUser := map[string]any{
			"username": "alice_123",
			"email":    "alice@example.com",
			"age":      25,
		}
		result := validator.Validate(validUser)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)

		// Invalid user - multiple errors
		invalidUser := map[string]any{
			"username": "a",                // Too short
			"email":    "not-an-email",     // Invalid format
			"age":      150,                // Out of range
		}
		result = validator.Validate(invalidUser)
		assert.False(t, result.Valid)
		assert.Len(t, result.Errors, 3)

		// Check error details
		errorRules := make(map[string]bool)
		for _, err := range result.Errors {
			errorRules[err.Rule] = true
		}
		assert.True(t, errorRules["length"])
		assert.True(t, errorRules["regex"])
		assert.True(t, errorRules["range"])
	})

	t.Run("API request validation with warnings", func(t *testing.T) {
		rules := NewRuleSet("required",
			Required("method"),
			Enum("method", "GET", "POST", "PUT", "DELETE"),
		)

		warnings := NewRuleSet("recommendations",
			Custom("deprecated-method", "method", func(value any) (bool, string) {
				if value == "DELETE" {
					return false, "DELETE method is deprecated, use POST with _method=delete"
				}
				return true, ""
			}),
		)

		validator := NewValidator(rules).WithWarnings(warnings)

		// Valid request with warning
		request := map[string]any{"method": "DELETE"}
		result := validator.Validate(request)
		assert.True(t, result.Valid) // Still valid
		assert.Empty(t, result.Errors)
		assert.Len(t, result.Warnings, 1)
		assert.Equal(t, "deprecated-method", result.Warnings[0].Rule)

		// Valid request without warning
		request = map[string]any{"method": "POST"}
		result = validator.Validate(request)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})
}
