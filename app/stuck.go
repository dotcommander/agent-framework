package app

import "fmt"

// Stuck detection identifies patterns that indicate the loop is unlikely to
// succeed without intervention. Early detection saves resources and provides
// actionable guidance.

// StuckPatternType categorizes stuck patterns.
type StuckPatternType int

const (
	// PatternRepeatAttempt indicates the same error occurred repeatedly.
	PatternRepeatAttempt StuckPatternType = iota
	// PatternOscillation indicates alternating pass/fail results.
	PatternOscillation
	// PatternFlakyTest indicates unreliable test results.
	PatternFlakyTest
)

func (t StuckPatternType) String() string {
	switch t {
	case PatternRepeatAttempt:
		return "repeat_attempt"
	case PatternOscillation:
		return "oscillation"
	case PatternFlakyTest:
		return "flaky_test"
	default:
		return "unknown"
	}
}

// StuckPattern represents a detected stuck pattern.
type StuckPattern struct {
	Type    StuckPatternType
	TaskID  string
	Message string
}

// StuckConfig holds detection thresholds.
type StuckConfig struct {
	// RepeatThreshold is how many identical errors trigger repeat_attempt pattern.
	// Default: 3
	RepeatThreshold int

	// OscillationWindow is how many results to check for oscillation.
	// Default: 4
	OscillationWindow int

	// ErrorHistorySize is how many errors to retain per task.
	// Default: 5
	ErrorHistorySize int

	// PassHistorySize is how many pass/fail results to retain.
	// Default: 6
	PassHistorySize int
}

// DefaultStuckConfig returns sensible defaults.
func DefaultStuckConfig() *StuckConfig {
	return &StuckConfig{
		RepeatThreshold:   3,
		OscillationWindow: 4,
		ErrorHistorySize:  5,
		PassHistorySize:   6,
	}
}

// StuckDetector identifies stuck patterns from task history.
type StuckDetector struct {
	config      *StuckConfig
	taskErrors  map[string][]string // errors per task
	taskResults map[string][]bool   // pass/fail history per task
}

// NewStuckDetector creates a detector with the given config.
// If cfg is nil, defaults are used.
func NewStuckDetector(cfg *StuckConfig) *StuckDetector {
	if cfg == nil {
		cfg = DefaultStuckConfig()
	}
	// Apply defaults for zero values
	if cfg.RepeatThreshold == 0 {
		cfg.RepeatThreshold = DefaultStuckConfig().RepeatThreshold
	}
	if cfg.OscillationWindow == 0 {
		cfg.OscillationWindow = DefaultStuckConfig().OscillationWindow
	}
	if cfg.ErrorHistorySize == 0 {
		cfg.ErrorHistorySize = DefaultStuckConfig().ErrorHistorySize
	}
	if cfg.PassHistorySize == 0 {
		cfg.PassHistorySize = DefaultStuckConfig().PassHistorySize
	}
	return &StuckDetector{
		config:      cfg,
		taskErrors:  make(map[string][]string),
		taskResults: make(map[string][]bool),
	}
}

// RecordError records an error for a task and checks for repeat pattern.
// Returns a StuckPattern if detected, nil otherwise.
func (d *StuckDetector) RecordError(taskID, normalizedError string) *StuckPattern {
	// Add error to history
	errors := d.taskErrors[taskID]
	errors = append(errors, normalizedError)

	// Trim to history size
	if len(errors) > d.config.ErrorHistorySize {
		errors = errors[len(errors)-d.config.ErrorHistorySize:]
	}
	d.taskErrors[taskID] = errors

	// Also record as a failure in pass/fail history
	d.recordResultInternal(taskID, false)

	// Check for repeat pattern
	return d.detectRepeatAttempt(taskID, errors, normalizedError)
}

// RecordResult records a pass/fail result and checks for oscillation.
// Returns a StuckPattern if detected, nil otherwise.
func (d *StuckDetector) RecordResult(taskID string, passed bool) *StuckPattern {
	d.recordResultInternal(taskID, passed)
	return d.detectOscillation(taskID)
}

