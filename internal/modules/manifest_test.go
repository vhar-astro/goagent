package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestParsingAndValidationAcceptsValidManifest(t *testing.T) {
	t.Parallel()

	moduleDir := t.TempDir()
	manifest, err := parseManifestContract(moduleDir, mustMarshalManifest(t, map[string]any{
		"name":                   "example-module",
		"version":                "1.0.0",
		"requested_capabilities": []string{"write", "shell"},
		"entrypoint":             "./bin/module",
		"tools": []map[string]any{
			{
				"name":        "tool_name",
				"description": "reads generated notes",
				"input_schema": map[string]any{
					"type": "object",
				},
				"capability": "module",
			},
			{
				"name":        "tool-name",
				"description": "runs module work",
				"input_schema": map[string]any{
					"type": "object",
				},
				"capability": "module",
			},
		},
	}))
	if err != nil {
		t.Fatalf("parseManifestContract() error = %v", err)
	}

	if manifest.Name != "example-module" {
		t.Fatalf("Manifest.Name = %q, want %q", manifest.Name, "example-module")
	}
	if manifest.Version != "1.0.0" {
		t.Fatalf("Manifest.Version = %q, want %q", manifest.Version, "1.0.0")
	}
	if manifest.Entrypoint != "./bin/module" {
		t.Fatalf("Manifest.Entrypoint = %q, want %q", manifest.Entrypoint, "./bin/module")
	}

	wantNames := []string{"tool_name", "tool-name"}
	gotNames := manifest.ToolNames()
	if len(gotNames) != len(wantNames) {
		t.Fatalf("len(Manifest.ToolNames()) = %d, want %d", len(gotNames), len(wantNames))
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Fatalf("Manifest.ToolNames()[%d] = %q, want %q", i, gotNames[i], wantNames[i])
		}
	}

	if got := ManifestPath(moduleDir); got != filepath.Join(moduleDir, ManifestFilename) {
		t.Fatalf("ManifestPath(%q) = %q, want %q", moduleDir, got, filepath.Join(moduleDir, ManifestFilename))
	}
}

func TestManifestValidationRequiresTopLevelFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		removeKey string
		wantErr   string
	}{
		{name: "missing name", removeKey: "name", wantErr: "name is required"},
		{name: "missing version", removeKey: "version", wantErr: "version is required"},
		{name: "missing requested capabilities", removeKey: "requested_capabilities", wantErr: "requested_capabilities is required"},
		{name: "missing entrypoint", removeKey: "entrypoint", wantErr: "entrypoint is required"},
		{name: "missing tools", removeKey: "tools", wantErr: "tools is required"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := validManifestPayload()
			delete(payload, tt.removeKey)

			_, err := parseManifestContract(t.TempDir(), mustMarshalManifest(t, payload))
			if err == nil {
				t.Fatalf("parseManifestContract() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseManifestContract() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestManifestValidationRejectsEntrypointOutsideModuleDirectory(t *testing.T) {
	t.Parallel()

	moduleDir := t.TempDir()
	outsideDir := t.TempDir()

	tests := []struct {
		name       string
		entrypoint string
	}{
		{name: "relative escape", entrypoint: filepath.Join("..", "escape", "module")},
		{name: "absolute outside path", entrypoint: filepath.Join(outsideDir, "module")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := validManifestPayload()
			payload["entrypoint"] = tt.entrypoint

			_, err := parseManifestContract(moduleDir, mustMarshalManifest(t, payload))
			if err == nil {
				t.Fatal("parseManifestContract() error = nil, want outside-module validation error")
			}
			if !strings.Contains(err.Error(), "entrypoint must stay inside module directory") {
				t.Fatalf("parseManifestContract() error = %q, want entrypoint containment error", err)
			}
		})
	}
}

func TestManifestValidationRejectsUnknownRequestedCapabilities(t *testing.T) {
	t.Parallel()

	payload := validManifestPayload()
	payload["requested_capabilities"] = []string{"write", "unknown"}

	_, err := parseManifestContract(t.TempDir(), mustMarshalManifest(t, payload))
	if err == nil {
		t.Fatal("parseManifestContract() error = nil, want unknown capability error")
	}
	if !strings.Contains(err.Error(), `unknown requested capability "unknown"`) {
		t.Fatalf("parseManifestContract() error = %q, want unknown capability error", err)
	}
}

func TestManifestValidationRejectsInvalidToolNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		wantErr  string
	}{
		{name: "blank tool name", toolName: "", wantErr: "tool name is required"},
		{name: "whitespace only tool name", toolName: "   ", wantErr: "tool name is required"},
		{name: "tool name with spaces", toolName: "bad tool", wantErr: `tool name "bad tool" must not contain whitespace`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := validManifestPayload()
			payload["tools"] = []map[string]any{
				{
					"name":        tt.toolName,
					"description": "invalid tool",
					"input_schema": map[string]any{
						"type": "object",
					},
					"capability": "module",
				},
			}

			_, err := parseManifestContract(t.TempDir(), mustMarshalManifest(t, payload))
			if err == nil {
				t.Fatalf("parseManifestContract() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseManifestContract() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func parseManifestContract(moduleDir string, data []byte) (Manifest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest envelope: %w", err)
	}

	for _, field := range []string{"name", "version", "requested_capabilities", "entrypoint", "tools"} {
		if _, ok := raw[field]; !ok {
			return Manifest{}, fmt.Errorf("%s is required", field)
		}
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	if strings.TrimSpace(manifest.Name) == "" {
		return Manifest{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return Manifest{}, fmt.Errorf("version is required")
	}
	if strings.TrimSpace(manifest.Entrypoint) == "" {
		return Manifest{}, fmt.Errorf("entrypoint is required")
	}
	if err := validateEntrypoint(moduleDir, manifest.Entrypoint); err != nil {
		return Manifest{}, err
	}
	for _, capability := range manifest.RequestedCapabilities {
		if _, ok := knownManifestCapabilities[capability]; !ok {
			return Manifest{}, fmt.Errorf("unknown requested capability %q", capability)
		}
	}
	for _, tool := range manifest.Tools {
		if err := validateToolName(tool.Name); err != nil {
			return Manifest{}, err
		}
	}

	return manifest, nil
}

func validateEntrypoint(moduleDir, entrypoint string) error {
	moduleAbs, err := filepath.Abs(moduleDir)
	if err != nil {
		return fmt.Errorf("resolve module directory: %w", err)
	}

	target := entrypoint
	if !filepath.IsAbs(target) {
		target = filepath.Join(moduleAbs, target)
	}

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve entrypoint: %w", err)
	}

	rel, err := filepath.Rel(moduleAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("compare entrypoint to module directory: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("entrypoint must stay inside module directory")
	}

	return nil
}

func validateToolName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("tool name is required")
	}
	if strings.IndexFunc(trimmed, unicodeIsSpace) >= 0 {
		return fmt.Errorf("tool name %q must not contain whitespace", name)
	}

	return nil
}

func validManifestPayload() map[string]any {
	return map[string]any{
		"name":                   "example-module",
		"version":                "1.0.0",
		"requested_capabilities": []string{"write", "shell"},
		"entrypoint":             "./bin/module",
		"tools": []map[string]any{
			{
				"name":        "tool_name",
				"description": "runs module work",
				"input_schema": map[string]any{
					"type": "object",
				},
				"capability": "module",
			},
		},
	}
}

func mustMarshalManifest(t *testing.T, payload map[string]any) []byte {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	return data
}

var knownManifestCapabilities = map[string]struct{}{
	"read":   {},
	"write":  {},
	"shell":  {},
	"web":    {},
	"module": {},
}

func unicodeIsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}
