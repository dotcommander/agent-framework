// Package main demonstrates file system state tracking.
//
// The state package provides file tracking, snapshotting, and change detection.
// This enables rollback capabilities and audit trails for file modifications.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/agent-framework/state"
)

func main() {
	fmt.Println("=== File System State Demo ===")
	fmt.Println()

	// Create a temporary directory for the demo
	tmpDir, err := os.MkdirTemp("", "state-demo-*")
	if err != nil {
		fmt.Printf("Error creating temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("Working directory: %s\n\n", tmpDir)

	// Create initial files
	files := map[string]string{
		"config.json": `{"version": "1.0", "debug": false}`,
		"main.go":     "package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n",
		"readme.md":   "# Demo Project\n\nThis is a demo.\n",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Printf("Error creating %s: %v\n", name, err)
			return
		}
		fmt.Printf("Created: %s\n", name)
	}
	fmt.Println()

	// Create a file system store
	store := state.NewFileSystemStore(tmpDir)

	// Track individual files
	fmt.Println("=== Tracking Files ===")
	fmt.Println()
	for name := range files {
		path := filepath.Join(tmpDir, name)
		if err := store.Track(path); err != nil {
			fmt.Printf("Error tracking %s: %v\n", name, err)
			continue
		}
		fmt.Printf("Tracking: %s\n", name)
	}
	fmt.Println()

	// Capture file state
	fmt.Println("=== File States ===")
	fmt.Println()
	configPath := filepath.Join(tmpDir, "config.json")
	configState, err := store.CaptureFile(configPath)
	if err != nil {
		fmt.Printf("Error capturing state: %v\n", err)
		return
	}

	fmt.Printf("File: %s\n", filepath.Base(configState.Path))
	fmt.Printf("  Exists: %v\n", configState.Exists)
	fmt.Printf("  Size: %d bytes\n", configState.Size)
	fmt.Printf("  Hash: %s...\n", configState.Hash[:16])
	fmt.Printf("  ModTime: %v\n", configState.ModTime.Format(time.RFC3339))
	fmt.Println()

	// Create initial snapshot
	fmt.Println("=== Creating Snapshot ===")
	fmt.Println()
	snapshot1 := store.CreateSnapshot("Initial state before modifications")
	fmt.Printf("Snapshot created: %s\n", snapshot1.ID)
	fmt.Printf("  Message: %s\n", snapshot1.Message)
	fmt.Printf("  Files: %d\n", len(snapshot1.Files))
	fmt.Printf("  Timestamp: %v\n", snapshot1.Timestamp.Format(time.RFC3339))
	fmt.Println()

	// Modify files
	fmt.Println("=== Modifying Files ===")
	fmt.Println()

	// Modify config.json
	newConfig := `{"version": "2.0", "debug": true, "feature_flags": ["beta"]}`
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		fmt.Printf("Error modifying config: %v\n", err)
		return
	}
	fmt.Println("Modified: config.json (updated version and debug)")

	// Modify main.go
	mainPath := filepath.Join(tmpDir, "main.go")
	newMain := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n}\n"
	if err := os.WriteFile(mainPath, []byte(newMain), 0644); err != nil {
		fmt.Printf("Error modifying main.go: %v\n", err)
		return
	}
	fmt.Println("Modified: main.go (added fmt import)")

	// Create new file
	newFilePath := filepath.Join(tmpDir, "utils.go")
	if err := os.WriteFile(newFilePath, []byte("package main\n\nfunc helper() {}\n"), 0644); err != nil {
		fmt.Printf("Error creating utils.go: %v\n", err)
		return
	}
	store.Track(newFilePath)
	fmt.Println("Created: utils.go")
	fmt.Println()

	// Detect changes
	fmt.Println("=== Detecting Changes ===")
	fmt.Println()
	changes, err := store.DetectChanges()
	if err != nil {
		fmt.Printf("Error detecting changes: %v\n", err)
		return
	}

	fmt.Printf("Detected %d changes:\n", len(changes))
	for _, change := range changes {
		fmt.Printf("  [%s] %s\n", change.ChangeType, filepath.Base(change.Path))
		if change.Before != nil && change.After != nil {
			fmt.Printf("    Size: %d -> %d bytes\n", change.Before.Size, change.After.Size)
		}
	}
	fmt.Println()

	// Record a manual change
	fmt.Println("=== Recording Manual Change ===")
	fmt.Println()
	if err := store.RecordChange(configPath, "modify"); err != nil {
		fmt.Printf("Error recording change: %v\n", err)
	} else {
		fmt.Println("Recorded manual change to config.json")
	}

	allChanges := store.GetChanges()
	fmt.Printf("Total recorded changes: %d\n\n", len(allChanges))

	// Create second snapshot
	snapshot2 := store.CreateSnapshot("After modifications")
	fmt.Printf("Created snapshot: %s\n\n", snapshot2.ID)

	// List all snapshots
	fmt.Println("=== All Snapshots ===")
	fmt.Println()
	for _, snap := range store.ListSnapshots() {
		fmt.Printf("  %s: %s (%d files)\n", snap.ID, snap.Message, len(snap.Files))
	}
	fmt.Println()

	// Get changes since snapshot
	fmt.Println("=== Changes Since First Snapshot ===")
	fmt.Println()
	changesSince := store.GetChangesSince(snapshot1.ID)
	fmt.Printf("Changes since %s: %d\n", snapshot1.ID, len(changesSince))
	for _, change := range changesSince {
		fmt.Printf("  [%s] %s at %v\n",
			change.ChangeType,
			filepath.Base(change.Path),
			change.Timestamp.Format("15:04:05"))
	}
	fmt.Println()

	// Demonstrate rollback concept (file content not captured in this demo)
	fmt.Println("=== Rollback Concept ===")
	fmt.Println()
	fmt.Println("To enable rollback, use CaptureFileWithContent:")
	fmt.Println("  state, _ := store.CaptureFileWithContent(path)")
	fmt.Println("  // state.Content now contains the file bytes")
	fmt.Println()
	fmt.Println("Then rollback with:")
	fmt.Println("  store.Rollback(snapshotID)")
	fmt.Println("  // or")
	fmt.Println("  store.RollbackChanges(count)")
	fmt.Println()

	// Retrieve specific snapshot
	fmt.Println("=== Retrieving Snapshot ===")
	fmt.Println()
	retrieved := store.GetSnapshot(snapshot1.ID)
	if retrieved != nil {
		fmt.Printf("Retrieved %s:\n", retrieved.ID)
		for path := range retrieved.Files {
			fmt.Printf("  - %s\n", filepath.Base(path))
		}
	}
	fmt.Println()

	// Clear and verify
	fmt.Println("=== Cleanup ===")
	fmt.Println()
	store.Clear()
	fmt.Printf("After Clear():\n")
	fmt.Printf("  Snapshots: %d\n", len(store.ListSnapshots()))
	fmt.Printf("  Changes: %d\n", len(store.GetChanges()))
}
