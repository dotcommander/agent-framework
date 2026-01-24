package output

import (
	"context"
	"encoding/json"
	"fmt"
)

// JSONFormatter formats output as JSON.
type JSONFormatter struct {
	indent bool
}

// NewJSONFormatter creates a new JSON formatter.
func NewJSONFormatter(indent bool) *JSONFormatter {
	return &JSONFormatter{indent: indent}
}

// Format formats the result as JSON.
func (f *JSONFormatter) Format(ctx context.Context, result any) (string, error) {
	var data []byte
	var err error

	if f.indent {
		data, err = json.MarshalIndent(result, "", "  ")
	} else {
		data, err = json.Marshal(result)
	}

	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(data), nil
}

// Name returns the formatter name.
func (f *JSONFormatter) Name() Format {
	return FormatJSON
}

// MarkdownFormatter formats output as Markdown.
type MarkdownFormatter struct{}

// NewMarkdownFormatter creates a new Markdown formatter.
func NewMarkdownFormatter() *MarkdownFormatter {
	return &MarkdownFormatter{}
}

// Format formats the result as Markdown.
func (f *MarkdownFormatter) Format(ctx context.Context, result any) (string, error) {
	// Simple markdown formatting - convert to string
	switch v := result.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("```\n%v\n```", v), nil
	}
}

// Name returns the formatter name.
func (f *MarkdownFormatter) Name() Format {
	return FormatMarkdown
}

// TextFormatter formats output as plain text.
type TextFormatter struct{}

// NewTextFormatter creates a new text formatter.
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{}
}

// Format formats the result as plain text.
func (f *TextFormatter) Format(ctx context.Context, result any) (string, error) {
	return fmt.Sprintf("%v", result), nil
}

// Name returns the formatter name.
func (f *TextFormatter) Name() Format {
	return FormatText
}
