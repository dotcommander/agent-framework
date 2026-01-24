package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"sync"
)

// ErrToolConflict is returned when multiple servers provide tools with the same name.
var ErrToolConflict = errors.New("tool name conflict")

// MCP-related limits for security.
const (
	// DefaultMaxJSONSize is the maximum size for JSON payloads (1MB).
	DefaultMaxJSONSize = 1 * 1024 * 1024
)

// MCPServer represents an MCP (Model Context Protocol) server.
type MCPServer struct {
	name            string
	version         string
	description     string
	protocolVersion string // MCP protocol version
	tools           map[string]*Tool
	resources       map[string]*Resource
	maxJSONSize     int64 // Maximum size for JSON payloads
	mu              sync.RWMutex
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string         `json:"uri"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	MimeType    string         `json:"mimeType,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// MCPServerOption configures an MCP server.
type MCPServerOption func(*MCPServer)

// WithMCPDescription sets the server description.
func WithMCPDescription(desc string) MCPServerOption {
	return func(s *MCPServer) {
		s.description = desc
	}
}

// WithMCPVersion sets the server version.
func WithMCPVersion(version string) MCPServerOption {
	return func(s *MCPServer) {
		s.version = version
	}
}

// WithMCPMaxJSONSize sets the maximum JSON payload size.
func WithMCPMaxJSONSize(maxSize int64) MCPServerOption {
	return func(s *MCPServer) {
		s.maxJSONSize = maxSize
	}
}

// WithProtocolVersion sets the MCP protocol version.
func WithProtocolVersion(version string) MCPServerOption {
	return func(s *MCPServer) {
		s.protocolVersion = version
	}
}

// NewMCPServer creates a new MCP server.
func NewMCPServer(name string, opts ...MCPServerOption) *MCPServer {
	s := &MCPServer{
		name:            name,
		version:         "1.0.0",
		protocolVersion: "2024-11-05", // Default MCP protocol version
		tools:           make(map[string]*Tool),
		resources:       make(map[string]*Resource),
		maxJSONSize:     DefaultMaxJSONSize,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// DecodeJSONSafe decodes JSON with size limits to prevent memory exhaustion.
func DecodeJSONSafe(data []byte, v any, maxSize int64) error {
	if int64(len(data)) > maxSize {
		return fmt.Errorf("JSON payload exceeds maximum size of %d bytes", maxSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // Optional: stricter parsing

	return decoder.Decode(v)
}

// DecodeJSONFromReaderSafe decodes JSON from a reader with size limits.
func DecodeJSONFromReaderSafe(r io.Reader, v any, maxSize int64) error {
	limitedReader := io.LimitReader(r, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	if int64(len(data)) > maxSize {
		return fmt.Errorf("JSON payload exceeds maximum size of %d bytes", maxSize)
	}

	return json.Unmarshal(data, v)
}

// RegisterTool adds a tool to the server.
func (s *MCPServer) RegisterTool(tool *Tool) error {
	if tool == nil || tool.Name == "" {
		return fmt.Errorf("invalid tool: name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tools[tool.Name]; exists {
		return fmt.Errorf("tool already registered: %s", tool.Name)
	}

	s.tools[tool.Name] = tool
	return nil
}

// RegisterTools adds multiple tools.
func (s *MCPServer) RegisterTools(tools ...*Tool) error {
	for _, tool := range tools {
		if err := s.RegisterTool(tool); err != nil {
			return err
		}
	}
	return nil
}

// UnregisterTool removes a tool.
func (s *MCPServer) UnregisterTool(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tools, name)
}

// GetTool retrieves a tool by name.
func (s *MCPServer) GetTool(name string) *Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tools[name]
}

// ListTools returns all registered tools.
func (s *MCPServer) ListTools() []*Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]*Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}
	return tools
}

// RegisterResource adds a resource.
func (s *MCPServer) RegisterResource(res *Resource) error {
	if res == nil || res.URI == "" {
		return fmt.Errorf("invalid resource: URI is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.resources[res.URI] = res
	return nil
}

// GetResource retrieves a resource by URI.
func (s *MCPServer) GetResource(uri string) *Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resources[uri]
}

// ListResources returns all registered resources.
func (s *MCPServer) ListResources() []*Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]*Resource, 0, len(s.resources))
	for _, res := range s.resources {
		resources = append(resources, res)
	}
	return resources
}

