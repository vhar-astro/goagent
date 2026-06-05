package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesDefaultsAndLegacyWorkspace(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	configPath := writeConfigFile(t, tempDir, map[string]any{
		"default_workspace": workspaceDir,
		"default_provider":  OpenRouterProviderName,
		"timeouts": map[string]any{
			"shell_seconds": 45,
		},
		"providers": map[string]any{
			OpenRouterProviderName: map[string]any{
				"model": "openai/custom-model",
				"extra_headers": map[string]string{
					"HTTP-Referer": "https://example.test",
				},
			},
		},
	})

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Workspace != workspaceDir {
		t.Fatalf("Workspace = %q, want %q", cfg.Workspace, workspaceDir)
	}
	if cfg.DefaultProvider != OpenRouterProviderName {
		t.Fatalf("DefaultProvider = %q, want %q", cfg.DefaultProvider, OpenRouterProviderName)
	}
	if !cfg.Streaming {
		t.Fatalf("Streaming = false, want true")
	}
	if cfg.Timeouts.ShellSeconds != 45 {
		t.Fatalf("ShellSeconds = %d, want 45", cfg.Timeouts.ShellSeconds)
	}
	if cfg.Timeouts.HTTPSeconds != DefaultHTTPTimeoutSeconds {
		t.Fatalf("HTTPSeconds = %d, want %d", cfg.Timeouts.HTTPSeconds, DefaultHTTPTimeoutSeconds)
	}
	if cfg.OutputLimits.ToolBytes != DefaultToolOutputBytes {
		t.Fatalf("ToolBytes = %d, want %d", cfg.OutputLimits.ToolBytes, DefaultToolOutputBytes)
	}

	profile, ok := cfg.Provider(OpenRouterProviderName)
	if !ok {
		t.Fatalf("Provider(%q) not found", OpenRouterProviderName)
	}
	if profile.Model != "openai/custom-model" {
		t.Fatalf("profile.Model = %q, want %q", profile.Model, "openai/custom-model")
	}
	if got := profile.ExtraHeaders["HTTP-Referer"]; got != "https://example.test" {
		t.Fatalf("ExtraHeaders[HTTP-Referer] = %q, want %q", got, "https://example.test")
	}
}

func TestLoadRejectsInvalidProviderProfile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := writeConfigFile(t, tempDir, map[string]any{
		"workspace": t.TempDir(),
		"providers": map[string]any{
			DefaultProviderName: map[string]any{
				"supports_streaming": false,
			},
		},
	})

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `provider "chutes" must support streaming`) {
		t.Fatalf("Load() error = %q, want streaming validation error", err)
	}
}

func TestResolveWorkspaceUsesOverrideThenConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWD); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	})
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	configuredDir := filepath.Join(tempDir, "configured")
	overrideDir := filepath.Join(tempDir, "override")
	for _, dir := range []string{configuredDir, overrideDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}

	cfg := Default()
	cfg.Workspace = "configured"

	resolvedConfigured, err := cfg.ResolveWorkspace("")
	if err != nil {
		t.Fatalf("ResolveWorkspace(\"\") error = %v", err)
	}
	if resolvedConfigured != configuredDir {
		t.Fatalf("ResolveWorkspace(\"\") = %q, want %q", resolvedConfigured, configuredDir)
	}

	resolvedOverride, err := cfg.ResolveWorkspace("override")
	if err != nil {
		t.Fatalf("ResolveWorkspace(%q) error = %v", "override", err)
	}
	if resolvedOverride != overrideDir {
		t.Fatalf("ResolveWorkspace(%q) = %q, want %q", "override", resolvedOverride, overrideDir)
	}
}

func writeConfigFile(t *testing.T, dir string, payload map[string]any) string {
	t.Helper()

	path := filepath.Join(dir, "config.json")
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
