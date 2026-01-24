# State Management

File system state tracking with snapshots, change detection, and rollback capabilities.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/dotcommander/agent/state"
)

func main() {
    // Create store for tracking
    store := state.NewFileSystemStore("/project")

    // Track files
    store.Track("main.go")
    store.Track("config.yaml")

    // Create initial snapshot
    snap := store.CreateSnapshot("before changes")

    // ... make changes to files ...

    // Detect what changed
    changes, _ := store.DetectChanges()
    for _, change := range changes {
        fmt.Printf("%s: %s\n", change.ChangeType, change.Path)
    }

    // Rollback if needed
    store.Rollback(snap.ID)
}
```

## Core Types

### FileState

Represents a file at a point in time:

```go
type FileState struct {
    Path    string      // Absolute file path
    Hash    string      // SHA256 hash of content
    Size    int64       // File size in bytes
    ModTime time.Time   // Last modification time
    Mode    os.FileMode // File permissions
    Exists  bool        // Whether file exists
    Content []byte      // Optional: file content
}
```

### FileChange

Represents a modification:

```go
type FileChange struct {
    Path       string     // File that changed
    ChangeType string     // "create", "modify", "delete"
    Before     *FileState // State before change
    After      *FileState // State after change
    Timestamp  time.Time  // When change was detected
}
```

### Snapshot

Captures state of multiple files:

```go
type Snapshot struct {
    ID        string                // Unique identifier
    Timestamp time.Time             // When created
    Files     map[string]*FileState // File states
    Message   string                // Description
}
```

## FileSystemStore

The main interface for state tracking.

### Creating a Store

```go
store := state.NewFileSystemStore("/path/to/project")
```

### Capturing File State

```go
// Capture state (without content)
fileState, err := store.CaptureFile("main.go")

// Capture state with content (for rollback)
fileState, err := store.CaptureFileWithContent("main.go")

fmt.Printf("Hash: %s\n", fileState.Hash)
fmt.Printf("Size: %d bytes\n", fileState.Size)
fmt.Printf("Exists: %v\n", fileState.Exists)
```

### Tracking Files

```go
// Track a single file
err := store.Track("main.go")

// Track all files in a directory
err := store.TrackDir("./src")

// Track with pattern filters
err := store.TrackDir("./src", "*.go", "*.yaml")
```

## Change Detection

### Automatic Detection

```go
// Detect changes in tracked files
changes, err := store.DetectChanges()

for _, change := range changes {
    switch change.ChangeType {
    case "create":
        fmt.Printf("Created: %s\n", change.Path)
    case "modify":
        fmt.Printf("Modified: %s (was %d bytes, now %d)\n",
            change.Path,
            change.Before.Size,
            change.After.Size)
    case "delete":
        fmt.Printf("Deleted: %s\n", change.Path)
    }
}
```

### Manual Recording

```go
// Manually record a change (useful for tracking your own modifications)
err := store.RecordChange("/path/to/file", "modify")
err := store.RecordChange("/path/to/new/file", "create")
err := store.RecordChange("/path/to/old/file", "delete")
```

### Getting Change History

```go
// All recorded changes
changes := store.GetChanges()

// Changes since a snapshot
changes := store.GetChangesSince(snapshotID)

for _, change := range changes {
    fmt.Printf("[%s] %s: %s\n",
        change.Timestamp.Format(time.RFC3339),
        change.ChangeType,
        change.Path)
}
```

## Snapshots

### Creating Snapshots

```go
// Create snapshot with description
snap := store.CreateSnapshot("before refactoring")

fmt.Printf("Snapshot ID: %s\n", snap.ID)
fmt.Printf("Files: %d\n", len(snap.Files))
```

### Retrieving Snapshots

```go
// Get specific snapshot
snap := store.GetSnapshot("snap-1")

// List all snapshots
snapshots := store.ListSnapshots()
for _, snap := range snapshots {
    fmt.Printf("%s: %s (%d files)\n",
        snap.ID,
        snap.Message,
        len(snap.Files))
}
```

## Rollback

### Rollback to Snapshot

Restore files to a previous snapshot state:

```go
// Create snapshot before changes
snap := store.CreateSnapshot("clean state")

// ... make changes ...

// Rollback to snapshot
err := store.Rollback(snap.ID)
if err != nil {
    panic(err)
}
```

Note: Rollback requires content to be captured. Use `CaptureFileWithContent` or ensure files are tracked with content.

### Rollback Recent Changes

Undo the most recent N changes:

```go
// Undo last 3 changes
err := store.RollbackChanges(3)

