// Package agent provides a convenience layer for building AI agents.
//
// This package is the "Laravel layer" - syntactic sugar that makes
// building agents as simple as possible while retaining full power.
//
// Quick start (1 line):
//
//	agent.Run("You are a helpful assistant.")
//
// Query and get response:
//
//	response, err := agent.Query(ctx, "What is 2+2?")
//
// Typed responses:
//
//	type Answer struct {
//	    Result int    `json:"result"`
//	    Reason string `json:"reason"`
//	}
//	answer, err := agent.QueryAs[Answer](ctx, "What is 2+2?")
//
// Fluent builder:
//
//	agent.New("assistant").
//	    Model("opus").
//	    System("You are helpful.").
//	    MaxTurns(10).
//	    Run()
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/agent-framework/app"
	"github.com/dotcommander/agent-sdk-go/claude"
)

// Run starts an interactive agent with the given system prompt.
// This is the simplest possible entry point.
//
// Example:
//
//	agent.Run("You are a helpful coding assistant.")
func Run(systemPrompt string, opts ...Option) error {
	return New("agent").System(systemPrompt).Apply(opts...).Run()
}

// Query sends a single prompt and returns the response.
// For scripts and one-shot tasks.
//
// Example:
//
//	response, err := agent.Query(ctx, "Explain goroutines in one sentence.")
func Query(ctx context.Context, prompt string, opts ...Option) (string, error) {
	resp, err := New("agent").Apply(opts...).QueryResponse(ctx, prompt)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// QueryResponse sends a prompt and returns a rich response with metadata.
//
// Example:
//
//	resp, err := agent.QueryResponse(ctx, "What is 2+2?")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(resp.Content)
//	if resp.Usage != nil {
//	    fmt.Printf("Tokens: %d\n", resp.Usage.TotalTokens)
//	}
func QueryResponse(ctx context.Context, prompt string, opts ...Option) (*Response, error) {
	return New("agent").Apply(opts...).QueryResponse(ctx, prompt)
}

// QueryAs sends a prompt and returns a typed response.
// The response is parsed as JSON into the type T.
//
// When validation rules are provided via WithValidation(), QueryAs automatically
// retries up to WithMaxRetries() times (default 3), feeding validation errors
// back to the LLM for self-correction.
//
// Example (basic):
//
//	type Summary struct {
//	    Title  string   `json:"title"`
//	    Points []string `json:"points"`
//	}
//	summary, err := agent.QueryAs[Summary](ctx, "Summarize this article...")
//
// Example (with validation):
//
//	type User struct {
//	    Age int `json:"age"`
//	}
//	user, err := agent.QueryAs[User](ctx, prompt,
//	    agent.WithValidation(validation.Range("age", 18, 150)),
//	    agent.WithMaxRetries(3),
//	)
func QueryAs[T any](ctx context.Context, prompt string, opts ...any) (*T, error) {
	// Separate query options from builder options
	var qopts queryOptions
	qopts.maxRetries = 3 // default

	var builderOpts []Option

	for _, opt := range opts {
		switch v := opt.(type) {
		case QueryOption:
			v(&qopts)
		case Option:
			builderOpts = append(builderOpts, v)
		}
	}

	// If no validation rules, use simple path (no retries needed)
	if len(qopts.rules) == 0 {
		return queryAsSimple[T](ctx, prompt, builderOpts)
	}

	// Self-healing path with validation and retries
	return queryAsWithValidation[T](ctx, prompt, builderOpts, &qopts)
}

// queryAsSimple is the original QueryAs logic without validation.
func queryAsSimple[T any](ctx context.Context, prompt string, opts []Option) (*T, error) {
	b := New("agent").Apply(opts...)

	// Add JSON schema for type T
	schema := SchemaFor[T]()
	b.sdkOpts = append(b.sdkOpts, claude.WithJSONSchema(schema))

	response, err := b.Query(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var result T
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("parse response as %T: %w", result, err)
	}

	return &result, nil
}

// queryAsWithValidation implements self-healing QueryAs with validation retries.
func queryAsWithValidation[T any](ctx context.Context, prompt string, opts []Option, qopts *queryOptions) (*T, error) {
	var lastResponse string
	var lastErrors []string

	for attempt := 0; attempt <= qopts.maxRetries; attempt++ {
		currentPrompt := prompt
		if attempt > 0 && len(lastErrors) > 0 {
			currentPrompt = buildRetryPrompt(prompt, lastResponse, lastErrors)
		}

		b := New("agent").Apply(opts...)

		schema := SchemaFor[T]()
		b.sdkOpts = append(b.sdkOpts, claude.WithJSONSchema(schema))

		response, err := b.Query(ctx, currentPrompt)
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}

		lastResponse = response

		// Parse JSON
		var result T
		if err := json.Unmarshal([]byte(response), &result); err != nil {
			lastErrors = []string{fmt.Sprintf("JSON parse error: %v", err)}
			continue
		}

		// Run validation
		errors := validateStruct(&result, qopts.rules)
		if len(errors) > 0 {
			lastErrors = errors
			continue
		}

		return &result, nil
	}

	return nil, fmt.Errorf("validation failed after %d attempts: %s", qopts.maxRetries+1, strings.Join(lastErrors, "; "))
}

// Stream sends a prompt and streams the response chunks.
//
// Example:
//
//	err := agent.Stream(ctx, "Write a poem about Go.", func(chunk string) {
//	    fmt.Print(chunk)
//	})
func Stream(ctx context.Context, prompt string, handler func(chunk string), opts ...Option) error {
	return New("agent").Apply(opts...).Stream(ctx, prompt, handler)
}

// Must wraps a (T, error) return and panics on error.
// Useful for scripts where error handling is verbose.
//
// Example:
//
//	response := agent.Must(agent.Query(ctx, "Hello"))
func Must[T any](val T, err error) T {
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		os.Exit(1)
	}
	return val
}

// defaultApp creates an app.App from builder settings.
func (b *Builder) buildApp() *app.App {
	opts := []app.Option{}

	// Apply SDK options
	for _, sdkOpt := range b.sdkOpts {
		opts = append(opts, app.WithSDKOption(sdkOpt))
	}

	// Apply tools (wrap with state if configured)
	for _, tool := range b.tools {
		wrappedTool := wrapToolWithState(tool, b.state)
		opts = append(opts, app.WithTool(wrappedTool))
	}

	// Set run function for query mode
	if b.queryMode {
		opts = append(opts, app.WithRunFunc(func(ctx context.Context, a *app.App, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no prompt provided")
			}
			response, err := a.Client().Query(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Println(response)
			return nil
		}))
	}

	return app.New(b.name, b.version, opts...)
}
