package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vhar-astro/goagent/internal/tools"
)

const ManifestFilename = "module.json"

// Tool describes one tool exported by a local module manifest.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Capability  string         `json:"capability"`
}

// Manifest is the minimal module declaration accepted by the agent.
type Manifest struct {
	Name                  string   `json:"name"`
	Version               string   `json:"version"`
	RequestedCapabilities []string `json:"requested_capabilities"`
	Entrypoint            string   `json:"entrypoint"`
	Tools                 []Tool   `json:"tools"`
}

// ManifestPath returns the expected manifest location for a module directory.
func ManifestPath(dir string) string {
	return filepath.Join(dir, ManifestFilename)
}

// Load reads, validates, and normalizes one module manifest from disk.
func Load(dir string) (Manifest, error) {
	moduleDir, err := filepath.Abs(dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve module directory: %w", err)
	}

	data, err := os.ReadFile(ManifestPath(moduleDir))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	return parse(moduleDir, data)
}

// ToolNames returns the exported tool names in declaration order.
func (m Manifest) ToolNames() []string {
	names := make([]string, 0, len(m.Tools))
	for _, tool := range m.Tools {
		names = append(names, tool.Name)
	}

	return names
}

func parse(moduleDir string, data []byte) (Manifest, error) {
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

	if err := normalizeManifest(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := validateManifestEntrypoint(moduleDir, manifest.Entrypoint); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func normalizeManifest(manifest *Manifest) error {
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return fmt.Errorf("name is required")
	}

	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		return fmt.Errorf("version is required")
	}

	manifest.Entrypoint = strings.TrimSpace(manifest.Entrypoint)
	if manifest.Entrypoint == "" {
		return fmt.Errorf("entrypoint is required")
	}

	capabilities := make([]string, 0, len(manifest.RequestedCapabilities))
	seenCapabilities := make(map[string]struct{}, len(manifest.RequestedCapabilities))
	for _, capability := range manifest.RequestedCapabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return fmt.Errorf("requested capability is required")
		}
		if !tools.IsKnownCapability(capability) {
			return fmt.Errorf("unknown requested capability %q", capability)
		}
		if _, exists := seenCapabilities[capability]; exists {
			continue
		}
		seenCapabilities[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	manifest.RequestedCapabilities = capabilities

	normalizedTools := make([]Tool, 0, len(manifest.Tools))
	for _, toolSpec := range manifest.Tools {
		toolSpec.Name = strings.TrimSpace(toolSpec.Name)
		if toolSpec.Name == "" {
			return fmt.Errorf("tool name is required")
		}
		if strings.IndexFunc(toolSpec.Name, isManifestSpace) >= 0 {
			return fmt.Errorf("tool name %q must not contain whitespace", toolSpec.Name)
		}

		toolSpec.Description = strings.TrimSpace(toolSpec.Description)
		toolSpec.Capability = strings.TrimSpace(toolSpec.Capability)
		if toolSpec.Capability != "" && !tools.IsKnownCapability(toolSpec.Capability) {
			return fmt.Errorf("tool %q has unknown capability %q", toolSpec.Name, toolSpec.Capability)
		}

		normalizedTools = append(normalizedTools, Tool{
			Name:        toolSpec.Name,
			Description: toolSpec.Description,
			InputSchema: cloneSchema(toolSpec.InputSchema),
			Capability:  toolSpec.Capability,
		})
	}
	manifest.Tools = normalizedTools

	return nil
}

func validateManifestEntrypoint(moduleDir, entrypoint string) error {
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

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = value
	}

	return cloned
}

func isManifestSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}
