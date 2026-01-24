package tools

import (
	"context"
	"fmt"
)

// Registry manages tool registration and invocation.
type Registry struct {
	tools map[string]*Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

// Register registers a tool.
func (r *Registry) Register(tool *Tool) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name)
	}
	r.tools[tool.Name] = tool
	return nil
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (*Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *Registry) List() []*Tool {
	tools := make([]*Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Invoke invokes a tool by name.
func (r *Registry) Invoke(ctx context.Context, name string, input map[string]any) (any, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool.Invoke(ctx, input)
}

// ToMCPFormat converts registered tools to MCP server format.
// This is a placeholder for MCP server generation.
func (r *Registry) ToMCPFormat() map[string]any {
	mcpTools := make([]map[string]any, 0, len(r.tools))

	for _, tool := range r.tools {
		mcpTools = append(mcpTools, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}

	return map[string]any{
		"tools": mcpTools,
	}
}
