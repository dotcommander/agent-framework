# Context Compaction

Manage conversation context to stay within token limits.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent-framework/client"
)

func main() {
    ctx := context.Background()

    // Create compactor with LLM summarization
    compactor := client.NewSimpleCompactor(nil, func(ctx context.Context, prompt string) (string, error) {
        // Your LLM call here
        return summarize(ctx, prompt)
    })

    messages := []client.CompactionMessage{
        {Role: "user", Content: "Long message 1...", Tokens: 500},
        {Role: "assistant", Content: "Long response 1...", Tokens: 800},
        {Role: "user", Content: "Long message 2...", Tokens: 600},
        // ... more messages
    }

    // Check if compaction needed
    if compactor.ShouldCompact(messages, 4000) {
        // Compact to stay under limit
        compacted, err := compactor.Compact(ctx, messages)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Reduced from %d to %d messages\n", len(messages), len(compacted))
    }
}
```

## Core Concepts

### CompactionMessage

Represents a conversation message:

```go
type CompactionMessage struct {
    Role    string // "user", "assistant", "system"
    Content string // Message content
    Tokens  int    // Token count (estimated if 0)
}
```

### Compactor Interface

```go
type Compactor interface {
    // Check if compaction is needed
    ShouldCompact(messages []CompactionMessage, maxTokens int) bool

    // Compact messages to reduce tokens
    Compact(ctx context.Context, messages []CompactionMessage) ([]CompactionMessage, error)

    // Estimate tokens for content
    EstimateTokens(content string) int
}
```

## SimpleCompactor

LLM-based summarization of older messages.

### How It Works

```
Before compaction:
┌────────────────────────────────────────────────────────────┐
│ [msg1] [msg2] [msg3] [msg4] [msg5] [msg6] [msg7] [msg8]   │
│ ◄──────── to summarize ─────────► ◄── keep recent ──►    │
└────────────────────────────────────────────────────────────┘

After compaction:
┌────────────────────────────────────────────────────────────┐
│ [summary of msg1-msg5]  [msg6] [msg7] [msg8]              │
└────────────────────────────────────────────────────────────┘
```

### Creating a SimpleCompactor

```go
// With default config
compactor := client.NewSimpleCompactor(nil, summarizerFunc)

// With custom config
config := &client.CompactorConfig{
    Threshold:        0.8,    // Compact at 80% of max
    KeepRecent:       5,      // Keep last 5 messages intact
    MaxSummaryTokens: 2000,   // Limit summary size
    SummaryPrompt:    "Summarize this conversation:\n%s",
}
compactor := client.NewSimpleCompactor(config, summarizerFunc)
```

### Configuration Options

```go
type CompactorConfig struct {
    // Threshold triggers compaction (0.0-1.0)
    // 0.8 = compact when 80% of maxTokens used
    Threshold float64

    // Number of recent messages to keep uncompacted
    KeepRecent int

    // Prompt template for summarization
    // %s is replaced with conversation text
    SummaryPrompt string

    // Maximum tokens for the summary
    MaxSummaryTokens int
}
```

### Default Configuration

```go
config := client.DefaultCompactorConfig()
// Threshold:        0.8
// KeepRecent:       5
// MaxSummaryTokens: 2000
// SummaryPrompt:    (preserves key decisions, context, status, questions)
```

### Usage Example

```go
func manageConversation(ctx context.Context, messages []client.CompactionMessage) ([]client.CompactionMessage, error) {
    maxTokens := 100000  // Claude's context limit

    compactor := client.NewSimpleCompactor(nil, func(ctx context.Context, prompt string) (string, error) {
        // Call your LLM to summarize
        return llmClient.Query(ctx, prompt)
    })

    // Check if we're approaching the limit
    if compactor.ShouldCompact(messages, maxTokens) {
        return compactor.Compact(ctx, messages)
    }

    return messages, nil
}
```

## SlidingWindowCompactor

Simple window-based compaction without LLM calls.

### How It Works

```
Before (window=4):
[msg1] [msg2] [msg3] [msg4] [msg5] [msg6] [msg7]

After:
                    [msg4] [msg5] [msg6] [msg7]
```

### Creating a SlidingWindowCompactor

```go
// Keep last 20 messages
compactor := client.NewSlidingWindowCompactor(20)
```

### Usage Example

```go
func simpleCompaction(messages []client.CompactionMessage) []client.CompactionMessage {
    compactor := client.NewSlidingWindowCompactor(10)

    if compactor.ShouldCompact(messages, 0) {  // maxTokens ignored
        compacted, _ := compactor.Compact(context.Background(), messages)
        return compacted
    }

    return messages
}
```

### When to Use

- Fast, deterministic behavior needed
- No LLM available for summarization
- Simple truncation is acceptable
- Cost-conscious scenarios

## Token Counting

### Built-in Estimators

```go
// Character-based (4 chars per token)
counter := &client.CharBasedCounter{}
tokens := counter.Count("Hello, world!")  // ~3 tokens

