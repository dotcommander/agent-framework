package agent

import (
	"context"
	"fmt"
)

// Pipeline chains multiple LLM calls, passing output from each step as input to the next.
// This is common in spec (5-layer snowflake) and learn (extract → filter → format).
//
// Example:
//
//	result, err := agent.NewPipeline(client).
//	    Step("extract", extractPrompt).
//	    Step("refine", refinePrompt).
//	    Step("polish", polishPrompt).
//	    Run(ctx, input)
type Pipeline struct {
	client *SimpleClient
	steps  []pipelineStep
	onStep func(name string, stepNum, total int) // Progress callback
}

type pipelineStep struct {
	name     string
	template string                                          // Static template (uses %s for input)
	builder  func(input string) string                       // Dynamic prompt builder
	post     func(output string) string                      // Post-process output
	onError  func(name string, err error) (fallback string, skip bool) // Error handler
}

// NewPipeline creates a new pipeline with the given client.
func NewPipeline(client *SimpleClient) *Pipeline {
	return &Pipeline{
		client: client,
	}
}

// Step adds a step with a static prompt template.
// Use %s as placeholder for the input from the previous step.
//
// Example:
//
//	pipeline.Step("extract", "Extract key points from:\n\n%s")
func (p *Pipeline) Step(name, template string) *Pipeline {
	p.steps = append(p.steps, pipelineStep{
		name:     name,
		template: template,
	})
	return p
}

// StepFunc adds a step with a dynamic prompt builder.
// Use when the prompt needs complex construction.
//
// Example:
//
//	pipeline.StepFunc("analyze", func(input string) string {
//	    return fmt.Sprintf("Analyze this code:\n```\n%s\n```", input)
//	})
func (p *Pipeline) StepFunc(name string, builder func(input string) string) *Pipeline {
	p.steps = append(p.steps, pipelineStep{
		name:    name,
		builder: builder,
	})
	return p
}

// StepWithPost adds a step with output post-processing.
// The post function transforms the LLM output before passing to the next step.
//
// Example:
//
//	pipeline.StepWithPost("extract", template, agent.ExtractMarkdown)
func (p *Pipeline) StepWithPost(name, template string, post func(string) string) *Pipeline {
	p.steps = append(p.steps, pipelineStep{
		name:     name,
		template: template,
		post:     post,
	})
	return p
}

// OnError sets an error handler for all steps.
// The handler can return a fallback value and whether to skip to next step.
// If skip is false and fallback is empty, the error propagates.
//
// Example:
//
//	pipeline.OnError(func(name string, err error) (string, bool) {
//	    log.Printf("Step %s failed: %v", name, err)
//	    return "", true // Skip failed step, continue with previous output
//	})
func (p *Pipeline) OnError(handler func(name string, err error) (fallback string, skip bool)) *Pipeline {
	for i := range p.steps {
		p.steps[i].onError = handler
	}
	return p
}

// OnProgress sets a callback for step progress.
//
// Example:
//
//	pipeline.OnProgress(func(name string, step, total int) {
//	    fmt.Printf("Step %d/%d: %s\n", step, total, name)
//	})
func (p *Pipeline) OnProgress(fn func(name string, stepNum, total int)) *Pipeline {
	p.onStep = fn
	return p
}

// Run executes the pipeline with the given initial input.
// Each step receives the output of the previous step as input.
func (p *Pipeline) Run(ctx context.Context, input string) (string, error) {
	current := input
	total := len(p.steps)

	for i, step := range p.steps {
		// Report progress
		if p.onStep != nil {
			p.onStep(step.name, i+1, total)
		}

		// Build prompt
		var prompt string
		switch {
		case step.builder != nil:
			prompt = step.builder(current)
		case step.template != "":
			prompt = fmt.Sprintf(step.template, current)
		default:
			prompt = current
		}

		// Execute LLM call
		result, err := p.client.Query(ctx, prompt)
		if err != nil {
			// Try error handler
			if step.onError != nil {
				fallback, skip := step.onError(step.name, err)
				if skip {
					continue // Use previous output
				}
				if fallback != "" {
					current = fallback
					continue
				}
			}
			return current, fmt.Errorf("step %q: %w", step.name, err)
		}

		// Post-process
		if step.post != nil {
			result = step.post(result)
		}

		current = result
	}

	return current, nil
}

// RunWithResults executes the pipeline and returns all intermediate results.
func (p *Pipeline) RunWithResults(ctx context.Context, input string) ([]StepResult, error) {
	results := make([]StepResult, 0, len(p.steps))
	current := input
	total := len(p.steps)

	for i, step := range p.steps {
		if p.onStep != nil {
			p.onStep(step.name, i+1, total)
		}

		var prompt string
		switch {
		case step.builder != nil:
			prompt = step.builder(current)
		case step.template != "":
			prompt = fmt.Sprintf(step.template, current)
		default:
			prompt = current
		}

		result, err := p.client.Query(ctx, prompt)

		stepResult := StepResult{
			Name:   step.name,
			Input:  current,
			Output: result,
			Error:  err,
		}

		if err != nil {
			if step.onError != nil {
				fallback, skip := step.onError(step.name, err)
				if skip {
					stepResult.Skipped = true
					results = append(results, stepResult)
					continue
				}
				if fallback != "" {
					stepResult.Output = fallback
					current = fallback
					results = append(results, stepResult)
					continue
				}
			}
			results = append(results, stepResult)
			return results, fmt.Errorf("step %q: %w", step.name, err)
		}

		if step.post != nil {
			result = step.post(result)
			stepResult.Output = result
		}

		current = result
		results = append(results, stepResult)
	}

	return results, nil
}

// StepResult holds the result of a single pipeline step.
type StepResult struct {
	Name    string
	Input   string
	Output  string
	Error   error
	Skipped bool
}

// Final returns the final output from a list of step results.
func Final(results []StepResult) string {
	if len(results) == 0 {
		return ""
	}
	// Find last non-skipped result
	for i := len(results) - 1; i >= 0; i-- {
		if !results[i].Skipped && results[i].Error == nil {
			return results[i].Output
		}
	}
	return results[len(results)-1].Output
}
