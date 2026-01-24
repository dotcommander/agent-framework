# Verification

Visual verification and hierarchical evaluation for validating AI outputs.

## Quick Start

### Visual Verification

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/verification"
)

func main() {
    ctx := context.Background()

    // Create verifier with pixel comparison
    verifier := verification.NewVisualVerifier(
        verification.DefaultVisualConfig(),
        capturer,   // Your ScreenshotCapturer implementation
        verification.NewPixelComparator(),
    )

    // Compare against baseline
    result, err := verifier.Verify(ctx, "https://example.com", baseline)
    if err != nil {
        panic(err)
    }

    if result.Passed {
        fmt.Println("Visual verification passed!")
    } else {
        fmt.Printf("Failed: %s\n", result.Message)
        fmt.Printf("Diff: %.2f%%\n", result.Diff.DiffPct*100)
    }
}
```

### Hierarchical Evaluation

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/verification"
)

func main() {
    ctx := context.Background()

    // Create evaluator
    evaluator := verification.NewEvaluator(
        verification.WithStopOnFail(true),
    )

    // Add checks at different levels
    evaluator.AddChecks(
        verification.CommonChecks{}.BuildCheck(buildFunc),
        verification.CommonChecks{}.LintCheck(lintFunc),
        verification.CommonChecks{}.TestCheck(testFunc),
    )

    // Run evaluation
    result, err := evaluator.Evaluate(ctx, target)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Score: %.1f%% (Grade: %s)\n",
        result.NormalizedScore*100,
        calculateGrade(result.NormalizedScore*100))
}
```

## Visual Verification

### Components

```
┌─────────────────────────────────────────────────────────┐
│                   VisualVerifier                        │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │  Capturer   │  │ Comparator  │  │  Analyzer   │     │
│  │(screenshots)│  │   (diff)    │  │    (AI)     │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
```

### Configuration

```go
config := &verification.VisualConfig{
    // Maximum allowed difference (0.0-1.0)
    DiffThreshold: 0.01,  // 1% tolerance

    // Areas to exclude from comparison
    IgnoreRegions: []verification.Region{
        {X: 0, Y: 0, Width: 100, Height: 50},  // Header with timestamp
    },

    // Comparison strategy
    CompareMode: "pixel",  // "pixel", "perceptual", "structural"

    // Where to save diff images
    OutputDir: "./diffs",

    // Save passing screenshots as new baselines
    SaveBaselines: false,
}
```

### Screenshot Structure

```go
type Screenshot struct {
    ID        string    // Unique identifier
    Path      string    // File path (if saved)
    Data      []byte    // Raw image data
    Format    string    // "png", "jpeg", "webp"
    Width     int       // Image width
    Height    int       // Image height
    Timestamp int64     // Unix timestamp
    Hash      string    // SHA256 hash for quick comparison
}
```

### Visual Diff

```go
type VisualDiff struct {
    Before     *Screenshot   // Original image
    After      *Screenshot   // New image
    DiffPixels int           // Number of different pixels
    DiffPct    float64       // Percentage different (0.0-1.0)
    Regions    []DiffRegion  // Areas that changed
    Threshold  float64       // Configured threshold
    Passed     bool          // DiffPct <= Threshold
}
```

### Implementing ScreenshotCapturer

```go
type ScreenshotCapturer interface {
    Capture(ctx context.Context, target string) (*Screenshot, error)
    CaptureElement(ctx context.Context, selector string) (*Screenshot, error)
    CaptureFullPage(ctx context.Context, url string) (*Screenshot, error)
}

// Example with chromedp
type ChromeCapturer struct {
    ctx context.Context
}

func (c *ChromeCapturer) Capture(ctx context.Context, target string) (*verification.Screenshot, error) {
    var buf []byte
    err := chromedp.Run(ctx,
        chromedp.Navigate(target),
        chromedp.FullScreenshot(&buf, 90),
    )
    if err != nil {
        return nil, err
    }

    return &verification.Screenshot{
        ID:        fmt.Sprintf("screenshot-%d", time.Now().UnixNano()),
        Data:      buf,
        Format:    "png",
        Timestamp: time.Now().Unix(),
    }, nil
}
```

