package integration

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhar-astro/goagent/internal/app"
	"github.com/vhar-astro/goagent/internal/config"
)

func TestBaseSessionScenario(t *testing.T) {
	t.Run("bootstrap initializes a fresh base session from config", func(t *testing.T) {
		t.Parallel()

		workspaceDir := t.TempDir()
		configPath := writeIntegrationConfig(t, workspaceDir, map[string]any{
			"default_provider": config.DefaultProviderName,
			"streaming":        true,
			"providers": map[string]any{
				config.DefaultProviderName: map[string]any{
					"name":                 config.DefaultProviderName,
					"base_url":             config.DefaultChutesBaseURL,
					"api_key_env":          config.DefaultChutesAPIKeyEnv,
					"model":                config.DefaultChutesModel,
					"supports_streaming":   true,
					"supports_tool_calling": true,
				},
			},
		})

		application, err := app.Bootstrap(app.BootstrapOptions{
			ConfigPath: configPath,
			Stdin:      strings.NewReader(""),
			Stdout:     io.Discard,
			Stderr:     io.Discard,
		})
		if err != nil {
			t.Fatalf("Bootstrap() error = %v", err)
		}

		options := application.Options()
		if options.ConfigPath != configPath {
			t.Fatalf("Options().ConfigPath = %q, want %q", options.ConfigPath, configPath)
		}
		if options.WorkspaceRoot != workspaceDir {
			t.Fatalf("Options().WorkspaceRoot = %q, want %q", options.WorkspaceRoot, workspaceDir)
		}
		if options.ProviderName != config.DefaultProviderName {
			t.Fatalf("Options().ProviderName = %q, want %q", options.ProviderName, config.DefaultProviderName)
		}
		if options.Model != config.DefaultChutesModel {
			t.Fatalf("Options().Model = %q, want %q", options.Model, config.DefaultChutesModel)
		}

		loadedConfig := application.Config()
		if loadedConfig.Workspace != workspaceDir {
			t.Fatalf("Config().Workspace = %q, want %q", loadedConfig.Workspace, workspaceDir)
		}
		if loadedConfig.DefaultProvider != config.DefaultProviderName {
			t.Fatalf("Config().DefaultProvider = %q, want %q", loadedConfig.DefaultProvider, config.DefaultProviderName)
		}

		profile := application.ProviderProfile()
		if profile.Name != config.DefaultProviderName {
			t.Fatalf("ProviderProfile().Name = %q, want %q", profile.Name, config.DefaultProviderName)
		}
		if profile.Model != config.DefaultChutesModel {
			t.Fatalf("ProviderProfile().Model = %q, want %q", profile.Model, config.DefaultChutesModel)
		}

		repl := application.REPL()
		if repl == nil {
			t.Fatal("REPL() = nil, want initialized REPL")
		}
		if repl.Prompt != "> " {
			t.Fatalf("REPL().Prompt = %q, want %q", repl.Prompt, "> ")
		}

		session := application.Session()
		if session == nil {
			t.Fatal("Session() = nil, want initialized session")
		}
		if session.ID == "" {
			t.Fatal("Session().ID = empty, want generated session id")
		}
		if session.WorkspaceRoot != workspaceDir {
			t.Fatalf("Session().WorkspaceRoot = %q, want %q", session.WorkspaceRoot, workspaceDir)
		}
		if session.ProviderName != config.DefaultProviderName {
			t.Fatalf("Session().ProviderName = %q, want %q", session.ProviderName, config.DefaultProviderName)
		}
		if session.Model != config.DefaultChutesModel {
			t.Fatalf("Session().Model = %q, want %q", session.Model, config.DefaultChutesModel)
		}
		if got := len(session.Messages); got != 0 {
			t.Fatalf("len(Session().Messages) = %d, want 0", got)
		}
		if got := len(session.ApprovedCapabilities); got != 0 {
			t.Fatalf("len(Session().ApprovedCapabilities) = %d, want 0", got)
		}
		if got := len(session.AttachedModules); got != 0 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 0", got)
		}
	})

	t.Run("bootstrap honors provider and workspace overrides", func(t *testing.T) {
		t.Parallel()

		configuredWorkspace := t.TempDir()
		overrideWorkspace := t.TempDir()
		overrideModel := "openai/custom-integration-model"
		configPath := writeIntegrationConfig(t, configuredWorkspace, map[string]any{
			"default_provider": config.DefaultProviderName,
			"providers": map[string]any{
				config.DefaultProviderName: map[string]any{
					"name":                 config.DefaultProviderName,
					"base_url":             config.DefaultChutesBaseURL,
					"api_key_env":          config.DefaultChutesAPIKeyEnv,
					"model":                config.DefaultChutesModel,
					"supports_streaming":   true,
					"supports_tool_calling": true,
				},
				config.OpenRouterProviderName: map[string]any{
					"name":                 config.OpenRouterProviderName,
					"base_url":             config.DefaultOpenRouterBaseURL,
					"api_key_env":          config.DefaultOpenRouterAPIKeyEnv,
					"model":                overrideModel,
					"supports_streaming":   true,
					"supports_tool_calling": true,
				},
			},
		})

		application, err := app.Bootstrap(app.BootstrapOptions{
			ConfigPath:        configPath,
			WorkspaceOverride: overrideWorkspace,
			ProviderOverride:  config.OpenRouterProviderName,
			Stdin:             strings.NewReader(""),
			Stdout:            io.Discard,
			Stderr:            io.Discard,
		})
		if err != nil {
			t.Fatalf("Bootstrap() error = %v", err)
		}

		options := application.Options()
		if options.WorkspaceRoot != overrideWorkspace {
			t.Fatalf("Options().WorkspaceRoot = %q, want %q", options.WorkspaceRoot, overrideWorkspace)
		}
		if options.ProviderName != config.OpenRouterProviderName {
			t.Fatalf("Options().ProviderName = %q, want %q", options.ProviderName, config.OpenRouterProviderName)
		}
		if options.Model != overrideModel {
			t.Fatalf("Options().Model = %q, want %q", options.Model, overrideModel)
		}

		if application.Config().Workspace != configuredWorkspace {
			t.Fatalf("Config().Workspace = %q, want configured workspace %q", application.Config().Workspace, configuredWorkspace)
		}

		profile := application.ProviderProfile()
		if profile.Name != config.OpenRouterProviderName {
			t.Fatalf("ProviderProfile().Name = %q, want %q", profile.Name, config.OpenRouterProviderName)
		}
		if profile.Model != overrideModel {
			t.Fatalf("ProviderProfile().Model = %q, want %q", profile.Model, overrideModel)
		}

		session := application.Session()
		if session.WorkspaceRoot != overrideWorkspace {
			t.Fatalf("Session().WorkspaceRoot = %q, want %q", session.WorkspaceRoot, overrideWorkspace)
		}
		if session.ProviderName != config.OpenRouterProviderName {
			t.Fatalf("Session().ProviderName = %q, want %q", session.ProviderName, config.OpenRouterProviderName)
		}
		if session.Model != overrideModel {
			t.Fatalf("Session().Model = %q, want %q", session.Model, overrideModel)
		}
	})

	t.Run("bootstrap rejects unknown provider overrides", func(t *testing.T) {
		t.Parallel()

		workspaceDir := t.TempDir()
		configPath := writeIntegrationConfig(t, workspaceDir, map[string]any{
			"default_provider": config.DefaultProviderName,
		})

		_, err := app.Bootstrap(app.BootstrapOptions{
			ConfigPath:       configPath,
			ProviderOverride: "missing-provider",
			Stdin:            strings.NewReader(""),
			Stdout:           io.Discard,
			Stderr:           io.Discard,
		})
		if err == nil {
			t.Fatal("Bootstrap() error = nil, want unknown provider error")
		}
		if !strings.Contains(err.Error(), `provider "missing-provider" is not configured`) {
			t.Fatalf("Bootstrap() error = %q, want unknown provider message", err)
		}
	})

	t.Run("bootstrap rejects missing workspace overrides", func(t *testing.T) {
		t.Parallel()

		configuredWorkspace := t.TempDir()
		missingWorkspace := filepath.Join(t.TempDir(), "missing")
		configPath := writeIntegrationConfig(t, configuredWorkspace, map[string]any{
			"default_provider": config.DefaultProviderName,
		})

		_, err := app.Bootstrap(app.BootstrapOptions{
			ConfigPath:        configPath,
			WorkspaceOverride: missingWorkspace,
			Stdin:             strings.NewReader(""),
			Stdout:            io.Discard,
			Stderr:            io.Discard,
		})
		if err == nil {
			t.Fatal("Bootstrap() error = nil, want missing workspace error")
		}
		if !strings.Contains(err.Error(), "stat workspace path:") {
			t.Fatalf("Bootstrap() error = %q, want workspace stat error", err)
		}
	})

	t.Run("inspect edit command and web execution loop remains pending", func(t *testing.T) {
		workspaceDir := t.TempDir()
		configPath := writeIntegrationConfig(t, workspaceDir, map[string]any{
			"default_provider": config.DefaultProviderName,
		})

		application, err := app.Bootstrap(app.BootstrapOptions{
			ConfigPath: configPath,
			Stdin:      strings.NewReader(""),
			Stdout:     io.Discard,
			Stderr:     io.Discard,
		})
		if err != nil {
			t.Fatalf("Bootstrap() error = %v", err)
		}
		if err := application.Run(context.Background()); err != nil {
			t.Fatalf("Run() error = %v, want nil from current empty runner", err)
		}

		session := application.Session()
		if got := len(session.Conversation()); got != 0 {
			t.Fatalf("len(Session().Conversation()) = %d, want 0 before the runtime loop exists", got)
		}
		if got := len(session.AttachedModules); got != 0 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 0 in the base launch path", got)
		}

		t.Skip("TODO: T017-T020 must replace the empty session runner and REPL loop with provider-driven inspect/edit/command/web execution")
	})
}

