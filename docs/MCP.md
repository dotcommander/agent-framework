# Model Context Protocol (MCP)

MCP provides a standardized way to expose and consume tools between AI systems.

## Quick Start

### Creating an MCP Server

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent/tools"
)

func main() {
    // Create server
    server := tools.NewMCPServer("my-tools",
        tools.WithMCPDescription("My custom tools"),
        tools.WithMCPVersion("1.0.0"),
    )

    // Register tools
    server.RegisterTool(tools.NewTool(
        "greet",
        "Greet a person by name",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "name": map[string]any{
                    "type":        "string",
                    "description": "Name to greet",
                },
            },
            "required": []string{"name"},
        },
        func(ctx context.Context, input map[string]any) (any, error) {
            name := input["name"].(string)
            return fmt.Sprintf("Hello, %s!", name), nil
        },
    ))

    // Server is ready to handle requests
    fmt.Printf("Server: %s v%s\n",
        server.GetServerInfo().Name,
        server.GetServerInfo().Version)
}
```

### Consuming Tools via MCP Client

```go
// Create client with transport
client := tools.NewMCPClient("http://localhost:8080", transport)

// List available tools
toolList, err := client.ListTools(ctx)
if err != nil {
    panic(err)
}

for _, tool := range toolList {
    fmt.Printf("Tool: %s - %s\n", tool.Name, tool.Description)
}

// Call a tool
result, err := client.CallTool(ctx, "greet", map[string]any{
    "name": "Alice",
})
fmt.Println(result)
```

## Core Components

### MCPServer

The `MCPServer` exposes tools via the MCP protocol:

```go
type MCPServer struct {
    name        string
    version     string
    description string
    tools       map[string]*Tool
    resources   map[string]*Resource
}
```

### MCPClient

The `MCPClient` connects to external MCP servers:

```go
type MCPClient struct {
    serverURL string
    transport MCPTransport
}
```

### ToolDiscovery

`ToolDiscovery` aggregates tools from multiple servers:

```go
type ToolDiscovery struct {
    clients []*MCPClient
    tools   map[string]*Tool
}
```

## Server Operations

### Creating a Server

```go
server := tools.NewMCPServer("server-name",
    tools.WithMCPDescription("Server description"),
    tools.WithMCPVersion("2.0.0"),
)
```

### Registering Tools

```go
// Single tool
err := server.RegisterTool(tool)

// Multiple tools
err := server.RegisterTools(tool1, tool2, tool3)
```

### Tool Schema

Tools use JSON Schema for input validation:

```go
schema := map[string]any{
    "type": "object",
    "properties": map[string]any{
        "path": map[string]any{
            "type":        "string",
            "description": "File path to read",
        },
        "encoding": map[string]any{
            "type":    "string",
            "enum":    []string{"utf-8", "ascii", "binary"},
            "default": "utf-8",
        },
    },
    "required": []string{"path"},
}
```

### Managing Tools

```go
// Get a tool
tool := server.GetTool("greet")

// List all tools
allTools := server.ListTools()

// Remove a tool
server.UnregisterTool("greet")
```

### Registering Resources

Resources are static data the AI can access:

```go
err := server.RegisterResource(&tools.Resource{
    URI:         "config://app/settings",
    Name:        "Application Settings",
    Description: "Current configuration",
    MimeType:    "application/json",
    Metadata: map[string]any{
        "version": "1.0",
    },
})
```

### Server Info

```go
info := server.GetServerInfo()
// Returns: name, version, description, capabilities
```

## Handling Requests

The server processes MCP requests:

```go
ctx := context.Background()
req := &tools.MCPRequest{
    Method: "tools/call",
    Params: json.RawMessage(`{"name":"greet","arguments":{"name":"Bob"}}`),
    ID:     1,
}

