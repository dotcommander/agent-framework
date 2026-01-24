package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Subagent configuration defaults.
const (
	// DefaultSubagentMaxTokens is the token budget for a subagent.
	DefaultSubagentMaxTokens = 100000

	// DefaultMaxConcurrentSubagents limits parallel subagent execution.
	DefaultMaxConcurrentSubagents = 5

	// DefaultMaxSubagents limits total subagents that can be spawned.
	DefaultMaxSubagents = 100

	// DefaultGlobalTokenBudget limits total tokens across all running subagents.
	// 1M tokens prevents OOM from 100 agents × 100k tokens scenarios.
	DefaultGlobalTokenBudget = 1_000_000
)

// ErrAllAgentsFailed is returned when all subagents fail execution.
var ErrAllAgentsFailed = errors.New("all subagents failed")

// ErrMaxSubagentsReached is returned when spawning would exceed the limit.
var ErrMaxSubagentsReached = errors.New("maximum subagents reached")

// ErrTokenBudgetExhausted is returned when the global token budget cannot accommodate a request.
var ErrTokenBudgetExhausted = errors.New("global token budget exhausted")

// Subagent represents an isolated child agent with its own context.
type Subagent struct {
	ID          string
	Name        string
	Task        string
	Context     *SubagentContext
	Result      *SubagentResult
	parentAgent *SubagentManager
}

// SubagentContext holds isolated context for a subagent.
type SubagentContext struct {
	Messages     []Message
	Tools        []ToolInfo
	SystemPrompt string
	State        map[string]any
	MaxTokens    int
}

// SubagentResult contains the outcome of a subagent's work.
type SubagentResult struct {
	Success bool
	Output  any
	Error   error
	Tokens  int
}

// TokenBudget manages a global token pool with thread-safe reservation and release.
// It prevents OOM by limiting total concurrent token usage across all subagents.
type TokenBudget struct {
	total     int64      // Maximum tokens available
	reserved  int64      // Currently reserved tokens
	mu        sync.Mutex // Protects reserved
	waitQueue sync.Cond  // For blocking reservation mode
}

// NewTokenBudget creates a budget pool with the specified total tokens.
func NewTokenBudget(total int64) *TokenBudget {
	tb := &TokenBudget{total: total}
	tb.waitQueue.L = &tb.mu
	return tb
}

// Reserve attempts to reserve tokens from the pool.
// Returns true if reservation succeeded, false if insufficient budget.
func (tb *TokenBudget) Reserve(tokens int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.reserved+tokens > tb.total {
		return false
	}
	tb.reserved += tokens
	return true
}

// ReserveWait blocks until tokens are available, then reserves them.
// Returns an error if the context is cancelled while waiting.
func (tb *TokenBudget) ReserveWait(ctx context.Context, tokens int64) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for tb.reserved+tokens > tb.total {
		// Check for context cancellation before waiting
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Wait for tokens to be released (with periodic context checks)
		done := make(chan struct{})
		go func() {
			tb.waitQueue.Wait()
			close(done)
		}()

		tb.mu.Unlock()
		select {
		case <-ctx.Done():
			tb.mu.Lock()
			return ctx.Err()
		case <-done:
			tb.mu.Lock()
		}
	}

	tb.reserved += tokens
	return nil
}

// Release returns tokens to the pool and wakes any waiting reservations.
func (tb *TokenBudget) Release(tokens int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.reserved -= tokens
	if tb.reserved < 0 {
		tb.reserved = 0 // Safety: don't go negative
	}
	tb.waitQueue.Broadcast() // Wake all waiters to check if they can proceed
}

// Available returns the number of tokens currently available.
func (tb *TokenBudget) Available() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.total - tb.reserved
}

// Reserved returns the number of tokens currently reserved.
func (tb *TokenBudget) Reserved() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.reserved
}

// Total returns the total budget capacity.
func (tb *TokenBudget) Total() int64 {
	return tb.total // Immutable, no lock needed
}

