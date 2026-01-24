package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotcommander/agent/internal/pathutil"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidatePathTraversalAttacks tests path traversal security.
func TestValidatePathTraversalAttacks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	tests := []struct {
		name        string
		path        string
		wantErr     error
		description string
	}{
		{
			name:        "basic_dotdot_slash",
			path:        filepath.Join(tmpDir, "../../../etc/passwd"),
			wantErr:     ErrPathTraversal,
			description: "Unix path traversal with ../",
		},
		{
			name:        "basic_dotdot_backslash",
			path:        filepath.Join(tmpDir, "..\\..\\..\\windows\\system32"),
			wantErr:     ErrPathTraversal,
			description: "Windows path traversal with ..\\",
		},
		{
			name:        "url_encoded_slash",
			path:        filepath.Join(tmpDir, "..%2f..%2fetc%2fpasswd"),
			wantErr:     ErrPathTraversal,
			description: "URL-encoded forward slash traversal",
		},
		{
			name:        "url_encoded_backslash",
			path:        filepath.Join(tmpDir, "..%5c..%5cwindows"),
			wantErr:     ErrPathTraversal,
			description: "URL-encoded backslash traversal",
		},
		{
			name:        "double_encoded_dotdot",
			path:        filepath.Join(tmpDir, "%2e%2e/etc/passwd"),
			wantErr:     ErrPathTraversal,
			description: "Double-encoded .. pattern",
		},
		{
			name:        "mixed_encoding",
			path:        filepath.Join(tmpDir, "..%2f%2e%2e/sensitive"),
			wantErr:     ErrPathTraversal,
			description: "Mixed encoding attack",
		},
		{
			name:        "valid_relative_path",
			path:        filepath.Join(tmpDir, "valid/file.txt"),
			wantErr:     nil,
			description: "Valid path within base should succeed",
		},
		{
			name:        "valid_direct_path",
			path:        filepath.Join(tmpDir, "file.txt"),
			wantErr:     nil,
			description: "Direct file in base should succeed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.validatePath(tt.path)
			if tt.wantErr != nil {
				require.Error(t, err, tt.description)
				assert.ErrorIs(t, err, tt.wantErr, "Expected %v, got %v", tt.wantErr, err)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestValidatePathAbsolutePathEnforcement tests absolute path handling.
func TestValidatePathAbsolutePathEnforcement(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		enforceBase bool
		path        string
		wantErr     error
	}{
		{
			name:        "enforce_blocks_outside",
			enforceBase: true,
			path:        "/etc/passwd",
			wantErr:     ErrPathOutsideBase,
		},
		{
			name:        "no_enforce_allows_outside",
			enforceBase: false,
			path:        "/tmp/external.txt",
			wantErr:     nil,
		},
		{
			name:        "enforce_allows_inside",
			enforceBase: true,
			path:        filepath.Join(tmpDir, "file.txt"),
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewFileSystemStore(tmpDir, WithEnforceBasePath(tt.enforceBase))
			_, err := store.validatePath(tt.path)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestContainsTraversalEdgeCases tests edge cases in traversal detection.
func TestContainsTraversalEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"empty", "", false},
		{"single_dot", ".", false},
		{"double_dot_alone", "..", false}, // Not a traversal without separator
		{"dotdot_slash", "../", true},
		{"dotdot_backslash", "..\\", true},
		{"valid_filename_with_dots", "file..txt", false},
		{"valid_dir_with_dots", "some..dir/file", false},
		{"lowercase_encoding", "..%2f", true},
		{"uppercase_encoding", "..%2F", true},
		{"backslash_encoding", "..%5c", true},
		{"double_encoded_dot", "%2e%2e/", true},
		{"triple_dot", ".../", false}, // Not a valid traversal
		{"space_before_dotdot", " ../", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathutil.ContainsTraversal(tt.path)
			assert.Equal(t, tt.expected, result, "Path: %s", tt.path)
		})
	}
}

// TestCaptureFileBasic tests basic file capture functionality.
func TestCaptureFileBasic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	require.NoError(t, os.WriteFile(testFile, content, 0644))

	state, err := store.CaptureFile(testFile)
	require.NoError(t, err)
	assert.True(t, state.Exists)
	assert.Equal(t, int64(len(content)), state.Size)
	assert.NotEmpty(t, state.Hash)
	assert.Equal(t, os.FileMode(0644), state.Mode)
}

