// Package state provides state tracking and persistence.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Security-related errors.
var (
	ErrPathOutsideBase = errors.New("path outside allowed base directory")
	ErrPathTraversal   = errors.New("path traversal detected")
)

// sanitizePathError returns a user-safe error message that doesn't expose
// sensitive path information. The original error is preserved for logging.
func sanitizePathError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Check for known error types and return sanitized messages
	if os.IsNotExist(err) {
		return fmt.Errorf("%s: file not found", operation)
	}
	if os.IsPermission(err) {
		return fmt.Errorf("%s: permission denied", operation)
	}
	if os.IsTimeout(err) {
		return fmt.Errorf("%s: operation timed out", operation)
	}

	// For path-related errors, return generic message
	return fmt.Errorf("%s: operation failed", operation)
}

// FileState represents the state of a file at a point in time.
type FileState struct {
	Path    string      `json:"path"`
	Hash    string      `json:"hash"`
	Size    int64       `json:"size"`
	ModTime time.Time   `json:"mod_time"`
	Mode    os.FileMode `json:"mode"`
	Exists  bool        `json:"exists"`
	Content []byte      `json:"-"` // Not serialized by default
}

// FileChange represents a change to a file.
type FileChange struct {
	Path       string     `json:"path"`
	ChangeType string     `json:"change_type"` // "create", "modify", "delete"
	Before     *FileState `json:"before,omitempty"`
	After      *FileState `json:"after,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
}

// Snapshot represents the state of multiple files at a point in time.
type Snapshot struct {
	ID        string                `json:"id"`
	Timestamp time.Time             `json:"timestamp"`
	Files     map[string]*FileState `json:"files"`
	Message   string                `json:"message,omitempty"`
}

// FileSystemStore tracks file system state and changes.
type FileSystemStore struct {
	basePath       string
	basePathAbs    string // Resolved absolute path of basePath
	enforceBase    bool   // If true, all operations must be within basePath
	snapshots      []*Snapshot
	current        map[string]*FileState
	changes        []*FileChange
	mu             sync.RWMutex
}

// FileSystemStoreOption configures FileSystemStore.
type FileSystemStoreOption func(*FileSystemStore)

// WithEnforceBasePath enables strict base path enforcement for all operations.
func WithEnforceBasePath(enforce bool) FileSystemStoreOption {
	return func(s *FileSystemStore) {
		s.enforceBase = enforce
	}
}

// NewFileSystemStore creates a new file system store with optional path enforcement.
func NewFileSystemStore(basePath string, opts ...FileSystemStoreOption) *FileSystemStore {
	s := &FileSystemStore{
		basePath:    basePath,
		enforceBase: true, // Default to enforcing base path for security
		snapshots:   make([]*Snapshot, 0),
		current:     make(map[string]*FileState),
		changes:     make([]*FileChange, 0),
	}

	// Resolve absolute path of base
	if basePath != "" {
		if abs, err := filepath.Abs(basePath); err == nil {
			s.basePathAbs = filepath.Clean(abs)
		}
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// validatePath ensures a path is within the allowed base directory.
func (s *FileSystemStore) validatePath(path string) (string, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	cleanPath := filepath.Clean(absPath)

	// Check for traversal patterns in original input
	if containsTraversal(path) {
		return "", ErrPathTraversal
	}

	// If base path enforcement is enabled, verify path is within base
	if s.enforceBase && s.basePathAbs != "" {
		rel, err := filepath.Rel(s.basePathAbs, cleanPath)
		if err != nil {
			return "", fmt.Errorf("resolve relative path: %w", err)
		}

		// If relative path starts with .., it's outside base directory
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return "", fmt.Errorf("%w: %s is outside %s", ErrPathOutsideBase, cleanPath, s.basePathAbs)
		}
	}

	return cleanPath, nil
}

// containsTraversal checks if a path contains obvious traversal patterns.
func containsTraversal(path string) bool {
	normalized := filepath.ToSlash(path)

	patterns := []string{
		"../",
		"..\\",
		"..%2f",
		"..%5c",
		"%2e%2e/",
		"%2e%2e\\",
	}

	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(normalized), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// CaptureFile captures the current state of a file.
// This is the public API that handles path validation.
func (s *FileSystemStore) CaptureFile(path string) (*FileState, error) {
	// Validate path before any operations
	validPath, err := s.validatePath(path)
	if err != nil {
		return nil, err
	}

	return s.captureFileInternal(validPath)
}

// captureFileInternal captures file state for an already-validated path.
// This internal method can be called while holding locks since it doesn't
// acquire any locks itself. The path must already be validated and cleaned.
func (s *FileSystemStore) captureFileInternal(validPath string) (*FileState, error) {
	info, err := os.Stat(validPath)
	if os.IsNotExist(err) {
		return &FileState{
			Path:   validPath,
			Exists: false,
		}, nil
	}
	if err != nil {
		return nil, sanitizePathError("stat", err)
	}

	state := &FileState{
		Path:    validPath,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
		Exists:  true,
	}

	// Calculate hash
	hash, err := hashFile(validPath)
	if err != nil {
		return nil, sanitizePathError("hash", err)
	}
	state.Hash = hash

	return state, nil
}

// CaptureFileWithContent captures state including file content.
func (s *FileSystemStore) CaptureFileWithContent(path string) (*FileState, error) {
	state, err := s.CaptureFile(path)
	if err != nil {
		return nil, err
	}

	if state.Exists {
		content, err := os.ReadFile(state.Path)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		state.Content = content
	}

	return state, nil
}

// Track starts tracking a file for changes.
func (s *FileSystemStore) Track(path string) error {
	state, err := s.CaptureFile(path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.current[state.Path] = state
	return nil
}

// TrackDir tracks all files in a directory.
func (s *FileSystemStore) TrackDir(dir string, patterns ...string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}

	return filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}

		// Check patterns if specified
		if len(patterns) > 0 {
			matched := false
			for _, pattern := range patterns {
				if m, _ := filepath.Match(pattern, filepath.Base(path)); m {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		return s.Track(path)
	})
}

// DetectChanges checks for changes since tracking started.
func (s *FileSystemStore) DetectChanges() ([]*FileChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var changes []*FileChange

	for path, before := range s.current {
		// Use captureFileInternal since paths in s.current are already validated
		// when they were added via Track(). This avoids redundant validation
		// and prevents any potential locking issues.
		after, err := s.captureFileInternal(path)
		if err != nil {
			continue
		}

		change := detectChange(before, after)
		if change != nil {
			changes = append(changes, change)
			s.changes = append(s.changes, change)
			s.current[path] = after
		}
	}

	return changes, nil
}

// RecordChange manually records a file change.
func (s *FileSystemStore) RecordChange(path string, changeType string) error {
	before := s.current[path]
	if before == nil {
		before = &FileState{Path: path, Exists: false}
	}

	after, err := s.CaptureFileWithContent(path)
	if err != nil && changeType != "delete" {
		return err
	}
	if changeType == "delete" {
		after = &FileState{Path: path, Exists: false}
	}

	change := &FileChange{
		Path:       path,
		ChangeType: changeType,
		Before:     before,
		After:      after,
		Timestamp:  time.Now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.changes = append(s.changes, change)
	s.current[path] = after

	return nil
}

// CreateSnapshot creates a snapshot of current tracked files.
func (s *FileSystemStore) CreateSnapshot(message string) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := &Snapshot{
		ID:        fmt.Sprintf("snap-%d", len(s.snapshots)+1),
		Timestamp: time.Now(),
		Files:     make(map[string]*FileState),
		Message:   message,
	}

	for path, state := range s.current {
		// Create a copy
		copy := &FileState{
			Path:    state.Path,
			Hash:    state.Hash,
			Size:    state.Size,
			ModTime: state.ModTime,
			Mode:    state.Mode,
			Exists:  state.Exists,
		}
		snapshot.Files[path] = copy
	}

	s.snapshots = append(s.snapshots, snapshot)
	return snapshot
}

// GetSnapshot retrieves a snapshot by ID.
func (s *FileSystemStore) GetSnapshot(id string) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, snap := range s.snapshots {
		if snap.ID == id {
			return snap
		}
	}
	return nil
}

// ListSnapshots returns all snapshots.
func (s *FileSystemStore) ListSnapshots() []*Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots
}

// Rollback restores files to a previous snapshot.
func (s *FileSystemStore) Rollback(snapshotID string) error {
	snapshot := s.GetSnapshot(snapshotID)
	if snapshot == nil {
		return fmt.Errorf("snapshot not found: %s", snapshotID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for path, state := range snapshot.Files {
		// Validate path before any write operations
		validPath, err := s.validatePath(path)
		if err != nil {
			return fmt.Errorf("invalid path %s: %w", path, err)
		}

		if state.Exists && state.Content != nil {
			if err := os.WriteFile(validPath, state.Content, state.Mode); err != nil {
				return fmt.Errorf("restore %s: %w", validPath, err)
			}
		} else if !state.Exists {
			// File should not exist, delete it
			if err := os.Remove(validPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete %s: %w", validPath, err)
			}
		}
	}

	return nil
}

// RollbackChanges undoes recent changes.
func (s *FileSystemStore) RollbackChanges(count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count > len(s.changes) {
		count = len(s.changes)
	}

	// Rollback in reverse order
	for i := len(s.changes) - 1; i >= len(s.changes)-count; i-- {
		change := s.changes[i]
		before := change.Before

		// Validate path before any write operations
		validPath, err := s.validatePath(before.Path)
		if err != nil {
			return fmt.Errorf("invalid path %s: %w", before.Path, err)
		}

		if before.Exists && before.Content != nil {
			if err := os.WriteFile(validPath, before.Content, before.Mode); err != nil {
				return fmt.Errorf("restore %s: %w", validPath, err)
			}
		} else if !before.Exists {
			// Also validate the change path for deletion
			validChangePath, err := s.validatePath(change.Path)
			if err != nil {
				return fmt.Errorf("invalid path %s: %w", change.Path, err)
			}
			if err := os.Remove(validChangePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete %s: %w", validChangePath, err)
			}
		}
	}

	// Remove rolled back changes
	s.changes = s.changes[:len(s.changes)-count]

	return nil
}

// GetChanges returns recorded changes.
func (s *FileSystemStore) GetChanges() []*FileChange {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.changes
}

// GetChangesSince returns changes since a snapshot.
func (s *FileSystemStore) GetChangesSince(snapshotID string) []*FileChange {
	snapshot := s.GetSnapshot(snapshotID)
	if snapshot == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var changes []*FileChange
	for _, change := range s.changes {
		if change.Timestamp.After(snapshot.Timestamp) {
			changes = append(changes, change)
		}
	}
	return changes
}

// Clear resets the store.
func (s *FileSystemStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots = make([]*Snapshot, 0)
	s.current = make(map[string]*FileState)
	s.changes = make([]*FileChange, 0)
}

// hashFile calculates SHA256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// detectChange compares two file states and returns a change if different.
func detectChange(before, after *FileState) *FileChange {
	if before.Exists != after.Exists {
		changeType := "create"
		if before.Exists {
			changeType = "delete"
		}
		return &FileChange{
			Path:       before.Path,
			ChangeType: changeType,
			Before:     before,
			After:      after,
			Timestamp:  time.Now(),
		}
	}

	if !before.Exists && !after.Exists {
		return nil
	}

	if before.Hash != after.Hash {
		return &FileChange{
			Path:       before.Path,
			ChangeType: "modify",
			Before:     before,
			After:      after,
			Timestamp:  time.Now(),
		}
	}

	return nil
}

// WatchConfig configures file watching.
type WatchConfig struct {
	Paths     []string
	Patterns  []string
	Recursive bool
	Debounce  time.Duration
}

// FileWatcher watches files for changes.
type FileWatcher struct {
	config   *WatchConfig
	store    *FileSystemStore
	onChange func([]*FileChange)
	stop     chan struct{}
	running  bool
	mu       sync.Mutex
}

// NewFileWatcher creates a file watcher.
func NewFileWatcher(config *WatchConfig, store *FileSystemStore) *FileWatcher {
	if config.Debounce == 0 {
		config.Debounce = 100 * time.Millisecond
	}
	return &FileWatcher{
		config: config,
		store:  store,
		stop:   make(chan struct{}),
	}
}

// OnChange sets the callback for file changes.
func (w *FileWatcher) OnChange(fn func([]*FileChange)) *FileWatcher {
	w.onChange = fn
	return w
}

// Start begins watching for changes.
func (w *FileWatcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	// Track initial state
	for _, path := range w.config.Paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() && w.config.Recursive {
			if err := w.store.TrackDir(path, w.config.Patterns...); err != nil {
				return err
			}
		} else {
			if err := w.store.Track(path); err != nil {
				return err
			}
		}
	}

	// Poll for changes (simplified watcher)
	go func() {
		ticker := time.NewTicker(w.config.Debounce)
		defer ticker.Stop()

		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				changes, _ := w.store.DetectChanges()
				if len(changes) > 0 && w.onChange != nil {
					w.onChange(changes)
				}
			}
		}
	}()

	return nil
}

// Stop stops watching.
func (w *FileWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		close(w.stop)
		w.running = false
	}
}
