package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultStuckConfig tests default configuration values.
func TestDefaultStuckConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultStuckConfig()
	assert.Equal(t, 3, cfg.RepeatThreshold)
	assert.Equal(t, 4, cfg.OscillationWindow)
	assert.Equal(t, 5, cfg.ErrorHistorySize)
	assert.Equal(t, 6, cfg.PassHistorySize)
}

// TestStuckPatternTypeString tests pattern type string conversion.
func TestStuckPatternTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		patternType StuckPatternType
		expected    string
	}{
		{PatternRepeatAttempt, "repeat_attempt"},
		{PatternOscillation, "oscillation"},
		{PatternFlakyTest, "flaky_test"},
		{StuckPatternType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.patternType.String())
		})
	}
}

// TestNewStuckDetectorNilConfig tests that nil config uses defaults.
func TestNewStuckDetectorNilConfig(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(nil)
	assert.Equal(t, 3, d.config.RepeatThreshold)
	assert.Equal(t, 4, d.config.OscillationWindow)
	assert.Equal(t, 5, d.config.ErrorHistorySize)
	assert.Equal(t, 6, d.config.PassHistorySize)
}

// TestNewStuckDetectorZeroValues tests that zero config values use defaults.
func TestNewStuckDetectorZeroValues(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(&StuckConfig{})
	assert.Equal(t, 3, d.config.RepeatThreshold)
	assert.Equal(t, 4, d.config.OscillationWindow)
}

// TestRepeatAttemptDetection tests detection of repeated errors.
func TestRepeatAttemptDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		errors     []string
		wantStuck  bool
		wantTaskID string
	}{
		{
			name:      "three_same_errors_triggers",
			errors:    []string{"error A", "error A", "error A"},
			wantStuck: true,
		},
		{
			name:      "two_errors_does_not_trigger",
			errors:    []string{"error A", "error A"},
			wantStuck: false,
		},
		{
			name:      "different_errors_do_not_trigger",
			errors:    []string{"error A", "error B", "error C"},
			wantStuck: false,
		},
		{
			name:      "mixed_with_same_last_three",
			errors:    []string{"error B", "error A", "error A", "error A"},
			wantStuck: true,
		},
		{
			name:      "mixed_without_consecutive",
			errors:    []string{"error A", "error B", "error A", "error B"},
			wantStuck: false,
		},
		{
			name:      "single_error",
			errors:    []string{"error A"},
			wantStuck: false,
		},
		{
			name:      "empty_errors",
			errors:    []string{},
			wantStuck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewStuckDetector(nil)
			taskID := "task-1"

			var lastPattern *StuckPattern
			for _, err := range tt.errors {
				lastPattern = d.RecordError(taskID, err)
			}

			if tt.wantStuck {
				require.NotNil(t, lastPattern, "Expected stuck pattern")
				assert.Equal(t, PatternRepeatAttempt, lastPattern.Type)
				assert.Equal(t, taskID, lastPattern.TaskID)
				assert.Contains(t, lastPattern.Message, "same error 3 times")
			} else {
				assert.Nil(t, lastPattern, "Expected no stuck pattern")
			}
		})
	}
}

// TestOscillationDetection tests detection of alternating pass/fail.
func TestOscillationDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		results   []bool // true=pass, false=fail
		wantStuck bool
	}{
		{
			name:      "alternating_four_triggers",
			results:   []bool{true, false, true, false},
			wantStuck: true,
		},
		{
			name:      "alternating_starting_fail",
			results:   []bool{false, true, false, true},
			wantStuck: true,
		},
		{
			name:      "three_results_not_enough",
			results:   []bool{true, false, true},
			wantStuck: false,
		},
		{
			name:      "consecutive_passes_no_trigger",
			results:   []bool{true, true, false, true},
			wantStuck: false,
		},
		{
			name:      "consecutive_fails_no_trigger",
			results:   []bool{false, false, true, false},
			wantStuck: false,
		},
		{
			name:      "all_passes",
			results:   []bool{true, true, true, true},
			wantStuck: false,
		},
		{
			name:      "all_fails",
			results:   []bool{false, false, false, false},
			wantStuck: false,
		},
		{
			name:      "longer_alternating",
			results:   []bool{true, false, true, false, true, false},
			wantStuck: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewStuckDetector(nil)
			taskID := "task-1"

			var lastPattern *StuckPattern
			for _, passed := range tt.results {
				lastPattern = d.RecordResult(taskID, passed)
			}

			if tt.wantStuck {
				require.NotNil(t, lastPattern, "Expected stuck pattern")
				assert.Equal(t, PatternOscillation, lastPattern.Type)
				assert.Equal(t, taskID, lastPattern.TaskID)
				assert.Contains(t, lastPattern.Message, "oscillating")
			} else {
				assert.Nil(t, lastPattern, "Expected no stuck pattern")
			}
		})
	}
}

// TestReset clears task history.
func TestReset(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(nil)
	taskID := "task-1"

	// Record enough errors to trigger
	d.RecordError(taskID, "error A")
	d.RecordError(taskID, "error A")

	// Reset before third
	d.Reset(taskID)

	// Third error should not trigger (history cleared)
	pattern := d.RecordError(taskID, "error A")
	assert.Nil(t, pattern, "Reset should clear history")
}

// TestResetAll clears all task history.
func TestResetAll(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(nil)

	// Record errors for multiple tasks
	d.RecordError("task-1", "error A")
	d.RecordError("task-1", "error A")
	d.RecordError("task-2", "error B")
	d.RecordError("task-2", "error B")

	// Reset all
	d.ResetAll()

	// Neither should trigger
	pattern1 := d.RecordError("task-1", "error A")
	pattern2 := d.RecordError("task-2", "error B")
	assert.Nil(t, pattern1, "ResetAll should clear task-1 history")
	assert.Nil(t, pattern2, "ResetAll should clear task-2 history")
}