// TestCaptureFileNonExistent tests capturing non-existent files.
func TestCaptureFileNonExistent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	state, err := store.CaptureFile(filepath.Join(tmpDir, "nonexistent.txt"))
	require.NoError(t, err)
	assert.False(t, state.Exists)
}

// TestCaptureFileWithContent tests capturing file with content.
func TestCaptureFileWithContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	require.NoError(t, os.WriteFile(testFile, content, 0644))

	state, err := store.CaptureFileWithContent(testFile)
	require.NoError(t, err)
	assert.True(t, state.Exists)
	assert.Equal(t, content, state.Content)
}

// TestTrackAndDetectChanges tests file tracking and change detection.
func TestTrackAndDetectChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "track.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("v1"), 0644))

	// Track initial state
	require.NoError(t, store.Track(testFile))

	// No changes yet
	changes, err := store.DetectChanges()
	require.NoError(t, err)
	assert.Empty(t, changes)

	// Modify file
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp
	require.NoError(t, os.WriteFile(testFile, []byte("v2"), 0644))

	// Detect change
	changes, err = store.DetectChanges()
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "modify", changes[0].ChangeType)
	assert.Equal(t, testFile, changes[0].Path)
}

// TestTrackFileOutsideBase tests tracking files outside base with enforcement.
func TestTrackFileOutsideBase(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir, WithEnforceBasePath(true))

	// Try to track file outside base
	err := store.Track("/etc/passwd")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathOutsideBase)
}

// TestCreateAndGetSnapshot tests snapshot creation and retrieval.
func TestCreateAndGetSnapshot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("content2"), 0644))

	// Track files
	require.NoError(t, store.Track(file1))
	require.NoError(t, store.Track(file2))

	// Create snapshot
	snap := store.CreateSnapshot("test snapshot")
	assert.NotEmpty(t, snap.ID)
	assert.Equal(t, "test snapshot", snap.Message)
	assert.Len(t, snap.Files, 2)
	assert.NotNil(t, snap.Files[file1])
	assert.NotNil(t, snap.Files[file2])

	// Retrieve snapshot
	retrieved := store.GetSnapshot(snap.ID)
	assert.Equal(t, snap.ID, retrieved.ID)
	assert.Equal(t, snap.Message, retrieved.Message)

	// Non-existent snapshot
	assert.Nil(t, store.GetSnapshot("nonexistent"))
}

// TestListSnapshots tests listing all snapshots.
func TestListSnapshots(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	// Create multiple snapshots
	snap1 := store.CreateSnapshot("snap1")
	snap2 := store.CreateSnapshot("snap2")
	snap3 := store.CreateSnapshot("snap3")

	snapshots := store.ListSnapshots()
	assert.Len(t, snapshots, 3)
	assert.Equal(t, snap1.ID, snapshots[0].ID)
	assert.Equal(t, snap2.ID, snapshots[1].ID)
	assert.Equal(t, snap3.ID, snapshots[2].ID)
}

// TestRollbackChanges tests rollback functionality.
func TestRollbackChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "rollback.txt")
	original := []byte("original")
	modified := []byte("modified")

	// Create and capture original
	require.NoError(t, os.WriteFile(testFile, original, 0644))
	before, err := store.CaptureFileWithContent(testFile)
	require.NoError(t, err)

	// Record change to modified
	require.NoError(t, os.WriteFile(testFile, modified, 0644))
	after, err := store.CaptureFileWithContent(testFile)
	require.NoError(t, err)

	change := &FileChange{
		Path:       testFile,
		ChangeType: "modify",
		Before:     before,
		After:      after,
		Timestamp:  time.Now(),
	}

	store.mu.Lock()
	store.changes = append(store.changes, change)
	store.current[testFile] = after
	store.mu.Unlock()

	// Rollback
	err = store.RollbackChanges(1)
	require.NoError(t, err)

	// Verify content restored
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, original, content)
}

