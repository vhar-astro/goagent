package modules

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vhar-astro/goagent/internal/tools"
)

var (
	ErrModuleNameRequired    = errors.New("module name is required")
	ErrModulePathRequired    = errors.New("module path is required")
	ErrModulePathNotAbsolute = errors.New("module path must be absolute")
	ErrModuleAlreadyAttached = errors.New("module is already attached")
	ErrToolNameRequired      = errors.New("module tool name is required")
	ErrToolAlreadyActive     = errors.New("tool name is already active")
)

// Attachment records one validated local module that is active in the session.
type Attachment struct {
	Manifest Manifest
	Path     string
}

// Name returns the manifest name for the attached module.
func (a Attachment) Name() string {
	return strings.TrimSpace(a.Manifest.Name)
}

// ToolSpecs projects manifest tools into the active session tool shape.
func (a Attachment) ToolSpecs() []tools.Spec {
	specs := make([]tools.Spec, 0, len(a.Manifest.Tools))
	source := a.Name()
	for _, toolSpec := range a.Manifest.Tools {
		specs = append(specs, tools.Spec{
			Name:        strings.TrimSpace(toolSpec.Name),
			Description: toolSpec.Description,
			InputSchema: cloneSchemaDeep(toolSpec.InputSchema),
			Source:      source,
			Capability:  strings.TrimSpace(toolSpec.Capability),
		})
	}

	return specs
}

// Registry tracks built-in tools plus user-attached module tool ownership.
type Registry struct {
	baseTools    []tools.Spec
	attachments  []Attachment
	byModule     map[string]Attachment
	byTool       map[string]tools.Spec
	toolOwners   map[string]string
	moduleByTool map[string]string
}

// NewRegistry constructs an attachment registry around the current built-in tool set.
func NewRegistry(baseTools []tools.Spec) (*Registry, error) {
	registry := &Registry{}
	if err := registry.SetBaseTools(baseTools); err != nil {
		return nil, err
	}

	return registry, nil
}

// SetBaseTools replaces the built-in tool baseline after validating for collisions.
func (r *Registry) SetBaseTools(baseTools []tools.Spec) error {
	r.init()

	sanitizedBase := cloneToolSpecs(baseTools)
	activeOwners := make(map[string]string)

	for idx := range sanitizedBase {
		spec := sanitizedBase[idx]
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return ErrToolNameRequired
		}

		spec.Name = name
		spec.Source = defaultBaseSource(spec.Source)
		if owner, exists := activeOwners[name]; exists {
			return fmt.Errorf("%w %q (owned by %q)", ErrToolAlreadyActive, name, owner)
		}

		sanitizedBase[idx] = spec
		activeOwners[name] = spec.Source
	}

	for _, attachment := range r.attachments {
		projected, err := sanitizeAttachment(attachment)
		if err != nil {
			return err
		}
		for _, spec := range projected.ToolSpecs() {
			if owner, exists := activeOwners[spec.Name]; exists {
				return fmt.Errorf("%w %q (owned by %q)", ErrToolAlreadyActive, spec.Name, owner)
			}
			activeOwners[spec.Name] = projected.Name()
		}
	}

	r.baseTools = sanitizedBase
	r.rebuildIndexes()
	return nil
}

// Attach adds one validated module after checking for duplicate module and tool names.
func (r *Registry) Attach(attachment Attachment) error {
	r.init()

	sanitized, err := sanitizeAttachment(attachment)
	if err != nil {
		return err
	}
	if _, exists := r.byModule[sanitized.Name()]; exists {
		return fmt.Errorf("%w %q", ErrModuleAlreadyAttached, sanitized.Name())
	}

	for _, spec := range sanitized.ToolSpecs() {
		if owner, exists := r.toolOwners[spec.Name]; exists {
			return fmt.Errorf("%w %q (owned by %q)", ErrToolAlreadyActive, spec.Name, owner)
		}
	}

	r.attachments = append(r.attachments, sanitized)
	r.byModule[sanitized.Name()] = cloneAttachment(sanitized)
	for _, spec := range sanitized.ToolSpecs() {
		r.byTool[spec.Name] = spec
		r.toolOwners[spec.Name] = sanitized.Name()
		r.moduleByTool[spec.Name] = sanitized.Name()
	}

	return nil
}