// Undo all changes
err := store.RollbackChanges(len(store.GetChanges()))
```

## File Watching

Watch for file system changes in real-time:

```go
// Configure watcher
config := &state.WatchConfig{
    Paths:     []string{"./src", "./config"},
    Patterns:  []string{"*.go", "*.yaml"},
    Recursive: true,
    Debounce:  100 * time.Millisecond,
}

// Create watcher
store := state.NewFileSystemStore(".")
watcher := state.NewFileWatcher(config, store)

// Set change handler
watcher.OnChange(func(changes []*state.FileChange) {
    for _, change := range changes {
        fmt.Printf("Detected: %s %s\n", change.ChangeType, change.Path)
    }
})

// Start watching
err := watcher.Start()
if err != nil {
    panic(err)
}

// ... your application runs ...

// Stop when done
watcher.Stop()
```

### Watch Configuration

```go
type WatchConfig struct {
    Paths     []string       // Directories/files to watch
    Patterns  []string       // Glob patterns to filter
    Recursive bool           // Watch subdirectories
    Debounce  time.Duration  // Delay before reporting changes
}
```

## Example: Safe Code Modification

```go
func safelyModifyCode(filePath string, modifier func(string) string) error {
    store := state.NewFileSystemStore(".")

    // Capture original state with content
    original, err := store.CaptureFileWithContent(filePath)
    if err != nil {
        return fmt.Errorf("capture: %w", err)
    }

    // Create snapshot
    snap := store.CreateSnapshot("before modification")

    // Read and modify
    content := string(original.Content)
    newContent := modifier(content)

    // Write changes
    if err := os.WriteFile(filePath, []byte(newContent), original.Mode); err != nil {
        return fmt.Errorf("write: %w", err)
    }

    // Record the change
    store.RecordChange(filePath, "modify")

    // Verify (e.g., run tests)
    if err := runTests(); err != nil {
        // Tests failed, rollback
        fmt.Println("Tests failed, rolling back...")
        if rbErr := store.Rollback(snap.ID); rbErr != nil {
            return fmt.Errorf("rollback failed: %w (original error: %v)", rbErr, err)
        }
        return fmt.Errorf("modification failed tests: %w", err)
    }

    return nil
}
```

## Example: Track Agent Changes

```go
func trackAgentChanges(ctx context.Context, agent *app.AgentLoop) {
    store := state.NewFileSystemStore(".")

    // Track all Go files
    store.TrackDir(".", "*.go")

    // Create baseline
    baseline := store.CreateSnapshot("before agent run")

    // Run agent
    runner := app.NewLoopRunner(agent, nil)
    _, err := runner.Run(ctx)
    if err != nil {
        panic(err)
    }

    // Report changes
    changes := store.GetChangesSince(baseline.ID)

    fmt.Printf("\nAgent made %d changes:\n", len(changes))
    for _, change := range changes {
        fmt.Printf("  %s: %s\n", change.ChangeType, change.Path)
        if change.ChangeType == "modify" {
            fmt.Printf("    Size: %d -> %d bytes\n",
                change.Before.Size,
                change.After.Size)
        }
    }
}
```

## Example: Batch Processing with Checkpoints

```go
func processFilesWithCheckpoints(files []string) error {
    store := state.NewFileSystemStore(".")

    for _, file := range files {
        // Track file
        store.Track(file)
    }

    // Create checkpoint before starting
    checkpoint := store.CreateSnapshot("batch-start")

    for i, file := range files {
        // Create checkpoint every 10 files
        if i > 0 && i%10 == 0 {
            checkpoint = store.CreateSnapshot(fmt.Sprintf("checkpoint-%d", i))
        }

        // Process file
        if err := processFile(file); err != nil {
            fmt.Printf("Error at file %d, rolling back to last checkpoint\n", i)
            store.Rollback(checkpoint.ID)
            return err
        }

        store.RecordChange(file, "modify")
    }

    return nil
}
```

## Clearing State

```go
// Clear all tracked state, snapshots, and changes
store.Clear()
```

## Thread Safety

All `FileSystemStore` operations are thread-safe and use internal locking.

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [AGENT-LOOP.md](AGENT-LOOP.md) - Using state in agent loops
- [VERIFICATION.md](VERIFICATION.md) - Verifying changes
