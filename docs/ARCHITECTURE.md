# Architecture

The agent framework provides composable building blocks for building AI-powered CLI applications.

## Package Overview

```
agent/
├── app/           # Application container and agent loop
├── client/        # AI client wrapper and compaction
├── cli/           # CLI scaffolding utilities
├── config/        # Configuration management
├── input/         # Input detection and processing
├── output/        # Output formatting and code generation
├── search/        # Semantic search and indexing
├── state/         # File system state tracking
├── tools/         # Tool registration and MCP support
├── validation/    # Rule-based validation
└── verification/  # Visual verification and evaluation
```

## Package Relationships

```
                    ┌─────────────────────────────────────┐
                    │              app.App                │
                    │  (orchestrates everything)          │
                    └──────────────┬──────────────────────┘
                                   │
       ┌───────────────┬───────────┼───────────────┬──────────────┐
       │               │           │               │              │
       ▼               ▼           ▼               ▼              ▼
┌─────────────┐ ┌───────────┐ ┌─────────┐ ┌────────────┐ ┌────────────┐
│   client    │ │   tools   │ │  output │ │   config   │ │    cli     │
│ (AI calls)  │ │ (registry)│ │(format) │ │ (settings) │ │ (parsing)  │
└─────────────┘ └───────────┘ └─────────┘ └────────────┘ └────────────┘
       │               │
       │               │
       ▼               ▼
┌─────────────┐ ┌───────────┐
│ compaction  │ │    MCP    │
│ (context)   │ │ (protocol)│
└─────────────┘ └───────────┘

       ┌─────────────────────────────────────────────────────────┐
       │                    Supporting Packages                   │
       ├──────────────┬──────────────┬──────────────┬────────────┤
       │    search    │    state     │  validation  │verification│
       │  (semantic)  │ (filesystem) │   (rules)    │  (visual)  │
       └──────────────┴──────────────┴──────────────┴────────────┘
```

## Design Principles

### 1. Composition Over Inheritance

The framework uses composition and interfaces. Each package exposes interfaces that can be implemented or replaced.

```go
// Use the default client
client, _ := client.New(ctx)

// Or bring your own implementation
type myClient struct { ... }
func (c *myClient) Query(ctx context.Context, prompt string) (string, error) { ... }
```

### 2. Functional Options

Configuration uses the functional options pattern for clean, extensible APIs.

```go
app := app.New("myapp", "1.0.0",
    app.WithSystemPrompt("You are helpful"),
    app.WithModel("claude-sonnet-4-20250514"),
    app.WithProvider(client.ProviderAnthropic),
)
```

### 3. Context Propagation

All operations accept `context.Context` as the first parameter for cancellation and timeout support.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

result, err := client.Query(ctx, prompt)
```

### 4. Explicit Dependencies

Packages declare their dependencies explicitly through constructors rather than global state.

```go
// Dependencies are injected
index := search.NewSemanticIndex(embedder, chunker)
manager := app.NewSubagentManager(config, executor)
```

## When to Use Each Package

| Package | Use When You Need To... |
|---------|------------------------|
| `app` | Build a complete CLI application |
| `app.LoopRunner` | Implement gather-decide-act-verify workflows |
| `app.SubagentManager` | Run multiple AI agents in parallel |
| `client` | Make direct AI API calls |
| `client.Compactor` | Manage context window limits |
| `tools` | Register tools for AI to use |
| `tools.MCPServer` | Expose tools via MCP protocol |
| `output` | Format responses as JSON/Markdown/text |
| `output.CodeGenerator` | Generate structured code output |
| `validation` | Validate AI outputs against rules |
| `verification` | Visual comparison and evaluation |
| `search` | Semantic search over documents |
| `state` | Track file changes and enable rollback |

## Quick Start

The minimal application:

```go
package main

import "github.com/dotcommander/agent/app"

func main() {
    a := app.New("myapp", "1.0.0",
        app.WithSystemPrompt("You are a helpful assistant."),
    )
    a.Run()
}
```

## Related Documentation

- [AGENT-LOOP.md](AGENT-LOOP.md) - The agent loop pattern
- [SUBAGENTS.md](SUBAGENTS.md) - Parallel agent execution
- [MCP.md](MCP.md) - Model Context Protocol
- [VALIDATION.md](VALIDATION.md) - Rule-based validation
- [VERIFICATION.md](VERIFICATION.md) - Visual and evaluation
- [STATE.md](STATE.md) - File system state tracking
- [SEARCH.md](SEARCH.md) - Semantic search
- [CODEGEN.md](CODEGEN.md) - Code generation output
- [COMPACTION.md](COMPACTION.md) - Context management
