package output

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Limits for diff generation to prevent resource exhaustion.
const (
	// MaxDiffLines is the maximum number of lines to process in a diff.
	// Files larger than this will be truncated with a warning.
	MaxDiffLines = 10000

	// MaxDiffOutputLines is the maximum number of diff output lines to generate.
	MaxDiffOutputLines = 5000
)

// CodeBlock represents a generated code block.
type CodeBlock struct {
	FilePath    string `json:"file_path"`
	Language    string `json:"language"`
	Content     string `json:"content"`
	StartLine   int    `json:"start_line,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	Description string `json:"description,omitempty"`
}

// CodeChange represents a file modification.
type CodeChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"` // "create", "modify", "delete"
	OldContent  string `json:"old_content,omitempty"`
	NewContent  string `json:"new_content"`
	Description string `json:"description,omitempty"`
}

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Type    string `json:"type"` // "context", "add", "remove"
	LineNum int    `json:"line_num,omitempty"`
	Content string `json:"content"`
}

// FileDiff represents a unified diff for a file.
type FileDiff struct {
	FilePath   string     `json:"file_path"`
	ChangeType string     `json:"change_type"`
	Lines      []DiffLine `json:"lines"`
	Stats      DiffStats  `json:"stats"`
}

// DiffStats contains diff statistics.
type DiffStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Changes   int `json:"changes"`
}

// CodeOutput contains structured code generation output.
type CodeOutput struct {
	Blocks  []CodeBlock  `json:"blocks,omitempty"`
	Changes []CodeChange `json:"changes,omitempty"`
	Diffs   []FileDiff   `json:"diffs,omitempty"`
	Summary string       `json:"summary,omitempty"`
}

// CodeGenerator produces structured code output.
type CodeGenerator struct {
	output *CodeOutput
}

// NewCodeGenerator creates a new code generator.
func NewCodeGenerator() *CodeGenerator {
	return &CodeGenerator{
		output: &CodeOutput{
			Blocks:  make([]CodeBlock, 0),
			Changes: make([]CodeChange, 0),
			Diffs:   make([]FileDiff, 0),
		},
	}
}

// AddBlock adds a code block to the output.
func (g *CodeGenerator) AddBlock(filePath, language, content string) *CodeGenerator {
	g.output.Blocks = append(g.output.Blocks, CodeBlock{
		FilePath: filePath,
		Language: detectLanguage(filePath, language),
		Content:  content,
	})
	return g
}

// AddBlockWithLines adds a code block with line range.
func (g *CodeGenerator) AddBlockWithLines(filePath, language, content string, startLine, endLine int) *CodeGenerator {
	g.output.Blocks = append(g.output.Blocks, CodeBlock{
		FilePath:  filePath,
		Language:  detectLanguage(filePath, language),
		Content:   content,
		StartLine: startLine,
		EndLine:   endLine,
	})
	return g
}

// AddCreate adds a file creation change.
func (g *CodeGenerator) AddCreate(filePath, content, description string) *CodeGenerator {
	g.output.Changes = append(g.output.Changes, CodeChange{
		FilePath:    filePath,
		ChangeType:  "create",
		NewContent:  content,
		Description: description,
	})
	return g
}

// AddModify adds a file modification change.
func (g *CodeGenerator) AddModify(filePath, oldContent, newContent, description string) *CodeGenerator {
	g.output.Changes = append(g.output.Changes, CodeChange{
		FilePath:    filePath,
		ChangeType:  "modify",
		OldContent:  oldContent,
		NewContent:  newContent,
		Description: description,
	})
	return g
}

// AddDelete adds a file deletion change.
func (g *CodeGenerator) AddDelete(filePath, oldContent, description string) *CodeGenerator {
	g.output.Changes = append(g.output.Changes, CodeChange{
		FilePath:    filePath,
		ChangeType:  "delete",
		OldContent:  oldContent,
		Description: description,
	})
	return g
}

// WithSummary sets the output summary.
func (g *CodeGenerator) WithSummary(summary string) *CodeGenerator {
	g.output.Summary = summary
	return g
}

// Build generates the final output.
func (g *CodeGenerator) Build() *CodeOutput {
	// Generate diffs for modifications
	for _, change := range g.output.Changes {
		if change.ChangeType == "modify" {
			diff := generateDiff(change.FilePath, change.OldContent, change.NewContent)
			g.output.Diffs = append(g.output.Diffs, diff)
		}
	}
	return g.output
}

// FormatMarkdown formats the output as markdown.
func (o *CodeOutput) FormatMarkdown() string {
	var sb strings.Builder

	if o.Summary != "" {
		fmt.Fprintf(&sb, "## Summary\n\n%s\n\n", o.Summary)
	}

	// Format code blocks
	if len(o.Blocks) > 0 {
		sb.WriteString("## Code Blocks\n\n")
		for _, block := range o.Blocks {
			if block.Description != "" {
				fmt.Fprintf(&sb, "### %s\n\n", block.Description)
			}
			fmt.Fprintf(&sb, "**File:** `%s`\n\n", block.FilePath)
			if block.StartLine > 0 {
				fmt.Fprintf(&sb, "**Lines:** %d-%d\n\n", block.StartLine, block.EndLine)
			}
			fmt.Fprintf(&sb, "```%s\n%s\n```\n\n", block.Language, block.Content)
		}
	}

	// Format changes
	if len(o.Changes) > 0 {
		sb.WriteString("## Changes\n\n")
		for _, change := range o.Changes {
			icon := changeIcon(change.ChangeType)
			fmt.Fprintf(&sb, "### %s `%s`\n\n", icon, change.FilePath)
			if change.Description != "" {
				fmt.Fprintf(&sb, "%s\n\n", change.Description)
			}
			if change.NewContent != "" && change.ChangeType != "delete" {
				lang := detectLanguage(change.FilePath, "")
				fmt.Fprintf(&sb, "```%s\n%s\n```\n\n", lang, change.NewContent)
			}
		}
	}

	// Format diffs
	if len(o.Diffs) > 0 {
		sb.WriteString("## Diffs\n\n")
		for _, diff := range o.Diffs {
			fmt.Fprintf(&sb, "### `%s`\n\n", diff.FilePath)
			fmt.Fprintf(&sb, "*+%d -%d*\n\n", diff.Stats.Additions, diff.Stats.Deletions)
			sb.WriteString("```diff\n")
			for _, line := range diff.Lines {
				prefix := " "
				switch line.Type {
				case "add":
					prefix = "+"
				case "remove":
					prefix = "-"
				}
				fmt.Fprintf(&sb, "%s%s\n", prefix, line.Content)
			}
			sb.WriteString("```\n\n")
		}
	}

	return sb.String()
}

