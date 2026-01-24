# Agent Package Reference

The `agent` package is the convenience layer for building AI agents. It provides syntactic sugar that reduces boilerplate from 50+ lines to as few as 1 line.

## Before & After

**Without agent package (50+ lines):**

```go
func main() {
    ctx := context.Background()
    sdkOpts := []claude.ClientOption{
        claude.WithModel("claude-opus-4-5-20251101"),
        claude.WithSystemPrompt("You are helpful."),
    }
    c, err := client.New(ctx, sdkOpts, client.WithRetry(nil))
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    response, err := c.Query(ctx, "Hello")
    if err != nil {
        log.Fatal(err)
    }

    // Parse JSON from response...
    response = strings.TrimSpace(response)
    start := strings.Index(response, "{")
    end := strings.LastIndex(response, "}")
    // ... 30 more lines of JSON extraction

    var result MyType
    json.Unmarshal([]byte(response[start:end+1]), &result)
}
```

**With agent package (3 lines):**

```go
func main() {
    result, _ := agent.QueryAs[MyType](context.Background(), "Hello",
        agent.WithModel("opus"))
}
```

---

## Table of Contents

- [Quick Start](#quick-start)
- [Common Recipes](#common-recipes)
- [JSON Extraction](#json-extraction)
- [Pipeline Pattern](#pipeline-pattern)
- [SimpleClient](#simpleclient)
- [Prompt Templates](#prompt-templates)
- [Provider Configuration](#provider-configuration)
- [Builder Pattern](#builder-pattern)
- [Tools](#tools)
- [Hooks & Lifecycle](#hooks--lifecycle)
- [Sessions & Permissions](#sessions--permissions)
- [Schema Generation](#schema-generation)
- [Complete Examples](#complete-examples)

---

## Quick Start

### One-Line Agent

```go
agent.Run("You are a helpful coding assistant.")
```

### Single Query

```go
response, err := agent.Query(ctx, "What is 2+2?")
```

### Typed Response

```go
type Answer struct {
    Result int    `json:"result"`
    Reason string `json:"reason"`
}

answer, err := agent.QueryAs[Answer](ctx, "What is 2+2? Explain.")
fmt.Printf("%d because %s\n", answer.Result, answer.Reason)
```

### Stream Response

```go
agent.Stream(ctx, "Write a poem", func(chunk string) {
    fmt.Print(chunk)
})
```

### Fail Fast (Scripts)

```go
response := agent.Must(agent.Query(ctx, "Hello"))
```

---

## Common Recipes

### Recipe: Extract Structured Data from Text

**Problem:** Parse entities from unstructured text.

```go
type Entities struct {
    People    []string `json:"people"`
    Places    []string `json:"places"`
    Dates     []string `json:"dates"`
}

text := "John met Mary in Paris on January 5th, 2025."
entities, _ := agent.QueryAs[Entities](ctx,
    "Extract entities from: "+text,
    agent.WithModel("haiku")) // Fast model for extraction
```

### Recipe: Multi-Step Processing Pipeline

**Problem:** Chain LLM calls where each step refines the previous.

```go
client, _ := agent.NewClient(ctx, agent.WithClientModel("opus"))
defer client.Close()

spec, _ := agent.NewPipeline(client).
    Step("extract", "Extract requirements from:\n\n%s").
    Step("structure", "Structure these requirements:\n\n%s").
    Step("polish", "Polish this spec for clarity:\n\n%s").
    OnProgress(func(name string, n, total int) {
        fmt.Printf("[%d/%d] %s\n", n, total, name)
    }).
    Run(ctx, roughNotes)
```

### Recipe: Parse LLM JSON Response

**Problem:** LLM wraps JSON in markdown or adds commentary.

```go
response := `Here's the data you requested:
\`\`\`json
{"name": "Alice", "score": 95}
\`\`\`
Let me know if you need anything else!`

type Result struct {
    Name  string `json:"name"`
    Score int    `json:"score"`
}

result, _ := agent.ExtractJSON[Result](response)
// result.Name = "Alice", result.Score = 95
```

### Recipe: Batch Process with Progress

```go
items := []string{"item1", "item2", "item3"}
client, _ := agent.NewClient(ctx, agent.WithClientModel("haiku"))
defer client.Close()

for i, item := range items {
    fmt.Printf("Processing %d/%d\n", i+1, len(items))
    response, _ := client.Query(ctx, "Analyze: "+item)
    results = append(results, agent.ExtractMarkdown(response))
}
```

### Recipe: Use Non-Anthropic Provider

```go
provider := agent.ProviderZAI // or agent.ProviderSynthetic

client, _ := agent.NewClient(ctx,
    agent.WithClientModel(provider.Model),
    agent.WithClientEnv(agent.ProviderEnv(provider)),
)
```

### Recipe: Embedded Prompts with User Override

```go
//go:embed prompts/*.prompt
var promptsFS embed.FS

prompts := agent.NewPromptRegistry("myapp", promptsFS)

// Uses ~/.config/myapp/prompts/analyze.prompt if exists,
// otherwise falls back to embedded prompts/analyze.prompt
text, _ := prompts.Format("analyze", userInput)
```

---

## JSON Extraction

LLMs often wrap JSON in markdown code blocks or add commentary. These helpers extract clean JSON.

### Extract Object

```go
type Result struct {
    Answer string `json:"answer"`
    Score  int    `json:"score"`
}

// Handles: markdown blocks, leading text, trailing text
response := `Here's my analysis:
{"answer": "Paris", "score": 95}
Hope that helps!`

result, err := agent.ExtractJSON[Result](response)
```

### Extract Array

```go
type Item struct {
    Name  string `json:"name"`
    Value int    `json:"value"`
}

response := `Found these items:
[{"name": "foo", "value": 1}, {"name": "bar", "value": 2}]`

items, err := agent.ExtractJSONArray[Item](response)
```

### Strip Markdown Wrapper

```go
response := "```markdown\n# Title\nContent here\n```"
clean := agent.ExtractMarkdown(response)
// Returns: "# Title\nContent here"
```

### Must Variants (Scripts)

```go
result := agent.MustExtractJSON[Result](response)     // Panics on error
items := agent.MustExtractJSONArray[Item](response)   // Panics on error
```

### What It Handles

| Input | Output |
|-------|--------|
| `{"key": "value"}` | `{"key": "value"}` |
| `Here's the JSON: {"key": "value"}` | `{"key": "value"}` |
| `` ```json\n{"key": "value"}\n``` `` | `{"key": "value"}` |
| `{"key": "value"} Hope this helps!` | `{"key": "value"}` |
| `[{"a": 1}, {"a": 2}]` | `[{"a": 1}, {"a": 2}]` |

---

## Pipeline Pattern

Chain multiple LLM calls where each step's output becomes the next step's input. Common in spec generation, content refinement, and multi-stage analysis.

### Basic Pipeline

```go
client, _ := agent.NewClient(ctx, agent.WithClientModel("opus"))
defer client.Close()

result, err := agent.NewPipeline(client).
    Step("extract", "Extract key points from:\n\n%s").
    Step("summarize", "Summarize these points:\n\n%s").
    Step("polish", "Polish this summary:\n\n%s").
    Run(ctx, rawInput)
```

### With Progress Reporting

```go
agent.NewPipeline(client).
    Step("layer1", prompt1).
    Step("layer2", prompt2).
    Step("layer3", prompt3).
    OnProgress(func(name string, step, total int) {
        fmt.Printf("  [%d/%d] %s...\n", step, total, name)
    }).
    Run(ctx, input)
```

### With Post-Processing

Strip markdown wrappers between steps:

```go
agent.NewPipeline(client).
    StepWithPost("generate", prompt, agent.ExtractMarkdown).
    StepWithPost("refine", refinePrompt, agent.ExtractMarkdown).
    Run(ctx, input)
```

### With Graceful Degradation

Continue with previous output if a step fails:

```go
agent.NewPipeline(client).
    Step("required", requiredPrompt).
    Step("optional", optionalPrompt).
    OnError(func(name string, err error) (string, bool) {
        log.Printf("Step %s failed: %v (continuing)", name, err)
        return "", true // Skip failed step
    }).
    Run(ctx, input)
```

### Dynamic Prompt Building

```go
agent.NewPipeline(client).
    StepFunc("analyze", func(input string) string {
        return fmt.Sprintf(`Analyze this %s code:
\`\`\`%s
%s
\`\`\``, language, language, input)
    }).
    Run(ctx, code)
```

### Get All Intermediate Results

```go
results, err := agent.NewPipeline(client).
    Step("step1", prompt1).
    Step("step2", prompt2).
    RunWithResults(ctx, input)

for _, r := range results {
    fmt.Printf("%s: %d chars\n", r.Name, len(r.Output))
}
final := agent.Final(results)
```

---

## SimpleClient

Direct LLM access without the full `app.App` machinery. Use for pipeline tools, batch processing, and scripts that just need query capability.

### Create and Query

```go
client, err := agent.NewClient(ctx,
    agent.WithClientModel("opus"),
    agent.WithClientSystem("You are a helpful assistant."),
    agent.WithClientRetry(), // Enabled by default
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

response, err := client.Query(ctx, "What is 2+2?")
```

### Query with JSON Parsing

```go
type Answer struct {
    Result int `json:"result"`
}

answer, err := agent.QueryJSON[Answer](client, ctx,
    "What is 2+2? Reply as JSON with 'result' field.")
```

### Query for Array

```go
type Item struct {
    Name string `json:"name"`
}

items, err := agent.QueryJSONArray[Item](client, ctx,
    "List 3 programming languages as JSON array with 'name' field.")
```

### Client Options

| Option | Purpose |
|--------|---------|
| `WithClientModel(m)` | Set model (accepts shortcuts) |
| `WithClientSystem(s)` | Set system prompt |
| `WithClientEnv(env)` | Set environment variables |
| `WithClientRetry()` | Enable retry with backoff (default) |

---

## Prompt Templates

Manage prompts with embedded files and runtime user overrides.

### Setup

```go
//go:embed prompts/*.prompt
var promptsFS embed.FS

prompts := agent.NewPromptRegistry("myapp", promptsFS)
```

File structure:
```
prompts/
  extraction.prompt
  analysis.prompt
  summary.prompt
```

### Simple Format (printf-style)

```go
// extraction.prompt contains:
// Extract entities from this text:
// %s

text, err := prompts.Format("extraction", userInput)
```

### Go Templates

```go
// analysis.prompt contains:
// Analyze {{.Input}} with max {{.MaxItems}} results.

text, err := prompts.Render("analysis", map[string]any{
    "Input":    data,
    "MaxItems": 10,
})
```

### Runtime Override

Users can customize prompts without recompiling:

```
~/.config/myapp/prompts/extraction.prompt
```

Runtime prompts take precedence over embedded prompts.

### Must Variants

```go
text := prompts.MustLoad("system")              // Panic if missing
text := prompts.MustFormat("query", input)      // Panic on error
text := prompts.MustRender("complex", data)     // Panic on error
```

### List Available Prompts

```go
names := prompts.List() // ["extraction", "analysis", "summary"]
```

### Quick One-Liner

For scripts without a registry:

```go
prompt := agent.SimplePrompt("Summarize:\n\n%s", text)
```

---

## Provider Configuration

Built-in support for Claude-compatible providers.

### Built-in Providers

| Provider | Constant | Default Model | Auth |
|----------|----------|---------------|------|
| Anthropic | `ProviderAnthropic` | `claude-opus-4-5-20251101` | `ANTHROPIC_API_KEY` |
| Z.AI | `ProviderZAI` | `GLM-4.7` | `ZAI_API_KEY` |
| Synthetic | `ProviderSynthetic` | `hf:zai-org/GLM-4.7` | `SYNTHETIC_API_KEY` |

### Use Alternate Provider

```go
provider := agent.ProviderZAI

client, err := agent.NewClient(ctx,
    agent.WithClientModel(provider.Model),
    agent.WithClientEnv(agent.ProviderEnv(provider)),
)
```

### Get Provider by Name

```go
provider := agent.GetProvider("zai")
if provider == nil {
    log.Fatal("unknown provider")
}
```

### Custom Provider

```go
myProvider := &agent.Provider{
    Name:       "custom",
    BaseURL:    "https://api.custom.ai/anthropic",
    AuthEnvVar: "CUSTOM_API_KEY",
    Model:      "custom-model-v1",
}
```

---

## Builder Pattern

The builder provides a fluent interface for full control.

### Basic Builder

```go
agent.New("my-agent").
    Model("opus").
    System("You are helpful.").
    MaxTurns(20).
    Budget(5.00).
    Run()
```

### All Builder Methods

**Model & Prompts:**
- `.Model(m)` - Set model (shortcuts: "opus", "sonnet", "haiku")
- `.Fallback(m)` - Fallback model if primary unavailable
- `.System(s)` - Set system prompt
- `.AppendSystem(s)` - Append to system prompt

**Limits:**
- `.MaxTurns(n)` - Limit conversation turns
- `.Budget(usd)` - Spending limit in USD
- `.MaxThinking(tokens)` - Limit thinking tokens
- `.Timeout(d)` - Operation timeout

**Environment:**
- `.WorkDir(path)` - Working directory
- `.AlsoAccess(paths...)` - Additional accessible directories
- `.Context(files...)` - Files always in context
- `.Env(vars)` - Environment variables
- `.User(id)` - User identifier

**Tools:**
- `.Tool(t)` - Add a tool
- `.OnlyTools(names...)` - Whitelist tools
- `.BlockTools(names...)` - Blacklist tools
- `.ClaudeCodeTools()` - Enable standard Claude Code tools

**Sessions:**
- `.Resume(id)` - Continue session
- `.Fork(id)` - Branch from session
- `.Continue()` - Resume most recent
- `.TrackFiles()` - Enable rollback

**Output:**
- `.OutputSchema(schema)` - JSON schema for output
- `.StreamPartial()` - Include partial messages
- `.Beta(features...)` - Enable beta features

**Execution:**
- `.Run()` - Start interactive session
- `.Query(ctx, prompt)` - Single query
- `.Stream(ctx, prompt, handler)` - Stream response

---

## Tools

### Type-Safe Tool

```go
type Input struct {
    Query string `json:"query" desc:"Search query"`
    Limit int    `json:"limit" max:"100"`
}

type Output struct {
    Results []string `json:"results"`
}

searchTool := agent.Tool("search", "Search the database",
    func(ctx context.Context, in Input) (Output, error) {
        results := doSearch(in.Query, in.Limit)
        return Output{Results: results}, nil
    },
)

agent.New("app").Tool(searchTool).Run()
```

### Simple String Tool

```go
upperTool := agent.SimpleTool("upper", "Uppercase text",
    func(s string) (string, error) {
        return strings.ToUpper(s), nil
    },
)
```

### Async Tool

```go
downloadTool := agent.AsyncTool("download", "Download file",
    func(ctx context.Context, url string) error {
        return downloadFile(url)
    },
)
// Returns {"status": "started"} immediately
```

### Tool with Explicit Schema

```go
calcTool := agent.ToolWithSchema("calc", "Evaluate math",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "expr": map[string]any{"type": "string"},
        },
    },
    func(ctx context.Context, input map[string]any) (any, error) {
        return evaluate(input["expr"].(string)), nil
    },
)
```

---

## Hooks & Lifecycle

### Before Tool Use

```go
agent.New("logged").
    OnPreToolUse(func(tool string, input map[string]any) bool {
        log.Printf("Tool: %s", tool)

        // Block dangerous commands
        if tool == "Bash" {
            cmd := input["command"].(string)
            if strings.Contains(cmd, "rm -rf") {
                return false // Block
            }
        }
        return true // Allow
    })
```

### After Tool Use

```go
agent.New("logged").
    OnPostToolUse(func(tool string, result any) {
        log.Printf("%s completed: %v", tool, result)
    })
```

### Session Lifecycle

```go
agent.New("tracked").
    OnSessionStart(func(id string) {
        log.Printf("Started: %s", id)
        saveSessionID(id)
    }).
    OnSessionEnd(func(id string) {
        log.Printf("Ended: %s", id)
    })
```

---

## Sessions & Permissions

### Resume Session

```go
agent.New("app").Resume("session-uuid").Run()
```

### Fork Session

```go
agent.New("app").Fork("session-uuid").Run()
```

### Custom Approval Logic

```go
agent.New("careful").
    RequireApproval(func(tool string, input map[string]any) agent.Approval {
        if tool == "Write" {
            path := input["file_path"].(string)
            if strings.HasPrefix(path, "/etc") {
                return agent.Deny("Cannot modify system files")
            }
        }
        return agent.Allow()
    })
```

### Permission Modes

```go
agent.New("auto").PermissionMode(agent.PermissionAcceptEdits)
```

| Mode | Behavior |
|------|----------|
| `PermissionDefault` | Ask for each action |
| `PermissionAcceptEdits` | Auto-accept file edits |
| `PermissionBypass` | Skip all checks |
| `PermissionPlan` | Plan mode only |

---

## Schema Generation

Generate JSON Schema from Go types for structured output.

```go
type UserProfile struct {
    Name   string   `json:"name" desc:"Full name"`
    Age    int      `json:"age" min:"0" max:"150"`
    Email  string   `json:"email"`
    Tags   []string `json:"tags,omitempty"`
    Active bool     `json:"active"`
}

schema := agent.SchemaFor[UserProfile]()
```

### Supported Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `json:"name"` | Field name | `json:"user_name"` |
| `json:",omitempty"` | Optional | `json:"nick,omitempty"` |
| `desc:"..."` | Description | `desc:"User's email"` |
| `enum:"a,b,c"` | Allowed values | `enum:"admin,user"` |
| `min:"N"` | Minimum | `min:"0"` |
| `max:"N"` | Maximum | `max:"100"` |
| `minLength:"N"` | Min length | `minLength:"1"` |
| `maxLength:"N"` | Max length | `maxLength:"255"` |
| `required:"true"` | Force required | `required:"true"` |

---

## Complete Examples

### Spec Generator (like go/src/spec)

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/agent"
)

func main() {
    ctx := context.Background()

    client, _ := agent.NewClient(ctx,
        agent.WithClientModel("opus"),
        agent.WithClientSystem(systemPrompt),
    )
    defer client.Close()

    spec, _ := agent.NewPipeline(client).
        Step("extract", "Extract requirements:\n\n%s").
        Step("structure", "Add user stories:\n\n%s").
        Step("detail", "Add acceptance criteria:\n\n%s").
        StepWithPost("polish", "Polish for clarity:\n\n%s", agent.ExtractMarkdown).
        OnProgress(func(name string, n, total int) {
            fmt.Printf("[%d/%d] %s\n", n, total, name)
        }).
        Run(ctx, roughNotes)

    fmt.Println(spec)
}
```

### Knowledge Extractor (like go/src/learn)

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/agent"
)

type Insight struct {
    Tier    int    `json:"tier"`
    Pattern string `json:"pattern"`
    Insight string `json:"insight"`
}

func main() {
    ctx := context.Background()

    client, _ := agent.NewClient(ctx, agent.WithClientModel("opus"))
    defer client.Close()

    response, _ := client.Query(ctx,
        "Extract insights from this text:\n\n"+inputText)

    insights, _ := agent.ExtractJSONArray[Insight](response)

    for _, i := range insights {
        if i.Tier >= 2 { // Filter novel insights
            fmt.Printf("[Tier %d] %s: %s\n", i.Tier, i.Pattern, i.Insight)
        }
    }
}
```

### Code Reviewer with Typed Output

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/agent"
)

type Review struct {
    Summary  string  `json:"summary"`
    Issues   []Issue `json:"issues"`
    Score    int     `json:"score" min:"0" max:"100"`
    Approved bool    `json:"approved"`
}

type Issue struct {
    Line     int    `json:"line"`
    Severity string `json:"severity" enum:"critical,major,minor"`
    Message  string `json:"message"`
}

func main() {
    code := `func add(a, b int) int { return a + b }`

    review, _ := agent.QueryAs[Review](context.Background(),
        fmt.Sprintf("Review this Go code:\n```go\n%s\n```", code),
        agent.WithModel("opus"),
    )

    fmt.Printf("Score: %d/100 (%v)\n", review.Score, review.Approved)
    for _, issue := range review.Issues {
        fmt.Printf("  [%s] L%d: %s\n", issue.Severity, issue.Line, issue.Message)
    }
}
```

### Secure Research Agent

```go
package main

import (
    "log"
    "strings"
    "github.com/dotcommander/agent-framework/agent"
)

func main() {
    agent.New("researcher").
        Model("sonnet").
        System("You research topics thoroughly.").
        Budget(2.00).
        MaxTurns(30).
        OnlyTools("Read", "Grep", "Glob", "WebSearch").
        OnPreToolUse(func(tool string, input map[string]any) bool {
            log.Printf("[%s] %v", tool, input)
            return true
        }).
        Run()
}
```

---

## Functional Options

These work with `Run()`, `Query()`, `QueryAs()`, and `Stream()`:

```go
agent.Query(ctx, prompt,
    agent.WithModel("opus"),
    agent.WithSystem("You are an expert."),
    agent.WithMaxTurns(5),
    agent.WithBudget(1.00),
    agent.WithTool(myTool),
    agent.WithWorkDir("/project"),
    agent.WithContext("README.md"),
    agent.WithEnv(map[string]string{"DEBUG": "1"}),
)
```

---

## Error Handling

### Standard

```go
response, err := agent.Query(ctx, prompt)
if err != nil {
    log.Fatalf("Query failed: %v", err)
}
```

### Must (Scripts)

```go
response := agent.Must(agent.Query(ctx, prompt))
```

### Typed Errors

```go
import "github.com/dotcommander/agent-framework/client"

response, err := agent.Query(ctx, prompt)
if err != nil {
    if errors.Is(err, client.ErrRateLimited) {
        // Wait and retry
    }
    var rlErr *client.RateLimitError
    if errors.As(err, &rlErr) {
        time.Sleep(rlErr.RetryAfter)
    }
}
```

---

## Migration from app Package

The `agent` package builds on `app`. Access underlying app for advanced features:

```go
import "github.com/dotcommander/agent-sdk-go/claude"

agent.New("hybrid").
    Model("sonnet").
    SDKOption(claude.WithSomeAdvancedFeature()).
    Run()
```
