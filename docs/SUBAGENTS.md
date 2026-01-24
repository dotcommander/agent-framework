# Subagents

Subagents enable parallel execution of multiple AI agents, each with isolated context.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent/app"
)

func main() {
    ctx := context.Background()

    // Create executor that defines how subagents run
    executor := app.SubagentExecutorFunc(func(ctx context.Context, agent *app.Subagent) (*app.SubagentResult, error) {
        // Your execution logic here
        return &app.SubagentResult{
            Success: true,
            Output:  fmt.Sprintf("Completed: %s", agent.Task),
            Tokens:  100,
        }, nil
    })

    // Create manager
    manager := app.NewSubagentManager(nil, executor)

    // Spawn subagents
    manager.Spawn("analyzer", "Analyze the code structure")
    manager.Spawn("reviewer", "Review code quality")
    manager.Spawn("tester", "Suggest test cases")

    // Run all in parallel
    results, err := manager.RunAll(ctx)
    if err != nil {
        panic(err)
    }

    // Process results
    for id, result := range results {
        fmt.Printf("%s: %v\n", id, result.Output)
    }
}
```

## Core Concepts

### SubagentManager

The `SubagentManager` coordinates spawning and running multiple subagents:

```go
type SubagentManager struct {
    config    *SubagentConfig
    subagents map[string]*Subagent
    executor  SubagentExecutor
}
```

### Subagent

Each `Subagent` represents an isolated child agent:

```go
type Subagent struct {
    ID      string           // Unique identifier (auto-generated)
    Name    string           // Human-readable name
    Task    string           // Task description
    Context *SubagentContext // Isolated context
    Result  *SubagentResult  // Outcome after execution
}
```

### SubagentContext

Each subagent has isolated context:

```go
type SubagentContext struct {
    Messages     []Message       // Conversation history
    Tools        []ToolInfo      // Available tools
    SystemPrompt string          // System instructions
    State        map[string]any  // Custom state
    MaxTokens    int             // Token limit
}
```

## Spawning Subagents

Use `Spawn` with options to configure each subagent:

```go
// Basic spawn
agent := manager.Spawn("analyzer", "Analyze code")

// Spawn with options
agent := manager.Spawn("analyzer", "Analyze code",
    app.WithSubagentPrompt("You are a code analysis expert."),
    app.WithSubagentTools([]app.ToolInfo{
        {Name: "read_file", Description: "Read a file"},
        {Name: "grep", Description: "Search in files"},
    }),
    app.WithSubagentState(map[string]any{
        "target_dir": "/path/to/code",
        "language":   "go",
    }),
    app.WithSubagentMaxTokens(50000),
)
```

### Available Options

| Option | Purpose |
|--------|---------|
| `WithSubagentPrompt` | Set system prompt |
| `WithSubagentTools` | Set available tools |
| `WithSubagentState` | Set initial state |
| `WithSubagentMessages` | Set initial messages |
| `WithSubagentMaxTokens` | Set token limit |

## Running Subagents

### Run All Subagents

```go
results, err := manager.RunAll(ctx)
```

### Run Specific Subagents

```go
// Spawn multiple
agent1 := manager.Spawn("fast-task", "Quick analysis")
agent2 := manager.Spawn("slow-task", "Deep analysis")

// Run only selected ones
results, err := manager.RunAgents(ctx, agent1, agent2)
```

### Run Single Subagent

```go
agent := manager.Spawn("single", "One task")
result, err := manager.Run(ctx, agent)
```

## Configuration

Configure the manager's behavior:

```go
config := &app.SubagentConfig{
    // Limit concurrent execution
    MaxConcurrent: 5,

    // Create fresh context per subagent
    IsolateContext: true,

    // Allow subagents to use parent's tools
    ShareTools: true,

    // Cancel children when parent is cancelled
    PropagateCancel: true,
}

