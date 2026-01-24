// Package main demonstrates the agent convenience layer.
//
// This example shows how the agent package reduces boilerplate
// from 50 lines to just a few lines.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/dotcommander/agent-framework/agent"
)

func main() {
	fmt.Println("=== Agent Framework Quick Start Examples ===")
	fmt.Println()

	// Example 1: One-liner (simplest possible)
	showOneLiner()

	// Example 2: Fluent builder
	showFluentBuilder()

	// Example 3: Type-safe tools
	showTypeSafeTools()

	// Example 4: Typed responses
	showTypedResponses()

	// Example 5: Hooks and permissions
	showHooksAndPermissions()

	// Example 6: Response helpers
	showResponseHelpers()
}

// showOneLiner demonstrates the simplest possible agent.
func showOneLiner() {
	fmt.Println("--- Example 1: One-Liner ---")
	fmt.Println()

	fmt.Println(`  // The simplest possible agent (1 line):
  agent.Run("You are a helpful coding assistant.")

  // Single query (scripts, one-shot tasks):
  response, err := agent.Query(ctx, "What is 2+2?")

  // With options:
  response, err := agent.Query(ctx, "Explain goroutines",
      agent.WithModel("opus"),
      agent.WithMaxTurns(5),
  )`)
	fmt.Println()
}

// showFluentBuilder demonstrates the builder pattern.
func showFluentBuilder() {
	fmt.Println("--- Example 2: Fluent Builder ---")
	fmt.Println()

	fmt.Println(`  // Fluent builder for more control:
  agent.New("code-reviewer").
      Model("opus").              // Short names: "opus", "sonnet", "haiku"
      System("You review code.").
      Budget(5.00).               // $5 USD limit
      MaxTurns(20).               // 20 turns max
      WorkDir("/path/to/project").
      OnlyTools("Read", "Grep"). // Whitelist tools
      Run()`)
	fmt.Println()

	// Show what a builder looks like
	b := agent.New("demo").
		Model("sonnet").
		System("You are a demo agent.").
		MaxTurns(10)

	fmt.Printf("  Builder: %s\n", b)
	fmt.Println()
}

// showTypeSafeTools demonstrates tool creation.
func showTypeSafeTools() {
	fmt.Println("--- Example 3: Type-Safe Tools ---")
	fmt.Println()

	// Define typed input/output
	type SearchInput struct {
		Query string `json:"query" desc:"Search query" required:"true"`
		Limit int    `json:"limit" desc:"Max results" max:"100"`
	}

	type SearchOutput struct {
		Results []string `json:"results"`
		Count   int      `json:"count"`
	}

	// Create tool with inferred schema
	searchTool := agent.Tool("search", "Search the codebase",
		func(ctx context.Context, in SearchInput) (SearchOutput, error) {
			// Simulate search
			results := []string{
				fmt.Sprintf("Result 1 for: %s", in.Query),
				fmt.Sprintf("Result 2 for: %s", in.Query),
			}
			if in.Limit > 0 && len(results) > in.Limit {
				results = results[:in.Limit]
			}
			return SearchOutput{
				Results: results,
				Count:   len(results),
			}, nil
		},
	)

	fmt.Printf(`  // Type-safe tool with schema inference:
  type SearchInput struct {
      Query string %cjson:"query" desc:"Search query" required:"true"%c
      Limit int    %cjson:"limit" desc:"Max results" max:"100"%c
  }

  searchTool := agent.Tool("search", "Search the codebase",
      func(ctx context.Context, in SearchInput) (SearchOutput, error) {
          // Your logic here
      },
  )

  // Tool schema (auto-generated):
`, '`', '`', '`', '`')

	schema := agent.SchemaFor[SearchInput]()
	fmt.Printf("  Schema: %+v\n", schema)
	fmt.Printf("  Tool: %s - %s\n", searchTool.Name, searchTool.Description)
	fmt.Println()

	// Simple tool example
	fmt.Println(`  // Even simpler - single string in/out:
  reverseTool := agent.SimpleTool("reverse", "Reverse a string",
      func(s string) (string, error) {
          // ...
      },
  )`)
	fmt.Println()
}

// showTypedResponses demonstrates QueryAs.
func showTypedResponses() {
	fmt.Println("--- Example 4: Typed Responses ---")
	fmt.Println()

	type CodeReview struct {
		Summary string   `json:"summary"`
		Issues  []string `json:"issues"`
		Score   int      `json:"score"`
	}

	fmt.Printf(`  // Get typed responses (no manual JSON parsing):
  type CodeReview struct {
      Summary string   %cjson:"summary"%c
      Issues  []string %cjson:"issues"%c
      Score   int      %cjson:"score"%c
  }

  review, err := agent.QueryAs[CodeReview](ctx, "Review this code...")
  if err != nil {
      log.Fatal(err)
  }
  fmt.Printf("Score: %%d, Issues: %%d\n", review.Score, len(review.Issues))

  // Schema auto-generated:
`, '`', '`', '`', '`', '`', '`')

	schema := agent.SchemaFor[CodeReview]()
	fmt.Printf("  Schema: %+v\n", schema)
	fmt.Println()
}

// showHooksAndPermissions demonstrates lifecycle hooks.
func showHooksAndPermissions() {
	fmt.Println("--- Example 5: Hooks and Permissions ---")
	fmt.Println()

	// Use os.Stdout to avoid go vet warning about Printf directives in example code
	_, _ = os.Stdout.WriteString(`  // Lifecycle hooks:
  agent.New("secure").
      OnPreToolUse(func(tool string, input map[string]any) bool {
          log.Printf("Tool called: %s", tool)
          if tool == "Bash" {
              return false // block Bash
          }
          return true
      }).
      OnPostToolUse(func(tool string, result any) {
          log.Printf("Tool completed: %s", tool)
      }).
      OnSessionStart(func(sessionID string) {
          log.Printf("Session: %s", sessionID)
      }).
      Run()

  // Runtime permission approval:
  agent.New("careful").
      RequireApproval(func(tool string, input map[string]any) agent.Approval {
          if tool == "Write" {
              // Could prompt user here
              return agent.Deny("Write access not allowed")
          }
          return agent.Allow()
      }).
      Run()
`)
	fmt.Println()
}

// showResponseHelpers demonstrates the Response struct.
func showResponseHelpers() {
	fmt.Println("--- Example 6: Response Helpers ---")
	fmt.Println()

	_, _ = os.Stdout.WriteString(`  // Get rich response with metadata:
  resp, err := agent.QueryResponse(ctx, "What is 2+2?")
  if err != nil {
      log.Fatal(err)
  }

  // String content
  fmt.Println(resp.Content)
  fmt.Println(resp.String()) // Same as Content

  // Parse JSON response
  var data MyStruct
  if err := resp.JSON(&data); err != nil {
      log.Fatal(err)
  }

  // Save to file
  resp.SaveTo("output.txt")

  // Check content
  if resp.Contains("error") {
      // handle error
  }

  // Line-by-line processing
  for _, line := range resp.Lines() {
      fmt.Println(line)
  }

  // Check for tool calls
  if resp.HasToolCalls() {
      for _, tc := range resp.ToolCalls {
          fmt.Printf("Tool: %s\n", tc.Name)
      }
  }
`)
	fmt.Println()
}

func init() {
	// Suppress unused import warning
	_ = log.Println
	_ = context.Background
}
