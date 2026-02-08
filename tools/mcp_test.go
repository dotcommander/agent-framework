package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMCPServer tests server creation with options.
func TestNewMCPServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		serverName  string
		opts        []MCPServerOption
		wantVersion string
		wantDesc    string
		wantMaxSize int64
	}{
		{
			name:        "default",
			serverName:  "test-server",
			opts:        nil,
			wantVersion: "1.0.0",
			wantDesc:    "",
			wantMaxSize: DefaultMaxJSONSize,
		},
		{
			name:       "with description",
			serverName: "test-server",
			opts: []MCPServerOption{
				WithMCPDescription("A test server"),
			},
			wantVersion: "1.0.0",
			wantDesc:    "A test server",
			wantMaxSize: DefaultMaxJSONSize,
		},
		{
			name:       "with version",
			serverName: "test-server",
			opts: []MCPServerOption{
				WithMCPVersion("2.0.0"),
			},
			wantVersion: "2.0.0",
			wantDesc:    "",
			wantMaxSize: DefaultMaxJSONSize,
		},
		{
			name:       "with max size",
			serverName: "test-server",
			opts: []MCPServerOption{
				WithMCPMaxJSONSize(512),
			},
			wantVersion: "1.0.0",
			wantDesc:    "",
			wantMaxSize: 512,
		},
		{
			name:       "with all options",
			serverName: "test-server",
			opts: []MCPServerOption{
				WithMCPDescription("Full server"),
				WithMCPVersion("3.0.0"),
				WithMCPMaxJSONSize(2048),
			},
			wantVersion: "3.0.0",
			wantDesc:    "Full server",
			wantMaxSize: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := NewMCPServer(tt.serverName, tt.opts...)

			assert.Equal(t, tt.serverName, server.name)
			assert.Equal(t, tt.wantVersion, server.version)
			assert.Equal(t, tt.wantDesc, server.description)
			assert.Equal(t, tt.wantMaxSize, server.maxJSONSize)
			assert.NotNil(t, server.tools)
			assert.NotNil(t, server.resources)
		})
	}
}

// TestInitializeHandshake tests the initialize request/response.
func TestInitializeHandshake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		serverName      string
		serverVersion   string
		serverDesc      string
		wantProtocol    string
		wantCapabilities []string
	}{
		{
			name:          "basic initialize",
			serverName:    "test-server",
			serverVersion: "1.0.0",
			serverDesc:    "",
			wantProtocol:  "2024-11-05",
			wantCapabilities: []string{
				"tools",
				"resources",
			},
		},
		{
			name:          "with description",
			serverName:    "my-server",
			serverVersion: "2.0.0",
			serverDesc:    "Test MCP server",
			wantProtocol:  "2024-11-05",
			wantCapabilities: []string{
				"tools",
				"resources",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := NewMCPServer(
				tt.serverName,
				WithMCPVersion(tt.serverVersion),
				WithMCPDescription(tt.serverDesc),
			)

			req := &MCPRequest{
				Method: "initialize",
				ID:     1,
			}

			resp := server.HandleRequest(context.Background(), req)

			require.NotNil(t, resp)
			assert.Nil(t, resp.Error)
			assert.Equal(t, req.ID, resp.ID)

			result, ok := resp.Result.(map[string]any)
			require.True(t, ok, "result should be map[string]any")

			serverInfo, ok := result["serverInfo"].(*ServerInfo)
			require.True(t, ok, "serverInfo should be *ServerInfo")

			assert.Equal(t, tt.serverName, serverInfo.Name)
			assert.Equal(t, tt.serverVersion, serverInfo.Version)
			assert.Equal(t, tt.serverDesc, serverInfo.Description)
			assert.Equal(t, tt.wantCapabilities, serverInfo.Capabilities)

			protocolVersion, ok := result["protocolVersion"].(string)
			require.True(t, ok, "protocolVersion should be string")
			assert.Equal(t, tt.wantProtocol, protocolVersion)
		})
	}
}

// TestGetServerInfo tests server info retrieval.
func TestGetServerInfo(t *testing.T) {
	t.Parallel()

	server := NewMCPServer(
		"test-server",
		WithMCPVersion("1.2.3"),
		WithMCPDescription("Test server description"),
	)

	info := server.GetServerInfo()

	assert.Equal(t, "test-server", info.Name)
	assert.Equal(t, "1.2.3", info.Version)
	assert.Equal(t, "Test server description", info.Description)
	assert.Equal(t, []string{"tools", "resources"}, info.Capabilities)
}