manager := app.NewSubagentManager(config, executor)
```

## Implementing Executors

The `SubagentExecutor` interface defines how subagents run:

```go
type SubagentExecutor interface {
    Execute(ctx context.Context, agent *Subagent) (*SubagentResult, error)
}
```

### Function Adapter

Use `SubagentExecutorFunc` for simple cases:

```go
executor := app.SubagentExecutorFunc(func(ctx context.Context, agent *app.Subagent) (*app.SubagentResult, error) {
    // Execute task
    return &app.SubagentResult{
        Success: true,
        Output:  "done",
    }, nil
})
```

### Custom Executor

Implement the interface for complex logic:

```go
type AIExecutor struct {
    client client.Client
}

func (e *AIExecutor) Execute(ctx context.Context, agent *app.Subagent) (*app.SubagentResult, error) {
    // Build prompt from context
    prompt := fmt.Sprintf("%s\n\nTask: %s",
        agent.Context.SystemPrompt,
        agent.Task)

    // Call AI
    response, err := e.client.Query(ctx, prompt)
    if err != nil {
        return &app.SubagentResult{
            Success: false,
            Error:   err,
        }, nil
    }

    return &app.SubagentResult{
        Success: true,
        Output:  response,
        Tokens:  len(response) / 4, // Rough estimate
    }, nil
}
```

## Processing Results

### SubagentResult Structure

```go
type SubagentResult struct {
    Success bool   // Whether execution succeeded
    Output  any    // Result data
    Error   error  // Error if failed
    Tokens  int    // Tokens used
}
```

### Filtering Results

```go
// Get only successful results
successful := app.FilterResults(results, true)

// Get only failed results
failed := app.FilterResults(results, false)
```

### Merging Outputs

```go
// Combine all outputs into a slice
outputs := app.MergeResults(results)
for _, output := range outputs {
    fmt.Println(output)
}
```

### Aggregating Tokens

```go
totalTokens := app.AggregateTokens(results)
fmt.Printf("Total tokens used: %d\n", totalTokens)
```

## Managing Subagents

### List All Subagents

```go
agents := manager.List()
for _, agent := range agents {
    fmt.Printf("%s: %s\n", agent.ID, agent.Name)
}
```

### Get Specific Subagent

```go
agent := manager.Get("subagent-1")
if agent != nil {
    fmt.Println(agent.Task)
}
```

### Clear All Subagents

```go
manager.Clear()
```

## Concurrency Control

The manager uses `errgroup` with configurable concurrency:

```go
config := &app.SubagentConfig{
    MaxConcurrent: 3, // Only 3 subagents run simultaneously
}
```

This prevents overwhelming the AI API with too many concurrent requests.

## Example: Parallel Code Analysis

```go
func analyzeCodebase(ctx context.Context, files []string) (map[string]string, error) {
    executor := app.SubagentExecutorFunc(func(ctx context.Context, agent *app.Subagent) (*app.SubagentResult, error) {
        filePath := agent.Context.State["file"].(string)
        content, _ := os.ReadFile(filePath)

        // Analyze file (your AI logic here)
        analysis := fmt.Sprintf("Analysis of %s: %d lines", filePath, len(strings.Split(string(content), "\n")))

        return &app.SubagentResult{
            Success: true,
            Output:  analysis,
        }, nil
    })

    manager := app.NewSubagentManager(&app.SubagentConfig{
        MaxConcurrent: 5,
    }, executor)

    // Spawn one subagent per file
    for _, file := range files {
        manager.Spawn("analyzer", fmt.Sprintf("Analyze %s", file),
            app.WithSubagentState(map[string]any{"file": file}),
        )
    }

    // Run all in parallel
    results, err := manager.RunAll(ctx)
    if err != nil {
        return nil, err
    }

    // Collect analyses
    analyses := make(map[string]string)
    for _, agent := range manager.List() {
        if agent.Result != nil && agent.Result.Success {
            file := agent.Context.State["file"].(string)
            analyses[file] = agent.Result.Output.(string)
        }
    }

    return analyses, nil
}
```

## Error Handling

Errors are collected per-subagent, not propagated globally:

```go
results, err := manager.RunAll(ctx)
// err is only non-nil for infrastructure failures

// Check individual results
for id, result := range results {
    if result.Error != nil {
        log.Printf("Subagent %s failed: %v", id, result.Error)
    }
}
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [AGENT-LOOP.md](AGENT-LOOP.md) - The agent loop pattern
- [COMPACTION.md](COMPACTION.md) - Managing context size