// MCPRequest represents an incoming MCP request.
type MCPRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     any             `json:"id,omitempty"`
}

// MCPResponse represents an MCP response.
type MCPResponse struct {
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
	ID     any    `json:"id,omitempty"`
}

// Error represents an MCP error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ServerInfo contains server metadata.
type ServerInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// GetServerInfo returns server metadata.
func (s *MCPServer) GetServerInfo() *ServerInfo {
	return &ServerInfo{
		Name:        s.name,
		Version:     s.version,
		Description: s.description,
		Capabilities: []string{
			"tools",
			"resources",
		},
	}
}

// HandleRequest processes an MCP request.
func (s *MCPServer) HandleRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	default:
		return &MCPResponse{
			Error: &Error{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
			ID: req.ID,
		}
	}
}

func (s *MCPServer) handleInitialize(req *MCPRequest) *MCPResponse {
	return &MCPResponse{
		Result: map[string]any{
			"serverInfo":      s.GetServerInfo(),
			"protocolVersion": s.protocolVersion,
		},
		ID: req.ID,
	}
}

func (s *MCPServer) handleToolsList(req *MCPRequest) *MCPResponse {
	tools := s.ListTools()

	toolList := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		toolList = append(toolList, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}

	return &MCPResponse{
		Result: map[string]any{
			"tools": toolList,
		},
		ID: req.ID,
	}
}

func (s *MCPServer) handleToolsCall(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	// Use safe JSON decoding with size limits
	if err := DecodeJSONSafe(req.Params, &params, s.maxJSONSize); err != nil {
		return &MCPResponse{
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("invalid params: %v", err),
			},
			ID: req.ID,
		}
	}

	tool := s.GetTool(params.Name)
	if tool == nil {
		return &MCPResponse{
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("tool not found: %s", params.Name),
			},
			ID: req.ID,
		}
	}

	// Unmarshal arguments to map with size limits
	var args map[string]any
	if len(params.Arguments) > 0 {
		if err := DecodeJSONSafe(params.Arguments, &args, s.maxJSONSize); err != nil {
			return &MCPResponse{
				Error: &Error{
					Code:    -32602,
					Message: fmt.Sprintf("invalid arguments: %v", err),
				},
				ID: req.ID,
			}
		}
	}

	// Invoke tool with panic recovery
	result, err := s.safeInvoke(ctx, tool, args)
	if err != nil {
		return &MCPResponse{
			Error: &Error{
				Code:    -32000,
				Message: fmt.Sprintf("tool execution failed: %v", err),
			},
			ID: req.ID,
		}
	}

	return &MCPResponse{
		Result: map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("%v", result),
				},
			},
		},
		ID: req.ID,
	}
}

// safeInvoke invokes a tool with panic recovery.
func (s *MCPServer) safeInvoke(ctx context.Context, tool *Tool, args map[string]any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Log stack trace for debugging before converting to error
			log.Printf("Tool %s panicked: %v\n%s", tool.Name, r, debug.Stack())
			err = fmt.Errorf("tool panicked: %v", r)
			result = nil
		}
	}()

	return tool.Invoke(ctx, args)
}

func (s *MCPServer) handleResourcesList(req *MCPRequest) *MCPResponse {
	resources := s.ListResources()

	resourceList := make([]map[string]any, 0, len(resources))
	for _, res := range resources {
		resourceList = append(resourceList, map[string]any{
			"uri":         res.URI,
			"name":        res.Name,
			"description": res.Description,
			"mimeType":    res.MimeType,
		})
	}

	return &MCPResponse{
		Result: map[string]any{
			"resources": resourceList,
		},
		ID: req.ID,
	}
}

func (s *MCPServer) handleResourcesRead(req *MCPRequest) *MCPResponse {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &MCPResponse{
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("invalid params: %v", err),
			},
			ID: req.ID,
		}
	}

	res := s.GetResource(params.URI)
	if res == nil {
		return &MCPResponse{
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("resource not found: %s", params.URI),
			},
			ID: req.ID,
		}
	}

	return &MCPResponse{
		Result: map[string]any{
			"contents": []map[string]any{
				{
					"uri":      res.URI,
					"mimeType": res.MimeType,
				},
			},
		},
		ID: req.ID,
	}
}

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
		return nil, err
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

		name, _ := tm["name"].(string)
		desc, _ := tm["description"].(string)
		schema, _ := tm["inputSchema"].(map[string]any)

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
		return nil, err
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
