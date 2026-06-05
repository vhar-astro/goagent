package tools

import (
	"encoding/json"
	"fmt"
)

const (
	CapabilityRead   = "read"
	CapabilityWrite  = "write"
	CapabilityShell  = "shell"
	CapabilityWeb    = "web"
	CapabilityModule = "module"

	SourceBuiltin = "builtin"
)

var knownCapabilities = map[string]struct{}{
	CapabilityRead:   {},
	CapabilityWrite:  {},
	CapabilityShell:  {},
	CapabilityWeb:    {},
	CapabilityModule: {},
}

// Spec describes one built-in or attached tool exposed to the provider.
type Spec struct {
	Name        string
	Description string
	InputSchema map[string]any
	Source      string
	Capability  string
}

// Registry keeps tool names unique across the active session.
type Registry struct {
	order        []Spec
	byName       map[string]Spec
	bySource     map[string][]string
	byCapability map[string][]string
	bySchema     map[string]string
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	r := &Registry{}
	r.init()
	return r
}

// IsKnownCapability reports whether a capability is part of the base buckets.
func IsKnownCapability(name string) bool {
	_, ok := knownCapabilities[name]
	return ok
}

// RegisterBuiltin adds a built-in tool and forces the built-in source label.
func (r *Registry) RegisterBuiltin(spec Spec) error {
	spec.Source = SourceBuiltin
	return r.Register(spec)
}

// RegisterBuiltins adds a batch of built-in tool specs in order.
func (r *Registry) RegisterBuiltins(specs []Spec) error {
	for _, spec := range specs {
		if err := r.RegisterBuiltin(spec); err != nil {
			return err
		}
	}

	return nil
}

// Register adds one tool after validating its name and capability bucket.
func (r *Registry) Register(spec Spec) error {
	r.init()

	if spec.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if spec.Capability != "" && !IsKnownCapability(spec.Capability) {
		return fmt.Errorf("unknown capability %q", spec.Capability)
	}
	if _, exists := r.byName[spec.Name]; exists {
		return fmt.Errorf("tool %q already registered", spec.Name)
	}
	if spec.Source == "" {
		spec.Source = SourceBuiltin
	}
	fingerprint, err := schemaFingerprint(spec.InputSchema)
	if err != nil {
		return fmt.Errorf("tool %q has invalid input schema: %w", spec.Name, err)
	}
	if fingerprint != "" {
		if existing, exists := r.bySchema[fingerprint]; exists {
			return fmt.Errorf("tool %q duplicates input schema already used by %q", spec.Name, existing)
		}
	}

	r.order = append(r.order, spec)
	r.byName[spec.Name] = spec
	r.bySource[spec.Source] = append(r.bySource[spec.Source], spec.Name)
	if spec.Capability != "" {
		r.byCapability[spec.Capability] = append(r.byCapability[spec.Capability], spec.Name)
	}
	if fingerprint != "" {
		r.bySchema[fingerprint] = spec.Name
	}

	return nil
}

// RegisterMany adds a batch of tool specs in order.
func (r *Registry) RegisterMany(specs []Spec) error {
	for _, spec := range specs {
		if err := r.Register(spec); err != nil {
			return err
		}
	}

	return nil
}

// Has reports whether a tool name is already active.
func (r *Registry) Has(name string) bool {
	if r.byName == nil {
		return false
	}

	_, ok := r.byName[name]
	return ok
}

// Get returns one active tool spec by name.
func (r *Registry) Get(name string) (Spec, bool) {
	if r.byName == nil {
		return Spec{}, false
	}

	spec, ok := r.byName[name]
	return spec, ok
}

// SpecsForCapability returns the active tools for one capability bucket.
func (r *Registry) SpecsForCapability(capability string) []Spec {
	return r.specsForNames(r.byCapability[capability])
}

// SpecsForSource returns the active tools registered from one source.
func (r *Registry) SpecsForSource(source string) []Spec {
	return r.specsForNames(r.bySource[source])
}

// Builtins returns the currently registered built-in tools.
func (r *Registry) Builtins() []Spec {
	return r.SpecsForSource(SourceBuiltin)
}

// HasCapability reports whether any tool is registered for one capability bucket.
func (r *Registry) HasCapability(capability string) bool {
	return len(r.byCapability[capability]) > 0
}

// SchemaOwner returns the registered tool that already uses the supplied schema.
func (r *Registry) SchemaOwner(schema map[string]any) (string, bool) {
	if r.bySchema == nil {
		return "", false
	}

	fingerprint, err := schemaFingerprint(schema)
	if err != nil || fingerprint == "" {
		return "", false
	}

	name, ok := r.bySchema[fingerprint]
	return name, ok
}

// Remove deletes one tool by name and returns the removed spec.
func (r *Registry) Remove(name string) (Spec, bool) {
	spec, ok := r.Get(name)
	if !ok {
		return Spec{}, false
	}

	r.deleteSpec(spec)
	return spec, true
}

// RemoveSource deletes every tool registered from one source in registration order.
func (r *Registry) RemoveSource(source string) []Spec {
	specs := r.SpecsForSource(source)
	for _, spec := range specs {
		r.deleteSpec(spec)
	}

	return specs
}

// Specs returns the active tool specs in registration order.
func (r *Registry) Specs() []Spec {
	return append([]Spec(nil), r.order...)
}

func (r *Registry) init() {
	if r.byName == nil {
		r.byName = make(map[string]Spec)
	}
	if r.bySource == nil {
		r.bySource = make(map[string][]string)
	}
	if r.byCapability == nil {
		r.byCapability = make(map[string][]string)
	}
	if r.bySchema == nil {
		r.bySchema = make(map[string]string)
	}
}

func (r *Registry) specsForNames(names []string) []Spec {
	if len(names) == 0 || r.byName == nil {
		return nil
	}

	specs := make([]Spec, 0, len(names))
	for _, name := range names {
		spec, ok := r.byName[name]
		if ok {
			specs = append(specs, spec)
		}
	}

	return specs
}

func (r *Registry) deleteSpec(spec Spec) {
	delete(r.byName, spec.Name)
	r.order = filterSpecs(r.order, spec.Name)
	r.bySource[spec.Source] = filterNames(r.bySource[spec.Source], spec.Name)
	if len(r.bySource[spec.Source]) == 0 {
		delete(r.bySource, spec.Source)
	}
	if spec.Capability != "" {
		r.byCapability[spec.Capability] = filterNames(r.byCapability[spec.Capability], spec.Name)
		if len(r.byCapability[spec.Capability]) == 0 {
			delete(r.byCapability, spec.Capability)
		}
	}
	if fingerprint, err := schemaFingerprint(spec.InputSchema); err == nil && fingerprint != "" {
		delete(r.bySchema, fingerprint)
	}
}

func filterSpecs(specs []Spec, name string) []Spec {
	if len(specs) == 0 {
		return nil
	}

	filtered := specs[:0]
	for _, spec := range specs {
		if spec.Name != name {
			filtered = append(filtered, spec)
		}
	}

	return filtered
}

func filterNames(names []string, target string) []string {
	if len(names) == 0 {
		return nil
	}

	filtered := names[:0]
	for _, name := range names {
		if name != target {
			filtered = append(filtered, name)
		}
	}

	return filtered
}

func schemaFingerprint(schema map[string]any) (string, error) {
	if len(schema) == 0 {
		return "", nil
	}

	encoded, err := json.Marshal(schema)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}