// TestRollbackChangesMultiple tests rolling back multiple changes.
func TestRollbackChangesMultiple(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	// Create files with content
	require.NoError(t, os.WriteFile(file1, []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("v1"), 0644))

	// Capture before states
	before1, err := store.CaptureFileWithContent(file1)
	require.NoError(t, err)
	before2, err := store.CaptureFileWithContent(file2)
	require.NoError(t, err)

	// Modify both files
	require.NoError(t, os.WriteFile(file1, []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("v2"), 0644))

	after1, err := store.CaptureFileWithContent(file1)
	require.NoError(t, err)
	after2, err := store.CaptureFileWithContent(file2)
	require.NoError(t, err)

	// Record changes
	store.mu.Lock()
	store.changes = append(store.changes, &FileChange{
		Path:       file1,
		ChangeType: "modify",
		Before:     before1,
		After:      after1,
		Timestamp:  time.Now(),
	})
	store.changes = append(store.changes, &FileChange{
		Path:       file2,
		ChangeType: "modify",
		Before:     before2,
		After:      after2,
		Timestamp:  time.Now(),
	})
	store.mu.Unlock()

	// Rollback both
	err = store.RollbackChanges(2)
	require.NoError(t, err)

	// Verify both restored
	content1, _ := os.ReadFile(file1)
	content2, _ := os.ReadFile(file2)
	assert.Equal(t, []byte("v1"), content1)
	assert.Equal(t, []byte("v1"), content2)
}

// TestRollbackToSnapshot tests rolling back to a specific snapshot.
func TestRollbackToSnapshot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "snapshot.txt")
	original := []byte("snapshot content")

	// Create file and capture with content
	require.NoError(t, os.WriteFile(testFile, original, 0644))
	state, err := store.CaptureFileWithContent(testFile)
	require.NoError(t, err)

	// Manually create snapshot with content
	store.mu.Lock()
	store.current[testFile] = state
	store.mu.Unlock()

	snap := store.CreateSnapshot("before modification")
	// Add content to snapshot
	snap.Files[testFile].Content = original

	// Modify file
	modified := []byte("modified content")
	require.NoError(t, os.WriteFile(testFile, modified, 0644))

	// Rollback to snapshot
	err = store.Rollback(snap.ID)
	require.NoError(t, err)

	// Verify restored
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, original, content)
}

// TestRollbackNonExistentSnapshot tests error on invalid snapshot ID.
func TestRollbackNonExistentSnapshot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	err := store.Rollback("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot not found")
}

// TestRollbackPathTraversalPrevention tests security in rollback operations.
func TestRollbackPathTraversalPrevention(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir, WithEnforceBasePath(true))

	// Create malicious snapshot with path traversal
	maliciousPath := "../../../etc/passwd"
	snap := &Snapshot{
		ID:        "malicious",
		Timestamp: time.Now(),
		Files: map[string]*FileState{
			maliciousPath: {
				Path:    maliciousPath,
				Exists:  true,
				Content: []byte("malicious"),
				Mode:    0644,
			},
		},
	}

	store.mu.Lock()
	store.snapshots = append(store.snapshots, snap)
	store.mu.Unlock()

	// Attempt rollback should fail
	err := store.Rollback("malicious")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathTraversal)
}

// TestRecordChange tests manual change recording.
func TestRecordChange(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "record.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	err := store.RecordChange(testFile, "create")
	require.NoError(t, err)

	changes := store.GetChanges()
	require.Len(t, changes, 1)
	assert.Equal(t, "create", changes[0].ChangeType)
	assert.Equal(t, testFile, changes[0].Path)
}

