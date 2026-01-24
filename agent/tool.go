package agent

import (
	"context"

	"github.com/dotcommander/agent-framework/tools"
)

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
func Tool[I, O any](name, description string, fn func(context.Context, I) (O, error)) *tools.Tool {
	return tools.Define(name, description, fn)
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
func SimpleTool(name, description string, fn func(string) (string, error)) *tools.Tool {
	return tools.DefineSimple(name, description, fn)
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
func AsyncTool(name, description string, fn func(context.Context, string) error) *tools.Tool {
	return tools.DefineAsync(name, description, fn)
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
func ToolWithSchema(name, description string, schema map[string]any, fn tools.Handler) *tools.Tool {
	return tools.DefineWithSchema(name, description, schema, fn)
}
