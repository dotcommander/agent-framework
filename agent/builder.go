package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// Builder provides a fluent interface for configuring agents.
type Builder struct {
	name    string
	version string

	// SDK options accumulated
	sdkOpts []claude.ClientOption

	// Tools to register
	tools []ToolDef

	// Internal flags
	queryMode bool
}

// New creates a new agent builder with the given name.
//
// Example:
//
//	agent.New("code-reviewer").
//	    Model("opus").
//	    System("You review code for bugs.").
//	    Run()
func New(name string) *Builder {
	return &Builder{
		name:    name,
		version: "1.0.0",
		sdkOpts: []claude.ClientOption{
			claude.WithModel(DefaultModel), // sensible default
		},
	}
}

// Apply applies multiple options at once.
func (b *Builder) Apply(opts ...Option) *Builder {
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Version sets the agent version.
func (b *Builder) Version(v string) *Builder {
	b.version = v
	return b
}

// Model sets the AI model. Accepts shortcuts: "opus", "sonnet", "haiku".
//
// Example:
//
//	agent.New("app").Model("opus")        // claude-opus-4-20250514
//	agent.New("app").Model("sonnet")      // claude-sonnet-4-20250514
//	agent.New("app").Model("haiku")       // claude-haiku-3-5-20241022
func (b *Builder) Model(m string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithModel(ExpandModel(m)))
	return b
}

// Fallback sets a fallback model if primary is unavailable.
func (b *Builder) Fallback(m string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithFallbackModel(ExpandModel(m)))
	return b
}

// System sets the system prompt.
func (b *Builder) System(prompt string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithSystemPrompt(prompt))
	return b
}

// AppendSystem appends to the system prompt.
func (b *Builder) AppendSystem(prompt string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithAppendSystemPrompt(prompt))
	return b
}

// Tool adds a tool to the agent.
func (b *Builder) Tool(t ToolDef) *Builder {
	b.tools = append(b.tools, t)
	return b
}

// MaxTurns limits the number of conversation turns.
func (b *Builder) MaxTurns(n int) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithMaxTurns(n))
	return b
}

// Budget sets a spending limit in USD.
func (b *Builder) Budget(usd float64) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithMaxBudgetUSD(usd))
	return b
}

// MaxThinking limits thinking tokens.
func (b *Builder) MaxThinking(tokens int) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithMaxThinkingTokens(tokens))
	return b
}

// Timeout sets the operation timeout.
func (b *Builder) Timeout(d time.Duration) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithTimeout(d.String()))
	return b
}

// WorkDir sets the working directory.
func (b *Builder) WorkDir(path string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithWorkingDirectory(path))
	return b
}

// AlsoAccess adds additional directories the agent can access.
func (b *Builder) AlsoAccess(paths ...string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithAdditionalDirectories(paths...))
	return b
}

// Context adds files to the agent's context.
func (b *Builder) Context(files ...string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithContextFiles(files...))
	return b
}

// Resume continues a previous session.
func (b *Builder) Resume(sessionID string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithResume(sessionID))
	return b
}

// Fork creates a branched conversation from a session.
func (b *Builder) Fork(sessionID string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithForkSession(sessionID))
	return b
}

// Continue resumes the most recent session.
func (b *Builder) Continue() *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithContinue())
	return b
}

// OnlyTools restricts the agent to specific tools (whitelist).
func (b *Builder) OnlyTools(tools ...string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithAllowedTools(tools...))
	return b
}

// BlockTools prevents the agent from using specific tools (blacklist).
func (b *Builder) BlockTools(tools ...string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithDisallowedTools(tools...))
	return b
}

// ClaudeCodeTools enables standard Claude Code tools.
func (b *Builder) ClaudeCodeTools() *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithClaudeCodeTools())
	return b
}

// PermissionMode sets how permissions are handled.
func (b *Builder) PermissionMode(mode PermissionMode) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithPermissionMode(string(mode)))
	return b
}

// TrackFiles enables file change tracking for rollback.
func (b *Builder) TrackFiles() *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithFileCheckpointing())
	return b
}

// Beta enables experimental beta features.
func (b *Builder) Beta(features ...string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithBetas(features...))
	return b
}

