// Package state provides state tracking and persistence.
package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/agent-framework/internal/pathutil"
	"github.com/fsnotify/fsnotify"
)

// File watcher defaults.
const (
	// DefaultWatchDebounce is the default debounce duration for file watch events.
	// Multiple rapid events are coalesced into a single change notification.
	DefaultWatchDebounce = 100 * time.Millisecond
)

// Security-related errors.
var (
	ErrPathOutsideBase = pathutil.ErrPathOutsideBase
	ErrPathTraversal   = pathutil.ErrPathTraversal
)

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
	maxSnapshots   int    // Maximum snapshots to keep (0 = unlimited)
	maxChanges     int    // Maximum changes to track (0 = unlimited)
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

// WithMaxSnapshots limits the number of snapshots kept (oldest evicted first).
func WithMaxSnapshots(max int) FileSystemStoreOption {
	return func(s *FileSystemStore) {
		s.maxSnapshots = max
	}
}

// WithMaxChanges limits the number of changes tracked (oldest evicted first).
func WithMaxChanges(max int) FileSystemStoreOption {
	return func(s *FileSystemStore) {
		s.maxChanges = max
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
	if pathutil.ContainsTraversal(path) {
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
		return nil, pathutil.SanitizeError("stat", err)
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
		return nil, pathutil.SanitizeError("hash", err)
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
	// Collect paths and before states under read lock (fast)
	s.mu.RLock()
	paths := make([]string, 0, len(s.current))
	beforeStates := make(map[string]*FileState, len(s.current))
	for path, state := range s.current {
		paths = append(paths, path)
		beforeStates[path] = state
	}
	s.mu.RUnlock()

	// Perform I/O without holding lock (slow, but doesn't block other operations)
	var changes []*FileChange
	afterStates := make(map[string]*FileState, len(paths))
	for _, path := range paths {
		after, err := s.captureFileInternal(path)
		if err != nil {
			continue
		}
		afterStates[path] = after

		change := detectChange(beforeStates[path], after)
		if change != nil {
			changes = append(changes, change)
		}
	}

	// Update state atomically under write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, change := range changes {
		s.changes = append(s.changes, change)
		s.current[change.Path] = afterStates[change.Path]
	}

	// Evict oldest changes if limit exceeded
	if s.maxChanges > 0 && len(s.changes) > s.maxChanges {
		s.changes = s.changes[len(s.changes)-s.maxChanges:]
	}

	return changes, nil
}

// RecordChange manually records a file change.
func (s *FileSystemStore) RecordChange(path string, changeType string) error {
	// Get before state under read lock
	s.mu.RLock()
	before := s.current[path]
	s.mu.RUnlock()

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

	// Evict oldest changes if limit exceeded
	if s.maxChanges > 0 && len(s.changes) > s.maxChanges {
		s.changes = s.changes[len(s.changes)-s.maxChanges:]
	}

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

	// Evict oldest snapshots if limit exceeded
	if s.maxSnapshots > 0 && len(s.snapshots) > s.maxSnapshots {
		s.snapshots = s.snapshots[len(s.snapshots)-s.maxSnapshots:]
	}

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

// FileWatcher watches files for changes using fsnotify.
// This provides event-driven file watching that scales to >10k files
// without the CPU overhead of polling.
type FileWatcher struct {
	config   *WatchConfig
	store    *FileSystemStore
	onChange func([]*FileChange)
	onError  func(error)
	watcher  *fsnotify.Watcher
	stop     chan struct{}
	done     chan struct{}
	ctx      context.Context    // Context for cancellation
	cancel   context.CancelFunc // Cancel function for context
	running  bool
	mu       sync.Mutex
}

// NewFileWatcher creates a file watcher.
func NewFileWatcher(config *WatchConfig, store *FileSystemStore) *FileWatcher {
	if config.Debounce == 0 {
		config.Debounce = DefaultWatchDebounce
	}
	return &FileWatcher{
		config: config,
		store:  store,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// OnChange sets the callback for file changes.
// Must be called before Start() or behavior is undefined.
func (w *FileWatcher) OnChange(fn func([]*FileChange)) *FileWatcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = fn
	return w
}

// OnError sets the callback for watcher errors.
// If not set, errors are logged to stderr.
// Must be called before Start() or behavior is undefined.
func (w *FileWatcher) OnError(fn func(error)) *FileWatcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onError = fn
	return w
}

// Start begins watching for changes.
// Deprecated: Use StartWithContext for proper context cancellation support.
func (w *FileWatcher) Start() error {
	return w.StartWithContext(context.Background())
}

// StartWithContext begins watching for changes with context cancellation support.
// The watcher will stop when the context is cancelled.
func (w *FileWatcher) StartWithContext(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}

	// Create fsnotify watcher - don't store it yet until setup succeeds
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Unlock()
		return fmt.Errorf("create watcher: %w", err)
	}

	// Prepare channels for potential restart after Stop()
	// Don't assign to struct fields until setup succeeds
	stopChan := make(chan struct{})
	doneChan := make(chan struct{})

	// Create a derived context so we can cancel it on Stop()
	watchCtx, cancel := context.WithCancel(ctx)
	w.mu.Unlock()

	// cleanup closes watcher and cancels context on error
	cleanup := func() {
		cancel()
		_ = watcher.Close()
	}

	// Track initial state and add paths to watcher
	// This is done outside the lock since it involves I/O
	// On any error, close watcher and return without modifying struct state
	for _, path := range w.config.Paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() && w.config.Recursive {
			if err := w.store.TrackDir(path, w.config.Patterns...); err != nil {
				cleanup()
				return err
			}
			// Add directory and subdirectories to watcher
			if err := w.addRecursiveWithWatcher(watcher, path); err != nil {
				cleanup()
				return err
			}
		} else {
			if err := w.store.Track(path); err != nil {
				cleanup()
				return err
			}
			if err := watcher.Add(path); err != nil {
				cleanup()
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
	}

	// Setup succeeded - atomically update all state under lock
	w.mu.Lock()
	w.watcher = watcher
	w.stop = stopChan
	w.done = doneChan
	w.ctx = watchCtx
	w.cancel = cancel
	w.running = true
	w.mu.Unlock()

	// Process events with context
	go w.eventLoop(watchCtx)

	return nil
}

// addRecursive adds a directory and all subdirectories to the watcher.
// Uses the watcher stored in w.watcher - only safe to call after Start() completes.
func (w *FileWatcher) addRecursive(root string) error {
	return w.addRecursiveWithWatcher(w.watcher, root)
}

// addRecursiveWithWatcher adds a directory and all subdirectories to the given watcher.
// This variant is used during initialization when w.watcher isn't set yet.
func (w *FileWatcher) addRecursiveWithWatcher(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}

// eventLoop processes fsnotify events with debouncing.
// The context is checked for cancellation to enable graceful shutdown.
func (w *FileWatcher) eventLoop(ctx context.Context) {
	defer close(w.done)

	// Debounce: collect events and process after quiet period
	pending := make(map[string]fsnotify.Event)
	var timer *time.Timer
	var timerC <-chan time.Time

	// getCallbacks safely retrieves callbacks under lock.
	// Returns copies to avoid holding lock during callback invocation.
	getCallbacks := func() (onChange func([]*FileChange), onError func(error)) {
		w.mu.Lock()
		onChange = w.onChange
		onError = w.onError
		w.mu.Unlock()
		return
	}

	processPending := func() {
		if len(pending) == 0 {
			return
		}

		var changes []*FileChange
		for path, event := range pending {
			change := w.processEvent(path, event)
			if change != nil {
				changes = append(changes, change)
			}
		}

		if len(changes) > 0 {
			onChange, _ := getCallbacks()
			if onChange != nil {
				onChange(changes)
			}
		}

		pending = make(map[string]fsnotify.Event)
	}

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - graceful shutdown
			if timer != nil {
				timer.Stop()
			}
			processPending()
			return

		case <-w.stop:
			if timer != nil {
				timer.Stop()
			}
			processPending()
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Skip events for paths that don't match patterns
			if !w.matchesPatterns(event.Name) {
				continue
			}

			// Accumulate event
			pending[event.Name] = event

			// Handle new directories for recursive watching
			if event.Has(fsnotify.Create) && w.config.Recursive {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(event.Name)
				}
			}

			// Reset debounce timer
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(w.config.Debounce)
			timerC = timer.C

		case <-timerC:
			processPending()
			timer = nil
			timerC = nil

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Call error callback or log to stderr (get callback safely)
			_, onError := getCallbacks()
			if onError != nil {
				onError(err)
			} else {
				fmt.Fprintf(os.Stderr, "file watcher error: %v\n", err)
			}
		}
	}
}

