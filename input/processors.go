package input

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/agent/internal/pathutil"
)

// Security-related errors.
var (
	ErrPathTraversal      = pathutil.ErrPathTraversal
	ErrPathOutsideBase    = pathutil.ErrPathOutsideBase
	ErrBlockedURL         = errors.New("URL is blocked")
	ErrPrivateIP          = errors.New("private IP addresses are not allowed")
	ErrInvalidScheme      = errors.New("only http and https schemes are allowed")
	ErrBlockedContentType = errors.New("content type is not allowed")
	ErrFileTooLarge       = errors.New("file exceeds maximum size limit")
	ErrTooManyFiles       = errors.New("glob pattern matched too many files")
	ErrTotalSizeExceeded  = errors.New("total file size exceeds limit")
)

// blockedContentTypes contains MIME types that should not be processed.
var blockedContentTypes = []string{
	"application/octet-stream",
	"application/x-executable",
	"application/x-msdos-program",
	"application/x-msdownload",
	"application/x-sharedlib",
	"application/x-dosexec",
}

// isBlockedContentType checks if a Content-Type should be rejected.
func isBlockedContentType(contentType string) bool {
	// Extract MIME type without parameters (e.g., "text/html; charset=utf-8" -> "text/html")
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	mimeType = strings.ToLower(mimeType)

	return slices.Contains(blockedContentTypes, mimeType)
}

// Default limits for file operations.
const (
	DefaultMaxFileSize   = 10 * 1024 * 1024  // 10MB
	DefaultMaxFileCount  = 100
	DefaultMaxTotalBytes = 50 * 1024 * 1024  // 50MB
	DefaultMaxURLBytes   = 10 * 1024 * 1024  // 10MB
)

// Processor processes input and returns Content.
type Processor interface {
	// Process processes the input value and returns Content.
	Process(ctx context.Context, value string) (*Content, error)

	// CanProcess returns true if this processor can handle the given type.
	CanProcess(typ Type) bool
}

// URLProcessor processes URL inputs with SSRF protection.
type URLProcessor struct {
	client       *http.Client
	maxBytes     int64
	timeout      time.Duration
	allowPrivate bool // Set to true only for testing
}

// URLProcessorOption configures URLProcessor.
type URLProcessorOption func(*URLProcessor)

// WithTimeout sets the HTTP client timeout (default 30s).
func WithTimeout(timeout time.Duration) URLProcessorOption {
	return func(p *URLProcessor) {
		p.timeout = timeout
	}
}

// WithMaxContentSize sets the maximum response body size (default 10MB).
func WithMaxContentSize(maxBytes int64) URLProcessorOption {
	return func(p *URLProcessor) {
		p.maxBytes = maxBytes
	}
}

// WithMaxURLBytes sets the maximum response body size (deprecated: use WithMaxContentSize).
func WithMaxURLBytes(maxBytes int64) URLProcessorOption {
	return WithMaxContentSize(maxBytes)
}

// WithAllowPrivateIPs allows fetching from private IP addresses (use only for testing).
func WithAllowPrivateIPs(allow bool) URLProcessorOption {
	return func(p *URLProcessor) {
		p.allowPrivate = allow
	}
}

// NewURLProcessor creates a new URL processor with SSRF protection.
func NewURLProcessor(opts ...URLProcessorOption) *URLProcessor {
	p := &URLProcessor{
		maxBytes:     DefaultMaxURLBytes,
		timeout:      30 * time.Second,
		allowPrivate: false,
	}

	// Apply options first to get timeout
	for _, opt := range opts {
		opt(p)
	}

	// Create client with configured timeout
	p.client = &http.Client{
		Timeout: p.timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}

	return p
}

// Process fetches the URL and returns its content.
func (p *URLProcessor) Process(ctx context.Context, value string) (*Content, error) {
	// Validate URL before making request
	if err := p.validateURL(value); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Validate Content-Type to block executable/binary content
	contentType := resp.Header.Get("Content-Type")
	if isBlockedContentType(contentType) {
		return nil, fmt.Errorf("%w: %s", ErrBlockedContentType, contentType)
	}

	// Use LimitReader to prevent reading excessive data
	limitedReader := io.LimitReader(resp.Body, p.maxBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check if we hit the limit (read more than maxBytes)
	if int64(len(data)) > p.maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes limit", p.maxBytes)
	}

	content := NewContent(TypeURL, value)
	content.Data = data
	content.Metadata["status_code"] = resp.StatusCode
	content.Metadata["content_type"] = resp.Header.Get("Content-Type")

	return content, nil
}

