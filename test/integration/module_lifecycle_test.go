package integration

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vhar-astro/goagent/internal/app"
	"github.com/vhar-astro/goagent/internal/cli"
	"github.com/vhar-astro/goagent/internal/config"
	"github.com/vhar-astro/goagent/internal/modules"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

func TestModuleLifecycleScenario(t *testing.T) {
	t.Run("manifest on disk defines attach prerequisites without mutating the base session", func(t *testing.T) {
		t.Parallel()

		application, workspaceDir := bootstrapIntegrationApp(t)
		moduleDir := writeStubModuleFixture(t, moduleFixtureManifest())
		manifest := readModuleManifest(t, moduleDir)

		assertBaseToolNames(t, application.Session().ActiveTools())
		assertManifestFixture(t, moduleDir, manifest)

		if manifestPath := modules.ManifestPath(moduleDir); manifestPath != filepath.Join(moduleDir, modules.ManifestFilename) {
			t.Fatalf("ManifestPath(%q) = %q, want %q", moduleDir, manifestPath, filepath.Join(moduleDir, modules.ManifestFilename))
		}
		if got, want := manifest.ToolNames(), []string{"stub_echo"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("Manifest.ToolNames() = %v, want %v", got, want)
		}

		sess := application.Session()
		if sess.WorkspaceRoot != workspaceDir {
			t.Fatalf("Session().WorkspaceRoot = %q, want %q", sess.WorkspaceRoot, workspaceDir)
		}
		if sess.IsApproved(tools.CapabilityModule) {
			t.Fatal("Session().IsApproved(module) = true, want false before attach approval")
		}
		if sess.IsApproved(tools.CapabilityWrite) {
			t.Fatal("Session().IsApproved(write) = true, want false before requested capability approval")
		}
		if got := len(sess.AttachedModules); got != 0 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 0 before attach", got)
		}
		if _, ok := sess.Module(manifest.Name); ok {
			t.Fatalf("Session().Module(%q) = found, want absent before attach", manifest.Name)
		}
		assertBaseToolNames(t, sess.ActiveTools())
	})

	t.Run("approved attach adds module tools and detach restores builtin tool set", func(t *testing.T) {
		t.Parallel()

		application, _ := bootstrapIntegrationApp(t)
		moduleDir := writeStubModuleFixture(t, moduleFixtureManifest())
		manifest := readModuleManifest(t, moduleDir)
		sess := application.Session()

		if err := sess.Approve(tools.CapabilityModule, time.Time{}); err != nil {
			t.Fatalf("Session().Approve(module) error = %v", err)
		}
		if err := sess.Approve(tools.CapabilityWrite, time.Time{}); err != nil {
			t.Fatalf("Session().Approve(write) error = %v", err)
		}

		baseTools := sess.ActiveTools()
		moduleProcess := session.ModuleProcess{
			Manifest: manifest,
			Path:     moduleDir,
			PID:      4242,
			Stdin:    io.Discard,
			Stdout:   strings.NewReader(""),
			Status:   session.ModuleStatusReady,
		}

		sess.AttachModule(moduleProcess)

		attached, ok := sess.Module(manifest.Name)
		if !ok {
			t.Fatalf("Session().Module(%q) = missing after attach", manifest.Name)
		}
		if attached.Path != moduleDir {
			t.Fatalf("attached module path = %q, want %q", attached.Path, moduleDir)
		}
		if attached.Status != session.ModuleStatusReady {
			t.Fatalf("attached module status = %q, want %q", attached.Status, session.ModuleStatusReady)
		}
		if got := len(sess.AttachedModules); got != 1 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 1 after attach", got)
		}

		activeTools := sess.ActiveTools()
		if got, want := len(activeTools), len(baseTools)+len(manifest.Tools); got != want {
			t.Fatalf("len(Session().ActiveTools()) = %d, want %d after attach", got, want)
		}
		moduleTool := activeTools[len(activeTools)-1]
		if moduleTool.Name != manifest.Tools[0].Name {
			t.Fatalf("attached tool name = %q, want %q", moduleTool.Name, manifest.Tools[0].Name)
		}
		if moduleTool.Source != manifest.Name {
			t.Fatalf("attached tool source = %q, want %q", moduleTool.Source, manifest.Name)
		}
		if moduleTool.Capability != manifest.Tools[0].Capability {
			t.Fatalf("attached tool capability = %q, want %q", moduleTool.Capability, manifest.Tools[0].Capability)
		}

		if detached := sess.DetachModule(manifest.Name); !detached {
			t.Fatalf("Session().DetachModule(%q) = false, want true", manifest.Name)
		}
		if detached := sess.DetachModule(manifest.Name); detached {
			t.Fatalf("Session().DetachModule(%q) second call = true, want false", manifest.Name)
		}
		if got := len(sess.AttachedModules); got != 0 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 0 after detach", got)
		}
		if _, ok := sess.Module(manifest.Name); ok {
			t.Fatalf("Session().Module(%q) = found after detach, want absent", manifest.Name)
		}
		assertBaseToolNames(t, sess.ActiveTools())
	})

	t.Run("live attach and detach command wiring remains pending", func(t *testing.T) {
		t.Parallel()

		application, _ := bootstrapIntegrationApp(t)
		moduleDir := writeStubModuleFixture(t, moduleFixtureManifest())
		command, err := cli.ParseSlashCommand(`/attach "` + moduleDir + `"`)
		if err != nil {
			t.Fatalf("ParseSlashCommand(/attach) error = %v", err)
		}

		attachCommand, ok := command.(cli.AttachCommand)
		if !ok {
			t.Fatalf("ParseSlashCommand(/attach) returned %T, want cli.AttachCommand", command)
		}
		if attachCommand.Path != moduleDir {
			t.Fatalf("AttachCommand.Path = %q, want %q", attachCommand.Path, moduleDir)
		}
		if got := len(application.Session().AttachedModules); got != 0 {
			t.Fatalf("len(Session().AttachedModules) = %d, want 0 before live /attach wiring", got)
		}

		t.Skip("TODO: T023-T027 must wire manifest loading, process startup, and live /attach /detach command handling")
	})
}

