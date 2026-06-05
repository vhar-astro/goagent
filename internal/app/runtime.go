package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultCommandTimeout     = 30 * time.Second
	DefaultWebTimeout         = 15 * time.Second
	DefaultShutdownTimeout    = 5 * time.Second
	DefaultToolOutputLimit    = 32 * 1024
	DefaultCommandOutputLimit = 64 * 1024
	DefaultWebContentLimit    = 64 * 1024
)

const truncatedMarker = "\n...[truncated]"

var (
	ErrWorkspaceRootRequired = errors.New("workspace root is required")
	ErrWorkspaceNotDirectory = errors.New("workspace root must be an existing directory")
	ErrPathRequired          = errors.New("path is required")
	ErrPathOutsideWorkspace  = errors.New("path resolves outside the workspace")
	ErrURLRequired           = errors.New("url is required")
	ErrURLMustBeAbsolute     = errors.New("url must be absolute")
	ErrUnsupportedURLScheme  = errors.New("url must use http or https")
	ErrURLHostRequired       = errors.New("url host is required")
	ErrURLCredentials        = errors.New("url must not include credentials")
)

// RuntimeOptions captures the shared boundary and limit settings for one session.
type RuntimeOptions struct {
	WorkspaceRoot      string
	CommandTimeout     time.Duration
	WebTimeout         time.Duration
	ShutdownTimeout    time.Duration
	ToolOutputLimit    int
	CommandOutputLimit int
	WebContentLimit    int
}

// LimitedText describes a possibly truncated text payload.
type LimitedText struct {
	Text          string
	OriginalBytes int
	Truncated     bool
}

// DroppedBytes reports how many bytes were removed during truncation.
func (l LimitedText) DroppedBytes() int {
	if !l.Truncated || l.OriginalBytes <= len(l.Text) {
		return 0
	}

	return l.OriginalBytes - len(l.Text)
}

// Runtime stores normalized workspace and execution limits for the session.
type Runtime struct {
	workspaceRoot      string
	commandTimeout     time.Duration
	webTimeout         time.Duration
	shutdownTimeout    time.Duration
	toolOutputLimit    int
	commandOutputLimit int
	webContentLimit    int
}

// DefaultRuntimeOptions returns the baseline runtime settings from the feature plan.
func DefaultRuntimeOptions() RuntimeOptions {
	return RuntimeOptions{
		CommandTimeout:     DefaultCommandTimeout,
		WebTimeout:         DefaultWebTimeout,
		ShutdownTimeout:    DefaultShutdownTimeout,
		ToolOutputLimit:    DefaultToolOutputLimit,
		CommandOutputLimit: DefaultCommandOutputLimit,
		WebContentLimit:    DefaultWebContentLimit,
	}
}

// NewRuntime constructs a runtime with default limits and a validated workspace root.
func NewRuntime(workspaceRoot string) (Runtime, error) {
	options := DefaultRuntimeOptions()
	options.WorkspaceRoot = workspaceRoot

	return NewRuntimeWithOptions(options)
}

// NewRuntimeWithOptions constructs a runtime with normalized workspace and limits.
func NewRuntimeWithOptions(options RuntimeOptions) (Runtime, error) {
	root, err := normalizeWorkspaceRoot(options.WorkspaceRoot)
	if err != nil {
		return Runtime{}, err
	}

	defaults := DefaultRuntimeOptions()

	return Runtime{
		workspaceRoot:      root,
		commandTimeout:     normalizeDuration(options.CommandTimeout, defaults.CommandTimeout),
		webTimeout:         normalizeDuration(options.WebTimeout, defaults.WebTimeout),
		shutdownTimeout:    normalizeDuration(options.ShutdownTimeout, defaults.ShutdownTimeout),
		toolOutputLimit:    normalizeLimit(options.ToolOutputLimit, defaults.ToolOutputLimit),
		commandOutputLimit: normalizeLimit(options.CommandOutputLimit, defaults.CommandOutputLimit),
		webContentLimit:    normalizeLimit(options.WebContentLimit, defaults.WebContentLimit),
	}, nil
}

// WorkspaceRoot returns the normalized absolute workspace boundary.
func (r Runtime) WorkspaceRoot() string {
	return r.workspaceRoot
}

// CommandTimeout returns the configured shell-command timeout.
func (r Runtime) CommandTimeout() time.Duration {
	return r.commandTimeout
}

// WebTimeout returns the configured explicit-fetch timeout.
func (r Runtime) WebTimeout() time.Duration {
	return r.webTimeout
}

