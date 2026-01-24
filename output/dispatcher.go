package output

import (
	"context"
	"fmt"
	"log"
	"os"
)

// OutputDispatcher defines the contract for output formatting and writing.
type OutputDispatcher interface {
	RegisterFormatter(f Formatter)
	Format(ctx context.Context, result any, format Format) (string, error)
	Write(ctx context.Context, result any, format Format, dest string) error
}

// Compile-time check that Dispatcher implements OutputDispatcher.
var _ OutputDispatcher = (*Dispatcher)(nil)

// Dispatcher handles output formatting and writing.
type Dispatcher struct {
	formatters map[Format]Formatter
}

// NewDispatcher creates a new output dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		formatters: make(map[Format]Formatter),
	}
}

// RegisterFormatter registers a formatter.
func (d *Dispatcher) RegisterFormatter(f Formatter) {
	d.formatters[f.Name()] = f
}

// Format formats the result using the specified format.
// If formatting fails, falls back to text format and logs a warning.
func (d *Dispatcher) Format(ctx context.Context, result any, format Format) (string, error) {
	f, ok := d.formatters[format]
	if !ok {
		return "", fmt.Errorf("unknown format: %s", format)
	}

	formatted, err := f.Format(ctx, result)
	if err != nil {
		// Fallback to text format
		log.Printf("WARNING: %s formatting failed (%v), falling back to text format", format, err)

		textFormatter, ok := d.formatters[FormatText]
		if !ok {
			// No text formatter available, return original error
			return "", err
		}

		textFormatted, textErr := textFormatter.Format(ctx, result)
		if textErr != nil {
			// Text formatting also failed, return original error
			return "", fmt.Errorf("formatting failed: %w (text fallback also failed: %v)", err, textErr)
		}

		return textFormatted, nil
	}

	return formatted, nil
}

// Write writes the formatted output to the specified destination.
// If dest is empty or "-", writes to stdout.
func (d *Dispatcher) Write(ctx context.Context, result any, format Format, dest string) error {
	formatted, err := d.Format(ctx, result, format)
	if err != nil {
		return err
	}

	// Write to stdout by default
	if dest == "" || dest == "-" {
		fmt.Println(formatted)
		return nil
	}

	// Write to file
	file, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(formatted); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
