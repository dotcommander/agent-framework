package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dotcommander/agent-framework/validation"
)

// QueryOption configures QueryAs behavior.
type QueryOption func(*queryOptions)

type queryOptions struct {
	rules      []validation.Rule
	maxRetries int
}

// WithValidation adds validation rules to check the parsed response.
// If validation fails, the error is fed back to the LLM for retry.
//
// Example:
//
//	type User struct {
//	    Age int `json:"age"`
//	}
//
//	user, err := agent.QueryAs[User](ctx, prompt,
//	    agent.WithValidation(validation.Range("age", 18, 150)),
//	)
func WithValidation(rules ...validation.Rule) QueryOption {
	return func(o *queryOptions) {
		o.rules = append(o.rules, rules...)
	}
}

// WithMaxRetries sets the maximum number of retry attempts when validation fails.
// Default is 3. Set to 0 for no retries.
//
// Example:
//
//	user, err := agent.QueryAs[User](ctx, prompt,
//	    agent.WithValidation(validation.Required("name")),
//	    agent.WithMaxRetries(5),
//	)
func WithMaxRetries(n int) QueryOption {
	return func(o *queryOptions) {
		o.maxRetries = n
	}
}

// validateStruct validates a struct using the provided rules.
func validateStruct(value any, rules []validation.Rule) []string {
	// Convert struct to map for validation
	data, err := json.Marshal(value)
	if err != nil {
		return []string{fmt.Sprintf("marshal error: %v", err)}
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return []string{fmt.Sprintf("unmarshal error: %v", err)}
	}

	var errors []string
	for _, rule := range rules {
		if verr := rule.Validate(m); verr != nil {
			errors = append(errors, verr.Error())
		}
	}
	return errors
}

// buildRetryPrompt creates a prompt that includes the error feedback.
func buildRetryPrompt(originalPrompt, lastResponse string, errors []string) string {
	var sb strings.Builder
	sb.WriteString(originalPrompt)
	sb.WriteString("\n\n---\n")
	sb.WriteString("Your previous response had validation errors. Please fix them.\n\n")
	sb.WriteString("Previous response:\n```json\n")
	sb.WriteString(lastResponse)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Errors:\n")
	for _, e := range errors {
		sb.WriteString("- ")
		sb.WriteString(e)
		sb.WriteString("\n")
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}