// TestGetChangesSince tests filtering changes by timestamp.
func TestGetChangesSince(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	snap := store.CreateSnapshot("baseline")

	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	testFile := filepath.Join(tmpDir, "change.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
	require.NoError(t, store.RecordChange(testFile, "create"))

	changes := store.GetChangesSince(snap.ID)
	assert.Len(t, changes, 1)

	// Non-existent snapshot
	changes = store.GetChangesSince("nonexistent")
	assert.Nil(t, changes)
}

// TestClear tests clearing all state.
func TestClear(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "clear.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
	require.NoError(t, store.Track(testFile))
	store.CreateSnapshot("test")

	store.Clear()

	assert.Empty(t, store.ListSnapshots())
	assert.Empty(t, store.GetChanges())
	assert.Empty(t, store.current)
}

// TestHashFile tests file hashing.
func TestHashFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hash.txt")
	content := []byte("test content for hashing")
	require.NoError(t, os.WriteFile(testFile, content, 0644))

	hash1, err := hashFile(testFile)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)
	assert.Len(t, hash1, 64) // SHA256 hex is 64 chars

	// Same content should produce same hash
	hash2, err := hashFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Different content should produce different hash
	require.NoError(t, os.WriteFile(testFile, []byte("different"), 0644))
	hash3, err := hashFile(testFile)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

// TestHashFileNonExistent tests hashing non-existent file.
func TestHashFileNonExistent(t *testing.T) {
	t.Parallel()

	_, err := hashFile("/nonexistent/file.txt")
	require.Error(t, err)
}

// TestDetectChange tests change detection logic.
func TestDetectChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		before         *FileState
		after          *FileState
		wantChangeType string
		wantChange     bool
	}{
		{
			name:           "file_created",
			before:         &FileState{Path: "/test", Exists: false},
			after:          &FileState{Path: "/test", Exists: true, Hash: "abc"},
			wantChangeType: "create",
			wantChange:     true,
		},
		{
			name:           "file_deleted",
			before:         &FileState{Path: "/test", Exists: true, Hash: "abc"},
			after:          &FileState{Path: "/test", Exists: false},
			wantChangeType: "delete",
			wantChange:     true,
		},
		{
			name:           "file_modified",
			before:         &FileState{Path: "/test", Exists: true, Hash: "abc"},
			after:          &FileState{Path: "/test", Exists: true, Hash: "xyz"},
			wantChangeType: "modify",
			wantChange:     true,
		},
		{
			name:       "file_unchanged",
			before:     &FileState{Path: "/test", Exists: true, Hash: "abc"},
			after:      &FileState{Path: "/test", Exists: true, Hash: "abc"},
			wantChange: false,
		},
		{
			name:       "both_nonexistent",
			before:     &FileState{Path: "/test", Exists: false},
			after:      &FileState{Path: "/test", Exists: false},
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := detectChange(tt.before, tt.after)
			if tt.wantChange {
				require.NotNil(t, change)
				assert.Equal(t, tt.wantChangeType, change.ChangeType)
			} else {
				assert.Nil(t, change)
			}
		})
	}
}

// TestSanitizePathError tests error sanitization.
func TestSanitizePathError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		operation string
		wantMsg   string
	}{
		{
			name:      "not_exist",
			err:       os.ErrNotExist,
			operation: "read",
			wantMsg:   "read: file not found",
		},
		{
			name:      "permission",
			err:       os.ErrPermission,
			operation: "write",
			wantMsg:   "write: permission denied",
		},
		{
			name:      "timeout",
			err:       os.ErrDeadlineExceeded,
			operation: "stat",
			wantMsg:   "stat: operation timed out",
		},
		{
			name:      "generic_error",
			err:       errors.New("some internal error"),
			operation: "open",
			wantMsg:   "open: operation failed",
		},
		{
			name:      "nil_error",
			err:       nil,
			operation: "test",
			wantMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathutil.SanitizeError(tt.operation, tt.err)
			if tt.err == nil {
				assert.Nil(t, result)
			} else {
				require.Error(t, result)
				assert.Equal(t, tt.wantMsg, result.Error())
			}
		})
	}
}

// TestFileWatcherBasic tests basic file watching functionality.
func TestFileWatcherBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watcher test in short mode")
	}

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "watched.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("v1"), 0644))

	config := &WatchConfig{
		Paths:    []string{testFile},
		Debounce: 50 * time.Millisecond,
	}

	changeChan := make(chan []*FileChange, 1)
	watcher := NewFileWatcher(config, store)
	watcher.OnChange(func(changes []*FileChange) {
		changeChan <- changes
	})

	require.NoError(t, watcher.Start())
	defer func() { _ = watcher.Stop() }()

	// Modify file
	time.Sleep(100 * time.Millisecond) // Let watcher initialize
	require.NoError(t, os.WriteFile(testFile, []byte("v2"), 0644))

	// Wait for change detection
	select {
	case changes := <-changeChan:
		require.NotEmpty(t, changes)
		assert.Equal(t, "modify", changes[0].ChangeType)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file change")
	}
}

