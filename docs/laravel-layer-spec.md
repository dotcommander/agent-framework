# Agent Framework: The Laravel Layer

**Vision**: 5 minutes from idea to working agent.

## The Laravel Philosophy Applied

| Laravel | Agent Framework Equivalent |
|---------|---------------------------|
| `php artisan make:model` | `agent new`, `agent add tool` |
| Eloquent ORM | Fluent agent builders |
| Facades | Package-level convenience functions |
| Middleware | Hook chain for tool/response interception |
| Blade templates | Typed prompt templates |
| Collections | Result processors and transformers |
| Events | Lifecycle hooks (PreToolUse, etc.) |
| Queues | Background agent execution |
| Migrations | Conversation checkpoints |

---

## Layer 1: One-Liner Agents

### Current (Verbose)
```go
package main

import (
    "context"
    "log"
    "github.com/dotcommander/agent-framework/app"
    "github.com/dotcommander/agent-sdk-go/claude"
)

func main() {
    ctx := context.Background()
    sdkOpts := []claude.ClientOption{
        claude.WithModel("claude-sonnet-4-20250514"),
    }

    application := app.New("myapp", "1.0.0",
        app.WithSDKOptions(sdkOpts...),
        app.WithSystemPrompt("You are helpful."),
    )

    if err := application.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### Proposed (5 Lines)
```go
package main

import "github.com/dotcommander/agent-framework/agent"

func main() {
    agent.Run("You are a helpful coding assistant.")
}
```

### Implementation: `agent/quick.go`
```go
package agent

import (
    "context"
    "os"

    "github.com/dotcommander/agent-framework/app"
    "github.com/dotcommander/agent-sdk-go/claude"
)

// Run starts an agent with sensible defaults.
// The simplest possible entry point.
func Run(systemPrompt string, opts ...Option) error {
    return New(systemPrompt, opts...).Run()
}

// Query sends a single prompt and returns the response.
// For scripts and one-shot tasks.
func Query(prompt string, opts ...Option) (string, error) {
    ctx := context.Background()
    a := New("", opts...)
    return a.Query(ctx, prompt)
}

// QueryTyped sends a prompt and returns typed response.
func QueryTyped[T any](prompt string, opts ...Option) (*T, error) {
    ctx := context.Background()
    a := New("", opts...)
    return QueryAs[T](ctx, a, prompt)
}

// Stream sends a prompt and streams the response.
func Stream(prompt string, handler func(chunk string), opts ...Option) error {
    ctx := context.Background()
    a := New("", opts...)
    return a.Stream(ctx, prompt, handler)
}
```

---

## Layer 2: Fluent Builders

### Proposed API
```go
package main

import "github.com/dotcommander/agent-framework/agent"

func main() {
    agent.New("code-reviewer").
        Model("opus").
        System("You review code for bugs and style issues.").
        Tool(searchTool).
        Tool(readFileTool).
        OnToolUse(func(name string, input any) bool {
            log.Printf("Using tool: %s", name)
            return true // allow
        }).
        Budget(5.00). // USD limit
        MaxTurns(20).
        Run()
}
```

### Implementation: `agent/builder.go`
```go
package agent

type Builder struct {
    name         string
    model        string
    systemPrompt string
    tools        []Tool
    hooks        []Hook
    budget       float64
    maxTurns     int
    // ... more fields
}

func New(name string) *Builder {
    return &Builder{
        name:  name,
        model: "sonnet", // sensible default
    }
}

func (b *Builder) Model(m string) *Builder {
    // Accept shorthand: "opus", "sonnet", "haiku"
    b.model = expandModel(m)
    return b
}

func (b *Builder) System(prompt string) *Builder {
    b.systemPrompt = prompt
    return b
}

func (b *Builder) Tool(t Tool) *Builder {
    b.tools = append(b.tools, t)
    return b
}

func (b *Builder) OnToolUse(fn func(name string, input any) bool) *Builder {
    b.hooks = append(b.hooks, Hook{
        Event:   HookPreToolUse,
        Handler: fn,
    })
    return b
}

func (b *Builder) Budget(usd float64) *Builder {
    b.budget = usd
    return b
}

func (b *Builder) MaxTurns(n int) *Builder {
    b.maxTurns = n
    return b
}

func (b *Builder) Run() error {
    return b.build().Run()
}