// Detach removes one attached module by manifest name.
func (r *Registry) Detach(name string) (Attachment, bool) {
	r.init()

	moduleName := strings.TrimSpace(name)
	attachment, ok := r.byModule[moduleName]
	if !ok {
		return Attachment{}, false
	}

	for idx := range r.attachments {
		if r.attachments[idx].Name() == moduleName {
			r.attachments = append(r.attachments[:idx], r.attachments[idx+1:]...)
			break
		}
	}

	delete(r.byModule, moduleName)
	for _, spec := range attachment.ToolSpecs() {
		delete(r.byTool, spec.Name)
		delete(r.toolOwners, spec.Name)
		delete(r.moduleByTool, spec.Name)
	}
	for _, spec := range r.baseTools {
		r.byTool[spec.Name] = spec
		r.toolOwners[spec.Name] = spec.Source
	}

	return cloneAttachment(attachment), true
}

// HasModule reports whether one module name is attached.
func (r *Registry) HasModule(name string) bool {
	r.init()
	_, ok := r.byModule[strings.TrimSpace(name)]
	return ok
}

// Module looks up one attached module by manifest name.
func (r *Registry) Module(name string) (Attachment, bool) {
	r.init()

	attachment, ok := r.byModule[strings.TrimSpace(name)]
	if !ok {
		return Attachment{}, false
	}

	return cloneAttachment(attachment), true
}

// Modules returns the attached modules in attach order.
func (r *Registry) Modules() []Attachment {
	r.init()
	return cloneAttachments(r.attachments)
}

// HasTool reports whether a tool name is active in the current session surface.
func (r *Registry) HasTool(name string) bool {
	r.init()
	_, ok := r.byTool[strings.TrimSpace(name)]
	return ok
}

// Tool returns one active tool spec by name.
func (r *Registry) Tool(name string) (tools.Spec, bool) {
	r.init()

	spec, ok := r.byTool[strings.TrimSpace(name)]
	if !ok {
		return tools.Spec{}, false
	}

	return cloneToolSpec(spec), true
}

// ToolOwner returns the built-in source or module name that owns a tool name.
func (r *Registry) ToolOwner(name string) (string, bool) {
	r.init()
	owner, ok := r.toolOwners[strings.TrimSpace(name)]
	return owner, ok
}

// ModuleForTool resolves the attached module that owns one active tool name.
func (r *Registry) ModuleForTool(toolName string) (Attachment, bool) {
	r.init()

	moduleName, ok := r.moduleByTool[strings.TrimSpace(toolName)]
	if !ok {
		return Attachment{}, false
	}

	return r.Module(moduleName)
}

// BaseTools returns the built-in tool baseline in registration order.
func (r *Registry) BaseTools() []tools.Spec {
	r.init()
	return cloneToolSpecs(r.baseTools)
}

// ModuleTools returns the attached-module tool surface in attach order.
func (r *Registry) ModuleTools() []tools.Spec {
	r.init()

	specs := make([]tools.Spec, 0)
	for _, attachment := range r.attachments {
		specs = append(specs, attachment.ToolSpecs()...)
	}

	return specs
}

// ActiveTools returns the full session tool surface: built-ins first, then modules.
func (r *Registry) ActiveTools() []tools.Spec {
	r.init()

	specs := make([]tools.Spec, 0, len(r.baseTools))
	specs = append(specs, cloneToolSpecs(r.baseTools)...)
	specs = append(specs, r.ModuleTools()...)
	return specs
}

func (r *Registry) init() {
	if r.byModule == nil {
		r.byModule = make(map[string]Attachment)
	}
	if r.byTool == nil {
		r.byTool = make(map[string]tools.Spec)
	}
	if r.toolOwners == nil {
		r.toolOwners = make(map[string]string)
	}
	if r.moduleByTool == nil {
		r.moduleByTool = make(map[string]string)
	}
}

