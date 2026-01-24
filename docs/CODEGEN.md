# Code Generation

Structured code generation output with diffs, blocks, and formatting.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/dotcommander/agent-framework/output"
)

func main() {
    // Create code generator
    gen := output.NewCodeGenerator()

    // Add code blocks
    gen.AddBlock("main.go", "go", `package main

func main() {
    fmt.Println("Hello, World!")
}`)

    // Add file changes
    gen.AddModify("config.yaml",
        "debug: false",
        "debug: true",
        "Enable debug mode")

    gen.WithSummary("Added main function and enabled debug mode")

    // Build output
    codeOutput := gen.Build()

    // Format as markdown
    fmt.Println(codeOutput.FormatMarkdown())
}
```

## Core Types

### CodeBlock

A discrete code snippet:

```go
type CodeBlock struct {
    FilePath    string // Target file
    Language    string // Programming language
    Content     string // Code content
    StartLine   int    // Optional: line range start
    EndLine     int    // Optional: line range end
    Description string // Optional: description
}
```

### CodeChange

A file modification:

```go
type CodeChange struct {
    FilePath    string // File being changed
    ChangeType  string // "create", "modify", "delete"
    OldContent  string // Previous content (for modify/delete)
    NewContent  string // New content (for create/modify)
    Description string // What the change does
}
```

### FileDiff

Unified diff representation:

```go
type FileDiff struct {
    FilePath   string     // File path
    ChangeType string     // Change type
    Lines      []DiffLine // Diff lines
    Stats      DiffStats  // Statistics
}

type DiffLine struct {
    Type    string // "context", "add", "remove"
    LineNum int    // Line number
    Content string // Line content
}

type DiffStats struct {
    Additions int // Lines added
    Deletions int // Lines removed
    Changes   int // Total changes
}
```

### CodeOutput

Complete generation output:

```go
type CodeOutput struct {
    Blocks  []CodeBlock  // Code snippets
    Changes []CodeChange // File changes
    Diffs   []FileDiff   // Generated diffs
    Summary string       // Overall summary
}
```

## CodeGenerator

### Creating a Generator

```go
gen := output.NewCodeGenerator()
```

### Adding Code Blocks

```go
// Basic block
gen.AddBlock("main.go", "go", `func hello() {
    fmt.Println("Hello")
}`)

// Block with line range
gen.AddBlockWithLines("main.go", "go", `func hello() {
    fmt.Println("Hello")
}`, 10, 15)
```

### Adding Changes

```go
// Create new file
gen.AddCreate("new_file.go", `package main

func NewFeature() {}
`, "Add new feature file")

// Modify existing file
gen.AddModify("existing.go",
    "// old code",
    "// new code",
    "Update implementation")

// Delete file
gen.AddDelete("obsolete.go", "// old content", "Remove deprecated code")
```

### Setting Summary

```go
gen.WithSummary("Refactored error handling across 3 files")
```

### Building Output

```go
output := gen.Build()
// This also generates diffs for all "modify" changes
```

## Formatting Output

### Markdown Format

```go
markdown := output.FormatMarkdown()
```

Output example:
```markdown
## Summary

Refactored error handling across 3 files

## Code Blocks

**File:** `main.go`

```go
func hello() {
    fmt.Println("Hello")
}
```

## Changes

### ~ `existing.go`

Update implementation

```go
// new code
```

## Diffs

### `existing.go`

*+5 -3*

```diff
-// old code
+// new code
```
```

### JSON Format

Use standard Go JSON marshaling:

```go
import "encoding/json"

jsonBytes, err := json.MarshalIndent(output, "", "  ")
```

## Language Detection

Languages are automatically detected from file extensions:

```go
// Automatic detection
gen.AddBlock("main.go", "", code)      // Detects "go"
gen.AddBlock("app.tsx", "", code)      // Detects "tsx"
gen.AddBlock("styles.scss", "", code)  // Detects "scss"

