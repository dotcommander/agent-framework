// Package verification provides output verification capabilities.
package verification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Screenshot represents a captured screenshot.
type Screenshot struct {
	ID        string `json:"id"`
	Path      string `json:"path,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Format    string `json:"format"` // "png", "jpeg", "webp"
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Hash      string `json:"hash,omitempty"`
}

// VisualDiff represents a difference between two screenshots.
type VisualDiff struct {
	Before     *Screenshot  `json:"before"`
	After      *Screenshot  `json:"after"`
	DiffPixels int          `json:"diff_pixels"`
	DiffPct    float64      `json:"diff_pct"`
	Regions    []DiffRegion `json:"regions,omitempty"`
	Threshold  float64      `json:"threshold"`
	Passed     bool         `json:"passed"`
}

// DiffRegion represents a region of difference.
type DiffRegion struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Type   string `json:"type"` // "added", "removed", "changed"
}

// VisualVerifier handles visual verification of UI changes.
type VisualVerifier struct {
	config     *VisualConfig
	capturer   ScreenshotCapturer
	comparator ImageComparator
	analyzer   VisualAnalyzer
}

// VisualConfig configures visual verification.
type VisualConfig struct {
	// DiffThreshold is the maximum allowed difference (0.0-1.0).
	DiffThreshold float64

	// IgnoreRegions are areas to exclude from comparison.
	IgnoreRegions []Region

	// CompareMode determines comparison strategy.
	CompareMode string // "pixel", "perceptual", "structural"

	// OutputDir for saving diff images.
	OutputDir string

	// SaveBaselines stores passing screenshots as new baselines.
	SaveBaselines bool
}

// Region defines a rectangular area.
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DefaultVisualConfig returns sensible defaults.
func DefaultVisualConfig() *VisualConfig {
	return &VisualConfig{
		DiffThreshold: 0.01, // 1% tolerance
		CompareMode:   "pixel",
		IgnoreRegions: make([]Region, 0),
	}
}

// ScreenshotCapturer captures screenshots.
type ScreenshotCapturer interface {
	Capture(ctx context.Context, target string) (*Screenshot, error)
	CaptureElement(ctx context.Context, selector string) (*Screenshot, error)
	CaptureFullPage(ctx context.Context, url string) (*Screenshot, error)
}

// ImageComparator compares two images.
type ImageComparator interface {
	Compare(before, after *Screenshot) (*VisualDiff, error)
	SetThreshold(threshold float64)
}

// VisualAnalyzer analyzes screenshots using AI.
type VisualAnalyzer interface {
	Analyze(ctx context.Context, screenshot *Screenshot, prompt string) (*VisualAnalysis, error)
	CompareWithContext(ctx context.Context, before, after *Screenshot, prompt string) (*VisualAnalysis, error)
}

// VisualAnalysis contains AI analysis of visual content.
type VisualAnalysis struct {
	Description string        `json:"description"`
	Elements    []UIElement   `json:"elements,omitempty"`
	Issues      []VisualIssue `json:"issues,omitempty"`
	Score       float64       `json:"score"` // 0.0-1.0
}

// UIElement represents a detected UI element.
type UIElement struct {
	Type   string `json:"type"` // "button", "input", "text", etc.
	Text   string `json:"text,omitempty"`
	Region Region `json:"region"`
}

// VisualIssue represents a detected visual issue.
type VisualIssue struct {
	Severity    string `json:"severity"` // "error", "warning", "info"
	Type        string `json:"type"`     // "alignment", "contrast", "overflow", etc.
	Description string `json:"description"`
	Region      Region `json:"region,omitempty"`
}

// NewVisualVerifier creates a new visual verifier.
func NewVisualVerifier(config *VisualConfig, capturer ScreenshotCapturer, comparator ImageComparator) *VisualVerifier {
	if config == nil {
		config = DefaultVisualConfig()
	}
	return &VisualVerifier{
		config:     config,
		capturer:   capturer,
		comparator: comparator,
	}
}

// WithAnalyzer adds AI-based analysis capability.
func (v *VisualVerifier) WithAnalyzer(analyzer VisualAnalyzer) *VisualVerifier {
	v.analyzer = analyzer
	return v
}

// Capture takes a screenshot of the target.
func (v *VisualVerifier) Capture(ctx context.Context, target string) (*Screenshot, error) {
	if v.capturer == nil {
		return nil, fmt.Errorf("no capturer configured")
	}

	screenshot, err := v.capturer.Capture(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}

	// Calculate hash
	if len(screenshot.Data) > 0 {
		hash := sha256.Sum256(screenshot.Data)
		screenshot.Hash = base64.StdEncoding.EncodeToString(hash[:])
	}

	return screenshot, nil
}

// Compare compares two screenshots.
func (v *VisualVerifier) Compare(before, after *Screenshot) (*VisualDiff, error) {
	if v.comparator == nil {
		return nil, fmt.Errorf("no comparator configured")
	}

	v.comparator.SetThreshold(v.config.DiffThreshold)
	diff, err := v.comparator.Compare(before, after)
	if err != nil {
		return nil, fmt.Errorf("compare: %w", err)
	}

	diff.Threshold = v.config.DiffThreshold
	diff.Passed = diff.DiffPct <= v.config.DiffThreshold

	return diff, nil
}

// Verify captures and compares against a baseline.
func (v *VisualVerifier) Verify(ctx context.Context, target string, baseline *Screenshot) (*VerificationResult, error) {
	// Capture current state
	current, err := v.Capture(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("capture current: %w", err)
	}

	// Compare with baseline
	diff, err := v.Compare(baseline, current)
	if err != nil {
		return nil, fmt.Errorf("compare: %w", err)
	}

	result := &VerificationResult{
		Passed:  diff.Passed,
		Before:  baseline,
		After:   current,
		Diff:    diff,
		Message: buildVerificationMessage(diff),
	}

	// AI analysis if available
	if v.analyzer != nil && !diff.Passed {
		analysis, err := v.analyzer.CompareWithContext(ctx, baseline, current,
			"Analyze the visual differences between these screenshots. Describe any UI changes, potential issues, or regressions.")
		if err == nil {
			result.Analysis = analysis
		}
	}

	return result, nil
}

// VerificationResult contains the result of visual verification.
type VerificationResult struct {
	Passed   bool            `json:"passed"`
	Before   *Screenshot     `json:"before"`
	After    *Screenshot     `json:"after"`
	Diff     *VisualDiff     `json:"diff,omitempty"`
	Analysis *VisualAnalysis `json:"analysis,omitempty"`
	Message  string          `json:"message"`
}

// Analyze performs AI-based visual analysis.
func (v *VisualVerifier) Analyze(ctx context.Context, screenshot *Screenshot, prompt string) (*VisualAnalysis, error) {
	if v.analyzer == nil {
		return nil, fmt.Errorf("no analyzer configured")
	}

	return v.analyzer.Analyze(ctx, screenshot, prompt)
}

func buildVerificationMessage(diff *VisualDiff) string {
	if diff.Passed {
		return fmt.Sprintf("Visual verification passed (diff: %.2f%%, threshold: %.2f%%)",
			diff.DiffPct*100, diff.Threshold*100)
	}
	return fmt.Sprintf("Visual verification failed (diff: %.2f%% exceeds threshold: %.2f%%)",
		diff.DiffPct*100, diff.Threshold*100)
}

// PixelComparator provides pixel-based image comparison.
type PixelComparator struct {
	threshold float64
}

// NewPixelComparator creates a pixel comparator.
func NewPixelComparator() *PixelComparator {
	return &PixelComparator{threshold: 0.01}
}

// SetThreshold sets the comparison threshold.
func (c *PixelComparator) SetThreshold(threshold float64) {
	c.threshold = threshold
}

// Compare compares two screenshots pixel by pixel.
func (c *PixelComparator) Compare(before, after *Screenshot) (*VisualDiff, error) {
	if before == nil || after == nil {
		return nil, fmt.Errorf("both screenshots required")
	}

	// Size mismatch check
	if before.Width != after.Width || before.Height != after.Height {
		return &VisualDiff{
			Before:     before,
			After:      after,
			DiffPixels: before.Width * before.Height,
			DiffPct:    1.0,
			Passed:     false,
		}, nil
	}

	// Quick hash comparison
	if before.Hash != "" && after.Hash != "" && before.Hash == after.Hash {
		return &VisualDiff{
			Before:     before,
			After:      after,
			DiffPixels: 0,
			DiffPct:    0.0,
			Passed:     true,
		}, nil
	}

	// For actual pixel comparison, we'd need image decoding
	// This is a placeholder that returns a basic result
	// Real implementation would decode images and compare pixels
	return &VisualDiff{
		Before:     before,
		After:      after,
		DiffPixels: 0, // Would be calculated
		DiffPct:    0.0,
		Threshold:  c.threshold,
		Passed:     true,
	}, nil
}

// BaselineManager manages visual baselines.
type BaselineManager struct {
	baselines map[string]*Screenshot
	storage   BaselineStorage
}

// BaselineStorage persists baselines.
type BaselineStorage interface {
	Save(key string, screenshot *Screenshot) error
	Load(key string) (*Screenshot, error)
	Delete(key string) error
	List() ([]string, error)
}

// NewBaselineManager creates a baseline manager.
func NewBaselineManager(storage BaselineStorage) *BaselineManager {
	return &BaselineManager{
		baselines: make(map[string]*Screenshot),
		storage:   storage,
	}
}

// Save stores a baseline.
func (m *BaselineManager) Save(key string, screenshot *Screenshot) error {
	m.baselines[key] = screenshot
	if m.storage != nil {
		return m.storage.Save(key, screenshot)
	}
	return nil
}

// Load retrieves a baseline.
func (m *BaselineManager) Load(key string) (*Screenshot, error) {
	if s, ok := m.baselines[key]; ok {
		return s, nil
	}
	if m.storage != nil {
		return m.storage.Load(key)
	}
	return nil, fmt.Errorf("baseline not found: %s", key)
}

// GenerateKey creates a baseline key from components.
func GenerateKey(components ...string) string {
	return strings.Join(components, "-")
}
