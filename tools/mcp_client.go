// Package tools provides the MCP client implementation.
//
// MCPClient connects to external MCP servers and provides tool discovery.
// ToolDiscovery aggregates tools from multiple MCP servers with configurable
// conflict resolution strategies.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ErrToolConflict is returned when multiple servers provide tools with the same name.
var ErrToolConflict = errors.New("tool name conflict")

// MCPClient connects to external MCP servers.
type MCPClient struct {
	serverURL string
	transport MCPTransport
}

// MCPTransport handles MCP communication.
type MCPTransport interface {
	Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error)
	Close() error
}

// NewMCPClient creates a new MCP client.
func NewMCPClient(serverURL string, transport MCPTransport) *MCPClient {
	return &MCPClient{
		serverURL: serverURL,
		transport: transport,
	}
}

// ListTools fetches available tools from the server.
func (c *MCPClient) ListTools(ctx context.Context) ([]*Tool, error) {
	resp, err := c.transport.Send(ctx, &MCPRequest{
		Method: "tools/list",
		ID:     1,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected tools format")
	}

	tools := make([]*Tool, 0, len(toolsRaw))
	for _, t := range toolsRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}

		name, ok := tm["name"].(string)
		if !ok || name == "" {
			continue // Skip malformed entries: name is required
		}

		desc, _ := tm["description"].(string) // Optional field, empty string is fine
		schema, _ := tm["inputSchema"].(map[string]any) // Optional field, nil is fine

		tools = append(tools, &Tool{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		})
	}

	return tools, nil
}

// CallTool invokes a tool on the remote server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args any) (any, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}

	paramsJSON, err := json.Marshal(map[string]any{
		"name":      name,
		"arguments": json.RawMessage(argsJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	resp, err := c.transport.Send(ctx, &MCPRequest{
		Method: "tools/call",
		Params: paramsJSON,
		ID:     1,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/call %q: %w", name, err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	return resp.Result, nil
}

// Close closes the client connection.
func (c *MCPClient) Close() error {
	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// ConflictStrategy defines how to handle tool name conflicts.
type ConflictStrategy int

const (
	// ConflictError returns an error on conflict (default).
	ConflictError ConflictStrategy = iota
	// ConflictFirstWins keeps the first tool, ignores later ones.
	ConflictFirstWins
	// ConflictLastWins overwrites with the latest tool (legacy behavior).
	ConflictLastWins
)

// ToolDiscovery discovers tools from multiple MCP servers.
type ToolDiscovery struct {
	clients          []*MCPClient
	tools            map[string]*Tool
	toolSources      map[string]string // tool name -> server URL
	conflictStrategy ConflictStrategy
	mu               sync.RWMutex
}

// DiscoveryOption configures ToolDiscovery.
type DiscoveryOption func(*ToolDiscovery)

// WithConflictStrategy sets the strategy for handling tool name conflicts.
func WithConflictStrategy(strategy ConflictStrategy) DiscoveryOption {
	return func(d *ToolDiscovery) {
		d.conflictStrategy = strategy
	}
}

// NewToolDiscovery creates a new tool discovery service.
func NewToolDiscovery(opts ...DiscoveryOption) *ToolDiscovery {
	d := &ToolDiscovery{
		clients:          make([]*MCPClient, 0),
		tools:            make(map[string]*Tool),
		toolSources:      make(map[string]string),
		conflictStrategy: ConflictError, // Default: error on conflict
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// AddServer adds an MCP server to discover tools from.
func (d *ToolDiscovery) AddServer(client *MCPClient) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clients = append(d.clients, client)
}

// Discover fetches tools from all registered servers.
func (d *ToolDiscovery) Discover(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, client := range d.clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			continue // Skip failed servers
		}

		for _, tool := range tools {
			if existingSource, exists := d.toolSources[tool.Name]; exists {
				switch d.conflictStrategy {
				case ConflictError:
					return fmt.Errorf("%w: tool %q provided by both %q and %q",
						ErrToolConflict, tool.Name, existingSource, client.serverURL)
				case ConflictFirstWins:
					continue // Keep existing tool
				case ConflictLastWins:
					// Fall through to overwrite
				}
			}
			d.tools[tool.Name] = tool
			d.toolSources[tool.Name] = client.serverURL
		}
	}

	return nil
}

// GetTools returns all discovered tools.
func (d *ToolDiscovery) GetTools() []*Tool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tools := make([]*Tool, 0, len(d.tools))
	for _, tool := range d.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolSource returns the server URL that provided the named tool.
func (d *ToolDiscovery) GetToolSource(name string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.toolSources[name]
}

// Close closes all client connections.
func (d *ToolDiscovery) Close() error {
	for _, client := range d.clients {
		if err := client.Close(); err != nil {
			return err
		}
	}
	return nil
}