// TestFileWatcherCreate tests watching file creation.
func TestFileWatcherCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watcher test in short mode")
	}

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	config := &WatchConfig{
		Paths:     []string{tmpDir},
		Recursive: true,
		Debounce:  50 * time.Millisecond,
	}

	changeChan := make(chan []*FileChange, 1)
	watcher := NewFileWatcher(config, store)
	watcher.OnChange(func(changes []*FileChange) {
		changeChan <- changes
	})

	require.NoError(t, watcher.Start())
	defer func() { _ = watcher.Stop() }()

	// Create new file
	time.Sleep(100 * time.Millisecond)
	newFile := filepath.Join(tmpDir, "new.txt")
	require.NoError(t, os.WriteFile(newFile, []byte("content"), 0644))

	// Wait for change
	select {
	case changes := <-changeChan:
		require.NotEmpty(t, changes)
		assert.Equal(t, "create", changes[0].ChangeType)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file creation")
	}
}

// TestFileWatcherDelete tests watching file deletion.
func TestFileWatcherDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watcher test in short mode")
	}

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "delete.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	config := &WatchConfig{
		Paths:    []string{testFile},
		Debounce: 50 * time.Millisecond,
	}

	changeChan := make(chan []*FileChange, 1)
	watcher := NewFileWatcher(config, store)
	watcher.OnChange(func(changes []*FileChange) {
		changeChan <- changes
	})

	require.NoError(t, watcher.Start())
	defer func() { _ = watcher.Stop() }()

	// Delete file
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.Remove(testFile))

	// Wait for change
	select {
	case changes := <-changeChan:
		require.NotEmpty(t, changes)
		assert.Equal(t, "delete", changes[0].ChangeType)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for file deletion")
	}
}

// TestFileWatcherPatterns tests pattern filtering.
func TestFileWatcherPatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watcher test in short mode")
	}

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	config := &WatchConfig{
		Paths:     []string{tmpDir},
		Patterns:  []string{"*.txt"},
		Recursive: true,
		Debounce:  50 * time.Millisecond,
	}

	changeChan := make(chan []*FileChange, 1)
	watcher := NewFileWatcher(config, store)
	watcher.OnChange(func(changes []*FileChange) {
		changeChan <- changes
	})

	require.NoError(t, watcher.Start())
	defer func() { _ = watcher.Stop() }()

	time.Sleep(100 * time.Millisecond)

	// Create .txt file - should be detected
	txtFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("content"), 0644))

	select {
	case changes := <-changeChan:
		require.NotEmpty(t, changes)
		assert.Contains(t, changes[0].Path, "test.txt")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for .txt file")
	}

	// Create .log file - should be ignored
	logFile := filepath.Join(tmpDir, "test.log")
	require.NoError(t, os.WriteFile(logFile, []byte("content"), 0644))

	select {
	case <-changeChan:
		t.Fatal("Should not detect .log file")
	case <-time.After(500 * time.Millisecond):
		// Expected - no change detected
	}
}

// TestFileWatcherRecursive tests recursive directory watching.
func TestFileWatcherRecursive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watcher test in short mode")
	}

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))

	config := &WatchConfig{
		Paths:     []string{tmpDir},
		Recursive: true,
		Debounce:  50 * time.Millisecond,
	}

	changeChan := make(chan []*FileChange, 1)
	watcher := NewFileWatcher(config, store)
	watcher.OnChange(func(changes []*FileChange) {
		changeChan <- changes
	})

	require.NoError(t, watcher.Start())
	defer func() { _ = watcher.Stop() }()

	time.Sleep(100 * time.Millisecond)

	// Create file in subdirectory
	testFile := filepath.Join(subDir, "nested.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	// Should detect change in subdirectory
	select {
	case changes := <-changeChan:
		require.NotEmpty(t, changes)
		assert.Contains(t, changes[0].Path, "nested.txt")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for nested file")
	}
}