// ShutdownTimeout returns the configured cleanup timeout for local processes.
func (r Runtime) ShutdownTimeout() time.Duration {
	return r.shutdownTimeout
}

// ToolOutputLimit returns the configured limit for tool-result reinjection.
func (r Runtime) ToolOutputLimit() int {
	return r.toolOutputLimit
}

// CommandOutputLimit returns the configured limit for shell command output.
func (r Runtime) CommandOutputLimit() int {
	return r.commandOutputLimit
}

// WebContentLimit returns the configured limit for explicit web fetch content.
func (r Runtime) WebContentLimit() int {
	return r.webContentLimit
}

// ResolvePath returns a cleaned absolute path after enforcing the workspace boundary.
func (r Runtime) ResolvePath(path string) (string, error) {
	return resolvePathWithinWorkspace(r.workspaceRoot, path)
}

// ContainsPath reports whether the provided path stays inside the workspace boundary.
func (r Runtime) ContainsPath(path string) bool {
	_, err := r.ResolvePath(path)
	return err == nil
}

// ValidateFetchURL parses and validates an explicit URL fetch target.
func (r Runtime) ValidateFetchURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, ErrURLRequired
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse url %q: %w", trimmed, err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("validate url %q: %w", trimmed, ErrURLMustBeAbsolute)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("validate url %q: %w", trimmed, ErrUnsupportedURLScheme)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("validate url %q: %w", trimmed, ErrURLHostRequired)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("validate url %q: %w", trimmed, ErrURLCredentials)
	}

	return parsed, nil
}

// WithCommandTimeout derives a timeout-scoped context for shell execution.
func (r Runtime) WithCommandTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, r.commandTimeout)
}

// WithWebTimeout derives a timeout-scoped context for explicit web fetches.
func (r Runtime) WithWebTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, r.webTimeout)
}

// WithShutdownTimeout derives a timeout-scoped context for process cleanup paths.
func (r Runtime) WithShutdownTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, r.shutdownTimeout)
}

// LimitToolOutput truncates tool output before it is reintroduced into model context.
func (r Runtime) LimitToolOutput(text string) LimitedText {
	return limitText(text, r.toolOutputLimit)
}

// LimitCommandOutput truncates shell output to a session-safe payload size.
func (r Runtime) LimitCommandOutput(text string) LimitedText {
	return limitText(text, r.commandOutputLimit)
}

// LimitWebContent truncates fetched web content to a session-safe payload size.
func (r Runtime) LimitWebContent(text string) LimitedText {
	return limitText(text, r.webContentLimit)
}

func normalizeWorkspaceRoot(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", ErrWorkspaceRootRequired
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root %q: %w", trimmed, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat workspace root %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate workspace root %q: %w", absPath, ErrWorkspaceNotDirectory)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root symlinks %q: %w", absPath, err)
	}

	return filepath.Clean(resolvedPath), nil
}

func resolvePathWithinWorkspace(root string, path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", ErrPathRequired
	}

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}

	candidate = filepath.Clean(candidate)

	resolvedCandidate, err := resolveForBoundaryCheck(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", candidate, err)
	}
	if !isWithinRoot(root, resolvedCandidate) {
		return "", fmt.Errorf("validate path %q: %w", candidate, ErrPathOutsideWorkspace)
	}

	return candidate, nil
}

func resolveForBoundaryCheck(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	probe := cleanPath
	var suffix []string

	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}

			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}

		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}

func isWithinRoot(root, path string) bool {
	if path == root {
		return true
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func normalizeDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}

	return fallback
}

func normalizeLimit(value, fallback int) int {
	if value > 0 {
		return value
	}

	return fallback
}

func limitText(text string, limit int) LimitedText {
	if limit <= 0 {
		return LimitedText{Text: text, OriginalBytes: len(text)}
	}

	originalBytes := len(text)
	if originalBytes <= limit {
		return LimitedText{Text: text, OriginalBytes: originalBytes}
	}

	marker := ""
	contentLimit := limit
	if limit > len(truncatedMarker) {
		marker = truncatedMarker
		contentLimit = limit - len(marker)
	}

	prefix := utf8SafePrefix(text, contentLimit)
	return LimitedText{
		Text:          prefix + marker,
		OriginalBytes: originalBytes,
		Truncated:     true,
	}
}

func utf8SafePrefix(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}

	cut := limit
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}

	return text[:cut]
}