// Word-based (words * 1.3)
counter := &client.WordBasedCounter{}
tokens := counter.Count("Hello, world!")  // ~3 tokens
```

### Using Compactor's Estimator

```go
compactor := client.NewSimpleCompactor(nil, nil)
tokens := compactor.EstimateTokens("Your text here...")
```

### Custom Token Counter

```go
type TokenCounter interface {
    Count(text string) int
}

// Example with tiktoken
type TiktokenCounter struct {
    encoder *tiktoken.Tiktoken
}

func (c *TiktokenCounter) Count(text string) int {
    tokens := c.encoder.Encode(text, nil, nil)
    return len(tokens)
}
```

## Example: Agent Loop Integration

```go
type CompactingLoop struct {
    compactor client.Compactor
    messages  []client.CompactionMessage
    maxTokens int
}

func (l *CompactingLoop) GatherContext(ctx context.Context, state *app.LoopState) (*app.LoopContext, error) {
    // Compact if needed
    if l.compactor.ShouldCompact(l.messages, l.maxTokens) {
        compacted, err := l.compactor.Compact(ctx, l.messages)
        if err != nil {
            return nil, fmt.Errorf("compact: %w", err)
        }
        l.messages = compacted
    }

    // Convert to loop context
    loopMessages := make([]app.Message, len(l.messages))
    for i, m := range l.messages {
        loopMessages[i] = app.Message{Role: m.Role, Content: m.Content}
    }

    return &app.LoopContext{
        Messages:   loopMessages,
        TokenCount: l.totalTokens(),
    }, nil
}

func (l *CompactingLoop) totalTokens() int {
    total := 0
    for _, m := range l.messages {
        if m.Tokens > 0 {
            total += m.Tokens
        } else {
            total += l.compactor.EstimateTokens(m.Content)
        }
    }
    return total
}
```

## Example: Conversation Manager

```go
type ConversationManager struct {
    messages  []client.CompactionMessage
    compactor client.Compactor
    maxTokens int
}

func NewConversationManager(maxTokens int, summarizer func(context.Context, string) (string, error)) *ConversationManager {
    return &ConversationManager{
        messages:  make([]client.CompactionMessage, 0),
        compactor: client.NewSimpleCompactor(nil, summarizer),
        maxTokens: maxTokens,
    }
}

func (m *ConversationManager) AddMessage(ctx context.Context, role, content string) error {
    tokens := m.compactor.EstimateTokens(content)
    m.messages = append(m.messages, client.CompactionMessage{
        Role:    role,
        Content: content,
        Tokens:  tokens,
    })

    // Auto-compact if needed
    if m.compactor.ShouldCompact(m.messages, m.maxTokens) {
        compacted, err := m.compactor.Compact(ctx, m.messages)
        if err != nil {
            return fmt.Errorf("compact: %w", err)
        }
        m.messages = compacted
    }

    return nil
}

func (m *ConversationManager) GetMessages() []client.CompactionMessage {
    return m.messages
}

func (m *ConversationManager) TotalTokens() int {
    total := 0
    for _, msg := range m.messages {
        total += msg.Tokens
    }
    return total
}
```

## Choosing a Strategy

| Strategy | Pros | Cons | Best For |
|----------|------|------|----------|
| SimpleCompactor | Preserves meaning | Requires LLM, costs money | Long conversations needing context |
| SlidingWindow | Fast, free, predictable | Loses old context | Simple Q&A, debugging |

## Tips

1. **Set threshold below 1.0**: Leave room for the next response
   ```go
   config.Threshold = 0.75  // Compact at 75%, leaving 25% buffer
   ```

2. **Keep critical messages**: Increase `KeepRecent` for complex tasks
   ```go
   config.KeepRecent = 10  // Keep more context
   ```

3. **Monitor token usage**: Track before/after compaction
   ```go
   before := totalTokens(messages)
   compacted, _ := compactor.Compact(ctx, messages)
   after := totalTokens(compacted)
   fmt.Printf("Compacted: %d -> %d tokens\n", before, after)
   ```

4. **Customize summary prompt**: Tailor for your use case
   ```go
   config.SummaryPrompt = `Summarize this technical discussion:
   - Preserve code snippets
   - Keep error messages
   - Note decisions made

   Conversation:
   %s`
   ```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [AGENT-LOOP.md](AGENT-LOOP.md) - Using compaction in loops
- [SUBAGENTS.md](SUBAGENTS.md) - Managing subagent context
