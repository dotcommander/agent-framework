// Package tools provides tool registration and integration.
package tools

import (
	"context"
)

// Handler is a function that handles tool invocations.
type Handler func(ctx context.Context, input map[string]any) (any, error)

// Tool represents a tool available to the AI.
type Tool struct {
	// Name is the tool name.
	Name string

	// Description describes what the tool does.
	Description string

	// InputSchema is the JSON schema for the tool's input.
	InputSchema map[string]any

	// Handler is the function that executes the tool.
	Handler Handler
}

// NewTool creates a new tool.
func NewTool(name, description string, schema map[string]any, handler Handler) *Tool {
	return &Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
		Handler:     handler,
	}
}

// Invoke invokes the tool with the given input.
func (t *Tool) Invoke(ctx context.Context, input map[string]any) (any, error) {
	return t.Handler(ctx, input)
}
