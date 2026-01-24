# SDK Feature Exposure

Features from agent-sdk-go that need passthrough in agent-framework.

## P0: Critical (Must Have)

### 1. Hooks System

**SDK**: `claude.WithHook()`, `claude.WithPreToolUseHook()`, etc.

**Expose as**:
```go
// Direct passthrough
agent.New("app").
    OnPreToolUse(func(ctx context.Context, tool string, input map[string]any) (bool, error) {
        if tool == "Bash" && strings.Contains(input["command"].(string), "rm") {
            return false, nil // block
        }
        return true, nil
    }).
    OnPostToolUse(func(ctx context.Context, tool string, result any) {
        log.Printf("Tool %s completed", tool)
    }).
    OnSessionStart(func(ctx context.Context, sessionID string) {
        log.Printf("Session started: %s", sessionID)
    })
```

### 2. Permission Control

**SDK**: `claude.WithCanUseTool()`

**Expose as**:
```go
agent.New("secure").
    RequireApproval(func(tool string, input any) Approval {
        if tool == "Write" {
            return agent.Ask("Allow writing to " + input.(WriteInput).Path + "?")
        }
        return agent.Allow()
    })
```

### 3. Structured Output

**SDK**: `claude.WithJSONSchema()`, `claude.WithOutputFormat()`

**Expose as**:
```go
// Type-inferred schema
type Analysis struct {
    Summary string   `json:"summary"`
    Score   int      `json:"score"`
    Issues  []string `json:"issues"`
}

result, err := agent.QueryAs[Analysis](ctx, "Analyze this code...")

// Or explicit schema
agent.New("analyzer").
    OutputSchema(map[string]any{
        "type": "object",
        "properties": map[string]any{
            "score": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
        },
    })
```

### 4. Session Management

**SDK**: `claude.WithResume()`, `claude.WithForkSession()`, `claude.WithContinue()`

**Expose as**:
```go
// Resume previous session
agent.Resume("session-uuid").Query(ctx, "Continue from where we left off")

// Fork a session (branch conversation)
agent.Fork("session-uuid").Query(ctx, "What if we tried a different approach?")

// Continue most recent
agent.Continue().Query(ctx, "One more thing...")
```

### 5. Sandbox/Security

**SDK**: `claude.WithSandboxSettings()`

**Expose as**:
```go
agent.New("untrusted").
    Sandbox(agent.SandboxDocker).           // or SandboxNsjail
    AllowNetwork(false).
    BlockCommands("rm", "sudo", "chmod").
    Run()

// Convenience preset
agent.New("safe").Sandboxed().Run()  // Sensible secure defaults
```

---

## P1: High Priority

### 6. Budget & Limits

**SDK**: `claude.WithMaxBudgetUSD()`, `claude.WithMaxTurns()`, `claude.WithMaxThinkingTokens()`

**Expose as**:
```go
agent.New("limited").
    Budget(5.00).         // $5 USD max
    MaxTurns(20).         // 20 conversation turns
    MaxThinking(4096).    // Thinking token limit
    Run()

// Or combined
agent.New("limited").
    Limits(agent.Limits{
        BudgetUSD:      5.00,
        MaxTurns:       20,
        ThinkingTokens: 4096,
        Timeout:        10 * time.Minute,
    })
```

### 7. Fallback Models

**SDK**: `claude.WithFallbackModel()`

**Expose as**:
```go
agent.New("reliable").
    Model("opus").
    Fallback("sonnet").  // Use sonnet if opus unavailable
    Run()
```

### 8. File Checkpointing

**SDK**: `claude.WithFileCheckpointing()`

**Expose as**:
```go
agent.New("careful").
    TrackFiles().  // Enable file change tracking
    Run()

// Then in code:
agent.RewindFiles()  // Undo all file changes
```

### 9. Custom Subagents

**SDK**: `claude.WithAgents()`