func (b *Builder) Query(ctx context.Context, prompt string) (string, error) {
    return b.build().Query(ctx, prompt)
}
```

---

## Layer 3: Smart Defaults & Presets

### Agent Presets
```go
// Pre-configured agents for common use cases
agent.Coder().Run()           // Code writing agent with file tools
agent.Reviewer().Run()        // Code review agent
agent.Researcher().Run()      // Web search + summarization
agent.DataAnalyst().Run()     // CSV/JSON processing
agent.ChatBot().Run()         // Conversational agent

// Customizable presets
agent.Coder().
    Language("go").
    Style("google").
    Run()
```

### Implementation: `agent/presets.go`
```go
package agent

// Coder returns a pre-configured coding agent.
func Coder() *Builder {
    return New("coder").
        System(coderPrompt).
        Tool(ReadFile()).
        Tool(WriteFile()).
        Tool(RunCommand()).
        Tool(Search())
}

// Reviewer returns a code review agent.
func Reviewer() *Builder {
    return New("reviewer").
        System(reviewerPrompt).
        Tool(ReadFile()).
        Tool(Diff()).
        OutputFormat(ReviewFormat{})
}

// Researcher returns a research agent.
func Researcher() *Builder {
    return New("researcher").
        System(researcherPrompt).
        Tool(WebSearch()).
        Tool(WebFetch()).
        Tool(Summarize())
}
```

---

## Layer 4: Tool Sugar

### Current (Verbose)
```go
tool := tools.TypedTool(
    "greet",
    "Greets a person",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "name": map[string]any{
                "type": "string",
                "description": "Name to greet",
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
        }{Message: fmt.Sprintf("Hello, %s!", input.Name)}, nil
    },
)
```

### Proposed (Inferred Schema)
```go
// Schema inferred from struct tags
type GreetInput struct {
    Name string `json:"name" desc:"Name to greet" required:"true"`
}

type GreetOutput struct {
    Message string `json:"message"`
}

greet := agent.Tool("greet", "Greets a person",
    func(ctx context.Context, in GreetInput) (GreetOutput, error) {
        return GreetOutput{Message: "Hello, " + in.Name + "!"}, nil
    },
)

// Even simpler for basic tools
search := agent.SimpleTool("search", "Search the web",
    func(query string) (string, error) {
        return doSearch(query), nil
    },
)
```

### Implementation: `agent/tool.go`
```go
package agent

import (
    "context"
    "reflect"
)

// Tool creates a type-safe tool with schema inferred from input type.
func Tool[I, O any](name, desc string, fn func(context.Context, I) (O, error)) ToolDef {
    schema := schemaFromType[I]()
    return ToolDef{
        Name:        name,
        Description: desc,
        Schema:      schema,
        Handler:     wrapHandler(fn),
    }
}

// SimpleTool creates a tool with a single string input/output.
func SimpleTool(name, desc string, fn func(string) (string, error)) ToolDef {
    return Tool(name, desc, func(_ context.Context, in struct {
        Input string `json:"input" required:"true"`
    }) (struct {
        Output string `json:"output"`
    }, error) {
        out, err := fn(in.Input)
        return struct {
            Output string `json:"output"`
        }{Output: out}, err
    })
}

// schemaFromType generates JSON schema from struct tags.
func schemaFromType[T any]() map[string]any {
    var zero T
    t := reflect.TypeOf(zero)
    return generateSchema(t)
}
```

---

## Layer 5: Structured Output Sugar

### Proposed API
```go
// Type-safe responses
type CodeReview struct {
    Summary string   `json:"summary"`
    Issues  []Issue  `json:"issues"`
    Score   int      `json:"score"`
}

review, err := agent.Query[CodeReview](ctx, "Review this code: ...")

// Streaming with typed chunks
agent.Stream[ProgressUpdate](ctx, prompt, func(chunk ProgressUpdate) {
    progressBar.Set(chunk.Percent)
})
```

---

## Layer 6: Conversation Management

### Proposed API
```go
// Start a conversation
conv := agent.Conversation("code-helper")

// Multi-turn with automatic context
conv.Say("I'm working on a Go project")
conv.Say("How do I handle errors properly?")
response := conv.Say("Show me an example")

// Save/resume conversations
conv.Save("my-session")
// Later...
conv := agent.Resume("my-session")

// Fork a conversation (branch)
fork := conv.Fork()
fork.Say("What about using panics instead?")
```

### Implementation: `agent/conversation.go`
```go
package agent

type Conversation struct {
    id       string
    agent    *Builder
    messages []Message
}

func Converse(agentName string) *Conversation {
    return &Conversation{
        id:    generateID(),
        agent: New(agentName),
    }
}