### Baseline Management

```go
// Create baseline manager
manager := verification.NewBaselineManager(storage)

// Save a baseline
manager.Save("homepage-desktop", screenshot)

// Load a baseline
baseline, err := manager.Load("homepage-desktop")

// Generate baseline key from components
key := verification.GenerateKey("homepage", "desktop", "1920x1080")
// Returns: "homepage-desktop-1920x1080"
```

### AI-Powered Analysis

```go
// Add AI analyzer to verifier
verifier.WithAnalyzer(analyzer)

// Analyze a screenshot
analysis, err := verifier.Analyze(ctx, screenshot,
    "Identify any UI issues or accessibility problems")

// Analysis result
type VisualAnalysis struct {
    Description string        // AI description
    Elements    []UIElement   // Detected UI elements
    Issues      []VisualIssue // Detected problems
    Score       float64       // Quality score 0.0-1.0
}
```

## Hierarchical Evaluation

### Evaluation Levels

Evaluation proceeds through levels in order:

```go
const (
    LevelSyntax     // Code compiles
    LevelSemantic   // Logic is correct
    LevelBehavioral // Tests pass
    LevelVisual     // Looks right
)
```

```
LevelSyntax ──► LevelSemantic ──► LevelBehavioral ──► LevelVisual
    │               │                  │                  │
    ▼               ▼                  ▼                  ▼
 Build OK?     Logic OK?         Tests pass?       UI matches?
```

### Creating Checks

```go
type Check interface {
    Name() string
    Level() EvaluationLevel
    Run(ctx context.Context, target any) (*CheckResult, error)
    Weight() float64
}
```

Using `NewCheck`:

```go
check := verification.NewCheck(
    "my-check",
    verification.LevelSemantic,
    func(ctx context.Context, target any) (*verification.CheckResult, error) {
        // Your check logic
        return &verification.CheckResult{
            Name:    "my-check",
            Passed:  true,
            Score:   0.95,
            Message: "Check passed with minor issues",
        }, nil
    },
    verification.WithCheckWeight(1.5),  // Higher importance
)
```

### Built-in Checks

```go
checks := verification.CommonChecks{}

// Build check (syntax level)
buildCheck := checks.BuildCheck(func(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "go", "build", "./...")
    return cmd.Run()
})

// Lint check (syntax level)
lintCheck := checks.LintCheck(func(ctx context.Context) ([]string, error) {
    cmd := exec.CommandContext(ctx, "golangci-lint", "run")
    output, err := cmd.Output()
    if err != nil {
        issues := strings.Split(string(output), "\n")
        return issues, nil
    }
    return nil, nil
})

// Test check (behavioral level)
testCheck := checks.TestCheck(func(ctx context.Context) (passed, total int, err error) {
    // Run tests, parse output
    return 45, 50, nil  // 45 of 50 tests passed
})
```

### Configuring Thresholds

```go
evaluator := verification.NewEvaluator(
    // Set thresholds per level
    verification.WithThreshold(verification.LevelSyntax, 1.0),     // Must pass all
    verification.WithThreshold(verification.LevelSemantic, 0.8),   // 80% required
    verification.WithThreshold(verification.LevelBehavioral, 0.7), // 70% required
    verification.WithThreshold(verification.LevelVisual, 0.5),     // 50% required

    // Stop at first failed level
    verification.WithStopOnFail(true),
)
```

### Evaluation Result

```go
type EvaluationResult struct {
    Passed          bool                        // All levels passed
    TotalScore      float64                     // Sum of check scores
    MaxScore        float64                     // Maximum possible
    NormalizedScore float64                     // TotalScore / MaxScore
    LevelScores     map[EvaluationLevel]float64 // Score per level
    CheckResults    []*CheckResult              // Individual results
    FailedLevel     *EvaluationLevel            // First failed level
    Message         string                      // Summary message
}
```

## Rubric-Based Evaluation

### Creating a Rubric

