// Package main demonstrates the validation rules system.
//
// The validation package provides a composable rule-based validation system
// with built-in rules (Required, Regex, Range, Enum, Length, Custom) and
// support for rule sets with error/warning separation.
package main

import (
	"fmt"
	"strings"

	"github.com/dotcommander/agent/validation"
)

func main() {
	fmt.Println("=== Validation Rules Demo ===")
	fmt.Println()

	// Create a rule set for user registration data
	userRules := validation.NewRuleSet("user_registration",
		// Required fields
		validation.Required("username"),
		validation.Required("email"),
		validation.Required("password"),
		validation.Required("age"),

		// Email format validation
		validation.Regex("email",
			`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
			"must be a valid email address"),

		// Username format: alphanumeric, 3-20 chars
		validation.Regex("username",
			`^[a-zA-Z0-9_]{3,20}$`,
			"must be 3-20 alphanumeric characters"),

		// Password length
		validation.Length("password", 8, 100),

		// Age range
		validation.Range("age", 13, 120),

		// Role must be one of allowed values
		validation.Enum("role", "user", "admin", "moderator"),
	)

	// Add a custom rule for password complexity
	userRules.Add(
		validation.Custom("password_complexity", "password",
			func(value any) (bool, string) {
				pwd, ok := value.(string)
				if !ok {
					return true, "" // Skip non-strings
				}

				hasUpper := false
				hasLower := false
				hasDigit := false

				for _, c := range pwd {
					switch {
					case c >= 'A' && c <= 'Z':
						hasUpper = true
					case c >= 'a' && c <= 'z':
						hasLower = true
					case c >= '0' && c <= '9':
						hasDigit = true
					}
				}

				if !hasUpper || !hasLower || !hasDigit {
					return false, "must contain uppercase, lowercase, and digit"
				}
				return true, ""
			}),
	)

	// Create warning rules (non-fatal)
	warningRules := validation.NewRuleSet("user_warnings",
		// Warn if username is too short
		validation.Custom("username_length_warning", "username",
			func(value any) (bool, string) {
				username, ok := value.(string)
				if !ok {
					return true, ""
				}
				if len(username) < 5 {
					return false, "short usernames may be harder to find"
				}
				return true, ""
			}),
		// Warn about common passwords
		validation.Custom("common_password_warning", "password",
			func(value any) (bool, string) {
				pwd, ok := value.(string)
				if !ok {
					return true, ""
				}
				common := []string{"password", "123456", "qwerty", "admin"}
				for _, c := range common {
					if strings.Contains(strings.ToLower(pwd), c) {
						return false, "password contains common pattern"
					}
				}
				return true, ""
			}),
	)

	// Create validator with rules and warnings
	validator := validation.NewValidator(userRules).WithWarnings(warningRules)

	// Test data sets
	testCases := []struct {
		name string
		data map[string]any
	}{
		{
			name: "Valid User",
			data: map[string]any{
				"username": "john_doe",
				"email":    "john@example.com",
				"password": "SecurePass123",
				"age":      25,
				"role":     "user",
			},
		},
		{
			name: "Missing Required Fields",
			data: map[string]any{
				"username": "alice",
			},
		},
		{
			name: "Invalid Email Format",
			data: map[string]any{
				"username": "bob_user",
				"email":    "not-an-email",
				"password": "SecurePass123",
				"age":      30,
				"role":     "user",
			},
		},
		{
			name: "Invalid Age Range",
			data: map[string]any{
				"username": "young_user",
				"email":    "young@example.com",
				"password": "ValidPass999",
				"age":      10, // Under 13
				"role":     "user",
			},
		},
		{
			name: "Invalid Role",
			data: map[string]any{
				"username": "hacker",
				"email":    "hack@example.com",
				"password": "HackPass123",
				"age":      25,
				"role":     "superadmin", // Not in allowed list
			},
		},
		{
			name: "Weak Password",
			data: map[string]any{
				"username": "weak_pass",
				"email":    "weak@example.com",
				"password": "password", // Common, no uppercase/digit
				"age":      25,
				"role":     "user",
			},
		},
		{
			name: "Short Username with Warning",
			data: map[string]any{
				"username": "bob", // Valid but short
				"email":    "bob@example.com",
				"password": "StrongPass123",
				"age":      25,
				"role":     "user",
			},
		},
	}

	// Run validations
	for _, tc := range testCases {
		fmt.Printf("Test: %s\n", tc.name)
		fmt.Printf("Data: %v\n", tc.data)

		result := validator.Validate(tc.data)

		if result.Valid {
			fmt.Println("Status: VALID")
		} else {
			fmt.Println("Status: INVALID")
		}

		// Show errors
		if len(result.Errors) > 0 {
			fmt.Println("Errors:")
			for _, err := range result.Errors {
				fmt.Printf("  - [%s] %s: %s\n", err.Rule, err.Field, err.Message)
			}
		}

		// Show warnings
		if len(result.Warnings) > 0 {
			fmt.Println("Warnings:")
			for _, warn := range result.Warnings {
				fmt.Printf("  - [%s] %s: %s\n", warn.Rule, warn.Field, warn.Message)
			}
		}

		fmt.Println()
	}

	// Demonstrate ValidateFirst (stop on first error)
	fmt.Println("=== ValidateFirst Demo ===")
	fmt.Println()

	invalidData := map[string]any{
		"username": "a",  // Too short
		"email":    "x",  // Invalid format
		"password": "12", // Too short
		"age":      5,    // Too young
	}

	firstError := userRules.ValidateFirst(invalidData)
	if firstError != nil {
		fmt.Printf("First error found: [%s] %s - %s\n",
			firstError.Rule, firstError.Field, firstError.Message)
	}

	// Demonstrate individual rule usage
	fmt.Println()
	fmt.Println("=== Individual Rule Tests ===")
	fmt.Println()

	// Test Required rule
	requiredRule := validation.Required("name")
	fmt.Printf("Required rule '%s':\n", requiredRule.Name())
	fmt.Printf("  Empty string: %v\n", requiredRule.Validate(map[string]any{"name": ""}))
	fmt.Printf("  With value: %v\n", requiredRule.Validate(map[string]any{"name": "John"}))

	// Test Range rule
	rangeRule := validation.Range("score", 0, 100)
	fmt.Printf("\nRange rule '%s' (0-100):\n", rangeRule.Name())
	fmt.Printf("  Value 50: %v\n", rangeRule.Validate(map[string]any{"score": 50}))
	fmt.Printf("  Value 150: %v\n", rangeRule.Validate(map[string]any{"score": 150}))

	// Test Enum rule
	enumRule := validation.Enum("status", "pending", "active", "closed")
	fmt.Printf("\nEnum rule '%s':\n", enumRule.Name())
	fmt.Printf("  Value 'active': %v\n", enumRule.Validate(map[string]any{"status": "active"}))
	fmt.Printf("  Value 'unknown': %v\n", enumRule.Validate(map[string]any{"status": "unknown"}))
}