// Manual override
gen.AddBlock("config", "yaml", code)   // Uses "yaml"
```

Supported extensions:
- Go: `.go`
- JavaScript/TypeScript: `.js`, `.ts`, `.jsx`, `.tsx`
- Python: `.py`
- Ruby: `.rb`
- Rust: `.rs`
- Java: `.java`
- C/C++: `.c`, `.h`, `.cpp`, `.hpp`, `.cc`
- C#: `.cs`
- PHP: `.php`
- Swift: `.swift`
- Kotlin: `.kt`
- Scala: `.scala`
- Shell: `.sh`, `.bash`
- SQL: `.sql`
- HTML/CSS: `.html`, `.css`, `.scss`
- Config: `.json`, `.yaml`, `.yml`, `.toml`, `.xml`
- Markdown: `.md`

## Diff Generation

Diffs are automatically generated for modifications using LCS (Longest Common Subsequence):

```go
gen.AddModify("file.go",
    `func old() {
    return 1
}`,
    `func new() {
    return 2
}`,
    "Rename function")

output := gen.Build()

// output.Diffs contains the unified diff
for _, diff := range output.Diffs {
    fmt.Printf("File: %s\n", diff.FilePath)
    fmt.Printf("Additions: %d\n", diff.Stats.Additions)
    fmt.Printf("Deletions: %d\n", diff.Stats.Deletions)
}
```

## Example: AI Code Review Output

```go
func formatCodeReview(review *Review) string {
    gen := output.NewCodeGenerator()

    gen.WithSummary(fmt.Sprintf(
        "Found %d issues, %d suggestions",
        len(review.Issues),
        len(review.Suggestions),
    ))

    // Add problematic code blocks
    for _, issue := range review.Issues {
        gen.AddBlockWithLines(
            issue.File,
            "",  // Auto-detect language
            issue.Code,
            issue.StartLine,
            issue.EndLine,
        )
    }

    // Add suggested fixes
    for _, suggestion := range review.Suggestions {
        gen.AddModify(
            suggestion.File,
            suggestion.Before,
            suggestion.After,
            suggestion.Reason,
        )
    }

    return gen.Build().FormatMarkdown()
}
```

## Example: Refactoring Tool Output

```go
func createRefactoringOutput(changes []FileChange) *output.CodeOutput {
    gen := output.NewCodeGenerator()

    var created, modified, deleted int

    for _, change := range changes {
        switch change.Type {
        case "create":
            gen.AddCreate(change.Path, change.NewContent, change.Description)
            created++
        case "modify":
            gen.AddModify(change.Path, change.OldContent, change.NewContent, change.Description)
            modified++
        case "delete":
            gen.AddDelete(change.Path, change.OldContent, change.Description)
            deleted++
        }
    }

    gen.WithSummary(fmt.Sprintf(
        "Refactoring complete: %d created, %d modified, %d deleted",
        created, modified, deleted,
    ))

    return gen.Build()
}
```

## Example: Code Generation MCP Tool

```go
func codeGenTool() *tools.Tool {
    return tools.NewTool(
        "generate_code",
        "Generate code with structured output",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "files": map[string]any{
                    "type": "array",
                    "items": map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "path":    map[string]any{"type": "string"},
                            "content": map[string]any{"type": "string"},
                        },
                    },
                },
                "summary": map[string]any{"type": "string"},
            },
            "required": []string{"files"},
        },
        func(ctx context.Context, input map[string]any) (any, error) {
            files := input["files"].([]any)
            summary, _ := input["summary"].(string)

            gen := output.NewCodeGenerator()

            for _, f := range files {
                file := f.(map[string]any)
                path := file["path"].(string)
                content := file["content"].(string)
                gen.AddCreate(path, content, "")
            }

            if summary != "" {
                gen.WithSummary(summary)
            }

            return gen.Build(), nil
        },
    )
}
```

## Chaining Operations

The generator supports method chaining:

```go
output := output.NewCodeGenerator().
    AddBlock("a.go", "go", codeA).
    AddBlock("b.go", "go", codeB).
    AddModify("c.go", old, new, "Fix bug").
    AddCreate("d.go", content, "New file").
    WithSummary("Multiple changes").
    Build()
```

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [MCP.md](MCP.md) - Creating code generation tools
- [STATE.md](STATE.md) - Tracking generated changes
