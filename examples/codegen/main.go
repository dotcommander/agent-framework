// Package main demonstrates code generation output formatting.
//
// The codegen package provides structured output for code changes,
// including blocks, modifications, and diffs with markdown formatting.
package main

import (
	"fmt"

	"github.com/dotcommander/agent-framework/output"
)

func main() {
	fmt.Println("=== Code Generation Output Demo ===")
	fmt.Println()

	// Create a code generator
	gen := output.NewCodeGenerator()

	// Add code blocks
	gen.AddBlock(
		"internal/handler/users.go",
		"go",
		`package handler

import (
	"encoding/json"
	"net/http"
)

// GetUser retrieves a user by ID.
func GetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := userService.Find(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(user)
}`,
	)

	// Add a block with line numbers
	gen.AddBlockWithLines(
		"internal/model/user.go",
		"go",
		`// User represents a user in the system.
type User struct {
	ID        string    `+"`json:\"id\"`"+`
	Email     string    `+"`json:\"email\"`"+`
	Name      string    `+"`json:\"name\"`"+`
	CreatedAt time.Time `+"`json:\"created_at\"`"+`
}`,
		15, // Start line
		23, // End line
	)

	// Add file creation
	gen.AddCreate(
		"internal/service/user_service.go",
		`package service

import "github.com/example/project/internal/model"

// UserService handles user operations.
type UserService struct {
	repo UserRepository
}

// NewUserService creates a new user service.
func NewUserService(repo UserRepository) *UserService {
	return &UserService{repo: repo}
}

// Find retrieves a user by ID.
func (s *UserService) Find(id string) (*model.User, error) {
	return s.repo.FindByID(id)
}`,
		"Create user service with repository pattern",
	)

	// Add file modification with old and new content
	oldConfig := `package config

type Config struct {
	Port     int
	Database string
}`

	newConfig := `package config

import "time"

type Config struct {
	Port         int
	Database     string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxConns     int
}`

	gen.AddModify(
		"internal/config/config.go",
		oldConfig,
		newConfig,
		"Add timeout and connection pool settings",
	)

	// Add another modification
	oldMain := `package main

func main() {
	http.ListenAndServe(":8080", nil)
}`

	newMain := `package main

import (
	"log"
	"net/http"
)

func main() {
	cfg := config.Load()
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	log.Fatal(srv.ListenAndServe())
}`

	gen.AddModify(
		"cmd/server/main.go",
		oldMain,
		newMain,
		"Use configuration for server settings",
	)

	// Add file deletion
	gen.AddDelete(
		"internal/legacy/old_handler.go",
		`package legacy

// Deprecated: Use handler package instead.
func OldHandler() {}`,
		"Remove deprecated legacy handler",
	)

	// Set summary
	gen.WithSummary(`Refactored the user module to use the repository pattern:

- Added UserService for business logic
- Added UserRepository interface for data access
- Updated configuration with timeout settings
- Removed deprecated legacy handler`)

	// Build the output
	out := gen.Build()

	// Display as markdown
	fmt.Println("=== Markdown Output ===")
	fmt.Println()
	fmt.Println(out.FormatMarkdown())

	// Display raw structure
	fmt.Println("=== Raw Structure ===")
	fmt.Println()

	fmt.Printf("Blocks: %d\n", len(out.Blocks))
	for i, block := range out.Blocks {
		fmt.Printf("  %d. %s (%s)", i+1, block.FilePath, block.Language)
		if block.StartLine > 0 {
			fmt.Printf(" lines %d-%d", block.StartLine, block.EndLine)
		}
		fmt.Println()
	}

	fmt.Printf("\nChanges: %d\n", len(out.Changes))
	for i, change := range out.Changes {
		fmt.Printf("  %d. [%s] %s: %s\n", i+1, change.ChangeType, change.FilePath, change.Description)
	}

	fmt.Printf("\nDiffs: %d\n", len(out.Diffs))
	for i, diff := range out.Diffs {
		fmt.Printf("  %d. %s: +%d -%d lines\n",
			i+1, diff.FilePath, diff.Stats.Additions, diff.Stats.Deletions)
	}

	// Demonstrate building changes incrementally
	fmt.Println("\n=== Incremental Building ===")
	fmt.Println()

	gen2 := output.NewCodeGenerator().
		AddBlock("file1.go", "", "package main").
		AddBlock("file2.go", "", "package util").
		AddCreate("file3.go", "package new", "New file").
		WithSummary("Quick changes")

	out2 := gen2.Build()
	fmt.Printf("Second generator: %d blocks, %d changes\n",
		len(out2.Blocks), len(out2.Changes))

	// Show diff line types
	fmt.Println("\n=== Diff Line Types ===")
	fmt.Println()

	if len(out.Diffs) > 0 {
		diff := out.Diffs[0]
		fmt.Printf("First diff (%s):\n", diff.FilePath)

		contextCount, addCount, removeCount := 0, 0, 0
		for _, line := range diff.Lines {
			switch line.Type {
			case "context":
				contextCount++
			case "add":
				addCount++
			case "remove":
				removeCount++
			}
		}

		fmt.Printf("  Context lines: %d\n", contextCount)
		fmt.Printf("  Added lines: %d\n", addCount)
		fmt.Printf("  Removed lines: %d\n", removeCount)
	}
}