// validateURL checks if the URL is safe to fetch (SSRF protection).
func (p *URLProcessor) validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	// Only allow http and https schemes
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	// Extract hostname (without port)
	host := parsed.Hostname()

	// Check for cloud metadata endpoints
	if isCloudMetadataHost(host) {
		return fmt.Errorf("%w: cloud metadata endpoint", ErrBlockedURL)
	}

	// Resolve hostname to IP and check if private
	if !p.allowPrivate {
		ips, err := net.LookupIP(host)
		if err != nil {
			// Fail-closed: block request if DNS resolution fails (prevents SSRF bypass)
			return fmt.Errorf("%w: DNS resolution failed for %s", ErrBlockedURL, host)
		}

		if slices.ContainsFunc(ips, isPrivateIP) {
			return ErrPrivateIP
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private, loopback, or link-local.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// Check for loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private ranges
	if ip.IsPrivate() {
		return true
	}

	// Additional check for IPv4-mapped IPv6 addresses
	if ip4 := ip.To4(); ip4 != nil {
		// Check specific private ranges
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 127.0.0.0/8 (loopback, double-check)
		if ip4[0] == 127 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}

	return false
}

// isCloudMetadataHost checks if the host is a cloud metadata endpoint.
func isCloudMetadataHost(host string) bool {
	metadataHosts := []string{
		"169.254.169.254",         // AWS, GCP, Azure
		"metadata.google.internal", // GCP
		"metadata",                 // Various clouds
		"100.100.100.200",         // Alibaba Cloud
		"fd00:ec2::254",           // AWS IPv6
	}

	for _, blocked := range metadataHosts {
		if strings.EqualFold(host, blocked) {
			return true
		}
	}

	return false
}

// CanProcess returns true for URL types.
func (p *URLProcessor) CanProcess(typ Type) bool {
	return typ == TypeURL
}

// FileProcessor processes file inputs with path traversal protection.
type FileProcessor struct {
	baseDir     string // If set, restricts file access to this directory
	maxFileSize int64
}

// FileProcessorOption configures FileProcessor.
type FileProcessorOption func(*FileProcessor)

// WithBaseDir restricts file access to the specified directory.
func WithBaseDir(dir string) FileProcessorOption {
	return func(p *FileProcessor) {
		p.baseDir = dir
	}
}

// WithMaxFileSize sets the maximum file size to read.
func WithMaxFileSize(maxBytes int64) FileProcessorOption {
	return func(p *FileProcessor) {
		p.maxFileSize = maxBytes
	}
}

// NewFileProcessor creates a new file processor with path validation.
func NewFileProcessor(opts ...FileProcessorOption) *FileProcessor {
	p := &FileProcessor{
		maxFileSize: DefaultMaxFileSize,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Process reads the file and returns its content.
func (p *FileProcessor) Process(ctx context.Context, value string) (*Content, error) {
	// Expand tilde
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("expand home dir: %w", err)
		}
		value = filepath.Join(home, value[1:])
	}

	// Validate path for traversal attacks
	cleanPath, err := p.validatePath(value)
	if err != nil {
		return nil, err
	}

	// Check file size before reading
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, pathutil.SanitizeError("stat file", err)
	}

	if info.Size() > p.maxFileSize {
		return nil, fmt.Errorf("%w: file exceeds size limit (max %d bytes)", ErrFileTooLarge, p.maxFileSize)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, pathutil.SanitizeError("read file", err)
	}

	content := NewContent(TypeFile, cleanPath)
	content.Data = data
	content.Metadata["size"] = info.Size()
	content.Metadata["mod_time"] = info.ModTime()

	return content, nil
}

// validatePath checks for path traversal and ensures path is within base directory.
func (p *FileProcessor) validatePath(path string) (string, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	// Clean the path to resolve any .. or . components
	cleanPath := filepath.Clean(absPath)

	// Check for path traversal patterns in original input
	if pathutil.ContainsTraversal(path) {
		return "", ErrPathTraversal
	}

	// If base directory is set, ensure path is within it
	if p.baseDir != "" {
		baseAbs, err := filepath.Abs(p.baseDir)
		if err != nil {
			return "", fmt.Errorf("resolve base dir: %w", err)
		}
		baseClean := filepath.Clean(baseAbs)

		// Use filepath.Rel to check if path is within base
		rel, err := filepath.Rel(baseClean, cleanPath)
		if err != nil {
			return "", fmt.Errorf("resolve relative path: %w", err)
		}

		// If relative path starts with .., it's outside base directory
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return "", ErrPathOutsideBase
		}
	}

	return cleanPath, nil
}

// CanProcess returns true for file types.
func (p *FileProcessor) CanProcess(typ Type) bool {
	return typ == TypeFile
}

// GlobProcessor processes glob pattern inputs with resource limits.
type GlobProcessor struct {
	fileProcessor *FileProcessor
	maxFileCount  int
	maxTotalBytes int64
	maxFileSize   int64
	baseDir       string
}

// GlobProcessorOption configures GlobProcessor.
type GlobProcessorOption func(*GlobProcessor)

// WithMaxMatches sets the maximum number of files to process (default 100).
func WithMaxMatches(count int) GlobProcessorOption {
	return func(p *GlobProcessor) {
		p.maxFileCount = count
	}
}

// WithGlobMaxFileCount sets the maximum number of files to process (deprecated: use WithMaxMatches).
func WithGlobMaxFileCount(count int) GlobProcessorOption {
	return WithMaxMatches(count)
}

