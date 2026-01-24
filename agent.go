// Package agent provides a reusable framework for building Claude-powered CLI tools.
//
// The framework is designed to be composable, allowing users to import packages
// and wire together custom CLI applications. It provides:
//
// - Multi-provider support (Anthropic, Z.AI, Synthetic)
// - Automatic input detection and processing (URLs, files, globs, text)
// - Flexible output formatting (JSON, Markdown, text)
// - Type-safe tool registration
// - CLI scaffolding with cobra
//
// Example usage:
//
//	import "github.com/dotcommander/agent-framework/app"
//
//	func main() {
//	    a := app.New("myapp", "1.0.0",
//	        app.WithSystemPrompt("You are a helpful assistant"),
//	        app.WithModel("claude-sonnet-4-20250514"),
//	    )
//	    a.Run()
//	}
package agent

import (
	"context"

	// Re-export key types for convenience
	"github.com/dotcommander/agent-framework/app"
	"github.com/dotcommander/agent-framework/client"
	"github.com/dotcommander/agent-framework/config"
	"github.com/dotcommander/agent-framework/tools"
)

// Version is the framework version.
const Version = "1.0.0"

// Convenience type aliases for common use
type (
	// App is the main application type.
	App = app.App

	// Querier is the minimal interface for querying an LLM.
	Querier = client.Querier

	// StreamingQuerier extends Querier with streaming support.
	StreamingQuerier = client.StreamingQuerier

	// Client is the AI client interface (alias for StreamingQuerier).
	// Deprecated: Use Querier or StreamingQuerier for narrower contracts.
	Client = client.Client

	// Config is the application configuration.
	Config = config.Config

	// Tool is a tool available to the AI.
	Tool = tools.Tool

	// Handler is a tool handler function.
	Handler = tools.Handler
)

// Convenience constructors
var (
	// NewApp creates a new application.
	NewApp = app.New

	// NewTool creates a new tool.
	NewTool = tools.NewTool
)

// TypedHandler creates a type-safe tool handler.
// Use this with NewTool for type-safe tools.
//
// Example:
//
//	handler := agent.TypedHandler(func(ctx context.Context, in MyInput) (MyOutput, error) { ... })
//	tool := agent.NewTool("name", "desc", schema, handler)
func TypedHandler[T any, R any](fn func(ctx context.Context, input T) (R, error)) Handler {
	return tools.TypedHandler(fn)
}

// TypedTool creates a type-safe tool with typed input/output.
//
// Example:
//
//	tool := agent.TypedTool[MyInput, MyOutput]("name", "desc", schema, myHandler)
func TypedTool[T any, R any](name, description string, schema map[string]any, fn func(ctx context.Context, input T) (R, error)) *Tool {
	return tools.TypedTool(name, description, schema, fn)
}

// Convenience options
var (
	// WithSystemPrompt sets the system prompt.
	WithSystemPrompt = app.WithSystemPrompt

	// WithModel sets the AI model.
	WithModel = app.WithModel

	// WithProvider sets the AI provider.
	WithProvider = app.WithProvider

	// WithTool adds a tool.
	WithTool = app.WithTool

	// WithRunFunc sets a custom run function.
	WithRunFunc = app.WithRunFunc
)

// Agent Loop types for building agentic workflows.
// Pattern: gather context → take action → verify → repeat
type (
	// AgentLoop defines the core agent loop interface.
	AgentLoop = app.AgentLoop

	// LoopRunner executes an agent loop.
	LoopRunner = app.LoopRunner

	// LoopConfig configures agent loop behavior.
	LoopConfig = app.LoopConfig

	// LoopState represents current loop iteration state.
	LoopState = app.LoopState

	// LoopContext holds gathered context for an iteration.
	LoopContext = app.LoopContext

	// Action represents an action to take.
	Action = app.Action

	// Result represents the outcome of an action.
	Result = app.Result

	// Feedback represents verification feedback.
	Feedback = app.Feedback
)

// Subagent types for parallel agent execution.
type (
	// Subagent represents an isolated child agent.
	Subagent = app.Subagent

	// SubagentManager coordinates multiple subagents.
	SubagentManager = app.SubagentManager

	// SubagentConfig configures subagent behavior.
	SubagentConfig = app.SubagentConfig

	// SubagentContext holds isolated context for a subagent.
	SubagentContext = app.SubagentContext

	// SubagentResult contains the outcome of a subagent's work.
	SubagentResult = app.SubagentResult

	// SubagentExecutor defines how to run a subagent.
	SubagentExecutor = app.SubagentExecutor

	// SubagentExecutorFunc is a function adapter.
	SubagentExecutorFunc = app.SubagentExecutorFunc

	// SubagentOption configures a subagent.
	SubagentOption = app.SubagentOption
)

// Agent Loop constructors
var (
	// NewLoopRunner creates a loop runner.
	NewLoopRunner = app.NewLoopRunner

	// NewSimpleLoop creates a configurable simple loop.
	NewSimpleLoop = app.NewSimpleLoop

	// DefaultLoopConfig returns sensible loop defaults.
	DefaultLoopConfig = app.DefaultLoopConfig
)

// SimpleLoop configuration options
var (
	// WithGatherFunc sets context gathering function.
	WithGatherFunc = app.WithGatherFunc

	// WithDecideFunc sets action decision function.
	WithDecideFunc = app.WithDecideFunc

	// WithActionFunc sets action execution function.
	WithActionFunc = app.WithActionFunc

	// WithVerifyFunc sets verification function.
	WithVerifyFunc = app.WithVerifyFunc

	// WithContinueFunc sets continuation check function.
	WithContinueFunc = app.WithContinueFunc
)

// Subagent constructors
var (
	// NewSubagentManager creates a subagent coordinator.
	NewSubagentManager = app.NewSubagentManager

	// DefaultSubagentConfig returns sensible subagent defaults.
	DefaultSubagentConfig = app.DefaultSubagentConfig
)

// Subagent options
var (
	// WithSubagentPrompt sets the subagent's system prompt.
	WithSubagentPrompt = app.WithSubagentPrompt

	// WithSubagentTools sets available tools.
	WithSubagentTools = app.WithSubagentTools

	// WithSubagentState sets initial state.
	WithSubagentState = app.WithSubagentState

	// WithSubagentMessages sets initial messages.
	WithSubagentMessages = app.WithSubagentMessages

	// WithSubagentMaxTokens sets the token limit.
	WithSubagentMaxTokens = app.WithSubagentMaxTokens
)

// Subagent result utilities
var (
	// FilterResults filters subagent results by success.
	FilterResults = app.FilterResults

	// MergeResults combines outputs from multiple results.
	MergeResults = app.MergeResults

	// AggregateTokens sums token usage across results.
	AggregateTokens = app.AggregateTokens
)