```go
rubric := &verification.Rubric{
    Name:        "Code Quality",
    Description: "Standard code quality rubric",
    MaxScore:    100,
    Criteria: []verification.RubricCriteria{
        {
            Name:        "correctness",
            Description: "Functional correctness",
            Weight:      30,
            Levels: []verification.ScoreLevel{
                {Score: 1.0, Description: "Fully correct"},
                {Score: 0.8, Description: "Minor issues"},
                {Score: 0.5, Description: "Partial"},
                {Score: 0.2, Description: "Major issues"},
                {Score: 0.0, Description: "Non-functional"},
            },
        },
        {
            Name:        "readability",
            Description: "Code clarity",
            Weight:      25,
            Levels: []verification.ScoreLevel{
                {Score: 1.0, Description: "Excellent clarity"},
                {Score: 0.8, Description: "Good naming"},
                {Score: 0.5, Description: "Acceptable"},
                {Score: 0.2, Description: "Hard to follow"},
                {Score: 0.0, Description: "Unreadable"},
            },
        },
    },
}
```

### Using Default Rubric

```go
rubric := verification.DefaultCodeRubric()
// Includes: correctness, readability, performance, testing, design
```

### Evaluating Against Rubric

```go
evaluator := verification.NewRubricEvaluator(rubric)

scores := map[string]float64{
    "correctness": 0.9,
    "readability": 0.8,
    "performance": 0.7,
    "testing":     0.6,
    "design":      0.8,
}

result := evaluator.Evaluate(scores)

fmt.Printf("Total: %.1f / %.1f\n", result.TotalScore, result.MaxScore)
fmt.Printf("Grade: %s (%.1f%%)\n", result.Grade, result.Percentage)
```

### Rubric Result

```go
type RubricResult struct {
    Rubric     string        // Rubric name
    Scores     []RubricScore // Score per criterion
    TotalScore float64       // Weighted sum
    MaxScore   float64       // Maximum possible
    Percentage float64       // Percentage score
    Grade      string        // Letter grade (A-F)
}
```

## Example: Complete Verification Pipeline

```go
func verifyCodeChange(ctx context.Context, code string) (*VerificationReport, error) {
    // Phase 1: Syntax checks
    evaluator := verification.NewEvaluator(
        verification.WithStopOnFail(true),
    )

    evaluator.AddCheck(verification.CommonChecks{}.BuildCheck(func(ctx context.Context) error {
        return exec.CommandContext(ctx, "go", "build", "./...").Run()
    }))

    evaluator.AddCheck(verification.CommonChecks{}.LintCheck(func(ctx context.Context) ([]string, error) {
        out, _ := exec.CommandContext(ctx, "golangci-lint", "run").Output()
        if len(out) > 0 {
            return strings.Split(string(out), "\n"), nil
        }
        return nil, nil
    }))

    // Phase 2: Behavioral checks
    evaluator.AddCheck(verification.CommonChecks{}.TestCheck(func(ctx context.Context) (int, int, error) {
        // Run tests
        return 48, 50, nil
    }))

    // Run evaluation
    evalResult, err := evaluator.Evaluate(ctx, code)
    if err != nil {
        return nil, err
    }

    // Phase 3: Rubric scoring
    rubricEval := verification.NewRubricEvaluator(verification.DefaultCodeRubric())
    rubricResult := rubricEval.Evaluate(map[string]float64{
        "correctness": evalResult.LevelScores[verification.LevelBehavioral],
        "readability": 0.8,  // From static analysis
        "performance": 0.7,  // From benchmarks
        "testing":     float64(48) / 50,
        "design":      0.85, // From architecture review
    })

    return &VerificationReport{
        Passed:     evalResult.Passed,
        Grade:      rubricResult.Grade,
        Score:      rubricResult.Percentage,
        Details:    evalResult,
        RubricEval: rubricResult,
    }, nil
}
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [VALIDATION.md](VALIDATION.md) - Rule-based validation
- [AGENT-LOOP.md](AGENT-LOOP.md) - Using verification in agent loops