// TestToolsListEmpty tests tools/list with no registered tools.
func TestToolsListEmpty(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	req := &MCPRequest{
		Method: "tools/list",
		ID:     1,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	tools, ok := result["tools"].([]map[string]any)
	require.True(t, ok)
	assert.Empty(t, tools)
}

// TestToolsListWithTools tests tools/list returns correct format.
func TestToolsListWithTools(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	// Register test tools
	tool1 := &Tool{
		Name:        "calculator",
		Description: "Performs calculations",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type": "string",
				},
			},
		},
	}

	tool2 := &Tool{
		Name:        "weather",
		Description: "Gets weather info",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{
					"type": "string",
				},
			},
		},
	}

	require.NoError(t, server.RegisterTool(tool1))
	require.NoError(t, server.RegisterTool(tool2))

	req := &MCPRequest{
		Method: "tools/list",
		ID:     2,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	tools, ok := result["tools"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, tools, 2)

	// Check tool format (order may vary due to map iteration)
	for _, tool := range tools {
		name, ok := tool["name"].(string)
		require.True(t, ok)

		switch name {
		case "calculator":
			assert.Equal(t, "Performs calculations", tool["description"])
			assert.NotNil(t, tool["inputSchema"])
		case "weather":
			assert.Equal(t, "Gets weather info", tool["description"])
			assert.NotNil(t, tool["inputSchema"])
		default:
			t.Errorf("unexpected tool name: %s", name)
		}
	}
}

