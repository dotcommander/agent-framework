# Validation

Rule-based validation for validating AI outputs and tool inputs.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/dotcommander/agent-framework/validation"
)

func main() {
    // Create rules
    rules := validation.NewRuleSet("user-validation",
        validation.Required("name"),
        validation.Length("name", 2, 50),
        validation.Regex("email", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, "invalid email format"),
        validation.Range("age", 0, 150),
    )

    // Validate data
    user := map[string]any{
        "name":  "Alice",
        "email": "alice@example.com",
        "age":   30,
    }

    errors := rules.Validate(user)
    if len(errors) > 0 {
        for _, err := range errors {
            fmt.Printf("Validation error: %s\n", err.Error())
        }
    } else {
        fmt.Println("Validation passed!")
    }
}
```

## Core Concepts

### Rule Interface

All rules implement this interface:

```go
type Rule interface {
    Name() string                      // Rule identifier
    Validate(value any) *ValidationError
}
```

### ValidationError

Errors contain full context:

```go
type ValidationError struct {
    Rule    string // Rule name that failed
    Field   string // Field that failed (if applicable)
    Message string // Human-readable message
    Value   any    // The invalid value
}
```

### RuleSet

Compose multiple rules:

```go
type RuleSet struct {
    name  string
    rules []Rule
}
```

## Built-in Rules

### Required

Ensures a value is not nil or empty:

```go
validation.Required("username")
```

Fails for:
- `nil`
- Empty string `""`
- Empty slice `[]`
- Empty map `{}`

### Regex

Validates strings against a pattern:

```go
// Email pattern
validation.Regex("email", `^[\w.-]+@[\w.-]+\.\w+$`, "must be valid email")

// Phone number
validation.Regex("phone", `^\+?[\d\s-]{10,}$`, "must be valid phone")

// URL
validation.Regex("website", `^https?://`, "must be valid URL")
```

### Enum

Restricts to allowed values:

```go
// Status must be one of these
validation.Enum("status", "pending", "active", "completed", "cancelled")

// Priority levels
validation.Enum("priority", "low", "medium", "high", "critical")
```

### Range

Validates numeric values within bounds:

```go
// Age between 0 and 150
validation.Range("age", 0, 150)

// Price between 0.01 and 10000
validation.Range("price", 0.01, 10000)

// Score percentage
validation.Range("score", 0, 100)
```

Supports: `int`, `int32`, `int64`, `float32`, `float64`

### Length

Validates string or slice length:

```go
// Username 3-20 characters
validation.Length("username", 3, 20)

// Tags list 1-10 items
validation.Length("tags", 1, 10)

// Bio up to 500 characters
validation.Length("bio", 0, 500)
```

### Custom

Create custom validation logic:

```go
validation.Custom("even-number", "value", func(value any) (bool, string) {
    n, ok := value.(int)
    if !ok {
        return false, "must be an integer"
    }
    if n%2 != 0 {
        return false, "must be an even number"
    }
    return true, ""
})
```

## Creating RuleSets

### Basic RuleSet

```go
rules := validation.NewRuleSet("name",
    validation.Required("field1"),
    validation.Length("field1", 1, 100),
)
```

### Adding Rules Dynamically

```go
rules := validation.NewRuleSet("dynamic")
rules.Add(validation.Required("name"))
rules.Add(validation.Regex("email", emailPattern, "invalid email"))
```

### Chaining

```go
rules := validation.NewRuleSet("chained").
    Add(validation.Required("a")).
    Add(validation.Required("b")).
    Add(validation.Required("c"))
```

## Validation Methods

### Validate All

Returns all validation errors:

```go
errors := rules.Validate(data)
for _, err := range errors {
    fmt.Printf("%s: %s\n", err.Field, err.Message)
}
```

### Validate First

Stops at first error (fail-fast):

```go
err := rules.ValidateFirst(data)
if err != nil {
    fmt.Printf("First error: %s\n", err.Error())
}
```

## Using the Validator

The `Validator` wrapper adds warnings support:

```go
// Create validator with rules
validator := validation.NewValidator(
    validation.NewRuleSet("required",
        validation.Required("name"),
        validation.Required("email"),
    ),
)

// Add warning rules (non-fatal)
validator.WithWarnings(
    validation.NewRuleSet("warnings",
        validation.Length("bio", 10, 500),  // Bio should be 10-500 chars
    ),
)

// Validate
result := validator.Validate(data)

