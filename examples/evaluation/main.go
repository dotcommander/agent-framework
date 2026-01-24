// Package main demonstrates hierarchical evaluation.
//
// The evaluation package provides multi-level verification with configurable
// thresholds. Levels progress from syntax -> semantic -> behavioral -> visual,
// with early termination on failure.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/agent-framework/verification"
)

func main() {
	fmt.Println("=== Hierarchical Evaluation Demo ===")
	fmt.Println()

	// Create an evaluator with custom thresholds
	evaluator := verification.NewEvaluator(
		verification.WithThreshold(verification.LevelSyntax, 1.0),     // Must pass all
		verification.WithThreshold(verification.LevelSemantic, 0.8),   // 80% required
		verification.WithThreshold(verification.LevelBehavioral, 0.7), // 70% required
		verification.WithStopOnFail(true),                             // Stop at first failing level
	)

	// Add syntax-level checks
	evaluator.AddChecks(
		// Build check
		verification.NewCheck("build", verification.LevelSyntax,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				code, ok := target.(string)
				if !ok {
					return &verification.CheckResult{
						Name:    "build",
						Passed:  false,
						Score:   0,
						Message: "Invalid target type",
					}, nil
				}

				// Simple syntax validation
				hasPackage := strings.Contains(code, "package ")
				if !hasPackage {
					return &verification.CheckResult{
						Name:    "build",
						Passed:  false,
						Score:   0,
						Message: "Missing package declaration",
					}, nil
				}

				return &verification.CheckResult{
					Name:    "build",
					Passed:  true,
					Score:   1.0,
					Message: "Build successful",
				}, nil
			},
			verification.WithCheckWeight(1.0),
		),

		// Lint check
		verification.NewCheck("lint", verification.LevelSyntax,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				code := target.(string)

				issues := []string{}

				// Check for common issues
				if strings.Contains(code, "fmt.Println") && !strings.Contains(code, `"fmt"`) {
					issues = append(issues, "using fmt without import")
				}

				if strings.Contains(code, "  \n") {
					issues = append(issues, "trailing whitespace")
				}

				score := 1.0
				if len(issues) > 0 {
					score = 0.5
				}

				return &verification.CheckResult{
					Name:    "lint",
					Passed:  len(issues) == 0,
					Score:   score,
					Message: fmt.Sprintf("Lint: %d issues", len(issues)),
					Details: map[string]any{"issues": issues},
				}, nil
			},
		),
	)

	// Add semantic-level checks
	evaluator.AddChecks(
		// Logic correctness check
		verification.NewCheck("logic", verification.LevelSemantic,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				code := target.(string)

				// Check for common logic patterns
				checks := map[string]bool{
					"error_handling": strings.Contains(code, "if err != nil"),
					"nil_checks":     strings.Contains(code, "if .* == nil") || strings.Contains(code, "!= nil"),
					"return_values":  strings.Contains(code, "return"),
				}

				passed := 0
				for _, ok := range checks {
					if ok {
						passed++
					}
				}

				score := float64(passed) / float64(len(checks))

				return &verification.CheckResult{
					Name:    "logic",
					Passed:  score >= 0.5,
					Score:   score,
					Message: fmt.Sprintf("Logic patterns: %d/%d", passed, len(checks)),
					Details: map[string]any{
						"error_handling": checks["error_handling"],
						"nil_checks":     checks["nil_checks"],
						"return_values":  checks["return_values"],
					},
				}, nil
			},
		),

		// API consistency check
		verification.NewCheck("api_consistency", verification.LevelSemantic,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				code := target.(string)

				// Check for consistent patterns
				hasContext := strings.Contains(code, "context.Context")
				hasError := strings.Contains(code, "error")

				score := 0.5
				if hasContext {
					score += 0.25
				}
				if hasError {
					score += 0.25
				}

				return &verification.CheckResult{
					Name:    "api_consistency",
					Passed:  score >= 0.75,
					Score:   score,
					Message: fmt.Sprintf("API consistency score: %.0f%%", score*100),
					Details: map[string]any{
						"has_context": hasContext,
						"has_error":   hasError,
					},
				}, nil
			},
			verification.WithCheckWeight(0.8),
		),
	)

	// Add behavioral-level checks
	evaluator.AddChecks(
		// Test coverage check
		verification.NewCheck("test_coverage", verification.LevelBehavioral,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				// Simulated test results
				passed := 8
				total := 10
				score := float64(passed) / float64(total)

				return &verification.CheckResult{
					Name:    "test_coverage",
					Passed:  score >= 0.7,
					Score:   score,
					Message: fmt.Sprintf("Tests: %d/%d passed", passed, total),
					Details: map[string]any{
						"passed":   passed,
						"failed":   total - passed,
						"total":    total,
						"coverage": "85%",
					},
				}, nil
			},
		),

		// Performance check
		verification.NewCheck("performance", verification.LevelBehavioral,
			func(ctx context.Context, target any) (*verification.CheckResult, error) {
				// Simulated performance metrics
				latencyP99 := 45.0 // ms
				threshold := 100.0

				score := 1.0
				if latencyP99 > threshold {
					score = threshold / latencyP99
				}

				return &verification.CheckResult{
					Name:    "performance",
					Passed:  latencyP99 <= threshold,
					Score:   score,
					Message: fmt.Sprintf("P99 latency: %.1fms", latencyP99),
					Details: map[string]any{
						"p99_latency_ms": latencyP99,
						"threshold_ms":   threshold,
					},
				}, nil
			},
			verification.WithCheckWeight(0.5),
		),
	)

	// Test code sample
	testCode := `package main

import (
	"context"
	"fmt"
)

func Process(ctx context.Context, data string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("empty data")
	}

	result := processData(data)
	if result == nil {
		return "", fmt.Errorf("processing failed")
	}

	return result.String(), nil
}
`

	// Run evaluation
	fmt.Println("Evaluating code sample...")
	fmt.Println()

	ctx := context.Background()
	result, err := evaluator.Evaluate(ctx, testCode)
	if err != nil {
		fmt.Printf("Evaluation error: %v\n", err)
		return
	}

	// Display results
	fmt.Println("=== Evaluation Results ===")
	fmt.Println()

	if result.Passed {
		fmt.Println("Status: PASSED")
	} else {
		fmt.Println("Status: FAILED")
		if result.FailedLevel != nil {
			fmt.Printf("Failed at: %s level\n", result.FailedLevel.String())
		}
	}
	fmt.Println()

	fmt.Printf("Overall Score: %.2f / %.2f (%.0f%%)\n",
		result.TotalScore, result.MaxScore, result.NormalizedScore*100)
	fmt.Println()

	// Level scores
	fmt.Println("Level Scores:")
	for level, score := range result.LevelScores {
		fmt.Printf("  %s: %.2f\n", level.String(), score)
	}
	fmt.Println()

	// Individual check results
	fmt.Println("Check Results:")
	for _, cr := range result.CheckResults {
		status := "PASS"
		if !cr.Passed {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %s (%s): %.2f - %s\n",
			status, cr.Name, cr.Level.String(), cr.Score, cr.Message)
		if len(cr.Details) > 0 {
			fmt.Printf("    Details: %v\n", cr.Details)
		}
	}
	fmt.Println()

	fmt.Printf("Message: %s\n", result.Message)

	// Demonstrate rubric evaluation
	fmt.Println()
	fmt.Println("=== Rubric Evaluation ===")
	fmt.Println()

	rubric := verification.DefaultCodeRubric()
	rubricEvaluator := verification.NewRubricEvaluator(rubric)

	// Provide scores for each criterion
	scores := map[string]float64{
		"correctness": 0.9,
		"readability": 0.8,
		"performance": 0.7,
		"testing":     0.6,
		"design":      0.85,
	}

	rubricResult := rubricEvaluator.Evaluate(scores)

	fmt.Printf("Rubric: %s\n", rubricResult.Rubric)
	fmt.Printf("Total Score: %.2f / %.2f\n", rubricResult.TotalScore, rubricResult.MaxScore)
	fmt.Printf("Percentage: %.1f%%\n", rubricResult.Percentage)
	fmt.Printf("Grade: %s\n\n", rubricResult.Grade)

	fmt.Println("Criterion Scores:")
	for _, score := range rubricResult.Scores {
		fmt.Printf("  %s: %.2f (%s)\n", score.Criterion, score.Score, score.Level)
	}

	// Show rubric criteria
	fmt.Println()
	fmt.Println("=== Rubric Criteria ===")
	fmt.Println()
	for _, criteria := range rubric.Criteria {
		fmt.Printf("%s (weight: %.0f):\n", criteria.Name, criteria.Weight)
		fmt.Printf("  %s\n", criteria.Description)
		for _, level := range criteria.Levels {
			fmt.Printf("    %.0f%%: %s\n", level.Score*100, level.Description)
		}
		fmt.Println()
	}

	// Demonstrate common checks
	fmt.Println("=== Common Checks (Helpers) ===")
	fmt.Println()

	common := verification.CommonChecks{}

	// Build check helper
	buildCheck := common.BuildCheck(func(ctx context.Context) error {
		// Simulated build
		return nil
	})
	fmt.Printf("BuildCheck: %s (level: %s)\n", buildCheck.Name(), buildCheck.Level().String())

	// Lint check helper
	lintCheck := common.LintCheck(func(ctx context.Context) ([]string, error) {
		return []string{}, nil
	})
	fmt.Printf("LintCheck: %s (level: %s)\n", lintCheck.Name(), lintCheck.Level().String())

	// Test check helper
	testCheck := common.TestCheck(func(ctx context.Context) (int, int, error) {
		return 10, 10, nil
	})
	fmt.Printf("TestCheck: %s (level: %s)\n", testCheck.Name(), testCheck.Level().String())
}