// recordResultInternal adds a result without checking patterns.
func (d *StuckDetector) recordResultInternal(taskID string, passed bool) {
	results := d.taskResults[taskID]
	results = append(results, passed)

	// Trim to history size
	if len(results) > d.config.PassHistorySize {
		results = results[len(results)-d.config.PassHistorySize:]
	}
	d.taskResults[taskID] = results
}

// detectRepeatAttempt checks if the same error is repeating.
func (d *StuckDetector) detectRepeatAttempt(taskID string, errors []string, latestError string) *StuckPattern {
	if len(errors) < d.config.RepeatThreshold {
		return nil
	}

	// Count occurrences of latest error in the most recent N errors
	count := 0
	start := max(len(errors)-d.config.RepeatThreshold, 0)
	for i := len(errors) - 1; i >= start; i-- {
		if errors[i] == latestError {
			count++
		}
	}

	if count >= d.config.RepeatThreshold {
		return &StuckPattern{
			Type:    PatternRepeatAttempt,
			TaskID:  taskID,
			Message: fmt.Sprintf("task %s failed with same error %d times", taskID, d.config.RepeatThreshold),
		}
	}

	return nil
}

// detectOscillation checks for alternating pass/fail patterns.
func (d *StuckDetector) detectOscillation(taskID string) *StuckPattern {
	passHistory := d.taskResults[taskID]
	if len(passHistory) < d.config.OscillationWindow {
		return nil
	}

	// Check for alternating pattern in the most recent window
	start := len(passHistory) - d.config.OscillationWindow
	window := passHistory[start:]

	isOscillating := true
	for i := 1; i < len(window); i++ {
		if window[i] == window[i-1] {
			isOscillating = false
			break
		}
	}

	if isOscillating {
		return &StuckPattern{
			Type:    PatternOscillation,
			TaskID:  taskID,
			Message: fmt.Sprintf("task %s oscillating between pass and fail", taskID),
		}
	}

	return nil
}

// Reset clears history for a task.
func (d *StuckDetector) Reset(taskID string) {
	delete(d.taskErrors, taskID)
	delete(d.taskResults, taskID)
}

// ResetAll clears all task history.
func (d *StuckDetector) ResetAll() {
	d.taskErrors = make(map[string][]string)
	d.taskResults = make(map[string][]bool)
}

// Hint returns detailed guidance for a stuck pattern type.
func (d *StuckDetector) Hint(patternType StuckPatternType) string {
	switch patternType {
	case PatternRepeatAttempt:
		return `Same error type 3+ times. Try a fundamentally different approach:
- If build fails: check for circular dependencies or missing exports
- If test fails: verify test setup/teardown isolation
- Consider simplifying the implementation`

	case PatternOscillation:
		return `Alternating pass/fail detected. This suggests:
- Test isolation issues (shared state between tests)
- Race conditions or timing-dependent code
- Consider mocking flaky dependencies`

	case PatternFlakyTest:
		return `Tests are unreliable. Consider:
- Mocking external dependencies (DB, network, time)
- Adding explicit waits or synchronization
- Checking for test pollution from other tests`

	default:
		return "Manual intervention may be needed."
	}
}

// StuckHint is a convenience function that returns guidance for a pattern type string.
// Accepts: "repeat_attempt", "oscillation", "flaky_test"
func StuckHint(patternType string) string {
	hints := map[string]string{
		"repeat_attempt": `Same error type 3+ times. Try a fundamentally different approach:
- If build fails: check for circular dependencies or missing exports
- If test fails: verify test setup/teardown isolation
- Consider simplifying the implementation`,

		"oscillation": `Alternating pass/fail detected. This suggests:
- Test isolation issues (shared state between tests)
- Race conditions or timing-dependent code
- Consider mocking flaky dependencies`,

		"flaky_test": `Tests are unreliable. Consider:
- Mocking external dependencies (DB, network, time)
- Adding explicit waits or synchronization
- Checking for test pollution from other tests`,
	}
	if hint, ok := hints[patternType]; ok {
		return hint
	}
	return "Manual intervention may be needed."
}
