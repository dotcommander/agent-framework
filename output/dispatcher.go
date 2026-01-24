package output

import (
	"context"
	"fmt"
	"os"
)

// Dispatcher handles output formatting and writing.
type Dispatcher struct {
	formatters map[Format]Formatter
	writers    map[string]*os.File
}

// NewDispatcher creates a new output dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		formatters: make(map[Format]Formatter),
		writers:    make(map[string]*os.File),
	}
}

// RegisterFormatter registers a formatter.
func (d *Dispatcher) RegisterFormatter(f Formatter) {
	d.formatters[f.Name()] = f
}

// Format formats the result using the specified format.
func (d *Dispatcher) Format(ctx context.Context, result any, format Format) (string, error) {
	f, ok := d.formatters[format]
	if !ok {
		return "", fmt.Errorf("unknown format: %s", format)
	}
	return f.Format(ctx, result)
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

// Close closes any open file handles.
func (d *Dispatcher) Close() error {
	for _, f := range d.writers {
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
