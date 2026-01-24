# Agent Framework

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Agent framework for building Claude-powered CLI tools using [agent-sdk-go](https://github.com/dotcommander/agent-sdk-go).

## Features

### Core Capabilities

- **Multi-provider support** - Anthropic (via agent-sdk-go), Z.AI, Synthetic (for testing)
- **Automatic input detection** - URLs, files, glob patterns, plain text
- **Flexible output formatting** - JSON, Markdown, plain text with code generation support
- **Type-safe tool registration** - Generic handlers with automatic type conversion
- **CLI scaffolding** - Built on Cobra for consistent CLI patterns
- **Composable design** - Import packages and wire together custom applications

### Key Features

| Feature | Description |
|---------|-------------|
| **Agent Loop** | Gather → decide → act → verify pattern with iteration limits, token budgets, and verification thresholds |
| **Subagent Spawning** | Parallel execution with isolated contexts via errgroup and result aggregation |
| **MCP Integration** | Full Model Context Protocol server/client for tool discovery and invocation |
| **Rules-Based Validation** | Composable validators: Required, Regex, Enum, Range, Length, Custom |
| **Visual Verification** | Screenshot capture, baseline comparison, pixel diffing, AI-powered analysis |
| **File System State** | Track files, create snapshots, detect changes, rollback |
| **Hierarchical Evaluation** | Multi-level checks (syntax, semantic, behavioral, visual) with weighted scoring |
| **Semantic Search** | Embedding-based code search with chunking and hybrid keyword/vector search |
| **Code Generation Output** | Structured diffs, code blocks, change tracking with markdown/JSON formatting |
| **Context Compaction** | Token-aware summarization for managing long conversations |
| **Graceful Shutdown** | Shutdown hooks with LIFO execution and timeout handling |
| **Resilience Patterns** | Circuit breaker and retry with exponential backoff |

## Installation

```bash
# Clone both repositories as siblings (uses replace directive)
git clone https://github.com/dotcommander/agent.git
git clone https://github.com/dotcommander/agent-sdk-go.git

# Build
cd agent && go build ./...
```

Or add to your project:

```bash
go get github.com/dotcommander/agent
```

## Quick Start

### One-Liner (Simplest)

```go
package main

import "github.com/dotcommander/agent-framework/agent"

func main() {
    // Start an interactive agent in 1 line
    agent.Run("You are a helpful coding assistant.")
}
```

### Single Query

```go
response, err := agent.Query(ctx, "What is 2+2?")
```

### Typed Responses

```go
type CodeReview struct {
    Summary string   `json:"summary"`
    Issues  []string `json:"issues"`
    Score   int      `json:"score"`
}

review, err := agent.QueryAs[CodeReview](ctx, "Review this code...")
fmt.Printf("Score: %d\n", review.Score)
```

### Fluent Builder

```go
agent.New("code-reviewer").
    Model("opus").              // Short: "opus", "sonnet", "haiku"
    System("You review code.").
    Budget(5.00).               // $5 USD limit
    MaxTurns(20).
    OnPreToolUse(func(tool string, _ map[string]any) bool {
        return tool != "Bash"   // Block Bash
    }).
    Run()
```

### Type-Safe Tools

```go
type SearchInput struct {
    Query string `json:"query" desc:"Search query"`
    Limit int    `json:"limit" max:"100"`
}

searchTool := agent.Tool("search", "Search codebase",
    func(ctx context.Context, in SearchInput) ([]string, error) {
        return doSearch(in.Query, in.Limit), nil
    },
)

agent.New("researcher").Tool(searchTool).Run()
```

### Full Control (app package)

```go
package main

import (
    "log"
    "github.com/dotcommander/agent-framework/app"
)

func main() {
    application := app.New("myapp", "1.0.0",
        app.WithSystemPrompt("You are a helpful assistant."),
        app.WithModel("claude-sonnet-4-20250514"),
    )

    if err := application.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### With Custom Tools

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dotcommander/agent/app"
    "github.com/dotcommander/agent/tools"
)

func main() {
    // Define a type-safe tool
    greetTool := tools.TypedTool(
        "greet",
        "Greets a person by name",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "name": map[string]any{
                    "type":        "string",
                    "description": "The name of the person to greet",
                },
            },
            "required": []string{"name"},
        },
        func(ctx context.Context, input struct {
            Name string `json:"name"`
        }) (struct {
            Message string `json:"message"`
        }, error) {
            return struct {
                Message string `json:"message"`
            }{
                Message: fmt.Sprintf("Hello, %s!", input.Name),
            }, nil
        },
    )

    application := app.New("myapp", "1.0.0",
        app.WithSystemPrompt("You are a helpful assistant."),
        app.WithTool(greetTool),
    )

    if err := application.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### Agent Loop Example

```go
package main

import (
    "context"
    "github.com/dotcommander/agent/app"
)

func main() {
    // Create a simple loop with gather -> decide -> act -> verify pattern
    loop := app.NewSimpleLoop(
        app.WithGatherFunc(func(ctx context.Context, state *app.LoopState) (*app.LoopContext, error) {
            // Gather context for this iteration
            return &app.LoopContext{State: make(map[string]any)}, nil
        }),
        app.WithDecideFunc(func(ctx context.Context, state *app.LoopState) (*app.Action, error) {
            // Decide what action to take
            return &app.Action{Type: "tool_call", ToolName: "search"}, nil
        }),
        app.WithVerifyFunc(func(ctx context.Context, state *app.LoopState) (*app.Feedback, error) {
            // Verify the result
            return &app.Feedback{Valid: true, Score: 0.95}, nil
        }),
    )

    runner := app.NewLoopRunner(loop, &app.LoopConfig{
        MaxIterations: 10,
        MaxTokens:     50000,
    })

    state, err := runner.Run(context.Background())
    // ...
}
```

## Architecture

The framework is organized into focused packages following Single Responsibility:

### `/app` - Application Composition & Agent Loop

Main application structure with functional options pattern, plus the core agent loop and subagent spawning.

```go
// Application setup
app := app.New("myapp", "1.0.0",
    app.WithSystemPrompt("..."),
    app.WithModel("..."),
    app.WithTool(myTool),
)

// Subagent management
manager := app.NewSubagentManager(app.DefaultSubagentConfig(), executor)
agent := manager.Spawn("analyzer", "Analyze this code",
    app.WithSubagentPrompt("You are a code analyzer."),
)
results, _ := manager.RunAll(ctx)
```

### `/client` - AI Client Wrapper & Compaction

Clean interface around the agent-sdk-go client with multi-provider support and context compaction.

```go
client, err := client.New(ctx, claude.WithModel("claude-sonnet-4-20250514"))
response, err := client.Query(ctx, "What is the capital of France?")

// Context compaction for long conversations
compactor := client.NewSimpleCompactor(nil, summarizer)
if compactor.ShouldCompact(messages, 100000) {
    messages, _ = compactor.Compact(ctx, messages)
}
```

### `/cli` - CLI Scaffolding

Cobra command builder with standard flags.

```go
cmd := cli.NewCommand("query", "Send a query", func(cmd *cobra.Command, args []string) error {
    // Handle command
    return nil
})
```

### `/config` - Configuration Types

Configuration types and functional options.

```go
cfg := config.NewConfig(
    config.WithModel("claude-sonnet-4-20250514"),
    config.WithTimeout(60 * time.Second),
)
```

### `/input` - Input Processing

Automatic detection and processing of URLs, files, globs, and text.

```go
registry := input.NewRegistry()
content, err := registry.Process(ctx, "https://example.com")
```

### `/output` - Output Formatting & Code Generation

Pluggable formatters for JSON, Markdown, and text, plus structured code output.

```go
// Basic formatting
dispatcher := output.NewDispatcher()
dispatcher.RegisterFormatter(output.NewJSONFormatter(true))
err := dispatcher.Write(ctx, result, output.FormatJSON, "output.json")

// Code generation
gen := output.NewCodeGenerator()
gen.AddModify("main.go", oldContent, newContent, "Fix bug in handler")
output := gen.Build()
markdown := output.FormatMarkdown()
```

### `/tools` - Tool Registration & MCP

Type-safe tool registration with generic handlers and MCP server/client.

```go
// Type-safe tools
tool := tools.TypedTool(
    "calculate",
    "Performs calculations",
    schema,
    func(ctx context.Context, input CalculateInput) (CalculateOutput, error) {
        // Handle tool invocation
    },
)

// MCP server
server := tools.NewMCPServer("my-server", tools.WithMCPVersion("1.0.0"))
server.RegisterTool(myTool)
response := server.HandleRequest(ctx, request)
```

### `/validation` - Rules-Based Validation

Composable validation rules for structured output validation.

```go
rules := validation.NewRuleSet("user",
    validation.Required("name"),
    validation.Regex("email", `^[\w-\.]+@[\w-]+\.\w+$`, "invalid email"),
    validation.Enum("role", "admin", "user", "guest"),
    validation.Range("age", 0, 150),
)

validator := validation.NewValidator(rules)
result := validator.Validate(data)
```

### `/verification` - Visual & Hierarchical Evaluation

Screenshot comparison and multi-level verification with rubrics.

```go
// Visual verification
verifier := verification.NewVisualVerifier(config, capturer, comparator)
result, _ := verifier.Verify(ctx, "http://localhost:8080", baseline)

// Hierarchical evaluation
evaluator := verification.NewEvaluator(
    verification.WithThreshold(verification.LevelSyntax, 1.0),
    verification.WithThreshold(verification.LevelBehavioral, 0.8),
)
evaluator.AddChecks(
    verification.CommonChecks{}.BuildCheck(buildFn),
    verification.CommonChecks{}.TestCheck(testFn),
)
result, _ := evaluator.Evaluate(ctx, target)
```

### `/state` - File System State Tracking

Track file changes, create snapshots, and rollback modifications.

```go
store := state.NewFileSystemStore("/project")
store.TrackDir("/project/src", "*.go")

// Create snapshot before changes
snapshot := store.CreateSnapshot("before refactoring")

// Detect changes
changes, _ := store.DetectChanges()

// Rollback if needed
store.Rollback(snapshot.ID)
```

### `/search` - Semantic Search

Embedding-based code search with chunking and hybrid search.

```go
index := search.NewSemanticIndex(embedder, search.NewFixedSizeChunker(512, 64))
index.Add(ctx, &search.Document{ID: "main.go", Content: code})

// Semantic search
results, _ := index.Search(ctx, "error handling pattern", 10)

// Hybrid search (semantic + keyword)
results, _ := index.HybridSearch(ctx, "parse JSON", 10, 0.3)
```

## Examples

The `examples/` directory contains runnable examples for each major feature:

| Example | Description |
|---------|-------------|
| `loop/` | Agent loop pattern (gather → decide → act → verify) |
| `subagents/` | Parallel subagent execution with result aggregation |
| `mcp/` | MCP server/client implementation |
| `validation/` | Rules-based validation with composable rules |
| `codegen/` | Structured code generation output |
| `search/` | Semantic search with embeddings |
| `state/` | File system state tracking and rollback |
| `evaluation/` | Hierarchical verification with rubrics |
| `advanced/` | Advanced patterns combining multiple features |

```bash
# Run an example
cd examples/loop && go run main.go
```

## Building

```bash
# Build all packages
go build ./...

# Build example
go build -o agent-example ./cmd/agent

# Install to PATH
ln -sf "$(pwd)/agent-example" ~/go/bin/agent-example
```

## Development Setup

This framework uses a `replace` directive for local development with agent-sdk-go:

```go
// go.mod
replace github.com/dotcommander/agent-sdk-go => ../agent-sdk-go
```

Clone both repositories as siblings for the replace directive to resolve.

## Design Principles

1. **Composable** - Import packages and wire together, not a monolithic framework
2. **Type-safe** - Generic handlers provide compile-time type checking
3. **Idiomatic Go** - Functional options, interfaces, clear error handling
4. **Focused packages** - Each package has a single responsibility
5. **Extensible** - Register custom processors, formatters, tools, and validators

## Providers

### Anthropic (Default)

Uses the agent-sdk-go library to communicate with Claude CLI.

```go
app.WithProvider("anthropic")
```

### Z.AI (Placeholder)

```go
app.WithProvider("zai")
```

### Synthetic (Placeholder)

For testing without API calls.

```go
app.WithProvider("synthetic")
```

## Common Patterns

These patterns address common DX needs. Many developers miss these existing features.

### Retry with Exponential Backoff

The client supports automatic retry with configurable backoff:

```go
import "github.com/dotcommander/agent-framework/client"

// Use defaults: 3 retries, 500ms initial, 2x backoff, 0.5 jitter
c, _ := client.New(ctx, sdkOpts,
    client.WithRetry(nil),
)

// Or customize
c, _ := client.New(ctx, sdkOpts,
    client.WithRetry(&client.RetryConfig{
        MaxRetries:          5,
        InitialInterval:     time.Second,
        MaxInterval:         30 * time.Second,
        Multiplier:          2.0,
        RandomizationFactor: 0.5,
    }),
)
```

### Typed Error Handling

Use `errors.Is` and `errors.As` for specific error handling:

```go
import "github.com/dotcommander/agent-framework/client"

response, err := c.Query(ctx, prompt)
if err != nil {
    // Check error types
    if errors.Is(err, client.ErrRateLimited) {
        // Wait and retry
    }
    if errors.Is(err, client.ErrMaxRetriesExceeded) {
        // All retries exhausted
    }
    if errors.Is(err, client.ErrCircuitOpen) {
        // Service unavailable, circuit breaker open
    }

    // Extract detailed error info
    var rlErr *client.RateLimitError
    if errors.As(err, &rlErr) {
        time.Sleep(rlErr.RetryAfter)
    }

    var srvErr *client.ServerError
    if errors.As(err, &srvErr) {
        log.Printf("Server error %d: %s", srvErr.StatusCode, srvErr.Message)
    }
}
```

### Circuit Breaker

Prevent cascading failures with circuit breaker protection:

```go
import (
    "github.com/dotcommander/agent-framework/client"
    "github.com/sony/gobreaker"
)

c, _ := client.New(ctx, sdkOpts,
    client.WithCircuitBreaker(&client.CircuitBreakerConfig{
        MaxFailures:   5,              // Open after 5 consecutive failures
        Timeout:       30 * time.Second, // Half-open after 30s
        ResetInterval: 60 * time.Second, // Reset counters after 60s
    }),
)

// Check circuit state before requests
if c.CircuitBreakerState() == gobreaker.StateOpen {
    // Service unavailable, use fallback
}
```

### All Resilience Features

Enable circuit breaker, retry, and rate limiting together:

```go
c, _ := client.New(ctx, sdkOpts,
    client.WithResilience(), // Enables all with defaults
)
```

### Context Compaction

Manage long conversations by summarizing older messages:

```go
import "github.com/dotcommander/agent-framework/client"

// LLM-based compaction (summarizes old messages, keeps recent N)
compactor := client.NewSimpleCompactor(client.CompactorConfig{
    MaxTokens:        100000,
    CompactThreshold: 0.8,  // Compact at 80% capacity
    KeepRecent:       5,    // Keep last 5 messages verbatim
})

// Check if compaction needed
if compactor.ShouldCompact(messages, currentTokenCount) {
    messages, _ = compactor.Compact(ctx, messages, summarizer)
}

// Or use simple sliding window (no LLM needed)
windowCompactor := client.NewSlidingWindowCompactor(20) // Keep last 20 messages
```

### Subagent Error Handling

Handle errors when running parallel subagents:

```go
import "github.com/dotcommander/agent-framework/app"

results, err := manager.RunAll(ctx)
if err != nil {
    if errors.Is(err, app.ErrAllAgentsFailed) {
        // All subagents failed
    }
    if errors.Is(err, app.ErrMaxSubagentsReached) {
        // Too many concurrent subagents
    }
    if errors.Is(err, app.ErrTokenBudgetExhausted) {
        // Token quota exceeded
    }
}
```

## License

MIT License - see [LICENSE](LICENSE) file.
