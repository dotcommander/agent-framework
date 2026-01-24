package agent

import (
	"github.com/dotcommander/agent-framework/tools/schema"
)

// SchemaFor generates a JSON Schema from a Go type using reflection.
// Supports structs, slices, maps, and primitive types.
//
// Example:
//
//	type Response struct {
//	    Name  string   `json:"name"`
//	    Count int      `json:"count"`
//	    Tags  []string `json:"tags"`
//	}
//	schema := agent.SchemaFor[Response]()
func SchemaFor[T any]() map[string]any {
	return schema.Generate[T]()
}