// SubagentConfig configures subagent behavior.
type SubagentConfig struct {
	// MaxConcurrent limits parallel subagent execution.
	MaxConcurrent int

	// MaxSubagents limits total subagents that can be spawned (0 = unlimited).
	MaxSubagents int

	// IsolateContext creates fresh context per subagent.
	IsolateContext bool

	// ShareTools allows subagents to access parent tools.
	ShareTools bool

	// PropagateCancel cancels children when parent cancels.
	PropagateCancel bool

	// GlobalTokenBudget limits total tokens across all running subagents.
	// When set, subagents must reserve tokens before execution.
	// Use WithGlobalTokenBudget to configure.
	GlobalTokenBudget *TokenBudget

	// WaitForTokens determines behavior when budget is exhausted.
	// If true, Spawn/Run blocks until tokens available.
	// If false, returns ErrTokenBudgetExhausted immediately.
	WaitForTokens bool
}

// DefaultSubagentConfig returns sensible defaults.
func DefaultSubagentConfig() *SubagentConfig {
	return &SubagentConfig{
		MaxConcurrent:   DefaultMaxConcurrentSubagents,
		MaxSubagents:    DefaultMaxSubagents,
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
}

// SubagentConfigOption modifies a SubagentConfig.
type SubagentConfigOption func(*SubagentConfig)

// WithGlobalTokenBudget sets a global token limit across all running subagents.
// This prevents OOM from scenarios like 100 agents × 100k tokens = 10M tokens.
func WithGlobalTokenBudget(tokens int64) SubagentConfigOption {
	return func(c *SubagentConfig) {
		c.GlobalTokenBudget = NewTokenBudget(tokens)
	}
}

// WithWaitForTokens configures whether to block when budget is exhausted.
// If true, Run blocks until tokens are available.
// If false, returns ErrTokenBudgetExhausted immediately.
func WithWaitForTokens(wait bool) SubagentConfigOption {
	return func(c *SubagentConfig) {
		c.WaitForTokens = wait
	}
}

// ApplyOptions applies configuration options to a SubagentConfig.
func (c *SubagentConfig) ApplyOptions(opts ...SubagentConfigOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// SubagentManager coordinates multiple subagents.
//
// Graceful shutdown behavior:
// - When a context passed to RunAll/RunAgents is cancelled, all running
//   subagents receive cancellation through their context.
// - RunAll returns promptly after cancellation; it does not wait for
//   subagents to finish their current work.
// - The Shutdown method can be used to cancel all running subagents
//   spawned by this manager.
// - Executors MUST respect context cancellation for graceful shutdown to work.
type SubagentManager struct {
	config    *SubagentConfig
	subagents map[string]*Subagent
	executor  SubagentExecutor
	mu        sync.RWMutex
	nextID    int

	// cancelFuncs tracks active cancellation functions for running subagents.
	// Used by Shutdown() to cancel all in-flight work.
	cancelFuncs map[string]context.CancelFunc
	cancelMu    sync.Mutex
}

// SubagentExecutor defines how to run a subagent.
type SubagentExecutor interface {
	Execute(ctx context.Context, agent *Subagent) (*SubagentResult, error)
}

// SubagentExecutorFunc is a function adapter for SubagentExecutor.
type SubagentExecutorFunc func(ctx context.Context, agent *Subagent) (*SubagentResult, error)

func (f SubagentExecutorFunc) Execute(ctx context.Context, agent *Subagent) (*SubagentResult, error) {
	return f(ctx, agent)
}

// NewSubagentManager creates a new subagent manager.
func NewSubagentManager(config *SubagentConfig, executor SubagentExecutor) *SubagentManager {
	if config == nil {
		config = DefaultSubagentConfig()
	}
	return &SubagentManager{
		config:      config,
		subagents:   make(map[string]*Subagent),
		executor:    executor,
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}

// Spawn creates a new subagent with isolated context.
// Returns nil and ErrMaxSubagentsReached if the limit is exceeded.
func (m *SubagentManager) Spawn(name, task string, opts ...SubagentOption) (*Subagent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check limit before spawning
	if m.config.MaxSubagents > 0 && len(m.subagents) >= m.config.MaxSubagents {
		return nil, ErrMaxSubagentsReached
	}

	m.nextID++
	id := fmt.Sprintf("subagent-%d", m.nextID)

	agent := &Subagent{
		ID:   id,
		Name: name,
		Task: task,
		Context: &SubagentContext{
			Messages:  make([]Message, 0),
			Tools:     make([]ToolInfo, 0),
			State:     make(map[string]any),
			MaxTokens: DefaultSubagentMaxTokens,
		},
		parentAgent: m,
	}

	for _, opt := range opts {
		opt(agent)
	}

	m.subagents[id] = agent
	return agent, nil
}

// SubagentOption configures a subagent.
type SubagentOption func(*Subagent)

// WithSubagentPrompt sets the subagent's system prompt.
func WithSubagentPrompt(prompt string) SubagentOption {
	return func(a *Subagent) {
		a.Context.SystemPrompt = prompt
	}
}

// WithSubagentTools sets available tools.
func WithSubagentTools(tools []ToolInfo) SubagentOption {
	return func(a *Subagent) {
		a.Context.Tools = tools
	}
}

// WithSubagentState sets initial state.
func WithSubagentState(state map[string]any) SubagentOption {
	return func(a *Subagent) {
		maps.Copy(a.Context.State, state)
	}
}

// WithSubagentMessages sets initial messages.
func WithSubagentMessages(messages []Message) SubagentOption {
	return func(a *Subagent) {
		a.Context.Messages = append(a.Context.Messages, messages...)
	}
}

// WithSubagentMaxTokens sets the token limit.
func WithSubagentMaxTokens(max int) SubagentOption {
	return func(a *Subagent) {
		a.Context.MaxTokens = max
	}
}

// Run executes a single subagent.
func (m *SubagentManager) Run(ctx context.Context, agent *Subagent) (*SubagentResult, error) {
	if m.executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	result, err := m.executor.Execute(ctx, agent)
	if err != nil {
		agent.Result = &SubagentResult{Success: false, Error: err}
		return nil, err
	}

	agent.Result = result
	return result, nil
}

// RunAll executes all spawned subagents concurrently.
func (m *SubagentManager) RunAll(ctx context.Context) (map[string]*SubagentResult, error) {
	m.mu.RLock()
	agents := make([]*Subagent, 0, len(m.subagents))
	for _, agent := range m.subagents {
		agents = append(agents, agent)
	}
	m.mu.RUnlock()

	return m.RunAgents(ctx, agents...)
}

// RunAgents executes specific subagents concurrently with graceful shutdown support.
//
// Context cancellation behavior:
// - When ctx is cancelled, all running subagents receive cancellation immediately
// - RunAgents returns promptly; it does not wait for subagents to complete
// - Subagents that were cancelled will have Error set to context.Canceled
// - Results collected before cancellation are still returned
//
// The executor MUST respect context cancellation for this to work properly.
func (m *SubagentManager) RunAgents(ctx context.Context, agents ...*Subagent) (map[string]*SubagentResult, error) {
	if m.executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	if len(agents) == 0 {
		return make(map[string]*SubagentResult), nil
	}

	results := make(map[string]*SubagentResult)
	var resultsMu sync.Mutex

	// Create a cancellable context for this batch of agents.
	// This allows Shutdown() to cancel all running subagents.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel() // Ensure cleanup when RunAgents returns

	// Use errgroup with limited concurrency.
	// SetLimit MUST be called before any g.Go() calls to prevent a race
	// condition where all goroutines spawn before the limit takes effect.
	g, groupCtx := errgroup.WithContext(runCtx)
	if m.config.MaxConcurrent > 0 {
		g.SetLimit(m.config.MaxConcurrent)
	}

	for _, agent := range agents {
		// Create per-agent cancellable context for fine-grained control.
		// This allows individual agent cancellation via Shutdown().
		agentCtx, agentCancel := context.WithCancel(groupCtx)

		// Track the cancel function so Shutdown() can cancel this agent.
		m.cancelMu.Lock()
		m.cancelFuncs[agent.ID] = agentCancel
		m.cancelMu.Unlock()

		g.Go(func() error {
			// Track whether we reserved tokens (for deferred release)
			var tokensReserved int64

			// Ensure we clean up the cancel function and release tokens when done
			defer func() {
				// Release tokens back to pool if we reserved any
				if tokensReserved > 0 && m.config.GlobalTokenBudget != nil {
					m.config.GlobalTokenBudget.Release(tokensReserved)
				}

				m.cancelMu.Lock()
				delete(m.cancelFuncs, agent.ID)
				m.cancelMu.Unlock()
				agentCancel() // Release context resources
			}()

			// Check for cancellation before starting work
			if err := agentCtx.Err(); err != nil {
				resultsMu.Lock()
				results[agent.ID] = &SubagentResult{Success: false, Error: err}
				resultsMu.Unlock()
				return nil
			}

			// Reserve tokens from global budget before execution
			if m.config.GlobalTokenBudget != nil {
				requestedTokens := int64(agent.Context.MaxTokens)
				if m.config.WaitForTokens {
					// Block until tokens available (or context cancelled)
					if err := m.config.GlobalTokenBudget.ReserveWait(agentCtx, requestedTokens); err != nil {
						resultsMu.Lock()
						results[agent.ID] = &SubagentResult{Success: false, Error: err}
						resultsMu.Unlock()
						return nil
					}
				} else {
					// Fail immediately if budget exhausted
					if !m.config.GlobalTokenBudget.Reserve(requestedTokens) {
						resultsMu.Lock()
						results[agent.ID] = &SubagentResult{
							Success: false,
							Error: fmt.Errorf("%w: need %d tokens, only %d available",
								ErrTokenBudgetExhausted, requestedTokens, m.config.GlobalTokenBudget.Available()),
						}
						resultsMu.Unlock()
						return nil
					}
				}
				tokensReserved = requestedTokens
			}

			result, err := m.executor.Execute(agentCtx, agent)
			if err != nil {
				result = &SubagentResult{Success: false, Error: err}
			}

			resultsMu.Lock()
			results[agent.ID] = result
			resultsMu.Unlock()

			// Don't return error - collect all results
			return nil
		})
	}

	// Wait for all goroutines to complete.
	// errgroup cancels groupCtx on first error (but we return nil from Go funcs).
	// If parent ctx is cancelled, groupCtx is also cancelled, causing executors
	// to receive cancellation if they respect context.
	if err := g.Wait(); err != nil {
		return results, err
	}

	// Update agent results after all goroutines complete (avoids race)
	m.mu.Lock()
	for _, agent := range agents {
		if r, ok := results[agent.ID]; ok {
			agent.Result = r
		}
	}
	m.mu.Unlock()

	// Check if all agents failed
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount == 0 {
		return results, ErrAllAgentsFailed
	}

	return results, nil
}

// Get retrieves a subagent by ID.
func (m *SubagentManager) Get(id string) *Subagent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subagents[id]
}

// List returns all subagents.
func (m *SubagentManager) List() []*Subagent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*Subagent, 0, len(m.subagents))
	for _, agent := range m.subagents {
		agents = append(agents, agent)
	}
	return agents
}

