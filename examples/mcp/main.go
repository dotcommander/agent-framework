// Package main demonstrates MCP (Model Context Protocol) server creation.
//
// MCP servers expose tools and resources to AI models through a standardized
// protocol. This example shows how to create a server, register tools, and
// handle requests.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/agent/tools"
)

func main() {
	fmt.Println("=== MCP Server Demo ===")
	fmt.Println()

	// Create an MCP server with options
	server := tools.NewMCPServer("demo-server",
		tools.WithMCPDescription("A demonstration MCP server with utility tools"),
		tools.WithMCPVersion("1.0.0"),
	)

	// Register tools
	err := server.RegisterTools(
		// Calculator tool
		tools.NewTool(
			"calculator",
			"Perform basic arithmetic operations",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{
						"type":        "string",
						"enum":        []string{"add", "subtract", "multiply", "divide"},
						"description": "The arithmetic operation to perform",
					},
					"a": map[string]any{
						"type":        "number",
						"description": "First operand",
					},
					"b": map[string]any{
						"type":        "number",
						"description": "Second operand",
					},
				},
				"required": []string{"operation", "a", "b"},
			},
			func(ctx context.Context, input map[string]any) (any, error) {
				op := input["operation"].(string)
				a := input["a"].(float64)
				b := input["b"].(float64)

				var result float64
				switch op {
				case "add":
					result = a + b
				case "subtract":
					result = a - b
				case "multiply":
					result = a * b
				case "divide":
					if b == 0 {
						return nil, fmt.Errorf("division by zero")
					}
					result = a / b
				default:
					return nil, fmt.Errorf("unknown operation: %s", op)
				}

				return map[string]any{"result": result}, nil
			},
		),

		// String manipulation tool
		tools.NewTool(
			"string_transform",
			"Transform strings in various ways",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The text to transform",
					},
					"transform": map[string]any{
						"type":        "string",
						"enum":        []string{"upper", "lower", "reverse", "length"},
						"description": "The transformation to apply",
					},
				},
				"required": []string{"text", "transform"},
			},
			func(ctx context.Context, input map[string]any) (any, error) {
				text := input["text"].(string)
				transform := input["transform"].(string)

				switch transform {
				case "upper":
					return strings.ToUpper(text), nil
				case "lower":
					return strings.ToLower(text), nil
				case "reverse":
					runes := []rune(text)
					for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
						runes[i], runes[j] = runes[j], runes[i]
					}
					return string(runes), nil
				case "length":
					return len(text), nil
				default:
					return nil, fmt.Errorf("unknown transform: %s", transform)
				}
			},
		),

		// Time tool
		tools.NewTool(
			"time_info",
			"Get current time information",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{
						"type":        "string",
						"enum":        []string{"unix", "iso", "human"},
						"description": "Output format for the time",
					},
					"timezone": map[string]any{
						"type":        "string",
						"description": "Timezone (e.g., 'UTC', 'America/New_York')",
						"default":     "UTC",
					},
				},
				"required": []string{"format"},
			},
			func(ctx context.Context, input map[string]any) (any, error) {
				format := input["format"].(string)
				tz := "UTC"
				if tzInput, ok := input["timezone"].(string); ok {
					tz = tzInput
				}

				loc, err := time.LoadLocation(tz)
				if err != nil {
					loc = time.UTC
				}
				now := time.Now().In(loc)

				switch format {
				case "unix":
					return now.Unix(), nil
				case "iso":
					return now.Format(time.RFC3339), nil
				case "human":
					return now.Format("Monday, January 2, 2006 3:04 PM"), nil
				default:
					return nil, fmt.Errorf("unknown format: %s", format)
				}
			},
		),
	)
	if err != nil {
		fmt.Printf("Error registering tools: %v\n", err)
		return
	}

	// Register resources
	server.RegisterResource(&tools.Resource{
		URI:         "config://app/settings",
		Name:        "Application Settings",
		Description: "Current application configuration",
		MimeType:    "application/json",
		Metadata: map[string]any{
			"readonly": true,
			"version":  "1.0",
		},
	})

	server.RegisterResource(&tools.Resource{
		URI:         "docs://api/reference",
		Name:        "API Reference",
		Description: "API documentation and examples",
		MimeType:    "text/markdown",
	})

	// Display server info
	info := server.GetServerInfo()
	fmt.Printf("Server: %s v%s\n", info.Name, info.Version)
	fmt.Printf("Description: %s\n", info.Description)
	fmt.Printf("Capabilities: %v\n\n", info.Capabilities)

	// List registered tools
	fmt.Println("Registered Tools:")
	for _, tool := range server.ListTools() {
		fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
	}
	fmt.Println()

	// List registered resources
	fmt.Println("Registered Resources:")
	for _, res := range server.ListResources() {
		fmt.Printf("  - %s (%s): %s\n", res.Name, res.URI, res.Description)
	}
	fmt.Println()

	// Simulate handling MCP requests
	ctx := context.Background()

	fmt.Println("=== Handling MCP Requests ===")
	fmt.Println()

	// Initialize request
	initReq := &tools.MCPRequest{
		Method: "initialize",
		ID:     1,
	}
	initResp := server.HandleRequest(ctx, initReq)
	printResponse("Initialize", initResp)

	// List tools request
	listToolsReq := &tools.MCPRequest{
		Method: "tools/list",
		ID:     2,
	}
	listResp := server.HandleRequest(ctx, listToolsReq)
	printResponse("List Tools", listResp)

	// Call calculator tool
	calcParams, _ := json.Marshal(map[string]any{
		"name": "calculator",
		"arguments": map[string]any{
			"operation": "multiply",
			"a":         7,
			"b":         6,
		},
	})
	calcReq := &tools.MCPRequest{
		Method: "tools/call",
		Params: calcParams,
		ID:     3,
	}
	calcResp := server.HandleRequest(ctx, calcReq)
	printResponse("Call Calculator (7 * 6)", calcResp)

	// Call string transform tool
	strParams, _ := json.Marshal(map[string]any{
		"name": "string_transform",
		"arguments": map[string]any{
			"text":      "Hello, MCP!",
			"transform": "reverse",
		},
	})
	strReq := &tools.MCPRequest{
		Method: "tools/call",
		Params: strParams,
		ID:     4,
	}
	strResp := server.HandleRequest(ctx, strReq)
	printResponse("Call String Transform (reverse 'Hello, MCP!')", strResp)

	// Call time tool
	timeParams, _ := json.Marshal(map[string]any{
		"name": "time_info",
		"arguments": map[string]any{
			"format": "human",
		},
	})
	timeReq := &tools.MCPRequest{
		Method: "tools/call",
		Params: timeParams,
		ID:     5,
	}
	timeResp := server.HandleRequest(ctx, timeReq)
	printResponse("Call Time Info (human format)", timeResp)

	// List resources
	listResReq := &tools.MCPRequest{
		Method: "resources/list",
		ID:     6,
	}
	listResResp := server.HandleRequest(ctx, listResReq)
	printResponse("List Resources", listResResp)

	// Handle unknown method
	unknownReq := &tools.MCPRequest{
		Method: "unknown/method",
		ID:     7,
	}
	unknownResp := server.HandleRequest(ctx, unknownReq)
	printResponse("Unknown Method", unknownResp)
}

func printResponse(label string, resp *tools.MCPResponse) {
	fmt.Printf("%s (ID: %v):\n", label, resp.ID)
	if resp.Error != nil {
		fmt.Printf("  Error: %s (code: %d)\n", resp.Error.Message, resp.Error.Code)
	} else {
		result, _ := json.MarshalIndent(resp.Result, "  ", "  ")
		fmt.Printf("  Result: %s\n", string(result))
	}
	fmt.Println()
}