// TestToolsCallSuccess tests successful tool invocation.
func TestToolsCallSuccess(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	// Register a simple tool
	tool := NewTool(
		"echo",
		"Echoes the input",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		func(ctx context.Context, input map[string]any) (any, error) {
			msg, _ := input["message"].(string)
			return fmt.Sprintf("Echo: %s", msg), nil
		},
	)

	require.NoError(t, server.RegisterTool(tool))

	params, err := json.Marshal(map[string]any{
		"name": "echo",
		"arguments": map[string]any{
			"message": "hello",
		},
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "tools/call",
		Params: params,
		ID:     3,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	content, ok := result["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 1)

	assert.Equal(t, "text", content[0]["type"])
	assert.Equal(t, "Echo: hello", content[0]["text"])
}

// TestToolsCallNotFound tests tool not found error.
func TestToolsCallNotFound(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	params, err := json.Marshal(map[string]any{
		"name": "nonexistent",
		"arguments": map[string]any{
			"foo": "bar",
		},
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "tools/call",
		Params: params,
		ID:     4,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Result)
	require.NotNil(t, resp.Error)

	assert.Equal(t, -32602, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "tool not found: nonexistent")
	assert.Equal(t, req.ID, resp.ID)
}

// TestToolsCallWithPanicRecovery tests panic recovery during tool execution.
func TestToolsCallWithPanicRecovery(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	// Register a tool that panics
	tool := NewTool(
		"panic-tool",
		"A tool that panics",
		map[string]any{
			"type": "object",
		},
		func(ctx context.Context, input map[string]any) (any, error) {
			panic("intentional panic")
		},
	)

	require.NoError(t, server.RegisterTool(tool))

	params, err := json.Marshal(map[string]any{
		"name":      "panic-tool",
		"arguments": map[string]any{},
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "tools/call",
		Params: params,
		ID:     5,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Result)
	require.NotNil(t, resp.Error)

	assert.Equal(t, -32000, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "tool execution failed")
	assert.Contains(t, resp.Error.Message, "tool panicked: intentional panic")
	assert.Equal(t, req.ID, resp.ID)
}

// TestToolsCallInvalidParams tests invalid params error.
func TestToolsCallInvalidParams(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	tests := []struct {
		name       string
		params     []byte
		wantErrMsg string
	}{
		{
			name:       "invalid JSON",
			params:     []byte(`{invalid json}`),
			wantErrMsg: "invalid params",
		},
		{
			name:       "missing name field",
			params:     []byte(`{"arguments": {}}`),
			wantErrMsg: "tool not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &MCPRequest{
				Method: "tools/call",
				Params: tt.params,
				ID:     6,
			}

			resp := server.HandleRequest(context.Background(), req)

			require.NotNil(t, resp)
			assert.Nil(t, resp.Result)
			require.NotNil(t, resp.Error)
			assert.Equal(t, -32602, resp.Error.Code)
			assert.Contains(t, resp.Error.Message, tt.wantErrMsg)
		})
	}
}

// TestResourcesListEmpty tests resources/list with no registered resources.
func TestResourcesListEmpty(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	req := &MCPRequest{
		Method: "resources/list",
		ID:     7,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	resources, ok := result["resources"].([]map[string]any)
	require.True(t, ok)
	assert.Empty(t, resources)
}

// TestResourcesListWithResources tests resources/list returns correct format.
func TestResourcesListWithResources(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	// Register test resources
	res1 := &Resource{
		URI:         "file:///test.txt",
		Name:        "test.txt",
		Description: "A test file",
		MimeType:    "text/plain",
	}

	res2 := &Resource{
		URI:         "file:///data.json",
		Name:        "data.json",
		Description: "JSON data",
		MimeType:    "application/json",
	}

	require.NoError(t, server.RegisterResource(res1))
	require.NoError(t, server.RegisterResource(res2))

	req := &MCPRequest{
		Method: "resources/list",
		ID:     8,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	resources, ok := result["resources"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, resources, 2)

	// Check resource format (order may vary)
	for _, res := range resources {
		uri, ok := res["uri"].(string)
		require.True(t, ok)

		switch uri {
		case "file:///test.txt":
			assert.Equal(t, "test.txt", res["name"])
			assert.Equal(t, "A test file", res["description"])
			assert.Equal(t, "text/plain", res["mimeType"])
		case "file:///data.json":
			assert.Equal(t, "data.json", res["name"])
			assert.Equal(t, "JSON data", res["description"])
			assert.Equal(t, "application/json", res["mimeType"])
		default:
			t.Errorf("unexpected resource URI: %s", uri)
		}
	}
}

// TestResourcesReadSuccess tests successful resource read.
func TestResourcesReadSuccess(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	res := &Resource{
		URI:      "file:///example.txt",
		Name:     "example.txt",
		MimeType: "text/plain",
	}

	require.NoError(t, server.RegisterResource(res))

	params, err := json.Marshal(map[string]any{
		"uri": "file:///example.txt",
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "resources/read",
		Params: params,
		ID:     9,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
	assert.Equal(t, req.ID, resp.ID)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)

	contents, ok := result["contents"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, contents, 1)

	assert.Equal(t, "file:///example.txt", contents[0]["uri"])
	assert.Equal(t, "text/plain", contents[0]["mimeType"])
}

// TestResourcesReadNotFound tests resource not found error.
func TestResourcesReadNotFound(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server")

	params, err := json.Marshal(map[string]any{
		"uri": "file:///nonexistent.txt",
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "resources/read",
		Params: params,
		ID:     10,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	assert.Nil(t, resp.Result)
	require.NotNil(t, resp.Error)

	assert.Equal(t, -32602, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "resource not found: file:///nonexistent.txt")
	assert.Equal(t, req.ID, resp.ID)
}

// TestJSONRPCErrorCodes tests all error code paths.
func TestJSONRPCErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupServer  func() *MCPServer
		request      *MCPRequest
		wantErrCode  int
		wantErrMsg   string
	}{
		{
			name: "method not found (-32601)",
			setupServer: func() *MCPServer {
				return NewMCPServer("test-server")
			},
			request: &MCPRequest{
				Method: "unknown/method",
				ID:     1,
			},
			wantErrCode: -32601,
			wantErrMsg:  "method not found: unknown/method",
		},
		{
			name: "invalid params tools/call (-32602)",
			setupServer: func() *MCPServer {
				return NewMCPServer("test-server")
			},
			request: &MCPRequest{
				Method: "tools/call",
				Params: []byte(`{invalid}`),
				ID:     2,
			},
			wantErrCode: -32602,
			wantErrMsg:  "invalid params",
		},
		{
			name: "tool not found (-32602)",
			setupServer: func() *MCPServer {
				return NewMCPServer("test-server")
			},
			request: &MCPRequest{
				Method: "tools/call",
				Params: mustMarshal(t, map[string]any{
					"name":      "missing",
					"arguments": map[string]any{},
				}),
				ID: 3,
			},
			wantErrCode: -32602,
			wantErrMsg:  "tool not found: missing",
		},
		{
			name: "tool execution failed (-32000)",
			setupServer: func() *MCPServer {
				server := NewMCPServer("test-server")
				tool := NewTool(
					"failing-tool",
					"A tool that fails",
					map[string]any{"type": "object"},
					func(ctx context.Context, input map[string]any) (any, error) {
						return nil, errors.New("execution error")
					},
				)
				_ = server.RegisterTool(tool)
				return server
			},
			request: &MCPRequest{
				Method: "tools/call",
				Params: mustMarshal(t, map[string]any{
					"name":      "failing-tool",
					"arguments": map[string]any{},
				}),
				ID: 4,
			},
			wantErrCode: -32000,
			wantErrMsg:  "tool execution failed",
		},
		{
			name: "invalid params resources/read (-32602)",
			setupServer: func() *MCPServer {
				return NewMCPServer("test-server")
			},
			request: &MCPRequest{
				Method: "resources/read",
				Params: []byte(`{invalid}`),
				ID:     5,
			},
			wantErrCode: -32602,
			wantErrMsg:  "invalid params",
		},
		{
			name: "resource not found (-32602)",
			setupServer: func() *MCPServer {
				return NewMCPServer("test-server")
			},
			request: &MCPRequest{
				Method: "resources/read",
				Params: mustMarshal(t, map[string]any{
					"uri": "file:///missing.txt",
				}),
				ID: 6,
			},
			wantErrCode: -32602,
			wantErrMsg:  "resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.setupServer()
			resp := server.HandleRequest(context.Background(), tt.request)

			require.NotNil(t, resp)
			assert.Nil(t, resp.Result)
			require.NotNil(t, resp.Error)

			assert.Equal(t, tt.wantErrCode, resp.Error.Code)
			assert.Contains(t, resp.Error.Message, tt.wantErrMsg)
			assert.Equal(t, tt.request.ID, resp.ID)
		})
	}
}

// TestDecodeJSONSafe tests safe JSON decoding with size limits.
func TestDecodeJSONSafe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      []byte
		maxSize   int64
		target    any
		wantErr   bool
		errMsg    string
	}{
		{
			name:    "within limit",
			data:    []byte(`{"key": "value"}`),
			maxSize: 100,
			target:  &map[string]string{},
			wantErr: false,
		},
		{
			name:    "exceeds limit",
			data:    []byte(`{"key": "value"}`),
			maxSize: 10,
			target:  &map[string]string{},
			wantErr: true,
			errMsg:  "exceeds maximum size",
		},
		{
			name:    "exactly at limit",
			data:    []byte(`{"key": "value"}`),
			maxSize: 17, // Exact length of JSON
			target:  &map[string]string{},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			data:    []byte(`{invalid}`),
			maxSize: 100,
			target:  &map[string]string{},
			wantErr: true,
			errMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := DecodeJSONSafe(tt.data, tt.target, tt.maxSize)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDecodeJSONFromReaderSafe tests safe JSON decoding from reader.
func TestDecodeJSONFromReaderSafe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		maxSize int64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "within limit",
			data:    `{"key": "value"}`,
			maxSize: 100,
			wantErr: false,
		},
		{
			name:    "exceeds limit",
			data:    `{"key": "value"}`,
			maxSize: 10,
			wantErr: true,
			errMsg:  "exceeds maximum size",
		},
		{
			name:    "large payload",
			data:    strings.Repeat("x", 1000),
			maxSize: 500,
			wantErr: true,
			errMsg:  "exceeds maximum size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := bytes.NewReader([]byte(tt.data))
			var target map[string]string

			err := DecodeJSONFromReaderSafe(reader, &target, tt.maxSize)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestMaxJSONSizeEnforcement tests that maxJSONSize is enforced.
func TestMaxJSONSizeEnforcement(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("test-server", WithMCPMaxJSONSize(100))

	tool := NewTool(
		"test-tool",
		"Test tool",
		map[string]any{"type": "object"},
		func(ctx context.Context, input map[string]any) (any, error) {
			return "ok", nil
		},
	)

	require.NoError(t, server.RegisterTool(tool))

	// Create a large payload that exceeds the limit
	largeArgs := make(map[string]string)
	for i := range 100 {
		largeArgs[fmt.Sprintf("key%d", i)] = "value"
	}

	params, err := json.Marshal(map[string]any{
		"name":      "test-tool",
		"arguments": largeArgs,
	})
	require.NoError(t, err)

	req := &MCPRequest{
		Method: "tools/call",
		Params: params,
		ID:     1,
	}

	resp := server.HandleRequest(context.Background(), req)

	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "invalid params")
}

// TestToolRegistration tests tool registration scenarios.
func TestToolRegistration(t *testing.T) {
	t.Parallel()

	t.Run("register nil tool", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		err := server.RegisterTool(nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tool")
	})

	t.Run("register tool with empty name", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		tool := &Tool{
			Name:        "",
			Description: "No name",
		}

		err := server.RegisterTool(tool)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("register duplicate tool", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		tool := &Tool{
			Name:        "duplicate",
			Description: "Duplicate tool",
		}

		err := server.RegisterTool(tool)
		require.NoError(t, err)

		err = server.RegisterTool(tool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("register multiple tools", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")

		tool1 := &Tool{Name: "tool1", Description: "First"}
		tool2 := &Tool{Name: "tool2", Description: "Second"}
		tool3 := &Tool{Name: "tool3", Description: "Third"}

		err := server.RegisterTools(tool1, tool2, tool3)
		require.NoError(t, err)

		tools := server.ListTools()
		assert.Len(t, tools, 3)
	})

	t.Run("unregister tool", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		tool := &Tool{Name: "removable", Description: "Will be removed"}

		require.NoError(t, server.RegisterTool(tool))
		assert.NotNil(t, server.GetTool("removable"))

		server.UnregisterTool("removable")
		assert.Nil(t, server.GetTool("removable"))
	})
}

// TestResourceRegistration tests resource registration scenarios.
func TestResourceRegistration(t *testing.T) {
	t.Parallel()

	t.Run("register nil resource", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		err := server.RegisterResource(nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource")
	})

	t.Run("register resource with empty URI", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		res := &Resource{
			URI:  "",
			Name: "No URI",
		}

		err := server.RegisterResource(res)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "URI is required")
	})

	t.Run("register and retrieve resource", func(t *testing.T) {
		t.Parallel()

		server := NewMCPServer("test-server")
		res := &Resource{
			URI:      "file:///test.txt",
			Name:     "test.txt",
			MimeType: "text/plain",
		}

		err := server.RegisterResource(res)
		require.NoError(t, err)

		retrieved := server.GetResource("file:///test.txt")
		assert.NotNil(t, retrieved)
		assert.Equal(t, res.URI, retrieved.URI)
		assert.Equal(t, res.Name, retrieved.Name)
	})
}

// TestFullRequestResponseCycle tests complete request/response flow.
func TestFullRequestResponseCycle(t *testing.T) {
	t.Parallel()

	server := NewMCPServer(
		"integration-test",
		WithMCPVersion("2.0.0"),
		WithMCPDescription("Integration test server"),
	)

	// Register a calculator tool
	calcTool := NewTool(
		"add",
		"Adds two numbers",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
		},
		func(ctx context.Context, input map[string]any) (any, error) {
			a, _ := input["a"].(float64)
			b, _ := input["b"].(float64)
			return a + b, nil
		},
	)

	require.NoError(t, server.RegisterTool(calcTool))

	// Register a resource
	res := &Resource{
		URI:      "file:///data.json",
		Name:     "data.json",
		MimeType: "application/json",
	}
	require.NoError(t, server.RegisterResource(res))

	ctx := context.Background()

	// 1. Initialize
	initResp := server.HandleRequest(ctx, &MCPRequest{
		Method: "initialize",
		ID:     1,
	})
	require.NotNil(t, initResp)
	assert.Nil(t, initResp.Error)

	// 2. List tools
	toolsResp := server.HandleRequest(ctx, &MCPRequest{
		Method: "tools/list",
		ID:     2,
	})
	require.NotNil(t, toolsResp)
	assert.Nil(t, toolsResp.Error)

	// 3. Call tool
	callResp := server.HandleRequest(ctx, &MCPRequest{
		Method: "tools/call",
		Params: mustMarshal(t, map[string]any{
			"name": "add",
			"arguments": map[string]any{
				"a": 5.0,
				"b": 3.0,
			},
		}),
		ID: 3,
	})
	require.NotNil(t, callResp)
	assert.Nil(t, callResp.Error)

	result, ok := callResp.Result.(map[string]any)
	require.True(t, ok)
	content, ok := result["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	assert.Contains(t, content[0]["text"].(string), "8")

	// 4. List resources
	resourcesResp := server.HandleRequest(ctx, &MCPRequest{
		Method: "resources/list",
		ID:     4,
	})
	require.NotNil(t, resourcesResp)
	assert.Nil(t, resourcesResp.Error)

	// 5. Read resource
	readResp := server.HandleRequest(ctx, &MCPRequest{
		Method: "resources/read",
		Params: mustMarshal(t, map[string]any{
			"uri": "file:///data.json",
		}),
		ID: 5,
	})
	require.NotNil(t, readResp)
	assert.Nil(t, readResp.Error)
}

// TestConcurrentRequests tests concurrent request handling.
func TestConcurrentRequests(t *testing.T) {
	t.Parallel()

	server := NewMCPServer("concurrent-test")

	tool := NewTool(
		"counter",
		"Returns a counter value",
		map[string]any{"type": "object"},
		func(ctx context.Context, input map[string]any) (any, error) {
			return "ok", nil
		},
	)

	require.NoError(t, server.RegisterTool(tool))

	ctx := context.Background()
	numRequests := 100

	var wg sync.WaitGroup
	wg.Add(numRequests)

	successCount := 0
	var mu sync.Mutex

	for i := range numRequests {
		go func(id int) {
			defer wg.Done()

			resp := server.HandleRequest(ctx, &MCPRequest{
				Method: "tools/call",
				Params: mustMarshal(t, map[string]any{
					"name":      "counter",
					"arguments": map[string]any{},
				}),
				ID: id,
			})

			if resp != nil && resp.Error == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, numRequests, successCount, "all concurrent requests should succeed")
}

// Mock transport for MCPClient tests.
type mockTransport struct {
	sendFunc func(ctx context.Context, req *MCPRequest) (*MCPResponse, error)
	closed   bool
}

func (m *mockTransport) Send(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, req)
	}
	return &MCPResponse{}, nil
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

// TestMCPClientListTools tests client tool listing.
func TestMCPClientListTools(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				assert.Equal(t, "tools/list", req.Method)

				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "test-tool",
								"description": "A test tool",
								"inputSchema": map[string]any{
									"type": "object",
								},
							},
						},
					},
				}, nil
			},
		}

		client := NewMCPClient("http://localhost:8080", transport)
		tools, err := client.ListTools(context.Background())

		require.NoError(t, err)
		require.Len(t, tools, 1)
		assert.Equal(t, "test-tool", tools[0].Name)
		assert.Equal(t, "A test tool", tools[0].Description)
	})

	t.Run("transport error", func(t *testing.T) {
		t.Parallel()

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return nil, errors.New("connection failed")
			},
		}

		client := NewMCPClient("http://localhost:8080", transport)
		tools, err := client.ListTools(context.Background())

		require.Error(t, err)
		assert.Nil(t, tools)
		assert.Contains(t, err.Error(), "connection failed")
	})

	t.Run("MCP error response", func(t *testing.T) {
		t.Parallel()

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Error: &Error{
						Code:    -32601,
						Message: "method not found",
					},
				}, nil
			},
		}

		client := NewMCPClient("http://localhost:8080", transport)
		tools, err := client.ListTools(context.Background())

		require.Error(t, err)
		assert.Nil(t, tools)
		assert.Contains(t, err.Error(), "MCP error")
	})
}

