package integration

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vhar-astro/goagent/internal/app"
	"github.com/vhar-astro/goagent/internal/config"
)

func TestLaunchTimeProviderOverrideBootstrapPath(t *testing.T) {
	t.Parallel()

	configuredWorkspace := t.TempDir()
	overrideWorkspace := t.TempDir()
	const overrideModel = "openai/provider-override-integration"

	configPath := writeIntegrationConfig(t, configuredWorkspace, map[string]any{
		"default_provider": config.DefaultProviderName,
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
				"model":                 overrideModel,
				"supports_streaming":    true,
				"supports_tool_calling": true,
			},
		},
	})

	defaultLaunch, err := app.Bootstrap(app.BootstrapOptions{
		ConfigPath: configPath,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	})
	if err != nil {
		t.Fatalf("Bootstrap(default) error = %v", err)
	}

	overrideLaunch, err := app.Bootstrap(app.BootstrapOptions{
		ConfigPath:        configPath,
		WorkspaceOverride: overrideWorkspace,
		ProviderOverride:  config.OpenRouterProviderName,
		Stdin:             strings.NewReader(""),
		Stdout:            io.Discard,
		Stderr:            io.Discard,
	})
	if err != nil {
		t.Fatalf("Bootstrap(override) error = %v", err)
	}

	assertBootstrapProviderSelection(t, defaultLaunch, bootstrapExpectation{
		configPath:    configPath,
		workspaceRoot: configuredWorkspace,
		providerName:  config.DefaultProviderName,
		model:         config.DefaultChutesModel,
	})
	assertBootstrapProviderSelection(t, overrideLaunch, bootstrapExpectation{
		configPath:    configPath,
		workspaceRoot: overrideWorkspace,
		providerName:  config.OpenRouterProviderName,
		model:         overrideModel,
	})

	if got := defaultLaunch.Config().DefaultProvider; got != config.DefaultProviderName {
		t.Fatalf("default launch Config().DefaultProvider = %q, want %q", got, config.DefaultProviderName)
	}
	if got := overrideLaunch.Config().DefaultProvider; got != config.DefaultProviderName {
		t.Fatalf("override launch Config().DefaultProvider = %q, want original config default %q", got, config.DefaultProviderName)
	}
	if got := overrideLaunch.Config().Workspace; got != configuredWorkspace {
		t.Fatalf("override launch Config().Workspace = %q, want configured workspace %q", got, configuredWorkspace)
	}

	defaultTools := activeToolNames(defaultLaunch)
	overrideTools := activeToolNames(overrideLaunch)
	if len(defaultTools) == 0 {
		t.Fatal("default launch active tools = empty, want built-in tool set")
	}
	if !sameStrings(defaultTools, overrideTools) {
		t.Fatalf("override launch active tools = %v, want same built-in tool set as default %v", overrideTools, defaultTools)
	}

	if err := defaultLaunch.Run(context.Background()); err != nil {
		t.Fatalf("default launch Run() error = %v", err)
	}
	if err := overrideLaunch.Run(context.Background()); err != nil {
		t.Fatalf("override launch Run() error = %v", err)
	}

	if got := len(defaultLaunch.Session().Conversation()); got != 0 {
		t.Fatalf("len(default launch Session().Conversation()) = %d, want 0 after empty-input run", got)
	}
	if got := len(overrideLaunch.Session().Conversation()); got != 0 {
		t.Fatalf("len(override launch Session().Conversation()) = %d, want 0 after empty-input run", got)
	}
}

type bootstrapExpectation struct {
	configPath    string
	workspaceRoot string
	providerName  string
	model         string
}

func assertBootstrapProviderSelection(t *testing.T, application *app.App, want bootstrapExpectation) {
	t.Helper()

	if application == nil {
		t.Fatal("application = nil, want initialized app")
	}

	options := application.Options()
	if options.ConfigPath != want.configPath {
		t.Fatalf("Options().ConfigPath = %q, want %q", options.ConfigPath, want.configPath)
	}
	if options.WorkspaceRoot != want.workspaceRoot {
		t.Fatalf("Options().WorkspaceRoot = %q, want %q", options.WorkspaceRoot, want.workspaceRoot)
	}
	if options.ProviderName != want.providerName {
		t.Fatalf("Options().ProviderName = %q, want %q", options.ProviderName, want.providerName)
	}
	if options.Model != want.model {
		t.Fatalf("Options().Model = %q, want %q", options.Model, want.model)
	}

	profile := application.ProviderProfile()
	if profile.Name != want.providerName {
		t.Fatalf("ProviderProfile().Name = %q, want %q", profile.Name, want.providerName)
	}
	if profile.Model != want.model {
		t.Fatalf("ProviderProfile().Model = %q, want %q", profile.Model, want.model)
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
	if session.WorkspaceRoot != want.workspaceRoot {
		t.Fatalf("Session().WorkspaceRoot = %q, want %q", session.WorkspaceRoot, want.workspaceRoot)
	}
	if session.ProviderName != want.providerName {
		t.Fatalf("Session().ProviderName = %q, want %q", session.ProviderName, want.providerName)
	}
	if session.Model != want.model {
		t.Fatalf("Session().Model = %q, want %q", session.Model, want.model)
	}
	if got := len(session.ApprovedCapabilities); got != 0 {
		t.Fatalf("len(Session().ApprovedCapabilities) = %d, want 0", got)
	}
	if got := len(session.AttachedModules); got != 0 {
		t.Fatalf("len(Session().AttachedModules) = %d, want 0", got)
	}
}

func activeToolNames(application *app.App) []string {
	specs := application.Session().ActiveTools()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}

	return names
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}

	return true
}
