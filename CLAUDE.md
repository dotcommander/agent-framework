# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build all packages
go build ./...

# Build example CLI
go build -o agent-example ./cmd/agent

# Run after build
./agent-example "your prompt here"

# Run tests (when they exist)
go test ./...

# Run single package test
go test ./app/...

# Lint (requires golangci-lint)
golangci-lint run
```

## Dependencies

Uses `replace` directive for local development:
```
replace github.com/dotcommander/agent-sdk-go => ../agent-sdk-go
```

Clone both repos as siblings for the build to work.

## Examples

The `examples/` directory contains runnable examples for each major feature:
- `examples/loop/` - Agent loop pattern
- `examples/subagents/` - Parallel subagent execution
- `examples/mcp/` - MCP server/client
- `examples/validation/` - Rules-based validation
- `examples/codegen/` - Code generation output
- `examples/search/` - Semantic search
- `examples/state/` - File system tracking
- `examples/evaluation/` - Hierarchical verification

## Architecture

### Package Overview

| Package | Purpose |
|---------|---------|
| `app/` | Application composition, agent loop, subagent spawning |
| `client/` | Claude SDK wrapper, provider abstraction, context compaction |
| `cli/` | Cobra command scaffolding, standard flags |
| `config/` | Configuration types, functional options |
| `input/` | Auto-detection of URLs, files, globs, text |
| `output/` | JSON/Markdown/text formatters, code generation output |
| `tools/` | Type-safe tool registration, MCP server/client |
| `validation/` | Composable rules (Required, Regex, Enum, Range, Length, Custom) |
| `verification/` | Visual verification, hierarchical evaluation |
| `state/` | File system tracking, snapshots, rollback |
| `search/` | Semantic search with embeddings, hybrid search |

### Core Patterns

**Functional Options**: All constructors use `With*` option functions:
```go
app.New("myapp", "1.0.0",
    app.WithSystemPrompt("..."),
    app.WithTool(tool),
)
```

**Type-Safe Tools**: Generic handlers with automatic JSON marshaling:
```go
tools.TypedTool("name", "desc", schema, func(ctx context.Context, input T) (R, error) { ... })
```

**Agent Loop**: Gather → Decide → Act → Verify cycle in `app/loop.go`:
- `AgentLoop` interface defines the contract
- `SimpleLoop` provides configurable implementation
- `LoopRunner` executes with limits (iterations, tokens, timeout)

**Subagent Spawning**: Parallel execution via `app/subagent.go`:
- `SubagentManager.Spawn()` creates isolated child agents
- `RunAll()` executes concurrently with errgroup
- `FilterResults()`, `MergeResults()`, `AggregateTokens()` for result handling

**MCP Protocol**: Model Context Protocol in `tools/mcp.go`:
- `MCPServer` handles initialize, tools/list, tools/call, resources/*
- `MCPClient` + `ToolDiscovery` for connecting to external servers

### Provider Abstraction

`client/provider.go` supports multiple backends:
- Anthropic (via agent-sdk-go) - default
- Z.AI - placeholder
- Synthetic - for testing without API calls
