# Quick Start Guide

Get started with the Agent CLI Framework in 5 minutes.

## Prerequisites

- Go 1.22 or later
- Claude CLI installed and configured

## Installation

```bash
# Clone the repository
cd ~/go/src
git clone <repo-url> agent
cd agent

# Verify build
go build ./...
```

## Your First Agent

Create `main.go`:

```go
package main

import (
    "log"
    "github.com/dotcommander/agent-framework/app"
)

func main() {
    app := app.New("hello", "1.0.0",
        app.WithSystemPrompt("You are a helpful assistant."),
    )

    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Build and run:

```bash
go build -o hello
./hello "What is the capital of France?"
```

## Adding Tools

Tools give Claude capabilities beyond text generation.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/dotcommander/agent-framework/app"
    "github.com/dotcommander/agent-framework/tools"
)

func main() {
    // Create a tool
    timeTool := tools.TypedTool(
        "current_time",
        "Returns the current time",
        map[string]any{
            "type": "object",
            "properties": map[string]any{},
        },
        func(ctx context.Context, _ struct{}) (struct {
            Time string `json:"time"`
        }, error) {
            return struct {
                Time string `json:"time"`
            }{
                Time: time.Now().Format(time.RFC3339),
            }, nil
        },
    )

    app := app.New("time-agent", "1.0.0",
        app.WithSystemPrompt("You can tell users the current time."),
        app.WithTool(timeTool),
    )

    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Custom Run Logic

Control how your agent processes input:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dotcommander/agent-framework/app"
)

func main() {
    app := app.New("custom", "1.0.0",
        app.WithRunFunc(func(ctx context.Context, a *app.App, args []string) error {
            if len(args) == 0 {
                fmt.Println("Usage: custom <prompt>")
                return nil
            }

            // Custom pre-processing
            prompt := fmt.Sprintf("Please respond concisely: %s", args[0])

            // Query Claude
            response, err := a.Client().Query(ctx, prompt)
            if err != nil {
                return err
            }

            // Custom post-processing
            fmt.Printf("Claude says: %s\n", response)
            return nil
        }),
    )

    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Input Processing

The framework automatically detects and processes different input types:

```bash
# Plain text
./myapp "Tell me a joke"

# File input
./myapp "/path/to/document.txt"

# URL input (fetches and processes)
./myapp "https://example.com/article.html"

# Glob pattern (reads multiple files)
./myapp "*.go"
```

## Output Formatting

Control output format:

```go
import (
    "github.com/dotcommander/agent-framework/output"
)

app := app.New("myapp", "1.0.0",
    app.WithOutputFormat(output.NewJSONFormatter(true)),
)
```

Or use the CLI flag:

```bash
./myapp --format json "What is 2+2?"
./myapp --format markdown "Explain Go interfaces"
```

## Examples

The repository includes examples:

### Basic Example

```bash
go build -o agent-example ./cmd/agent
./agent-example "What is the meaning of life?"
```

### Advanced Example

```bash
go build -o advanced ./examples/advanced
./advanced "What is 42 multiplied by 2?"
./advanced "What time is it in Tokyo?"
```

## Next Steps

1. Read the full [README.md](../README.md) for architecture details
2. Explore the [examples](../examples/) directory
3. Check out the [agent-sdk-go documentation](https://github.com/dotcommander/agent-sdk-go)
4. Build your own tools and share them!

## Troubleshooting

### "Claude CLI not found"

Ensure Claude CLI is installed and in your PATH:

```bash
which claude
```

### "Module not found"

Run go mod tidy to fetch dependencies:

```bash
go mod tidy
```

### Build errors

Run go mod tidy:

```bash
go mod tidy
go build ./...
```

## Resources

- [Claude CLI Documentation](https://docs.anthropic.com/claude/docs/quickstart)
- [Agent SDK Go Repository](https://github.com/dotcommander/agent-sdk-go)
- [Cobra Documentation](https://github.com/spf13/cobra)
