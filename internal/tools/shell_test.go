package tools_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhar-astro/goagent/internal/app"
)

func TestShellRuntimeResolvePathAllowsWorkspaceRootAndNestedWorkdirs(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	nested := filepath.Join(workspace, "subdir", "build")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	runtime := mustNewToolRuntime(t, workspace)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "workspace root",
			path: ".",
			want: workspace,
		},
		{
			name: "nested relative workdir",
			path: filepath.Join("subdir", "build"),
			want: nested,
		},
		{
			name: "absolute nested workdir",
			path: nested,
			want: nested,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := runtime.ResolvePath(tt.path)
			if err != nil {
				t.Fatalf("ResolvePath(%q) error = %v", tt.path, err)
			}
			if got != tt.want {
				t.Fatalf("ResolvePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestShellRuntimeRejectsOutsideAndSymlinkedWorkdirs(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outside := t.TempDir()
	escapeLink := filepath.Join(workspace, "escape")
	if err := os.Symlink(outside, escapeLink); err != nil {
		t.Skipf("Symlink() unsupported in this environment: %v", err)
	}

	runtime := mustNewToolRuntime(t, workspace)

	tests := []struct {
		name string
		path string
	}{
		{
			name: "relative path escapes workspace",
			path: filepath.Join("..", filepath.Base(outside)),
		},
		{
			name: "absolute path escapes workspace",
			path: outside,
		},
		{
			name: "symlinked workdir escapes workspace",
			path: "escape",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := runtime.ResolvePath(tt.path)
			if !errors.Is(err, app.ErrPathOutsideWorkspace) {
				t.Fatalf("ResolvePath(%q) error = %v, want %v", tt.path, err, app.ErrPathOutsideWorkspace)
			}
		})
	}
}

func TestShellRuntimeCommandTimeoutAndOutputLimit(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := app.NewRuntimeWithOptions(app.RuntimeOptions{
		WorkspaceRoot:      workspace,
		CommandTimeout:     150 * time.Millisecond,
		CommandOutputLimit: 20,
	})
	if err != nil {
		t.Fatalf("NewRuntimeWithOptions() error = %v", err)
	}

	before := time.Now()
	ctx, cancel := runtime.WithCommandTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("WithCommandTimeout() context has no deadline")
	}
	remaining := deadline.Sub(before)
	if remaining < 100*time.Millisecond || remaining > 500*time.Millisecond {
		t.Fatalf("WithCommandTimeout() deadline delta = %v, want between 100ms and 500ms", remaining)
	}

	limited := runtime.LimitCommandOutput("abcdefghijklmnop界界")
	if !limited.Truncated {
		t.Fatal("LimitCommandOutput() Truncated = false, want true")
	}
	if limited.OriginalBytes != len("abcdefghijklmnop界界") {
		t.Fatalf("LimitCommandOutput() OriginalBytes = %d, want %d", limited.OriginalBytes, len("abcdefghijklmnop界界"))
	}
	if !strings.Contains(limited.Text, "[truncated]") {
		t.Fatalf("LimitCommandOutput() Text = %q, want truncation marker", limited.Text)
	}
	if len(limited.Text) > runtime.CommandOutputLimit() {
		t.Fatalf("LimitCommandOutput() len(Text) = %d, want <= %d", len(limited.Text), runtime.CommandOutputLimit())
	}
}

func TestWebRuntimeValidateFetchURL(t *testing.T) {
	t.Parallel()

	runtime := mustNewToolRuntime(t, t.TempDir())

	tests := []struct {
		name    string
		rawURL  string
		wantErr error
	}{
		{
			name:   "absolute https url is allowed",
			rawURL: "https://example.com/docs?q=goagent",
		},
		{
			name:    "blank url is rejected",
			rawURL:  "   ",
			wantErr: app.ErrURLRequired,
		},
		{
			name:    "relative url is rejected",
			rawURL:  "/docs",
			wantErr: app.ErrURLMustBeAbsolute,
		},
		{
			name:    "unsupported scheme is rejected",
			rawURL:  "ftp://example.com/file.txt",
			wantErr: app.ErrUnsupportedURLScheme,
		},
		{
			name:    "missing host is rejected",
			rawURL:  "https:///docs",
			wantErr: app.ErrURLHostRequired,
		},
		{
			name:    "credentials are rejected",
			rawURL:  "https://user:pass@example.com/private",
			wantErr: app.ErrURLCredentials,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := runtime.ValidateFetchURL(tt.rawURL)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateFetchURL(%q) error = %v", tt.rawURL, err)
				}
				if parsed == nil {
					t.Fatalf("ValidateFetchURL(%q) = nil URL, want parsed URL", tt.rawURL)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateFetchURL(%q) error = %v, want %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}
