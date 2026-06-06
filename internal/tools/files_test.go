package tools_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhar-astro/goagent/internal/app"
)

func TestFileRuntimeResolvePathAllowsWorkspaceFilesAndNewTargets(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	existingDir := filepath.Join(workspace, "docs")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existingFile := filepath.Join(existingDir, "guide.txt")
	if err := os.WriteFile(existingFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := mustNewToolRuntime(t, workspace)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "relative existing file",
			path: "docs/guide.txt",
			want: existingFile,
		},
		{
			name: "absolute existing file",
			path: existingFile,
			want: existingFile,
		},
		{
			name: "new nested target stays inside workspace",
			path: "drafts/new-note.txt",
			want: filepath.Join(workspace, "drafts", "new-note.txt"),
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

func TestFileRuntimeResolvePathRejectsBlankAndOutsideTargets(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outside := t.TempDir()
	runtime := mustNewToolRuntime(t, workspace)

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "blank path",
			path:    "   ",
			wantErr: app.ErrPathRequired,
		},
		{
			name:    "relative path escapes workspace",
			path:    filepath.Join("..", filepath.Base(outside), "secret.txt"),
			wantErr: app.ErrPathOutsideWorkspace,
		},
		{
			name:    "absolute path escapes workspace",
			path:    filepath.Join(outside, "secret.txt"),
			wantErr: app.ErrPathOutsideWorkspace,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := runtime.ResolvePath(tt.path)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ResolvePath(%q) error = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestFileRuntimeResolvePathRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outside := t.TempDir()
	escapeLink := filepath.Join(workspace, "escape")

	if err := os.Symlink(outside, escapeLink); err != nil {
		t.Skipf("Symlink() unsupported in this environment: %v", err)
	}

	runtime := mustNewToolRuntime(t, workspace)

	_, err := runtime.ResolvePath(filepath.Join("escape", "secret.txt"))
	if !errors.Is(err, app.ErrPathOutsideWorkspace) {
		t.Fatalf("ResolvePath(symlink escape) error = %v, want %v", err, app.ErrPathOutsideWorkspace)
	}
	if runtime.ContainsPath(filepath.Join("escape", "secret.txt")) {
		t.Fatal("ContainsPath(symlink escape) = true, want false")
	}
}

func mustNewToolRuntime(t *testing.T, workspace string) app.Runtime {
	t.Helper()

	runtime, err := app.NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime(%q) error = %v", workspace, err)
	}

	return runtime
}
