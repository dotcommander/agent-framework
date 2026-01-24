# Agent Loop

The agent loop implements a gather-decide-act-verify cycle for autonomous agent workflows.

## Quick Start

```go
package main

import (
    "context"
    "github.com/dotcommander/agent/app"
)

func main() {
    // Create a simple loop with custom logic
    loop := app.NewSimpleLoop(
        app.WithGatherFunc(gatherContext),
        app.WithDecideFunc(decideAction),
        app.WithActionFunc(executeAction),
        app.WithVerifyFunc(verifyResult),
        app.WithContinueFunc(shouldContinue),
    )

    // Run with default config
    runner := app.NewLoopRunner(loop, nil)

    ctx := context.Background()
    state, err := runner.Run(ctx)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Completed in %d iterations\n", state.Iteration)
}
```

## The Loop Pattern

The agent loop follows a consistent four-phase cycle:

```
┌─────────────────────────────────────────────────────────┐
│                     Agent Loop                          │
│                                                         │
│    ┌─────────┐    ┌─────────┐    ┌─────────┐          │
│    │ Gather  │───▶│ Decide  │───▶│   Act   │          │
│    │ Context │    │ Action  │    │         │          │
│    └─────────┘    └─────────┘    └────┬────┘          │
│         ▲                             │               │
│         │                             ▼               │
│         │         ┌─────────┐    ┌─────────┐          │
│         └─────────│Continue?│◀───│ Verify  │          │
│                   └─────────┘    │ Result  │          │
│                        │         └─────────┘          │
│                        ▼                              │
│                   [Complete]                          │
└─────────────────────────────────────────────────────────┘
```

### Phase 1: Gather Context

Collect relevant information for the current iteration.

```go
func gatherContext(ctx context.Context, state *app.LoopState) (*app.LoopContext, error) {
    return &app.LoopContext{
        Messages: []app.Message{
            {Role: "user", Content: "Analyze this code"},
        },
        Tools: []app.ToolInfo{
            {Name: "read_file", Description: "Read a file"},
        },
        State: map[string]any{
            "files_analyzed": state.Iteration,
        },
        TokenCount: 1000,
    }, nil
}
```

### Phase 2: Decide Action

Determine what action to take based on gathered context.

```go
func decideAction(ctx context.Context, state *app.LoopState) (*app.Action, error) {
    // Check if we have context
    if state.Context == nil {
        return &app.Action{
            Type:     "response",
            Response: "No context available",
        }, nil
    }

    // Decide based on state
    if state.Iteration == 1 {
        return &app.Action{
            Type:      "tool_call",
            ToolName:  "read_file",
            ToolInput: map[string]any{"path": "main.go"},
        }, nil
    }

    return &app.Action{
        Type:     "response",
        Response: "Analysis complete",
    }, nil
}
```

### Phase 3: Take Action

Execute the decided action.

```go
func executeAction(ctx context.Context, action *app.Action) (*app.Result, error) {
    switch action.Type {
    case "tool_call":
        // Execute tool
        result := executeTool(action.ToolName, action.ToolInput)
        return &app.Result{
            Success: true,
            Output:  result,
            Tokens:  500,
        }, nil

    case "response":
        return &app.Result{
            Success: true,
            Output:  action.Response,
        }, nil

    default:
        return &app.Result{
            Success: false,
            Error:   fmt.Errorf("unknown action type: %s", action.Type),
        }, nil
    }
}
```

### Phase 4: Verify Result

Validate the action result and provide feedback.

```go
func verifyResult(ctx context.Context, state *app.LoopState) (*app.Feedback, error) {
    if state.LastResult == nil {
        return &app.Feedback{Valid: false, Score: 0}, nil
    }

    if state.LastResult.Error != nil {
        return &app.Feedback{
            Valid:  false,
            Issues: []string{state.LastResult.Error.Error()},
            Score:  0,
        }, nil
    }

    return &app.Feedback{
        Valid: true,
        Score: 1.0,
    }, nil
}
```

### Continuation Check

Decide whether to continue looping.

```go
func shouldContinue(state *app.LoopState) bool {
    // Stop after 10 iterations
    if state.Iteration >= 10 {
        return false
    }

    // Stop if last action was a response (task complete)
    if state.LastAction != nil && state.LastAction.Type == "response" {
        return false
    }

    // Stop if verification failed
    if state.LastVerify != nil && !state.LastVerify.Valid {
        return false
    }

    return true
}
```