func (c *Conversation) Say(message string) string {
    c.messages = append(c.messages, Message{Role: "user", Content: message})
    response, _ := c.agent.QueryWithHistory(context.Background(), c.messages)
    c.messages = append(c.messages, Message{Role: "assistant", Content: response})
    return response
}

func (c *Conversation) Save(name string) error {
    return saveConversation(name, c)
}

func Resume(name string) (*Conversation, error) {
    return loadConversation(name)
}

func (c *Conversation) Fork() *Conversation {
    return &Conversation{
        id:       generateID(),
        agent:    c.agent,
        messages: append([]Message{}, c.messages...), // copy
    }
}
```

---

## Layer 7: Background Agents

### Proposed API
```go
// Fire and forget
job := agent.Background("analyzer").
    Query("Analyze all Go files in this repo").
    OnComplete(func(result string) {
        notify.Send("Analysis complete")
    }).
    Start()

// Check status
if job.Done() {
    result := job.Result()
}

// Wait with timeout
result, err := job.Wait(5 * time.Minute)

// Parallel agents
results := agent.Parallel(
    agent.New("reviewer").Query("Review auth.go"),
    agent.New("reviewer").Query("Review db.go"),
    agent.New("reviewer").Query("Review api.go"),
).Wait()
```

---

## Layer 8: Middleware/Hooks

### Proposed API
```go
agent.New("secure-agent").
    // Log all tool usage
    Use(agent.Logger()).

    // Rate limit tool calls
    Use(agent.RateLimit(10, time.Minute)).

    // Block dangerous commands
    Use(agent.BlockCommands("rm -rf", "DROP TABLE")).

    // Custom middleware
    Use(func(next agent.Handler) agent.Handler {
        return func(ctx context.Context, tool string, input any) (any, error) {
            start := time.Now()
            result, err := next(ctx, tool, input)
            log.Printf("%s took %v", tool, time.Since(start))
            return result, err
        }
    }).
    Run()
```

---

## Layer 9: Error Handling Sugar

### Proposed API
```go
result, err := agent.Query(ctx, prompt)

// Fluent error handling
err.IfRateLimited(func(retryAfter time.Duration) {
    time.Sleep(retryAfter)
    // retry
})

err.IfBudgetExceeded(func(spent, limit float64) {
    alert.Send("Budget exceeded: $%.2f/$%.2f", spent, limit)
})

// Or use Must for scripts
result := agent.Must(agent.Query(ctx, prompt))
```

---

## Layer 10: CLI Scaffolding

```bash
# Create new agent project
agent new my-assistant
cd my-assistant

# Add tools
agent add tool search "Search the web"
agent add tool read-file "Read a file"

# Add presets
agent add preset coder
agent add preset researcher

# Run
agent run

# Test with a prompt
agent query "What is the capital of France?"
```

---

## Package Structure

```
agent/
├── agent.go          # Main entry: Run(), Query(), New()
├── builder.go        # Fluent builder pattern
├── presets.go        # Coder(), Reviewer(), Researcher()
├── tool.go           # Tool(), SimpleTool(), schema inference
├── conversation.go   # Multi-turn conversation management
├── background.go     # Background(), Parallel()
├── middleware.go     # Use(), built-in middleware
├── errors.go         # Error handling sugar
├── quick.go          # One-liner convenience functions
└── defaults.go       # Smart defaults, model shortcuts
```

---

## Migration Path

1. **Keep existing API** - `app.New()` still works
2. **Add `agent` package** - New convenience layer
3. **`agent` imports `app`** - Sugar on top, not replacement
4. **Gradual adoption** - Use what you need

---

## Implementation Priority

### Week 1: Core Sugar
- [ ] `agent.Run()`, `agent.Query()` one-liners
- [ ] `agent.New().Model().System().Run()` builder
- [ ] Model shortcuts: "opus", "sonnet", "haiku"

### Week 2: Tools & Output
- [ ] `agent.Tool()` with schema inference
- [ ] `agent.QueryTyped[T]()` structured output
- [ ] Built-in tools: ReadFile, WriteFile, Search

### Week 3: Conversations & Background
- [ ] `agent.Conversation()` multi-turn
- [ ] `agent.Background()` async agents
- [ ] `agent.Parallel()` concurrent agents

### Week 4: Middleware & Presets
- [ ] Middleware chain
- [ ] Built-in middleware (Logger, RateLimit, BlockCommands)
- [ ] Agent presets (Coder, Reviewer, Researcher)

---

## Success Metric

**Before**: 50 lines to start an agent
**After**: 1 line to start, 10 lines for production-ready