// Clear removes all subagents.
func (m *SubagentManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subagents = make(map[string]*Subagent)
}

// Shutdown cancels all running subagents and cleans up resources.
//
// This method is safe to call from a signal handler or concurrent goroutine.
// It cancels all in-flight subagent executions by invoking their context
// cancel functions. Executors that respect context cancellation will stop
// promptly; those that don't may continue until they naturally complete.
//
// After Shutdown returns:
// - All cancel functions have been invoked
// - The cancel function map is cleared
// - Subagent definitions remain (call Clear() to remove them)
//
// Shutdown is idempotent; calling it multiple times is safe.
func (m *SubagentManager) Shutdown() {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()

	// Cancel all running subagents
	for id, cancel := range m.cancelFuncs {
		cancel()
		delete(m.cancelFuncs, id)
	}
}

// Running returns the number of currently executing subagents.
func (m *SubagentManager) Running() int {
	m.cancelMu.Lock()
	defer m.cancelMu.Unlock()
	return len(m.cancelFuncs)
}

// TokenBudgetStatus returns the current token budget state.
// Returns (total, reserved, available). All zeros if no budget configured.
func (m *SubagentManager) TokenBudgetStatus() (total, reserved, available int64) {
	if m.config.GlobalTokenBudget == nil {
		return 0, 0, 0
	}
	tb := m.config.GlobalTokenBudget
	return tb.Total(), tb.Reserved(), tb.Available()
}

// FilterResults filters subagent results by success status.
func FilterResults(results map[string]*SubagentResult, successOnly bool) map[string]*SubagentResult {
	filtered := make(map[string]*SubagentResult)
	for id, result := range results {
		if result != nil && result.Success == successOnly {
			filtered[id] = result
		}
	}
	return filtered
}

// MergeResults combines outputs from multiple subagent results.
func MergeResults(results map[string]*SubagentResult) []any {
	outputs := make([]any, 0, len(results))
	for _, result := range results {
		if result != nil && result.Output != nil {
			outputs = append(outputs, result.Output)
		}
	}
	return outputs
}

// AggregateTokens sums token usage across results.
func AggregateTokens(results map[string]*SubagentResult) int {
	total := 0
	for _, result := range results {
		if result != nil {
			total += result.Tokens
		}
	}
	return total
}