func writeIntegrationConfig(t *testing.T, workspaceDir string, overrides map[string]any) string {
	t.Helper()

	payload := map[string]any{
		"workspace":        workspaceDir,
		"default_provider": config.DefaultProviderName,
		"streaming":        true,
		"timeouts": map[string]any{
			"shell_seconds": config.DefaultShellTimeoutSeconds,
			"http_seconds":  config.DefaultHTTPTimeoutSeconds,
		},
		"output_limits": map[string]any{
			"tool_bytes": config.DefaultToolOutputBytes,
		},
		"providers": map[string]any{
			config.DefaultProviderName: map[string]any{
				"name":                  config.DefaultProviderName,
				"base_url":              config.DefaultChutesBaseURL,
				"api_key_env":           config.DefaultChutesAPIKeyEnv,
				"model":                 config.DefaultChutesModel,
				"supports_streaming":    true,
				"supports_tool_calling": true,
			},
			config.OpenRouterProviderName: map[string]any{
				"name":                  config.OpenRouterProviderName,
				"base_url":              config.DefaultOpenRouterBaseURL,
				"api_key_env":           config.DefaultOpenRouterAPIKeyEnv,
				"model":                 config.DefaultOpenRouterModel,
				"supports_streaming":    true,
				"supports_tool_calling": true,
			},
		},
	}

	for key, value := range overrides {
		payload[key] = value
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	file, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", configPath, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(payload); err != nil {
		t.Fatalf("Encode(%q) error = %v", configPath, err)
	}

	return configPath
}