resp := server.HandleRequest(ctx, req)
if resp.Error != nil {
    fmt.Printf("Error: %s\n", resp.Error.Message)
} else {
    fmt.Printf("Result: %v\n", resp.Result)
}
```

### Supported Methods

| Method | Description |
|--------|-------------|
| `initialize` | Initialize connection |
| `tools/list` | List available tools |
| `tools/call` | Execute a tool |
| `resources/list` | List available resources |
| `resources/read` | Read a resource |

## Client Operations

### Creating a Client

```go
// With a transport implementation
client := tools.NewMCPClient("server-url", transport)
```

### Implementing Transport

```go
type MCPTransport interface {
    Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error)
    Close() error
}

// Example HTTP transport
type HTTPTransport struct {
    client *http.Client
    url    string
}

func (t *HTTPTransport) Send(ctx context.Context, req *tools.MCPRequest) (*tools.MCPResponse, error) {
    body, _ := json.Marshal(req)
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(body))
    httpResp, err := t.client.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer httpResp.Body.Close()

    var resp tools.MCPResponse
    json.NewDecoder(httpResp.Body).Decode(&resp)
    return &resp, nil
}

func (t *HTTPTransport) Close() error {
    return nil
}
```

### Listing Tools

```go
tools, err := client.ListTools(ctx)
for _, tool := range tools {
    fmt.Printf("%s: %s\n", tool.Name, tool.Description)
}
```

### Calling Tools

```go
result, err := client.CallTool(ctx, "read_file", map[string]any{
    "path": "/etc/hosts",
})
```

## Tool Discovery

Aggregate tools from multiple servers:

```go
// Create discovery service
discovery := tools.NewToolDiscovery()

// Add MCP servers
discovery.AddServer(tools.NewMCPClient("http://server1:8080", transport1))
discovery.AddServer(tools.NewMCPClient("http://server2:8080", transport2))

// Discover all tools
err := discovery.Discover(ctx)
if err != nil {
    panic(err)
}

// Get combined tool list
allTools := discovery.GetTools()

// Clean up
discovery.Close()
```

## Error Handling

MCP uses standard JSON-RPC error codes:

```go
type Error struct {
    Code    int    // Error code
    Message string // Human-readable message
    Data    any    // Additional data
}
```

Common error codes:
- `-32601`: Method not found
- `-32602`: Invalid params
- `-32000`: Tool execution failed

## Example: Complete Server

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"

    "github.com/dotcommander/agent/tools"
)

func main() {
    server := tools.NewMCPServer("file-tools",
        tools.WithMCPDescription("File system tools"),
    )

    // Read file tool
    server.RegisterTool(tools.NewTool(
        "read_file",
        "Read contents of a file",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path": map[string]any{"type": "string"},
            },
            "required": []string{"path"},
        },
        func(ctx context.Context, input map[string]any) (any, error) {
            path := input["path"].(string)
            content, err := os.ReadFile(path)
            if err != nil {
                return nil, err
            }
            return string(content), nil
        },
    ))

    // Write file tool
    server.RegisterTool(tools.NewTool(
        "write_file",
        "Write contents to a file",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path":    map[string]any{"type": "string"},
                "content": map[string]any{"type": "string"},
            },
            "required": []string{"path", "content"},
        },
        func(ctx context.Context, input map[string]any) (any, error) {
            path := input["path"].(string)
            content := input["content"].(string)
            err := os.WriteFile(path, []byte(content), 0644)
            return err == nil, err
        },
    ))

    // HTTP handler
    http.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
        var req tools.MCPRequest
        json.NewDecoder(r.Body).Decode(&req)

        resp := server.HandleRequest(r.Context(), &req)

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    })

    fmt.Println("MCP server running on :8080")
    http.ListenAndServe(":8080", nil)
}
```

## Integration with Tool Registry

Convert registry tools to MCP format:

```go
registry := tools.NewRegistry()
registry.Register(myTool)

// Export as MCP format
mcpFormat := registry.ToMCPFormat()
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [VALIDATION.md](VALIDATION.md) - Validating tool inputs
- [AGENT-LOOP.md](AGENT-LOOP.md) - Using tools in agent loops