if !result.Valid {
    // Hard failures
    for _, err := range result.Errors {
        fmt.Printf("ERROR: %s\n", err.Error())
    }
}

// Soft warnings
for _, warn := range result.Warnings {
    fmt.Printf("WARNING: %s\n", warn.Error())
}
```

### Validation Result

```go
type Result struct {
    Valid    bool               // True if no errors
    Errors   []*ValidationError // Hard failures
    Warnings []*ValidationError // Soft issues
}
```

## Field Extraction

Rules automatically extract fields from maps:

```go
// This works with map[string]any
data := map[string]any{
    "user": map[string]any{
        "name": "Alice",
    },
}

// Validates data["name"], not nested
validation.Required("name").Validate(data)
```

For nested validation, extract first:

```go
userData := data["user"].(map[string]any)
validation.Required("name").Validate(userData)
```

## Example: API Input Validation

```go
func validateCreateUser(input map[string]any) error {
    rules := validation.NewRuleSet("create-user",
        // Required fields
        validation.Required("username"),
        validation.Required("email"),
        validation.Required("password"),

        // Format validation
        validation.Length("username", 3, 30),
        validation.Regex("username", `^[a-zA-Z0-9_]+$`, "alphanumeric and underscore only"),
        validation.Regex("email", `^[\w.-]+@[\w.-]+\.\w+$`, "invalid email format"),
        validation.Length("password", 8, 100),

        // Optional but validated if present
        validation.Enum("role", "user", "admin", "moderator"),
    )

    errors := rules.Validate(input)
    if len(errors) > 0 {
        // Collect error messages
        messages := make([]string, len(errors))
        for i, err := range errors {
            messages[i] = err.Error()
        }
        return fmt.Errorf("validation failed: %s", strings.Join(messages, "; "))
    }

    return nil
}
```

## Example: AI Output Validation

```go
func validateCodeReviewOutput(output map[string]any) *validation.Result {
    validator := validation.NewValidator(
        validation.NewRuleSet("required-fields",
            validation.Required("summary"),
            validation.Required("issues"),
            validation.Required("score"),
        ),
    )

    validator.WithWarnings(
        validation.NewRuleSet("quality",
            validation.Length("summary", 50, 500),
            validation.Range("score", 0, 100),
            validation.Custom("issues-format", "issues", func(v any) (bool, string) {
                issues, ok := v.([]any)
                if !ok {
                    return false, "issues must be an array"
                }
                if len(issues) == 0 {
                    return false, "should identify at least one issue or improvement"
                }
                return true, ""
            }),
        ),
    )

    return validator.Validate(output)
}
```

## Creating Custom Rules

### Simple Custom Rule

```go
type PositiveRule struct {
    field string
}

func Positive(field string) *PositiveRule {
    return &PositiveRule{field: field}
}

func (r *PositiveRule) Name() string { return "positive" }

func (r *PositiveRule) Validate(value any) *validation.ValidationError {
    // Extract field from map
    v := value
    if m, ok := value.(map[string]any); ok {
        v = m[r.field]
    }

    n, ok := v.(float64)
    if !ok {
        return nil // Skip non-numeric
    }

    if n <= 0 {
        return &validation.ValidationError{
            Rule:    r.Name(),
            Field:   r.field,
            Message: "must be positive",
            Value:   n,
        }
    }
    return nil
}
```

### Complex Custom Rule

```go
type PasswordStrengthRule struct {
    field    string
    minScore int
}

func PasswordStrength(field string, minScore int) *PasswordStrengthRule {
    return &PasswordStrengthRule{field: field, minScore: minScore}
}

func (r *PasswordStrengthRule) Name() string { return "password-strength" }

func (r *PasswordStrengthRule) Validate(value any) *validation.ValidationError {
    m, ok := value.(map[string]any)
    if !ok {
        return nil
    }

    password, ok := m[r.field].(string)
    if !ok {
        return nil
    }

    score := 0
    if len(password) >= 8 { score++ }
    if len(password) >= 12 { score++ }
    if regexp.MustCompile(`[a-z]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[A-Z]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[0-9]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) { score++ }

    if score < r.minScore {
        return &validation.ValidationError{
            Rule:    r.Name(),
            Field:   r.field,
            Message: fmt.Sprintf("password strength %d/%d, need at least %d", score, 6, r.minScore),
            Value:   password,
        }
    }
    return nil
}
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [MCP.md](MCP.md) - Validating tool inputs
- [VERIFICATION.md](VERIFICATION.md) - Output verification