func (r *Registry) rebuildIndexes() {
	r.byModule = make(map[string]Attachment, len(r.attachments))
	r.byTool = make(map[string]tools.Spec, len(r.baseTools))
	r.toolOwners = make(map[string]string, len(r.baseTools))
	r.moduleByTool = make(map[string]string)

	for _, spec := range r.baseTools {
		r.byTool[spec.Name] = cloneToolSpec(spec)
		r.toolOwners[spec.Name] = spec.Source
	}
	for _, attachment := range r.attachments {
		cloned := cloneAttachment(attachment)
		r.byModule[cloned.Name()] = cloned
		for _, spec := range cloned.ToolSpecs() {
			r.byTool[spec.Name] = spec
			r.toolOwners[spec.Name] = cloned.Name()
			r.moduleByTool[spec.Name] = cloned.Name()
		}
	}
}

func sanitizeAttachment(attachment Attachment) (Attachment, error) {
	name := strings.TrimSpace(attachment.Manifest.Name)
	if name == "" {
		return Attachment{}, ErrModuleNameRequired
	}

	path := strings.TrimSpace(attachment.Path)
	if path == "" {
		return Attachment{}, ErrModulePathRequired
	}
	if !filepath.IsAbs(path) {
		return Attachment{}, fmt.Errorf("%w: %q", ErrModulePathNotAbsolute, path)
	}

	sanitized := cloneAttachment(attachment)
	sanitized.Manifest.Name = name
	sanitized.Path = filepath.Clean(path)

	seenNames := make(map[string]struct{}, len(sanitized.Manifest.Tools))
	for idx := range sanitized.Manifest.Tools {
		toolName := strings.TrimSpace(sanitized.Manifest.Tools[idx].Name)
		if toolName == "" {
			return Attachment{}, ErrToolNameRequired
		}
		if _, exists := seenNames[toolName]; exists {
			return Attachment{}, fmt.Errorf("%w %q (owned by %q)", ErrToolAlreadyActive, toolName, name)
		}

		sanitized.Manifest.Tools[idx].Name = toolName
		sanitized.Manifest.Tools[idx].Capability = strings.TrimSpace(sanitized.Manifest.Tools[idx].Capability)
		seenNames[toolName] = struct{}{}
	}

	return sanitized, nil
}

func defaultBaseSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return tools.SourceBuiltin
	}

	return source
}

func cloneAttachments(attachments []Attachment) []Attachment {
	cloned := make([]Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		cloned = append(cloned, cloneAttachment(attachment))
	}

	return cloned
}

func cloneAttachment(attachment Attachment) Attachment {
	return Attachment{
		Manifest: cloneManifest(attachment.Manifest),
		Path:     attachment.Path,
	}
}

func cloneManifest(manifest Manifest) Manifest {
	cloned := Manifest{
		Name:                  manifest.Name,
		Version:               manifest.Version,
		RequestedCapabilities: append([]string(nil), manifest.RequestedCapabilities...),
		Entrypoint:            manifest.Entrypoint,
		Tools:                 make([]Tool, 0, len(manifest.Tools)),
	}

	for _, toolSpec := range manifest.Tools {
		cloned.Tools = append(cloned.Tools, Tool{
			Name:        toolSpec.Name,
			Description: toolSpec.Description,
			InputSchema: cloneSchemaDeep(toolSpec.InputSchema),
			Capability:  toolSpec.Capability,
		})
	}

	return cloned
}

func cloneToolSpecs(specs []tools.Spec) []tools.Spec {
	cloned := make([]tools.Spec, 0, len(specs))
	for _, spec := range specs {
		cloned = append(cloned, cloneToolSpec(spec))
	}

	return cloned
}

func cloneToolSpec(spec tools.Spec) tools.Spec {
	return tools.Spec{
		Name:        spec.Name,
		Description: spec.Description,
		InputSchema: cloneSchemaDeep(spec.InputSchema),
		Source:      spec.Source,
		Capability:  spec.Capability,
	}
}

func cloneSchemaDeep(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = cloneAny(value)
	}

	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaDeep(typed)
	case []any:
		cloned := make([]any, len(typed))
		for idx, item := range typed {
			cloned[idx] = cloneAny(item)
		}
		return cloned
	default:
		return typed
	}
}
