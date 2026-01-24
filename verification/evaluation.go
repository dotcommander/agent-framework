package verification

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// EvaluationLevel represents a tier of verification.
type EvaluationLevel int

const (
	// LevelSyntax checks syntactic correctness.
	LevelSyntax EvaluationLevel = iota
	// LevelSemantic checks logical correctness.
	LevelSemantic
	// LevelBehavioral checks runtime behavior.
	LevelBehavioral
	// LevelVisual checks visual appearance.
	LevelVisual
)

// String returns the level name.
func (l EvaluationLevel) String() string {
	switch l {
	case LevelSyntax:
		return "syntax"
	case LevelSemantic:
		return "semantic"
	case LevelBehavioral:
		return "behavioral"
	case LevelVisual:
		return "visual"
	default:
		return "unknown"
	}
}

// Check represents a single verification check.
type Check interface {
	// Name returns the check identifier.
	Name() string

	// Level returns the evaluation level.
	Level() EvaluationLevel

	// Run executes the check.
	Run(ctx context.Context, target any) (*CheckResult, error)

	// Weight returns the check's importance (0.0-1.0).
	Weight() float64
}

// CheckResult contains the outcome of a check.
type CheckResult struct {
	Name     string          `json:"name"`
	Level    EvaluationLevel `json:"level"`
	Passed   bool            `json:"passed"`
	Score    float64         `json:"score"` // 0.0-1.0
	Message  string          `json:"message"`
	Details  map[string]any  `json:"details,omitempty"`
	Duration int64           `json:"duration_ms"`
}

// Rubric defines scoring criteria.
type Rubric struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Criteria    []RubricCriteria `json:"criteria"`
	MaxScore    float64          `json:"max_score"`
}

// RubricCriteria defines a single criterion.
type RubricCriteria struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Weight      float64      `json:"weight"`
	Levels      []ScoreLevel `json:"levels"`
}

