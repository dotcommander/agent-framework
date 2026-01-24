package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ErrAllAgentsFailed is returned when all subagents fail execution.
var ErrAllAgentsFailed = errors.New("all subagents failed")

// ErrMaxSubagentsReached is returned when spawning would exceed the limit.
var ErrMaxSubagentsReached = errors.New("maximum subagents reached")

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
}

// DefaultSubagentConfig returns sensible defaults.
func DefaultSubagentConfig() *SubagentConfig {
	return &SubagentConfig{
		MaxConcurrent:   5,
		MaxSubagents:    100,
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
}

// SubagentManager coordinates multiple subagents.
type SubagentManager struct {
	config    *SubagentConfig
	subagents map[string]*Subagent
	executor  SubagentExecutor
	mu        sync.RWMutex
	nextID    int
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
		config:    config,
		subagents: make(map[string]*Subagent),
		executor:  executor,
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
			MaxTokens: 100000,
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

// RunAgents executes specific subagents concurrently.
func (m *SubagentManager) RunAgents(ctx context.Context, agents ...*Subagent) (map[string]*SubagentResult, error) {
	if m.executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	if len(agents) == 0 {
		return make(map[string]*SubagentResult), nil
	}

	results := make(map[string]*SubagentResult)
	var mu sync.Mutex

	// Use errgroup with limited concurrency
	g, ctx := errgroup.WithContext(ctx)
	if m.config.MaxConcurrent > 0 {
		g.SetLimit(m.config.MaxConcurrent)
	}

	for _, agent := range agents {
		g.Go(func() error {
			result, err := m.executor.Execute(ctx, agent)
			if err != nil {
				result = &SubagentResult{Success: false, Error: err}
			}

			mu.Lock()
			results[agent.ID] = result
			mu.Unlock()

			// Don't return error - collect all results
			return nil
		})
	}

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