// WithGlobMaxTotalBytes sets the maximum total bytes to read across all files.
func WithGlobMaxTotalBytes(bytes int64) GlobProcessorOption {
	return func(p *GlobProcessor) {
		p.maxTotalBytes = bytes
	}
}

// WithGlobMaxFileSize sets the maximum size per file.
func WithGlobMaxFileSize(bytes int64) GlobProcessorOption {
	return func(p *GlobProcessor) {
		p.maxFileSize = bytes
	}
}

// WithGlobBaseDir restricts glob to files within the specified directory.
func WithGlobBaseDir(dir string) GlobProcessorOption {
	return func(p *GlobProcessor) {
		p.baseDir = dir
	}
}

// NewGlobProcessor creates a new glob processor with resource limits.
func NewGlobProcessor(opts ...GlobProcessorOption) *GlobProcessor {
	p := &GlobProcessor{
		maxFileCount:  DefaultMaxFileCount,
		maxTotalBytes: DefaultMaxTotalBytes,
		maxFileSize:   DefaultMaxFileSize,
	}

	for _, opt := range opts {
		opt(p)
	}

	// Create file processor with matching settings
	fileOpts := []FileProcessorOption{
		WithMaxFileSize(p.maxFileSize),
	}
	if p.baseDir != "" {
		fileOpts = append(fileOpts, WithBaseDir(p.baseDir))
	}
	p.fileProcessor = NewFileProcessor(fileOpts...)

	return p
}

// Process expands the glob pattern and reads matching files with limits.
func (p *GlobProcessor) Process(ctx context.Context, value string) (*Content, error) {
	matches, err := filepath.Glob(value)
	if err != nil {
		return nil, fmt.Errorf("glob pattern: %w", err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no files matched pattern: %s", value)
	}

	// Check file count limit
	if len(matches) > p.maxFileCount {
		return nil, fmt.Errorf("%w: pattern matched %d files (max %d)", ErrTooManyFiles, len(matches), p.maxFileCount)
	}

	// Pre-check total size before reading
	var totalSize int64
	validMatches := make([]string, 0, len(matches))

	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue // Skip files we can't stat
		}

		// Skip directories
		if info.IsDir() {
			continue
		}

		// Check individual file size
		if info.Size() > p.maxFileSize {
			return nil, fmt.Errorf("%w: %s is %d bytes (max %d)", ErrFileTooLarge, match, info.Size(), p.maxFileSize)
		}

		// Check cumulative size
		if totalSize+info.Size() > p.maxTotalBytes {
			return nil, fmt.Errorf("%w: would exceed %d bytes after reading %s", ErrTotalSizeExceeded, p.maxTotalBytes, match)
		}

		totalSize += info.Size()
		validMatches = append(validMatches, match)
	}

	if len(validMatches) == 0 {
		return nil, fmt.Errorf("no valid files matched pattern: %s", value)
	}

	content := NewContent(TypeGlob, value)
	content.Metadata["matches"] = validMatches
	content.Metadata["count"] = len(validMatches)

	// Read all matching files
	var combined strings.Builder
	for i, match := range validMatches {
		fileContent, err := p.fileProcessor.Process(ctx, match)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", match, err)
		}

		if i > 0 {
			combined.WriteString("\n---\n")
		}
		fmt.Fprintf(&combined, "# %s\n", match)
		combined.Write(fileContent.Data)
	}

	content.Data = []byte(combined.String())
	content.Metadata["total_bytes"] = totalSize

	return content, nil
}

// CanProcess returns true for glob types.
func (p *GlobProcessor) CanProcess(typ Type) bool {
	return typ == TypeGlob
}

// TextProcessor processes plain text inputs.
type TextProcessor struct{}

// NewTextProcessor creates a new text processor.
func NewTextProcessor() *TextProcessor {
	return &TextProcessor{}
}

// Process returns the text as-is.
func (p *TextProcessor) Process(ctx context.Context, value string) (*Content, error) {
	content := NewContent(TypeText, value)
	content.Data = []byte(value)
	return content, nil
}

// CanProcess returns true for text types.
func (p *TextProcessor) CanProcess(typ Type) bool {
	return typ == TypeText
}

// Registry manages processors for different input types.
type Registry struct {
	processors []Processor
}

// NewRegistry creates a new processor registry with default processors.
func NewRegistry() *Registry {
	return &Registry{
		processors: []Processor{
			NewURLProcessor(),
			NewFileProcessor(),
			NewGlobProcessor(),
			NewTextProcessor(),
		},
	}
}

// Register adds a custom processor to the registry.
func (r *Registry) Register(p Processor) {
	r.processors = append(r.processors, p)
}

// Process detects the input type and processes it with the appropriate processor.
func (r *Registry) Process(ctx context.Context, value string) (*Content, error) {
	typ := Detect(value)

	for _, p := range r.processors {
		if p.CanProcess(typ) {
			return p.Process(ctx, value)
		}
	}

	return nil, fmt.Errorf("no processor for type: %s", typ)
}
