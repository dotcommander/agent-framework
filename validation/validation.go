// Package validation provides rules-based output validation.
package validation

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dotcommander/agent/internal/conv"
)

// ValidationError represents a validation failure.
type ValidationError struct {
	Rule    string
	Field   string
	Message string
	Value   any
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field: %s)", e.Rule, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Rule, e.Message)
}

// Rule defines a validation rule interface.
type Rule interface {
	// Name returns the rule identifier.
	Name() string

	// Validate checks the value and returns an error if invalid.
	Validate(value any) *ValidationError
}

// RuleSet composes multiple rules.
type RuleSet struct {
	name  string
	rules []Rule
}

// NewRuleSet creates a new rule set.
func NewRuleSet(name string, rules ...Rule) *RuleSet {
	return &RuleSet{
		name:  name,
		rules: rules,
	}
}

// Name returns the rule set name.
func (rs *RuleSet) Name() string {
	return rs.name
}

// Add adds rules to the set.
func (rs *RuleSet) Add(rules ...Rule) *RuleSet {
	rs.rules = append(rs.rules, rules...)
	return rs
}

// Validate runs all rules and returns all errors.
func (rs *RuleSet) Validate(value any) []*ValidationError {
	var errors []*ValidationError
	for _, rule := range rs.rules {
		if err := rule.Validate(value); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// ValidateFirst runs rules until first error.
func (rs *RuleSet) ValidateFirst(value any) *ValidationError {
	for _, rule := range rs.rules {
		if err := rule.Validate(value); err != nil {
			return err
		}
	}
	return nil
}

// Result contains validation results.
type Result struct {
	Valid    bool
	Errors   []*ValidationError
	Warnings []*ValidationError
}

// Validator validates values against rule sets.
type Validator struct {
	rules    *RuleSet
	warnings *RuleSet
}

// NewValidator creates a new validator.
func NewValidator(rules *RuleSet) *Validator {
	return &Validator{
		rules:    rules,
		warnings: NewRuleSet("warnings"),
	}
}

// WithWarnings adds warning rules (non-fatal).
func (v *Validator) WithWarnings(rules *RuleSet) *Validator {
	v.warnings = rules
	return v
}

// Validate checks a value against all rules.
func (v *Validator) Validate(value any) *Result {
	result := &Result{Valid: true}

	if v.rules != nil {
		result.Errors = v.rules.Validate(value)
		if len(result.Errors) > 0 {
			result.Valid = false
		}
	}

	if v.warnings != nil {
		result.Warnings = v.warnings.Validate(value)
	}

	return result
}

// --- Built-in Rules ---

// RequiredRule checks that a value is not nil or empty.
type RequiredRule struct {
	field string
}

// Required creates a required field rule.
func Required(field string) *RequiredRule {
	return &RequiredRule{field: field}
}

func (r *RequiredRule) Name() string { return "required" }

func (r *RequiredRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	if isEmpty(v) {
		return &ValidationError{
			Rule:    r.Name(),
			Field:   r.field,
			Message: "field is required",
			Value:   v,
		}
	}
	return nil
}

// RegexRule validates a string against a regex pattern.
type RegexRule struct {
	field   string
	pattern *regexp.Regexp
	message string
}

// Regex creates a regex validation rule.
func Regex(field, pattern, message string) *RegexRule {
	return &RegexRule{
		field:   field,
		pattern: regexp.MustCompile(pattern),
		message: message,
	}
}

func (r *RegexRule) Name() string { return "regex" }

func (r *RegexRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	s, ok := v.(string)
	if !ok {
		return nil // Skip non-strings
	}
	if !r.pattern.MatchString(s) {
		return &ValidationError{
			Rule:    r.Name(),
			Field:   r.field,
			Message: r.message,
			Value:   s,
		}
	}
	return nil
}

// EnumRule validates that a value is in a set of allowed values.
type EnumRule struct {
	field   string
	allowed []string
}

// Enum creates an enum validation rule.
func Enum(field string, allowed ...string) *EnumRule {
	return &EnumRule{
		field:   field,
		allowed: allowed,
	}
}

func (r *EnumRule) Name() string { return "enum" }

func (r *EnumRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	s, ok := v.(string)
	if !ok {
		return nil
	}
	if slices.Contains(r.allowed, s) {
		return nil
	}
	return &ValidationError{
		Rule:    r.Name(),
		Field:   r.field,
		Message: fmt.Sprintf("must be one of: %s", strings.Join(r.allowed, ", ")),
		Value:   s,
	}
}

// RangeRule validates that a numeric value is within a range.
type RangeRule struct {
	field string
	min   float64
	max   float64
}

// Range creates a range validation rule.
func Range(field string, min, max float64) *RangeRule {
	return &RangeRule{
		field: field,
		min:   min,
		max:   max,
	}
}

func (r *RangeRule) Name() string { return "range" }

func (r *RangeRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	n, ok := conv.ToFloat64(v)
	if !ok {
		return nil
	}
	if n < r.min || n > r.max {
		return &ValidationError{
			Rule:    r.Name(),
			Field:   r.field,
			Message: fmt.Sprintf("must be between %.2f and %.2f", r.min, r.max),
			Value:   n,
		}
	}
	return nil
}

// LengthRule validates string or slice length.
type LengthRule struct {
	field string
	min   int
	max   int
}

// Length creates a length validation rule.
func Length(field string, min, max int) *LengthRule {
	return &LengthRule{
		field: field,
		min:   min,
		max:   max,
	}
}

func (r *LengthRule) Name() string { return "length" }

func (r *LengthRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	length := getLength(v)
	if length < 0 {
		return nil // Not a length-able type
	}
	if length < r.min || length > r.max {
		return &ValidationError{
			Rule:    r.Name(),
			Field:   r.field,
			Message: fmt.Sprintf("length must be between %d and %d", r.min, r.max),
			Value:   length,
		}
	}
	return nil
}

// CustomRule allows custom validation logic.
type CustomRule struct {
	name    string
	field   string
	checkFn func(value any) (bool, string)
}

// Custom creates a custom validation rule.
func Custom(name, field string, check func(value any) (valid bool, message string)) *CustomRule {
	return &CustomRule{
		name:    name,
		field:   field,
		checkFn: check,
	}
}

func (r *CustomRule) Name() string { return r.name }

func (r *CustomRule) Validate(value any) *ValidationError {
	v := extractField(value, r.field)
	valid, message := r.checkFn(v)
	if !valid {
		return &ValidationError{
			Rule:    r.Name(),
			Field:   r.field,
			Message: message,
			Value:   v,
		}
	}
	return nil
}

// --- Helper Functions ---

// extractField extracts a field from a map or struct.
func extractField(value any, field string) any {
	if field == "" {
		return value
	}

	switch v := value.(type) {
	case map[string]any:
		return v[field]
	case map[string]string:
		return v[field]
	default:
		return nil
	}
}

// isEmpty checks if a value is empty.
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// getLength returns the length of a string or slice.
func getLength(value any) int {
	switch v := value.(type) {
	case string:
		return len(v)
	case []any:
		return len(v)
	case []string:
		return len(v)
	}
	return -1
}