**Expose as**:
```go
agent.New("orchestrator").
    Subagent("coder", agent.SubagentConfig{
        Model:       "sonnet",
        Tools:       []string{"Read", "Write", "Bash"},
        Description: "Writes and modifies code",
    }).
    Subagent("reviewer", agent.SubagentConfig{
        Model:       "opus",
        Tools:       []string{"Read"},
        Description: "Reviews code for issues",
    }).
    Run()
```

### 10. MCP Servers

**SDK**: `claude.WithMcpServers()`

**Expose as**:
```go
agent.New("extended").
    MCP("filesystem", agent.MCPStdio{
        Command: "npx",
        Args:    []string{"-y", "@anthropic/mcp-fs"},
    }).
    MCP("database", agent.MCPHTTP{
        URL: "http://localhost:3000/mcp",
    }).
    Run()
```

### 11. Permission Modes

**SDK**: `claude.WithPermissionMode()`

**Expose as**:
```go
agent.New("autonomous").
    PermissionMode(agent.PermissionAcceptEdits).  // Auto-accept file edits
    Run()

agent.New("interactive").
    PermissionMode(agent.PermissionDefault).      // Ask for each action
    Run()

agent.New("planning").
    PermissionMode(agent.PermissionPlan).         // Plan mode
    Run()
```

### 12. Working Directory

**SDK**: `claude.WithWorkingDirectory()`, `claude.WithAdditionalDirectories()`

**Expose as**:
```go
agent.New("project").
    WorkDir("/path/to/project").
    AlsoAccess("/path/to/libs", "/path/to/data").
    Run()
```

### 13. Context Files

**SDK**: `claude.WithContextFiles()`

**Expose as**:
```go
agent.New("informed").
    Context("README.md", "ARCHITECTURE.md").
    Run()
```

### 14. Tool Restrictions

**SDK**: `claude.WithDisallowedTools()`, `claude.WithClaudeCodeTools()`

**Expose as**:
```go
// Whitelist
agent.New("limited").
    OnlyTools("Read", "Grep", "Glob").
    Run()

// Blacklist
agent.New("safe").
    BlockTools("Bash", "Write").
    Run()

// Preset
agent.New("claude-code").
    ClaudeCodeTools().
    Run()
```

---

## P2: Medium Priority

### 15. Beta Features

**SDK**: `claude.WithBetas()`

**Expose as**:
```go
agent.New("experimental").
    Beta("context-1m-2025-08-07").
    Run()
```

### 16. Partial Messages (Streaming)

**SDK**: `claude.WithIncludePartialMessages()`

**Expose as**:
```go
agent.New("streaming").
    StreamPartial().  // Include incomplete messages in stream
    Stream(ctx, prompt, func(chunk string) {
        fmt.Print(chunk)
    })
```

### 17. Debug Output

**SDK**: `claude.WithDebugWriter()`, `claude.WithStderrCallback()`

**Expose as**:
```go
var debugLog bytes.Buffer
agent.New("debug").
    Debug(&debugLog).
    Run()

// Or callback
agent.New("debug").
    OnStderr(func(line string) {
        log.Printf("CLAUDE: %s", line)
    }).
    Run()
```

### 18. User Tracking

**SDK**: `claude.WithUser()`

**Expose as**:
```go
agent.New("multi-tenant").
    User("user-12345").  // For usage tracking
    Run()
```

### 19. Environment Variables

**SDK**: `claude.WithEnv()`

**Expose as**:
```go
agent.New("configured").
    Env(map[string]string{
        "API_KEY":    os.Getenv("API_KEY"),
        "DEBUG":      "true",
    }).
    Run()
```

---

## Implementation Strategy

### Option 1: Direct Passthrough (Fastest)

```go
// In agent/options.go
func (b *Builder) OnPreToolUse(fn PreToolUseFunc) *Builder {
    b.sdkOpts = append(b.sdkOpts, claude.WithPreToolUseHook(adaptHook(fn)))
    return b
}
```

### Option 2: Simplified Wrapper (Better DX)

