package tools

import (
	"strings"
	"testing"
)

func TestRegisterBuiltinTracksIndexes(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	spec := Spec{
		Name:        "read-file",
		Description: "Read one file from the workspace",
		InputSchema: map[string]any{
			"type": "object",
		},
		Capability: CapabilityRead,
	}

	if err := registry.RegisterBuiltin(spec); err != nil {
		t.Fatalf("RegisterBuiltin() error = %v", err)
	}

	registered, ok := registry.Get("read-file")
	if !ok {
		t.Fatal("Get(read-file) = not found, want registered spec")
	}
	if registered.Source != SourceBuiltin {
		t.Fatalf("registered.Source = %q, want %q", registered.Source, SourceBuiltin)
	}
	if !registry.Has("read-file") {
		t.Fatal("Has(read-file) = false, want true")
	}
	if !registry.HasCapability(CapabilityRead) {
		t.Fatalf("HasCapability(%q) = false, want true", CapabilityRead)
	}
	if got := registry.Builtins(); len(got) != 1 || got[0].Name != "read-file" {
		t.Fatalf("Builtins() = %#v, want one read-file spec", got)
	}
	if got := registry.SpecsForCapability(CapabilityRead); len(got) != 1 || got[0].Name != "read-file" {
		t.Fatalf("SpecsForCapability(%q) = %#v, want one read-file spec", CapabilityRead, got)
	}
	if owner, ok := registry.SchemaOwner(spec.InputSchema); !ok || owner != "read-file" {
		t.Fatalf("SchemaOwner() = (%q, %t), want (%q, true)", owner, ok, "read-file")
	}
}

func TestRegisterRejectsUnknownCapabilityAndDuplicateSchema(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	first := Spec{
		Name:        "write-file",
		Description: "Write one file into the workspace",
		InputSchema: map[string]any{
			"type": "object",
		},
		Capability: CapabilityWrite,
	}
	if err := registry.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}

	if err := registry.Register(Spec{
		Name:        "invalid-tool",
		Description: "Invalid capability",
		Capability:  "unknown",
	}); err == nil || !strings.Contains(err.Error(), `unknown capability "unknown"`) {
		t.Fatalf("Register(invalid capability) error = %v, want unknown capability error", err)
	}

	err := registry.Register(Spec{
		Name:        "write-file-copy",
		Description: "Duplicate schema",
		InputSchema: map[string]any{
			"type": "object",
		},
		Capability: CapabilityWrite,
	})
	if err == nil {
		t.Fatal("Register(duplicate schema) error = nil, want duplicate schema error")
	}
	if !strings.Contains(err.Error(), `duplicates input schema already used by "write-file"`) {
		t.Fatalf("Register(duplicate schema) error = %q, want duplicate schema message", err)
	}
}

func TestRemoveAndRemoveSourceUpdateRegistryState(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	specs := []Spec{
		{
			Name:        "read-file",
			Description: "Read files",
			InputSchema: map[string]any{"type": "object", "title": "read"},
			Capability:  CapabilityRead,
			Source:      SourceBuiltin,
		},
		{
			Name:        "module-run",
			Description: "Run module task",
			InputSchema: map[string]any{"type": "object", "title": "module-run"},
			Capability:  CapabilityModule,
			Source:      "module-a",
		},
		{
			Name:        "module-read",
			Description: "Read from module",
			InputSchema: map[string]any{"type": "object", "title": "module-read"},
			Capability:  CapabilityRead,
			Source:      "module-a",
		},
	}
	if err := registry.RegisterMany(specs); err != nil {
		t.Fatalf("RegisterMany() error = %v", err)
	}

	removed, ok := registry.Remove("module-run")
	if !ok {
		t.Fatal("Remove(module-run) = not found, want removed spec")
	}
	if removed.Name != "module-run" {
		t.Fatalf("Remove(module-run) removed %q, want %q", removed.Name, "module-run")
	}
	if registry.Has("module-run") {
		t.Fatal("Has(module-run) = true after removal, want false")
	}
	if owner, ok := registry.SchemaOwner(specs[1].InputSchema); ok || owner != "" {
		t.Fatalf("SchemaOwner(removed schema) = (%q, %t), want (\"\", false)", owner, ok)
	}

	removedSource := registry.RemoveSource("module-a")
	if len(removedSource) != 1 || removedSource[0].Name != "module-read" {
		t.Fatalf("RemoveSource(module-a) = %#v, want one module-read spec", removedSource)
	}
	if got := registry.SpecsForSource("module-a"); len(got) != 0 {
		t.Fatalf("SpecsForSource(module-a) = %#v, want empty", got)
	}
	if got := registry.Specs(); len(got) != 1 || got[0].Name != "read-file" {
		t.Fatalf("Specs() = %#v, want only read-file remaining", got)
	}
}