// TestMCPClientCallTool tests client tool invocation.
func TestMCPClientCallTool(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				assert.Equal(t, "tools/call", req.Method)

				return &MCPResponse{
					Result: map[string]any{
						"content": []map[string]any{
							{
								"type": "text",
								"text": "result",
							},
						},
					},
				}, nil
			},
		}

		client := NewMCPClient("http://localhost:8080", transport)
		result, err := client.CallTool(context.Background(), "test", map[string]any{"key": "value"})

		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("error response", func(t *testing.T) {
		t.Parallel()

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Error: &Error{
						Code:    -32000,
						Message: "execution failed",
					},
				}, nil
			},
		}

		client := NewMCPClient("http://localhost:8080", transport)
		result, err := client.CallTool(context.Background(), "test", map[string]any{})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "execution failed")
	})
}

// TestMCPClientClose tests client cleanup.
func TestMCPClientClose(t *testing.T) {
	t.Parallel()

	transport := &mockTransport{}
	client := NewMCPClient("http://localhost:8080", transport)

	err := client.Close()

	require.NoError(t, err)
	assert.True(t, transport.closed)
}

// TestToolDiscovery tests tool discovery from multiple servers.
func TestToolDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("discover from multiple servers", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery()

		// Add first server
		transport1 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "tool1",
								"description": "First tool",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		client1 := NewMCPClient("http://server1", transport1)
		discovery.AddServer(client1)

		// Add second server
		transport2 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "tool2",
								"description": "Second tool",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		client2 := NewMCPClient("http://server2", transport2)
		discovery.AddServer(client2)

		// Discover tools
		err := discovery.Discover(context.Background())
		require.NoError(t, err)

		tools := discovery.GetTools()
		assert.Len(t, tools, 2)
	})

	t.Run("skip failed servers", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery()

		// Working server
		transport1 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "working-tool",
								"description": "Works",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://server1", transport1))

		// Failed server
		transport2 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return nil, errors.New("server down")
			},
		}
		discovery.AddServer(NewMCPClient("http://server2", transport2))

		// Should succeed despite one failure
		err := discovery.Discover(context.Background())
		require.NoError(t, err)

		tools := discovery.GetTools()
		assert.Len(t, tools, 1)
		assert.Equal(t, "working-tool", tools[0].Name)
	})

	t.Run("close all clients", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery()

		transport := &mockTransport{}
		client := NewMCPClient("http://server", transport)
		discovery.AddServer(client)

		err := discovery.Close()
		require.NoError(t, err)
		assert.True(t, transport.closed)
	})

	t.Run("conflict error by default", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery() // Default: ConflictError

		// Both servers provide "shared-tool"
		for _, serverURL := range []string{"http://server1", "http://server2"} {
			transport := &mockTransport{
				sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
					return &MCPResponse{
						Result: map[string]any{
							"tools": []any{
								map[string]any{
									"name":        "shared-tool",
									"description": "Conflict",
									"inputSchema": map[string]any{"type": "object"},
								},
							},
						},
					}, nil
				},
			}
			discovery.AddServer(NewMCPClient(serverURL, transport))
		}

		err := discovery.Discover(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolConflict)
		assert.Contains(t, err.Error(), "shared-tool")
		assert.Contains(t, err.Error(), "server1")
		assert.Contains(t, err.Error(), "server2")
	})

	t.Run("conflict first wins", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery(WithConflictStrategy(ConflictFirstWins))

		// Server 1
		transport1 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "shared-tool",
								"description": "From server 1",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://server1", transport1))

		// Server 2
		transport2 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "shared-tool",
								"description": "From server 2",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://server2", transport2))

		err := discovery.Discover(context.Background())
		require.NoError(t, err)

		tools := discovery.GetTools()
		require.Len(t, tools, 1)
		assert.Equal(t, "From server 1", tools[0].Description)
		assert.Equal(t, "http://server1", discovery.GetToolSource("shared-tool"))
	})

	t.Run("conflict last wins", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery(WithConflictStrategy(ConflictLastWins))

		// Server 1
		transport1 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "shared-tool",
								"description": "From server 1",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://server1", transport1))

		// Server 2
		transport2 := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "shared-tool",
								"description": "From server 2",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://server2", transport2))

		err := discovery.Discover(context.Background())
		require.NoError(t, err)

		tools := discovery.GetTools()
		require.Len(t, tools, 1)
		assert.Equal(t, "From server 2", tools[0].Description)
		assert.Equal(t, "http://server2", discovery.GetToolSource("shared-tool"))
	})

	t.Run("get tool source", func(t *testing.T) {
		t.Parallel()

		discovery := NewToolDiscovery(WithConflictStrategy(ConflictFirstWins))

		transport := &mockTransport{
			sendFunc: func(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
				return &MCPResponse{
					Result: map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "my-tool",
								"description": "Test",
								"inputSchema": map[string]any{"type": "object"},
							},
						},
					},
				}, nil
			},
		}
		discovery.AddServer(NewMCPClient("http://origin-server", transport))

		err := discovery.Discover(context.Background())
		require.NoError(t, err)

		assert.Equal(t, "http://origin-server", discovery.GetToolSource("my-tool"))
		assert.Equal(t, "", discovery.GetToolSource("nonexistent"))
	})
}

// Helper function to marshal JSON (panics on error, for test setup).
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