```go
// Wrap SDK types with simpler signatures
type PreToolUseFunc func(tool string, input map[string]any) bool

func (b *Builder) OnPreToolUse(fn PreToolUseFunc) *Builder {
    b.sdkOpts = append(b.sdkOpts, claude.WithPreToolUseHook(
        func(ctx context.Context, input *shared.PreToolUseHookInput) (*shared.SyncHookOutput, error) {
            if fn(input.ToolName, input.ToolInput) {
                return &shared.SyncHookOutput{Continue: true}, nil
            }
            return &shared.SyncHookOutput{Decision: "block"}, nil
        },
    ))
    return b
}
```

### Option 3: Both (Maximum Flexibility)

```go
// Simple version
func (b *Builder) OnPreToolUse(fn func(tool string, input map[string]any) bool) *Builder

// Full SDK version for power users
func (b *Builder) OnPreToolUseRaw(fn func(context.Context, *shared.PreToolUseHookInput) (*shared.SyncHookOutput, error)) *Builder
```

---

## Full Builder Interface

```go
type Builder interface {
    // Identity
    Name(string) Builder
    Version(string) Builder

    // Model
    Model(string) Builder              // "opus", "sonnet", "haiku", or full name
    Fallback(string) Builder

    // Prompts
    System(string) Builder
    AppendSystem(string) Builder

    // Tools
    Tool(ToolDef) Builder
    OnlyTools(...string) Builder
    BlockTools(...string) Builder
    ClaudeCodeTools() Builder

    // Hooks
    OnPreToolUse(func(string, map[string]any) bool) Builder
    OnPostToolUse(func(string, any)) Builder
    OnSessionStart(func(string)) Builder
    OnSessionEnd(func(string)) Builder
    RequireApproval(func(string, any) Approval) Builder

    // Sessions
    Resume(string) Builder
    Fork(string) Builder
    Continue() Builder

    // Limits
    Budget(float64) Builder
    MaxTurns(int) Builder
    MaxThinking(int) Builder
    Timeout(time.Duration) Builder

    // Security
    Sandbox(SandboxType) Builder
    Sandboxed() Builder  // Sensible defaults
    AllowNetwork(bool) Builder
    BlockCommands(...string) Builder
    PermissionMode(PermissionMode) Builder

    // Environment
    WorkDir(string) Builder
    AlsoAccess(...string) Builder
    Context(...string) Builder
    Env(map[string]string) Builder

    // Extensions
    MCP(name string, config MCPConfig) Builder
    Subagent(name string, config SubagentConfig) Builder
    Beta(...string) Builder

    // Output
    OutputSchema(map[string]any) Builder
    StreamPartial() Builder

    // Files
    TrackFiles() Builder

    // Debug
    Debug(io.Writer) Builder
    OnStderr(func(string)) Builder

    // Multi-tenant
    User(string) Builder

    // Execution
    Run() error
    Query(context.Context, string) (string, error)
    Stream(context.Context, string, func(string)) error
}
```

---

## What This Enables

### Before (Raw SDK)
```go
client, err := claude.NewClient(
    claude.WithModel("claude-sonnet-4-20250514"),
    claude.WithSystemPrompt("You are a code reviewer."),
    claude.WithPreToolUseHook(func(ctx context.Context, input *shared.PreToolUseHookInput) (*shared.SyncHookOutput, error) {
        if input.ToolName == "Bash" {
            return &shared.SyncHookOutput{Decision: "block", Reason: "Bash not allowed"}, nil
        }
        return &shared.SyncHookOutput{Continue: true}, nil
    }),
    claude.WithMaxTurns(20),
    claude.WithMaxBudgetUSD(5.00),
    claude.WithWorkingDirectory("/path/to/project"),
    claude.WithDisallowedTools("Write"),
)
```

### After (Agent Framework)
```go
agent.New("reviewer").
    Model("sonnet").
    System("You are a code reviewer.").
    OnPreToolUse(func(tool string, _ map[string]any) bool {
        return tool != "Bash"
    }).
    MaxTurns(20).
    Budget(5.00).
    WorkDir("/path/to/project").
    BlockTools("Write").
    Run()
```

**Line count**: 15 → 10 (33% reduction)
**Cognitive load**: Much lower (no SDK types, no error handling boilerplate)