func bootstrapIntegrationApp(t *testing.T) (*app.App, string) {
	t.Helper()

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

	return application, workspaceDir
}

func moduleFixtureManifest() modules.Manifest {
	return modules.Manifest{
		Name:                  "stub-module",
		Version:               "0.1.0",
		RequestedCapabilities: []string{tools.CapabilityWrite},
		Entrypoint:            filepath.Join("bin", "stub-module.sh"),
		Tools: []modules.Tool{
			{
				Name:        "stub_echo",
				Description: "Return a fixed response from the stub module.",
				Capability:  tools.CapabilityRead,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input": map[string]any{
							"type": "string",
						},
					},
					"required":             []string{"input"},
					"additionalProperties": false,
				},
			},
		},
	}
}

func writeStubModuleFixture(t *testing.T, manifest modules.Manifest) string {
	t.Helper()

	moduleDir := filepath.Join(t.TempDir(), manifest.Name)
	if err := os.MkdirAll(filepath.Join(moduleDir, filepath.Dir(manifest.Entrypoint)), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", moduleDir, err)
	}

	entrypointPath := filepath.Join(moduleDir, manifest.Entrypoint)
	entrypoint := "#!/bin/sh\nprintf '{}\\n'\n"
	if err := os.WriteFile(entrypointPath, []byte(entrypoint), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", entrypointPath, err)
	}

	manifestPath := modules.ManifestPath(moduleDir)
	file, err := os.Create(manifestPath)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", manifestPath, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(manifest); err != nil {
		t.Fatalf("Encode(%q) error = %v", manifestPath, err)
	}

	return moduleDir
}

func readModuleManifest(t *testing.T, moduleDir string) modules.Manifest {
	t.Helper()

	file, err := os.Open(modules.ManifestPath(moduleDir))
	if err != nil {
		t.Fatalf("Open(%q) error = %v", modules.ManifestPath(moduleDir), err)
	}
	defer file.Close()

	var manifest modules.Manifest
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("Decode(%q) error = %v", modules.ManifestPath(moduleDir), err)
	}

	return manifest
}

func assertManifestFixture(t *testing.T, moduleDir string, manifest modules.Manifest) {
	t.Helper()

	if manifest.Name == "" {
		t.Fatal("manifest.Name = empty, want declared module name")
	}
	if manifest.Version == "" {
		t.Fatal("manifest.Version = empty, want declared module version")
	}
	if manifest.Entrypoint == "" {
		t.Fatal("manifest.Entrypoint = empty, want declared module entrypoint")
	}
	if got := len(manifest.Tools); got != 1 {
		t.Fatalf("len(manifest.Tools) = %d, want 1", got)
	}
	if got := len(manifest.RequestedCapabilities); got != 1 {
		t.Fatalf("len(manifest.RequestedCapabilities) = %d, want 1", got)
	}
	for _, capability := range manifest.RequestedCapabilities {
		if !tools.IsKnownCapability(capability) {
			t.Fatalf("manifest requested capability %q is not recognized by the host", capability)
		}
	}

	entrypointPath := filepath.Join(moduleDir, manifest.Entrypoint)
	info, err := os.Stat(entrypointPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", entrypointPath, err)
	}
	if info.IsDir() {
		t.Fatalf("module entrypoint %q is a directory, want executable file", entrypointPath)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("module entrypoint %q mode = %v, want executable bit set", entrypointPath, info.Mode())
	}
}

func assertBaseToolNames(t *testing.T, specs []tools.Spec) {
	t.Helper()

	got := make([]string, 0, len(specs))
	for _, spec := range specs {
		got = append(got, spec.Name)
		if spec.Source != tools.SourceBuiltin {
			t.Fatalf("base tool %q source = %q, want %q", spec.Name, spec.Source, tools.SourceBuiltin)
		}
	}

	want := []string{
		tools.ReadFileToolName,
		tools.WriteFileToolName,
		tools.ToolNameRunShell,
		tools.ToolNameFetchURL,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active base tool names = %v, want %v", got, want)
	}
}
