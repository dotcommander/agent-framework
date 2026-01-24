package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dotcommander/agent-framework/tools"
)

// ToolDef represents a tool definition.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
	Handler     func(ctx context.Context, input map[string]any) (any, error)
}

// toAppTool converts ToolDef to a tools.Tool for the app package.
func (t ToolDef) toAppTool() *tools.Tool {
	return &tools.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.Schema,
		Handler:     t.Handler,
	}
}

// Tool creates a type-safe tool with schema inferred from the input type.
// The input type's JSON schema is automatically generated from struct tags.
//
// Example:
//
//	type SearchInput struct {
//	    Query string `json:"query" desc:"Search query" required:"true"`
//	    Limit int    `json:"limit" desc:"Max results" max:"100"`
//	}
//
//	type SearchOutput struct {
//	    Results []string `json:"results"`
//	}
//
//	searchTool := agent.Tool("search", "Search the web",
//	    func(ctx context.Context, in SearchInput) (SearchOutput, error) {
//	        results := doSearch(in.Query, in.Limit)
//	        return SearchOutput{Results: results}, nil
//	    },
//	)
func Tool[I, O any](name, description string, fn func(context.Context, I) (O, error)) ToolDef {
	schema := SchemaFor[I]()

	return ToolDef{
		Name:        name,
		Description: description,
		Schema:      schema,
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			// Marshal input to JSON then unmarshal to typed struct
			data, err := json.Marshal(input)
			if err != nil {
				return nil, fmt.Errorf("marshal input: %w", err)
			}

			var typedInput I
			if err := json.Unmarshal(data, &typedInput); err != nil {
				return nil, fmt.Errorf("unmarshal to %T: %w", typedInput, err)
			}

			// Call the handler
			result, err := fn(ctx, typedInput)
			if err != nil {
				return nil, err
			}

			return result, nil
		},
	}
}

// SimpleTool creates a tool with a single string input and output.
// Perfect for simple transformations or queries.
//
// Example:
//
//	reverseTool := agent.SimpleTool("reverse", "Reverse a string",
//	    func(s string) (string, error) {
//	        runes := []rune(s)
//	        for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
//	            runes[i], runes[j] = runes[j], runes[i]
//	        }
//	        return string(runes), nil
//	    },
//	)
func SimpleTool(name, description string, fn func(string) (string, error)) ToolDef {
	return ToolDef{
		Name:        name,
		Description: description,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Input value",
				},
			},
			"required": []string{"input"},
		},
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			s, ok := input["input"].(string)
			if !ok {
				return nil, fmt.Errorf("input must be a string")
			}

			result, err := fn(s)
			if err != nil {
				return nil, err
			}

			return map[string]any{"output": result}, nil
		},
	}
}

// AsyncTool creates a tool that runs asynchronously and returns immediately.
// The handler runs in a goroutine and results can be retrieved later.
//
// Example:
//
//	downloadTool := agent.AsyncTool("download", "Download a file",
//	    func(ctx context.Context, url string) error {
//	        return downloadFile(url)
//	    },
//	)
func AsyncTool(name, description string, fn func(context.Context, string) error) ToolDef {
	return ToolDef{
		Name:        name,
		Description: description,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Input value",
				},
			},
			"required": []string{"input"},
		},
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			s, ok := input["input"].(string)
			if !ok {
				return nil, fmt.Errorf("input must be a string")
			}

			// Run in goroutine
			go func() {
				_ = fn(ctx, s)
			}()

			return map[string]any{"status": "started"}, nil
		},
	}
}

// ToolWithSchema creates a tool with an explicit schema.
// Use when you need precise control over the schema.
//
// Example:
//
//	calcTool := agent.ToolWithSchema("calc", "Calculate expression",
//	    map[string]any{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "expression": map[string]any{
//	                "type": "string",
//	                "pattern": "^[0-9+\\-*/()\\s]+$",
//	            },
//	        },
//	        "required": []string{"expression"},
//	    },
//	    func(ctx context.Context, input map[string]any) (any, error) {
//	        expr := input["expression"].(string)
//	        return evaluate(expr), nil
//	    },
//	)
func ToolWithSchema(name, description string, schema map[string]any, fn func(context.Context, map[string]any) (any, error)) ToolDef {
	return ToolDef{
		Name:        name,
		Description: description,
		Schema:      schema,
		Handler:     fn,
	}
}