// FormatJSON formats the output as JSON (via standard marshaling).
func (o *CodeOutput) FormatJSON() ([]byte, error) {
	// Use standard JSON marshaling from encoding/json
	// Caller should use json.Marshal(o) directly
	return nil, nil
}

// detectLanguage infers language from file extension.
func detectLanguage(filePath, override string) string {
	if override != "" {
		return override
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".md":
		return "markdown"
	default:
		return ""
	}
}

// changeIcon returns an emoji for the change type.
func changeIcon(changeType string) string {
	switch changeType {
	case "create":
		return "+"
	case "modify":
		return "~"
	case "delete":
		return "-"
	default:
		return "?"
	}
}

// generateDiff creates a simple line-by-line diff.
func generateDiff(filePath, oldContent, newContent string) FileDiff {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Enforce input size limits to prevent memory exhaustion
	inputTruncated := false
	if len(oldLines) > MaxDiffLines {
		oldLines = oldLines[:MaxDiffLines]
		inputTruncated = true
	}
	if len(newLines) > MaxDiffLines {
		newLines = newLines[:MaxDiffLines]
		inputTruncated = true
	}

	// Compute LCS and build diff lines
	lcs := computeLCS(oldLines, newLines)
	lines, stats, outputTruncated := buildDiffLines(oldLines, newLines, lcs, MaxDiffOutputLines)

	// Add truncation notice if limits were hit
	if inputTruncated || outputTruncated {
		lines = append(lines, DiffLine{
			Type:    "context",
			Content: "... (diff truncated due to size limits)",
		})
	}

	return FileDiff{
		FilePath:   filePath,
		ChangeType: "modify",
		Lines:      lines,
		Stats:      stats,
	}
}

// buildDiffLines constructs diff lines by comparing old/new against LCS.
// Returns the diff lines, statistics, and whether output was truncated.
func buildDiffLines(oldLines, newLines, lcs []string, limit int) ([]DiffLine, DiffStats, bool) {
	lines := make([]DiffLine, 0)
	stats := DiffStats{}
	truncated := false

	oldIdx, newIdx, lcsIdx := 0, 0, 0

	// Helper to check if output limit reached
	atLimit := func() bool {
		if len(lines) >= limit {
			truncated = true
			return true
		}
		return false
	}

	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		if atLimit() {
			break
		}

		if lcsIdx < len(lcs) {
			// Output removals (old lines not in LCS)
			for oldIdx < len(oldLines) && oldLines[oldIdx] != lcs[lcsIdx] && !atLimit() {
				lines = append(lines, DiffLine{
					Type:    "remove",
					LineNum: oldIdx + 1,
					Content: oldLines[oldIdx],
				})
				stats.Deletions++
				oldIdx++
			}

			// Output additions (new lines not in LCS)
			for newIdx < len(newLines) && newLines[newIdx] != lcs[lcsIdx] && !atLimit() {
				lines = append(lines, DiffLine{
					Type:    "add",
					LineNum: newIdx + 1,
					Content: newLines[newIdx],
				})
				stats.Additions++
				newIdx++
			}

			// Output context (matching line)
			if oldIdx < len(oldLines) && newIdx < len(newLines) && !atLimit() {
				lines = append(lines, DiffLine{
					Type:    "context",
					LineNum: newIdx + 1,
					Content: newLines[newIdx],
				})
				oldIdx++
				newIdx++
				lcsIdx++
			}
		} else {
			// No more LCS, output remaining as removals/additions
			for oldIdx < len(oldLines) && !atLimit() {
				lines = append(lines, DiffLine{
					Type:    "remove",
					LineNum: oldIdx + 1,
					Content: oldLines[oldIdx],
				})
				stats.Deletions++
				oldIdx++
			}
			for newIdx < len(newLines) && !atLimit() {
				lines = append(lines, DiffLine{
					Type:    "add",
					LineNum: newIdx + 1,
					Content: newLines[newIdx],
				})
				stats.Additions++
				newIdx++
			}
		}
	}

	stats.Changes = stats.Additions + stats.Deletions
	return lines, stats, truncated
}

// computeLCS finds the longest common subsequence between two slices of strings.
func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)

	// DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	// Backtrack to find LCS
	lcs := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		switch {
		case a[i-1] == b[j-1]:
			lcs = append([]string{a[i-1]}, lcs...)
			i--
			j--
		case dp[i-1][j] > dp[i][j-1]:
			i--
		default:
			j--
		}
	}

	return lcs
}
