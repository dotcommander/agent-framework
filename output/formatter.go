// Package output provides output formatting and dispatching.
package output

import "context"

// Format represents an output format.
type Format string

const (
	// FormatJSON outputs JSON.
	FormatJSON Format = "json"

	// FormatMarkdown outputs Markdown.
	FormatMarkdown Format = "markdown"

	// FormatText outputs plain text.
	FormatText Format = "text"
)

// Formatter formats output.
type Formatter interface {
	// Format formats the result.
	Format(ctx context.Context, result any) (string, error)

	// Name returns the formatter name.
	Name() Format
}
