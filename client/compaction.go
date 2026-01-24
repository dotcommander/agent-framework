package client

import (
	"context"
	"fmt"
	"strings"
)

// CompactionMessage represents a conversation message for compaction.
type CompactionMessage struct {
	Role    string
	Content string
	Tokens  int // Estimated token count
}

// Compactor handles context compaction when approaching token limits.
type Compactor interface {
	// ShouldCompact returns true if compaction is needed.
	ShouldCompact(messages []CompactionMessage, maxTokens int) bool

	// Compact summarizes messages to reduce token count.
	Compact(ctx context.Context, messages []CompactionMessage) ([]CompactionMessage, error)

	// EstimateTokens estimates token count for a message.
	EstimateTokens(content string) int
}

// CompactorConfig configures compaction behavior.
type CompactorConfig struct {
	// Threshold is the percentage of max tokens that triggers compaction (0.0-1.0).
	Threshold float64

	// KeepRecent is the number of recent messages to preserve uncompacted.
	KeepRecent int

	// SummaryPrompt is the prompt used for summarization.
	SummaryPrompt string

	// MaxSummaryTokens limits the summary size.
	MaxSummaryTokens int
}

// DefaultCompactorConfig returns sensible defaults.
func DefaultCompactorConfig() *CompactorConfig {
	return &CompactorConfig{
		Threshold:        0.8, // Compact at 80% of max
		KeepRecent:       5,   // Keep last 5 messages
		MaxSummaryTokens: 2000,
		SummaryPrompt: `Summarize the following conversation concisely, preserving:
- Key decisions made
- Important context and constraints
- Current task status
- Any unresolved questions

Conversation:
%s

Summary:`,
	}
}

// SimpleCompactor provides basic compaction using token estimation.
type SimpleCompactor struct {
	config     *CompactorConfig
	summarizer func(ctx context.Context, prompt string) (string, error)
}

// NewSimpleCompactor creates a new compactor.
func NewSimpleCompactor(config *CompactorConfig, summarizer func(ctx context.Context, prompt string) (string, error)) *SimpleCompactor {
	if config == nil {
		config = DefaultCompactorConfig()
	}
	return &SimpleCompactor{
		config:     config,
		summarizer: summarizer,
	}
}

// ShouldCompact checks if compaction is needed.
func (c *SimpleCompactor) ShouldCompact(messages []CompactionMessage, maxTokens int) bool {
	total := c.totalTokens(messages)
	threshold := int(float64(maxTokens) * c.config.Threshold)
	return total >= threshold
}

// Compact summarizes older messages while keeping recent ones.
func (c *SimpleCompactor) Compact(ctx context.Context, messages []CompactionMessage) ([]CompactionMessage, error) {
	if len(messages) <= c.config.KeepRecent {
		return messages, nil // Nothing to compact
	}

	// Split into old (to summarize) and recent (to keep)
	splitIdx := len(messages) - c.config.KeepRecent
	oldMessages := messages[:splitIdx]
	recentMessages := messages[splitIdx:]

	// Build conversation text for summarization
	var conversationText strings.Builder
	for _, msg := range oldMessages {
		fmt.Fprintf(&conversationText, "[%s]: %s\n\n", msg.Role, msg.Content)
	}

	// Create summary prompt
	prompt := fmt.Sprintf(c.config.SummaryPrompt, conversationText.String())

	// Get summary from LLM
	summary, err := c.summarizer(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("summarize: %w", err)
	}

	// Build compacted message list
	compacted := make([]CompactionMessage, 0, 1+len(recentMessages))

	// Add summary as system message
	compacted = append(compacted, CompactionMessage{
		Role:    "system",
		Content: fmt.Sprintf("[Conversation Summary]\n%s", summary),
		Tokens:  c.EstimateTokens(summary),
	})

	// Add recent messages
	compacted = append(compacted, recentMessages...)

	return compacted, nil
}

// EstimateTokens provides a rough token estimate (4 chars per token).
func (c *SimpleCompactor) EstimateTokens(content string) int {
	return len(content) / 4
}

// totalTokens sums token counts for all messages.
func (c *SimpleCompactor) totalTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		if msg.Tokens > 0 {
			total += msg.Tokens
		} else {
			total += c.EstimateTokens(msg.Content)
		}
	}
	return total
}

// SlidingWindowCompactor uses a sliding window approach (no LLM needed).
type SlidingWindowCompactor struct {
	windowSize int
}

// NewSlidingWindowCompactor creates a simple sliding window compactor.
func NewSlidingWindowCompactor(windowSize int) *SlidingWindowCompactor {
	if windowSize <= 0 {
		windowSize = 20
	}
	return &SlidingWindowCompactor{windowSize: windowSize}
}

// ShouldCompact checks if messages exceed window size.
func (c *SlidingWindowCompactor) ShouldCompact(messages []CompactionMessage, _ int) bool {
	return len(messages) > c.windowSize
}

// Compact keeps only the most recent messages within window.
func (c *SlidingWindowCompactor) Compact(_ context.Context, messages []CompactionMessage) ([]CompactionMessage, error) {
	if len(messages) <= c.windowSize {
		return messages, nil
	}
	return messages[len(messages)-c.windowSize:], nil
}

// EstimateTokens provides a rough estimate.
func (c *SlidingWindowCompactor) EstimateTokens(content string) int {
	return len(content) / 4
}

// TokenCounter provides accurate token counting.
type TokenCounter interface {
	Count(text string) int
}

// CharBasedCounter estimates tokens as chars/4.
type CharBasedCounter struct{}

func (c *CharBasedCounter) Count(text string) int {
	return len(text) / 4
}

// WordBasedCounter estimates tokens as words * 1.3.
type WordBasedCounter struct{}

func (c *WordBasedCounter) Count(text string) int {
	words := len(strings.Fields(text))
	return int(float64(words) * 1.3)
}