// TestFileWatcherStopIdempotent tests that Stop can be called multiple times.
func TestFileWatcherStopIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	config := &WatchConfig{
		Paths:    []string{testFile},
		Debounce: 50 * time.Millisecond,
	}

	watcher := NewFileWatcher(config, store)
	require.NoError(t, watcher.Start())

	// Stop multiple times should not panic
	_ = watcher.Stop()
	_ = watcher.Stop()
	_ = watcher.Stop()
}

// TestMatchesPatterns tests pattern matching logic.
func TestMatchesPatterns(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	tests := []struct {
		name     string
		patterns []string
		path     string
		expected bool
	}{
		{
			name:     "no_patterns_matches_all",
			patterns: []string{},
			path:     "/any/file.txt",
			expected: true,
		},
		{
			name:     "matches_extension",
			patterns: []string{"*.txt"},
			path:     "/dir/file.txt",
			expected: true,
		},
		{
			name:     "no_match_extension",
			patterns: []string{"*.txt"},
			path:     "/dir/file.log",
			expected: false,
		},
		{
			name:     "matches_one_of_many",
			patterns: []string{"*.txt", "*.log", "*.md"},
			path:     "/dir/file.log",
			expected: true,
		},
		{
			name:     "matches_pattern",
			patterns: []string{"test_*"},
			path:     "/dir/test_file.go",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WatchConfig{Patterns: tt.patterns}
			watcher := NewFileWatcher(config, store)
			result := watcher.matchesPatterns(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessEvent tests event processing logic.
func TestProcessEvent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "process.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("v1"), 0644))
	require.NoError(t, store.Track(testFile))

	config := &WatchConfig{Paths: []string{testFile}}
	watcher := NewFileWatcher(config, store)

	// Test write event
	require.NoError(t, os.WriteFile(testFile, []byte("v2"), 0644))
	event := fsnotify.Event{Name: testFile, Op: fsnotify.Write}
	change := watcher.processEvent(context.Background(), testFile, event)
	require.NotNil(t, change)
	assert.Equal(t, "modify", change.ChangeType)

	// Test remove event
	event = fsnotify.Event{Name: testFile, Op: fsnotify.Remove}
	change = watcher.processEvent(context.Background(), testFile, event)
	require.NotNil(t, change)
	assert.Equal(t, "delete", change.ChangeType)
}

// TestTrackDir tests directory tracking.
func TestTrackDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	// Create test structure
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file3.log"), []byte("3"), 0644))

	// Track only .txt files
	err := store.TrackDir(tmpDir, "*.txt")
	require.NoError(t, err)

	store.mu.RLock()
	count := len(store.current)
	store.mu.RUnlock()

	assert.Equal(t, 2, count, "Should track 2 .txt files")
}

// TestWithEnforceBasePath tests the functional option.
func TestWithEnforceBasePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Test default (enforced)
	store1 := NewFileSystemStore(tmpDir)
	assert.True(t, store1.enforceBase)

	// Test explicitly disabled
	store2 := NewFileSystemStore(tmpDir, WithEnforceBasePath(false))
	assert.False(t, store2.enforceBase)

	// Test explicitly enabled
	store3 := NewFileSystemStore(tmpDir, WithEnforceBasePath(true))
	assert.True(t, store3.enforceBase)
}

// TestConcurrentAccess tests thread safety.
func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := NewFileSystemStore(tmpDir)

	testFile := filepath.Join(tmpDir, "concurrent.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	// Concurrent Track calls
	done := make(chan bool)
	for range 10 {
		go func() {
			_ = store.Track(testFile)
			done <- true
		}()
	}

	for range 10 {
		<-done
	}

	// Concurrent snapshot creation
	for range 10 {
		go func() {
			store.CreateSnapshot("snapshot")
			done <- true
		}()
	}

	for range 10 {
		<-done
	}

	snapshots := store.ListSnapshots()
	assert.Len(t, snapshots, 10)
}