// matchesPatterns checks if a path matches the configured patterns.
func (w *FileWatcher) matchesPatterns(path string) bool {
	if len(w.config.Patterns) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, pattern := range w.config.Patterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// processEvent converts an fsnotify event to a FileChange.
func (w *FileWatcher) processEvent(path string, event fsnotify.Event) *FileChange {
	var changeType string
	var before, after *FileState

	// Get before state from store
	w.store.mu.RLock()
	before = w.store.current[path]
	w.store.mu.RUnlock()

	if before == nil {
		before = &FileState{Path: path, Exists: false}
	}

	switch {
	case event.Has(fsnotify.Create):
		changeType = "create"
		after, _ = w.store.CaptureFile(path)
		if after == nil {
			after = &FileState{Path: path, Exists: false}
		}

	case event.Has(fsnotify.Write):
		changeType = "modify"
		after, _ = w.store.CaptureFile(path)
		if after == nil {
			return nil
		}
		// Skip if hash unchanged (editor save without modifications)
		if before.Hash == after.Hash {
			return nil
		}

	case event.Has(fsnotify.Remove):
		changeType = "delete"
		after = &FileState{Path: path, Exists: false}

	case event.Has(fsnotify.Rename):
		// Treat rename as delete; the new name will trigger Create
		changeType = "delete"
		after = &FileState{Path: path, Exists: false}

	default:
		return nil
	}

	change := &FileChange{
		Path:       path,
		ChangeType: changeType,
		Before:     before,
		After:      after,
		Timestamp:  time.Now(),
	}

	// Update store state
	w.store.mu.Lock()
	w.store.changes = append(w.store.changes, change)
	w.store.current[path] = after
	// Evict oldest changes if limit exceeded
	if w.store.maxChanges > 0 && len(w.store.changes) > w.store.maxChanges {
		w.store.changes = w.store.changes[len(w.store.changes)-w.store.maxChanges:]
	}
	w.store.mu.Unlock()

	return change
}

// Stop stops watching and cleans up resources.
func (w *FileWatcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	cancel := w.cancel
	w.mu.Unlock()

	// Cancel context to signal shutdown
	if cancel != nil {
		cancel()
	}

	// Signal stop and wait for eventLoop to finish
	close(w.stop)
	<-w.done

	// Close the watcher
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}