// ScoreLevel defines a scoring level within a criterion.
type ScoreLevel struct {
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

// EvaluationResult contains the complete evaluation outcome.
type EvaluationResult struct {
	Passed          bool                        `json:"passed"`
	TotalScore      float64                     `json:"total_score"`
	MaxScore        float64                     `json:"max_score"`
	NormalizedScore float64                     `json:"normalized_score"` // 0.0-1.0
	LevelScores     map[EvaluationLevel]float64 `json:"level_scores"`
	CheckResults    []*CheckResult              `json:"check_results"`
	FailedLevel     *EvaluationLevel            `json:"failed_level,omitempty"`
	Message         string                      `json:"message"`
}

// Evaluator performs hierarchical evaluation.
type Evaluator struct {
	checks     []Check
	rubric     *Rubric
	thresholds map[EvaluationLevel]float64
	stopOnFail bool
	mu         sync.RWMutex
}

// EvaluatorOption configures an evaluator.
type EvaluatorOption func(*Evaluator)

// WithRubric sets the scoring rubric.
func WithRubric(rubric *Rubric) EvaluatorOption {
	return func(e *Evaluator) {
		e.rubric = rubric
	}
}

// WithThreshold sets a pass threshold for a level.
func WithThreshold(level EvaluationLevel, threshold float64) EvaluatorOption {
	return func(e *Evaluator) {
		e.thresholds[level] = threshold
	}
}

// WithStopOnFail configures early termination.
func WithStopOnFail(stop bool) EvaluatorOption {
	return func(e *Evaluator) {
		e.stopOnFail = stop
	}
}

// NewEvaluator creates a new evaluator.
func NewEvaluator(opts ...EvaluatorOption) *Evaluator {
	e := &Evaluator{
		checks:     make([]Check, 0),
		thresholds: make(map[EvaluationLevel]float64),
		stopOnFail: true, // Default: stop on first level failure
	}

	// Default thresholds
	e.thresholds[LevelSyntax] = 1.0     // Must pass all syntax checks
	e.thresholds[LevelSemantic] = 0.8   // 80% semantic correctness
	e.thresholds[LevelBehavioral] = 0.7 // 70% behavioral correctness
	e.thresholds[LevelVisual] = 0.5     // 50% visual similarity

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// AddCheck adds a check to the evaluator.
func (e *Evaluator) AddCheck(check Check) *Evaluator {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.checks = append(e.checks, check)
	return e
}

// AddChecks adds multiple checks.
func (e *Evaluator) AddChecks(checks ...Check) *Evaluator {
	for _, check := range checks {
		e.AddCheck(check)
	}
	return e
}

// Evaluate runs hierarchical evaluation.
func (e *Evaluator) Evaluate(ctx context.Context, target any) (*EvaluationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &EvaluationResult{
		Passed:       true,
		LevelScores:  make(map[EvaluationLevel]float64),
		CheckResults: make([]*CheckResult, 0),
	}

	// Group checks by level
	checksByLevel := e.groupChecksByLevel()

	// Evaluate each level in order
	levels := []EvaluationLevel{LevelSyntax, LevelSemantic, LevelBehavioral, LevelVisual}

	for _, level := range levels {
		checks, ok := checksByLevel[level]
		if !ok || len(checks) == 0 {
			continue
		}

		levelScore, levelPassed, checkResults := e.evaluateLevel(ctx, level, checks, target)
		result.LevelScores[level] = levelScore
		result.CheckResults = append(result.CheckResults, checkResults...)

		if !levelPassed {
			result.Passed = false
			result.FailedLevel = &level

			if e.stopOnFail {
				result.Message = fmt.Sprintf("Evaluation failed at %s level (score: %.2f, threshold: %.2f)",
					level.String(), levelScore, e.thresholds[level])
				break
			}
		}
	}

	// Calculate total score
	e.calculateTotalScore(result)

	if result.Passed {
		result.Message = fmt.Sprintf("All evaluations passed (score: %.2f%%)", result.NormalizedScore*100)
	}

	return result, nil
}

func (e *Evaluator) groupChecksByLevel() map[EvaluationLevel][]Check {
	groups := make(map[EvaluationLevel][]Check)
	for _, check := range e.checks {
		level := check.Level()
		groups[level] = append(groups[level], check)
	}
	return groups
}

func (e *Evaluator) evaluateLevel(ctx context.Context, level EvaluationLevel, checks []Check, target any) (float64, bool, []*CheckResult) {
	var results []*CheckResult
	var totalWeight, weightedScore float64

	for _, check := range checks {
		checkResult, err := check.Run(ctx, target)
		if err != nil {
			checkResult = &CheckResult{
				Name:    check.Name(),
				Level:   level,
				Passed:  false,
				Score:   0,
				Message: fmt.Sprintf("Check error: %v", err),
			}
		}
		checkResult.Level = level

		results = append(results, checkResult)
		weight := check.Weight()
		totalWeight += weight
		weightedScore += checkResult.Score * weight
	}

	levelScore := 0.0
	if totalWeight > 0 {
		levelScore = weightedScore / totalWeight
	}

	threshold := e.thresholds[level]
	passed := levelScore >= threshold

	return levelScore, passed, results
}

func (e *Evaluator) calculateTotalScore(result *EvaluationResult) {
	if len(result.CheckResults) == 0 {
		return
	}

	var total float64
	for _, r := range result.CheckResults {
		total += r.Score
	}

	result.TotalScore = total
	result.MaxScore = float64(len(result.CheckResults))
	result.NormalizedScore = total / result.MaxScore
}

// BaseCheck provides a base implementation for checks.
type BaseCheck struct {
	name   string
	level  EvaluationLevel
	weight float64
	runFn  func(ctx context.Context, target any) (*CheckResult, error)
}

// CheckOption configures a check.
type CheckOption func(*BaseCheck)

// WithCheckWeight sets the check weight.
func WithCheckWeight(weight float64) CheckOption {
	return func(c *BaseCheck) {
		c.weight = weight
	}
}

// NewCheck creates a new check.
func NewCheck(name string, level EvaluationLevel, runFn func(ctx context.Context, target any) (*CheckResult, error), opts ...CheckOption) *BaseCheck {
	c := &BaseCheck{
		name:   name,
		level:  level,
		weight: 1.0,
		runFn:  runFn,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *BaseCheck) Name() string           { return c.name }
func (c *BaseCheck) Level() EvaluationLevel { return c.level }
func (c *BaseCheck) Weight() float64        { return c.weight }

func (c *BaseCheck) Run(ctx context.Context, target any) (*CheckResult, error) {
	return c.runFn(ctx, target)
}

// CommonChecks provides commonly used check implementations.
type CommonChecks struct{}

// BuildCheck creates a syntax check that verifies build success.
func (CommonChecks) BuildCheck(buildFn func(ctx context.Context) error) *BaseCheck {
	return NewCheck("build", LevelSyntax, func(ctx context.Context, target any) (*CheckResult, error) {
		err := buildFn(ctx)
		if err != nil {
			return &CheckResult{
				Name:    "build",
				Passed:  false,
				Score:   0,
				Message: fmt.Sprintf("Build failed: %v", err),
			}, nil
		}
		return &CheckResult{
			Name:    "build",
			Passed:  true,
			Score:   1.0,
			Message: "Build succeeded",
		}, nil
	})
}

// LintCheck creates a syntax check for linting.
func (CommonChecks) LintCheck(lintFn func(ctx context.Context) ([]string, error)) *BaseCheck {
	return NewCheck("lint", LevelSyntax, func(ctx context.Context, target any) (*CheckResult, error) {
		issues, err := lintFn(ctx)
		if err != nil {
			return &CheckResult{
				Name:    "lint",
				Passed:  false,
				Score:   0,
				Message: fmt.Sprintf("Lint failed: %v", err),
			}, nil
		}

		if len(issues) > 0 {
			return &CheckResult{
				Name:    "lint",
				Passed:  false,
				Score:   0.5,
				Message: fmt.Sprintf("Lint found %d issues", len(issues)),
				Details: map[string]any{"issues": issues},
			}, nil
		}

		return &CheckResult{
			Name:    "lint",
			Passed:  true,
			Score:   1.0,
			Message: "No lint issues",
		}, nil
	})
}

// TestCheck creates a behavioral check for tests.
func (CommonChecks) TestCheck(testFn func(ctx context.Context) (passed, total int, err error)) *BaseCheck {
	return NewCheck("tests", LevelBehavioral, func(ctx context.Context, target any) (*CheckResult, error) {
		passed, total, err := testFn(ctx)
		if err != nil {
			return &CheckResult{
				Name:    "tests",
				Passed:  false,
				Score:   0,
				Message: fmt.Sprintf("Tests failed: %v", err),
			}, nil
		}

		score := 1.0
		if total > 0 {
			score = float64(passed) / float64(total)
		}

		return &CheckResult{
			Name:    "tests",
			Passed:  passed == total,
			Score:   score,
			Message: fmt.Sprintf("Tests: %d/%d passed", passed, total),
			Details: map[string]any{"passed": passed, "total": total},
		}, nil
	})
}

// RubricEvaluator evaluates against a rubric.
type RubricEvaluator struct {
	rubric *Rubric
}

// NewRubricEvaluator creates a rubric evaluator.
func NewRubricEvaluator(rubric *Rubric) *RubricEvaluator {
	return &RubricEvaluator{rubric: rubric}
}

// RubricScore represents a score against rubric criteria.
type RubricScore struct {
	Criterion string  `json:"criterion"`
	Score     float64 `json:"score"`
	Level     string  `json:"level"`
	Feedback  string  `json:"feedback"`
}

// RubricResult contains rubric evaluation results.
type RubricResult struct {
	Rubric     string        `json:"rubric"`
	Scores     []RubricScore `json:"scores"`
	TotalScore float64       `json:"total_score"`
	MaxScore   float64       `json:"max_score"`
	Percentage float64       `json:"percentage"`
	Grade      string        `json:"grade"`
}

// Evaluate scores against the rubric.
func (e *RubricEvaluator) Evaluate(scores map[string]float64) *RubricResult {
	result := &RubricResult{
		Rubric:   e.rubric.Name,
		Scores:   make([]RubricScore, 0),
		MaxScore: e.rubric.MaxScore,
	}

	for _, criterion := range e.rubric.Criteria {
		score, ok := scores[criterion.Name]
		if !ok {
			score = 0
		}

		// Find matching level
		levelDesc := "No score"
		var sortedLevels []ScoreLevel
		sortedLevels = append(sortedLevels, criterion.Levels...)
		sort.Slice(sortedLevels, func(i, j int) bool {
			return sortedLevels[i].Score > sortedLevels[j].Score
		})

		for _, level := range sortedLevels {
			if score >= level.Score {
				levelDesc = level.Description
				break
			}
		}

		result.Scores = append(result.Scores, RubricScore{
			Criterion: criterion.Name,
			Score:     score * criterion.Weight,
			Level:     levelDesc,
		})

		result.TotalScore += score * criterion.Weight
	}

	if result.MaxScore > 0 {
		result.Percentage = (result.TotalScore / result.MaxScore) * 100
	}

	result.Grade = calculateGrade(result.Percentage)

	return result
}

func calculateGrade(percentage float64) string {
	switch {
	case percentage >= 90:
		return "A"
	case percentage >= 80:
		return "B"
	case percentage >= 70:
		return "C"
	case percentage >= 60:
		return "D"
	default:
		return "F"
	}
}

// DefaultCodeRubric returns a standard code quality rubric.
func DefaultCodeRubric() *Rubric {
	return &Rubric{
		Name:        "Code Quality",
		Description: "Standard code quality evaluation rubric",
		MaxScore:    100,
		Criteria: []RubricCriteria{
			{
				Name:        "correctness",
				Description: "Functional correctness and bug-free implementation",
				Weight:      30,
				Levels: []ScoreLevel{
					{Score: 1.0, Description: "Fully correct, all tests pass"},
					{Score: 0.8, Description: "Minor issues, most tests pass"},
					{Score: 0.5, Description: "Partial implementation, some tests pass"},
					{Score: 0.2, Description: "Major issues, few tests pass"},
					{Score: 0.0, Description: "Non-functional or failing build"},
				},
			},
			{
				Name:        "readability",
				Description: "Code clarity and maintainability",
				Weight:      25,
				Levels: []ScoreLevel{
					{Score: 1.0, Description: "Exceptionally clear and well-documented"},
					{Score: 0.8, Description: "Clear with good naming and structure"},
					{Score: 0.5, Description: "Acceptable readability"},
					{Score: 0.2, Description: "Difficult to follow"},
					{Score: 0.0, Description: "Unreadable or obfuscated"},
				},
			},
			{
				Name:        "performance",
				Description: "Efficiency and resource usage",
				Weight:      20,
				Levels: []ScoreLevel{
					{Score: 1.0, Description: "Optimal performance"},
					{Score: 0.8, Description: "Good performance, minor optimizations possible"},
					{Score: 0.5, Description: "Acceptable performance"},
					{Score: 0.2, Description: "Performance issues"},
					{Score: 0.0, Description: "Severe performance problems"},
				},
			},
			{
				Name:        "testing",
				Description: "Test coverage and quality",
				Weight:      15,
				Levels: []ScoreLevel{
					{Score: 1.0, Description: "Comprehensive tests with edge cases"},
					{Score: 0.8, Description: "Good coverage of main paths"},
					{Score: 0.5, Description: "Basic tests present"},
					{Score: 0.2, Description: "Minimal testing"},
					{Score: 0.0, Description: "No tests"},
				},
			},
			{
				Name:        "design",
				Description: "Architecture and design patterns",
				Weight:      10,
				Levels: []ScoreLevel{
					{Score: 1.0, Description: "Excellent design, follows best practices"},
					{Score: 0.8, Description: "Good design with clear structure"},
					{Score: 0.5, Description: "Acceptable design"},
					{Score: 0.2, Description: "Poor design choices"},
					{Score: 0.0, Description: "No discernible design"},
				},
			},
		},
	}
}
