# Structured Output DX Improvements

**Status**: Draft
**Source**: Developer feedback from go/src/learn
**Pain Level**: High - 35 lines of manual JSON parsing that should be 5 lines

## Problem Statement

The agent framework wraps the SDK but doesn't expose structured output features. Developers manually parse streaming JSON responses when the SDK already supports:
- `claude.WithJSONSchema(schema)` - force JSON output conforming to schema
- `claude.WithEnableStructuredOutput(true)` - enable structured responses
- `claude/parser/` - streaming JSON parser with buffering

## Current Pain (from developer feedback)

```go
// 35 lines of manual JSON parsing in parseInsightsJSON()
// - Manual nesting depth tracking
// - Finding first complete array
// - Brittle string manipulation
```

## Proposed Solution

### 1. Expose SDK Structured Output in Agent Framework

**File**: `client/client.go`

```go
// New client options to add:

// WithStructuredOutput enables JSON schema enforcement on responses.
func WithStructuredOutput(schema map[string]any) ClientOption {
    return func(o *clientOptions) {
        o.jsonSchema = schema
    }
}

// WithStructuredOutputType generates schema from Go struct.
func WithStructuredOutputType[T any]() ClientOption {
    return func(o *clientOptions) {
        o.jsonSchema = schemaFromType[T]()
    }
}
```

**Usage**:
```go
type Insights struct {
    Items []Insight `json:"insights"`
}

client, _ := client.New(ctx, sdkOpts,
    client.WithStructuredOutputType[Insights](),
    client.WithRetry(nil), // already exists
)

// Response is guaranteed valid JSON matching schema
response, err := client.Query(ctx, prompt)
var insights Insights
json.Unmarshal([]byte(response), &insights) // Always succeeds
```

### 2. Add Typed Query Method

**File**: `client/structured.go` (new)

```go
// QueryTyped sends a prompt and returns a typed response.
// Automatically enables structured output with schema derived from T.
func QueryTyped[T any](ctx context.Context, q Querier, prompt string) (*T, error) {
    // Generate schema from T
    schema := schemaFromType[T]()

    // Enable structured output mode
    response, err := q.Query(ctx, prompt)
    if err != nil {
        return nil, err
    }

    var result T
    if err := json.Unmarshal([]byte(response), &result); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    return &result, nil
}
```

**Usage** (the 5-line version):
```go
insights, err := client.QueryTyped[[]Insight](ctx, llm, prompt)
if err != nil {
    return err
}
// Done. No manual parsing.
```

### 3. Streaming with Accumulator

**File**: `client/streaming.go` (new)

```go
// StreamAccumulator collects streaming chunks into complete JSON.
type StreamAccumulator[T any] struct {
    parser    *parser.Parser
    result    T
    complete  bool
    onChunk   func(partial T) // Optional progress callback
}

// QueryStreamTyped streams response and returns typed result when complete.
func QueryStreamTyped[T any](ctx context.Context, q StreamingQuerier, prompt string,
    opts ...StreamOption) (*T, error) {

    acc := &StreamAccumulator[T]{
        parser: parser.NewParser(),
    }

    // Apply options (progress callbacks, etc.)
    for _, opt := range opts {
        opt(acc)
    }

    msgChan, errChan := q.QueryStream(ctx, prompt)

    for msg := range msgChan {
        // Accumulate text content
        acc.parser.ParseMessage(extractText(msg))

        // Try to parse partial result for progress
        if acc.onChunk != nil {
            if partial, ok := tryParse[T](acc.parser.GetBuffer()); ok {
                acc.onChunk(partial)
            }
        }
    }

    // Check for streaming errors
    if err := <-errChan; err != nil {
        return nil, err
    }

    // Parse final result
    var result T
    if err := json.Unmarshal([]byte(acc.parser.GetBuffer()), &result); err != nil {
        return nil, fmt.Errorf("parse final result: %w", err)
    }

    return &result, nil
}

// Stream options
type StreamOption func(*streamOptions)

func WithProgress[T any](fn func(partial T)) StreamOption {
    return func(o *streamOptions) {
        o.onProgress = fn
    }
}

func WithBytesProgress(fn func(bytes int)) StreamOption {
    return func(o *streamOptions) {
        o.onBytes = fn
    }
}
```

**Usage**:
```go
insights, err := client.QueryStreamTyped[[]Insight](ctx, llm, prompt,
    client.WithBytesProgress(func(n int) {
        spinner.SetBytes(n)
    }),
)
```

### 4. Schema Generation from Go Types

**File**: `client/schema.go` (new)

```go
// schemaFromType generates JSON Schema from Go struct using reflection.
func schemaFromType[T any]() map[string]any {
    var zero T
    return generateSchema(reflect.TypeOf(zero))
}

func generateSchema(t reflect.Type) map[string]any {
    // Handle pointer types
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }

    switch t.Kind() {
    case reflect.String:
        return map[string]any{"type": "string"}
    case reflect.Int, reflect.Int64:
        return map[string]any{"type": "integer"}
    case reflect.Float64:
        return map[string]any{"type": "number"}
    case reflect.Bool:
        return map[string]any{"type": "boolean"}
    case reflect.Slice:
        return map[string]any{
            "type":  "array",
            "items": generateSchema(t.Elem()),
        }
    case reflect.Struct:
        return generateStructSchema(t)
    default:
        return map[string]any{"type": "object"}
    }
}

func generateStructSchema(t reflect.Type) map[string]any {
    props := make(map[string]any)
    required := []string{}

    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        jsonTag := field.Tag.Get("json")
        if jsonTag == "-" {
            continue
        }

        name := strings.Split(jsonTag, ",")[0]
        if name == "" {
            name = field.Name
        }

        props[name] = generateSchema(field.Type)

        // Check for required tag or pointer (optional)
        if field.Type.Kind() != reflect.Ptr {
            required = append(required, name)
        }
    }

    return map[string]any{
        "type":       "object",
        "properties": props,
        "required":   required,
    }
}
```

## Implementation Order

1. `WithStructuredOutput` option pass-through to SDK
2. `QueryTyped[T]` helper function
3. `schemaFromType[T]` reflection-based schema generation
4. Update README with existing feature docs
5. `QueryStreamTyped[T]` with accumulator
6. Progress callback options

## Success Criteria

- Developer's 35-line `parseInsightsJSON()` becomes 5 lines
- No manual JSON nesting depth tracking
- Type-safe responses with compile-time guarantees
- Progress callbacks for streaming UX

## Not In Scope

- Prompt templates (separate spec)
- Session persistence (separate spec)
- Response caching (already have retry/circuit breaker)