// TestMultipleTasks tests isolation between tasks.
func TestMultipleTasks(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(nil)

	// Task 1 gets 2 errors
	d.RecordError("task-1", "error A")
	d.RecordError("task-1", "error A")

	// Task 2 gets 3 different errors
	d.RecordError("task-2", "error X")
	d.RecordError("task-2", "error Y")
	pattern := d.RecordError("task-2", "error Z")
	assert.Nil(t, pattern, "Different errors should not trigger")

	// Task 1 gets third same error - should trigger
	pattern = d.RecordError("task-1", "error A")
	require.NotNil(t, pattern)
	assert.Equal(t, "task-1", pattern.TaskID)
}

// TestErrorHistoryTrimming tests that error history is bounded.
func TestErrorHistoryTrimming(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(&StuckConfig{
		ErrorHistorySize: 3,
		RepeatThreshold:  3,
	})

	taskID := "task-1"

	// Add errors beyond history size
	d.RecordError(taskID, "error A")
	d.RecordError(taskID, "error A")
	d.RecordError(taskID, "error B") // Breaks the streak
	d.RecordError(taskID, "error B")
	d.RecordError(taskID, "error B") // History is now [B, B, B] since A was trimmed

	// Check internal state
	errors := d.taskErrors[taskID]
	assert.Len(t, errors, 3)

	// New error A shouldn't trigger (old A's are trimmed)
	d.RecordError(taskID, "error A")
	d.RecordError(taskID, "error A")
	pattern := d.RecordError(taskID, "error A")
	require.NotNil(t, pattern, "Should trigger with 3 A's")
}

// TestPassHistoryTrimming tests that pass/fail history is bounded.
func TestPassHistoryTrimming(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(&StuckConfig{
		PassHistorySize:   4,
		OscillationWindow: 4,
	})

	taskID := "task-1"

	// Add results that don't oscillate initially
	d.RecordResult(taskID, true)
	d.RecordResult(taskID, true)
	d.RecordResult(taskID, false)
	d.RecordResult(taskID, true)

	// Check internal state
	results := d.taskResults[taskID]
	assert.Len(t, results, 4)

	// Add more - history should be trimmed
	d.RecordResult(taskID, false)
	results = d.taskResults[taskID]
	assert.Len(t, results, 4)
}

// TestHint tests hint messages for each pattern type.
func TestHint(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(nil)

	tests := []struct {
		patternType StuckPatternType
		contains    string
	}{
		{PatternRepeatAttempt, "Same error type 3+ times"},
		{PatternOscillation, "Alternating pass/fail"},
		{PatternFlakyTest, "Tests are unreliable"},
		{StuckPatternType(99), "Manual intervention"},
	}

	for _, tt := range tests {
		t.Run(tt.patternType.String(), func(t *testing.T) {
			hint := d.Hint(tt.patternType)
			assert.Contains(t, hint, tt.contains)
		})
	}
}

// TestStuckHintFunction tests the package-level hint function.
func TestStuckHintFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		patternType string
		contains    string
	}{
		{"repeat_attempt", "Same error type 3+ times"},
		{"oscillation", "Alternating pass/fail"},
		{"flaky_test", "Tests are unreliable"},
		{"unknown", "Manual intervention"},
	}

	for _, tt := range tests {
		t.Run(tt.patternType, func(t *testing.T) {
			hint := StuckHint(tt.patternType)
			assert.Contains(t, hint, tt.contains)
		})
	}
}

// TestRecordErrorAlsoRecordsFailure tests that RecordError adds to pass/fail history.
func TestRecordErrorAlsoRecordsFailure(t *testing.T) {
	t.Parallel()

	d := NewStuckDetector(&StuckConfig{
		OscillationWindow: 4,
		PassHistorySize:   6,
	})

	taskID := "task-1"

	// Errors count as failures, passes as passes
	d.RecordResult(taskID, true)  // pass
	d.RecordError(taskID, "err")  // fail (via error)
	d.RecordResult(taskID, true)  // pass
	d.RecordError(taskID, "err2") // fail (via error)

	// Should detect oscillation
	results := d.taskResults[taskID]
	assert.Equal(t, []bool{true, false, true, false}, results)
}

// TestCustomThresholds tests with custom configuration.
func TestCustomThresholds(t *testing.T) {
	t.Parallel()

	t.Run("custom_repeat_threshold", func(t *testing.T) {
		d := NewStuckDetector(&StuckConfig{
			RepeatThreshold: 5,
		})

		taskID := "task-1"

		// 4 errors should not trigger with threshold of 5
		for range 4 {
			pattern := d.RecordError(taskID, "error A")
			assert.Nil(t, pattern)
		}

		// 5th should trigger
		pattern := d.RecordError(taskID, "error A")
		require.NotNil(t, pattern)
		assert.Contains(t, pattern.Message, "5 times")
	})

	t.Run("custom_oscillation_window", func(t *testing.T) {
		d := NewStuckDetector(&StuckConfig{
			OscillationWindow: 6,
		})

		taskID := "task-1"

		// 5 alternating results should not trigger with window of 6
		for i := range 5 {
			pattern := d.RecordResult(taskID, i%2 == 0)
			assert.Nil(t, pattern)
		}

		// 6th should trigger
		pattern := d.RecordResult(taskID, true)
		require.NotNil(t, pattern)
	})
}