## Loop Configuration

Configure loop behavior with `LoopConfig`:

```go
config := &app.LoopConfig{
    // Safety limits
    MaxIterations: 50,        // Stop after 50 iterations
    MaxTokens:     100000,    // Stop if tokens exceed limit
    Timeout:       30 * time.Minute,

    // Error handling
    StopOnError: false,       // Continue on non-fatal errors
    MinScore:    0.5,         // Minimum verification score

    // Hooks for observability
    OnIterationStart: func(state *app.LoopState) {
        log.Printf("Starting iteration %d", state.Iteration)
    },
    OnIterationEnd: func(state *app.LoopState) {
        log.Printf("Completed iteration %d", state.Iteration)
    },
    OnError: func(err error, state *app.LoopState) {
        log.Printf("Error at iteration %d: %v", state.Iteration, err)
    },
}

runner := app.NewLoopRunner(loop, config)
```

## Loop State

The `LoopState` tracks progress through iterations:

```go
type LoopState struct {
    Iteration   int           // Current iteration number (1-based)
    Context     *LoopContext  // Most recent gathered context
    LastAction  *Action       // Most recent action taken
    LastResult  *Result       // Result of last action
    LastVerify  *Feedback     // Verification feedback
    StartedAt   time.Time     // When loop started
    CompletedAt time.Time     // When loop completed
}
```

## Implementing AgentLoop Interface

For full control, implement the `AgentLoop` interface:

```go
type AgentLoop interface {
    GatherContext(ctx context.Context, state *LoopState) (*LoopContext, error)
    DecideAction(ctx context.Context, state *LoopState) (*Action, error)
    TakeAction(ctx context.Context, action *Action) (*Result, error)
    Verify(ctx context.Context, state *LoopState) (*Feedback, error)
    ShouldContinue(state *LoopState) bool
}
```

Example custom implementation:

```go
type CodeReviewLoop struct {
    client client.Client
    files  []string
}

func (l *CodeReviewLoop) GatherContext(ctx context.Context, state *app.LoopState) (*app.LoopContext, error) {
    // Read next file to review
    if state.Iteration <= len(l.files) {
        content, _ := os.ReadFile(l.files[state.Iteration-1])
        return &app.LoopContext{
            State: map[string]any{
                "file":    l.files[state.Iteration-1],
                "content": string(content),
            },
        }, nil
    }
    return &app.LoopContext{}, nil
}

func (l *CodeReviewLoop) DecideAction(ctx context.Context, state *app.LoopState) (*app.Action, error) {
    file := state.Context.State["file"].(string)
    content := state.Context.State["content"].(string)

    return &app.Action{
        Type:      "tool_call",
        ToolName:  "review_code",
        ToolInput: map[string]any{"file": file, "content": content},
    }, nil
}

func (l *CodeReviewLoop) TakeAction(ctx context.Context, action *app.Action) (*app.Result, error) {
    // Call AI to review the code
    prompt := fmt.Sprintf("Review this code:\n%s", action.ToolInput["content"])
    review, err := l.client.Query(ctx, prompt)
    return &app.Result{Success: err == nil, Output: review, Error: err}, nil
}

func (l *CodeReviewLoop) Verify(ctx context.Context, state *app.LoopState) (*app.Feedback, error) {
    if state.LastResult.Success {
        return &app.Feedback{Valid: true, Score: 1.0}, nil
    }
    return &app.Feedback{Valid: false, Score: 0}, nil
}

func (l *CodeReviewLoop) ShouldContinue(state *app.LoopState) bool {
    return state.Iteration < len(l.files)
}
```

## Action Types

The framework supports several action types:

| Type | Purpose | Fields Used |
|------|---------|------------|
| `tool_call` | Execute a tool | `ToolName`, `ToolInput` |
| `response` | Return a response | `Response` |
| `delegate` | Delegate to subagent | `Subagent`, `SubagentTask` |

## Error Handling

Errors are captured in results and can trigger different behaviors:

```go
// Configure to stop on first error
config.StopOnError = true

// Or handle errors gracefully
config.OnError = func(err error, state *app.LoopState) {
    // Log and continue
    log.Printf("Recoverable error: %v", err)
}
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [SUBAGENTS.md](SUBAGENTS.md) - Parallel agent execution
- [VERIFICATION.md](VERIFICATION.md) - Verification and evaluation