// StreamPartial includes partial/incomplete messages in streams.
func (b *Builder) StreamPartial() *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithIncludePartialMessages(true))
	return b
}

// User sets a user identifier for multi-tenant tracking.
func (b *Builder) User(userID string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithUser(userID))
	return b
}

// Env sets environment variables for the agent subprocess.
func (b *Builder) Env(vars map[string]string) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithEnv(vars))
	return b
}

// OutputSchema sets a JSON schema for structured output.
func (b *Builder) OutputSchema(schema map[string]any) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithJSONSchema(schema))
	return b
}

// OnPreToolUse registers a hook called before each tool use.
// Return false to block the tool call.
func (b *Builder) OnPreToolUse(fn func(tool string, input map[string]any) bool) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithPreToolUseHook(
		func(ctx context.Context, input *claude.PreToolUseHookInput) (*claude.SyncHookOutput, error) {
			if fn(input.ToolName, input.ToolInput) {
				return &claude.SyncHookOutput{Continue: true}, nil
			}
			return &claude.SyncHookOutput{Decision: "block"}, nil
		},
	))
	return b
}

// OnPostToolUse registers a hook called after each tool use.
func (b *Builder) OnPostToolUse(fn func(tool string, result any)) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithPostToolUseHook(
		func(ctx context.Context, input *claude.PostToolUseHookInput) (*claude.SyncHookOutput, error) {
			fn(input.ToolName, input.ToolResponse)
			return &claude.SyncHookOutput{Continue: true}, nil
		},
	))
	return b
}

// OnSessionStart registers a hook called when a session starts.
func (b *Builder) OnSessionStart(fn func(sessionID string)) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithSessionStartHook(
		func(ctx context.Context, input *claude.SessionStartHookInput) (*claude.SyncHookOutput, error) {
			fn(input.SessionID)
			return &claude.SyncHookOutput{Continue: true}, nil
		},
	))
	return b
}

// OnSessionEnd registers a hook called when a session ends.
func (b *Builder) OnSessionEnd(fn func(sessionID string)) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithSessionEndHook(
		func(ctx context.Context, input *claude.SessionEndHookInput) (*claude.SyncHookOutput, error) {
			fn(input.SessionID)
			return &claude.SyncHookOutput{Continue: true}, nil
		},
	))
	return b
}

// RequireApproval sets a callback for runtime tool approval.
func (b *Builder) RequireApproval(fn func(tool string, input map[string]any) Approval) *Builder {
	b.sdkOpts = append(b.sdkOpts, claude.WithCanUseTool(
		func(ctx context.Context, toolName string, toolInput map[string]any, opts claude.CanUseToolOptions) (claude.PermissionResult, error) {
			approval := fn(toolName, toolInput)
			switch approval.decision {
			case approvalAllow:
				return claude.NewPermissionResultAllow(), nil
			case approvalDeny:
				return claude.NewPermissionResultDeny(approval.reason), nil
			default:
				return claude.NewPermissionResultAllow(), nil
			}
		},
	))
	return b
}

// Run starts the agent interactively.
func (b *Builder) Run() error {
	return b.buildApp().Run()
}

// Query sends a prompt and returns the response.
func (b *Builder) Query(ctx context.Context, prompt string) (string, error) {
	b.queryMode = true
	a := b.buildApp()

	// We need to execute with the prompt as an argument
	// For now, use a simple approach
	a.RootCmd().SetArgs([]string{prompt})
	return "", a.Run()
}

// Stream sends a prompt and streams the response.
func (b *Builder) Stream(ctx context.Context, prompt string, handler func(chunk string)) error {
	// Enable partial messages for streaming
	b.StreamPartial()

	// TODO: Implement proper streaming with handler
	// For now, fall back to Query
	response, err := b.Query(ctx, prompt)
	if err != nil {
		return err
	}
	handler(response)
	return nil
}

// SDKOption adds a raw SDK option for advanced use cases.
func (b *Builder) SDKOption(opt claude.ClientOption) *Builder {
	b.sdkOpts = append(b.sdkOpts, opt)
	return b
}

// Print utility for debugging builder state
func (b *Builder) String() string {
	return fmt.Sprintf("Agent{name=%s, version=%s, tools=%d, sdkOpts=%d}",
		b.name, b.version, len(b.tools), len(b.sdkOpts))
}
