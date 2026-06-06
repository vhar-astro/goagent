package session

import (
	"fmt"
	"io"
	"time"

	"github.com/vhar-astro/goagent/internal/modules"
	"github.com/vhar-astro/goagent/internal/tools"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ModuleStatus describes the lifecycle state of one attached local module.
type ModuleStatus string

const (
	ModuleStatusStarting ModuleStatus = "starting"
	ModuleStatusReady    ModuleStatus = "ready"
	ModuleStatusFailed   ModuleStatus = "failed"
	ModuleStatusStopped  ModuleStatus = "stopped"
)

var supportedRoles = map[string]struct{}{
	RoleSystem:    {},
	RoleUser:      {},
	RoleAssistant: {},
	RoleTool:      {},
}

// Message is the session-owned conversation record.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolName   string
}

// Validate checks the runtime constraints for one conversation message.
func (m Message) Validate() error {
	if _, ok := supportedRoles[m.Role]; !ok {
		return fmt.Errorf("unsupported message role %q", m.Role)
	}
	if m.Role == RoleTool && m.ToolCallID == "" {
		return fmt.Errorf("tool messages require a tool call id")
	}

	return nil
}

// CapabilityApproval tracks one risky capability for the current launch.
type CapabilityApproval struct {
	Name       string
	Approved   bool
	ApprovedAt time.Time
}

// ModuleProcess stores the session-visible state for one attached local module.
type ModuleProcess struct {
	Manifest modules.Manifest
	Path     string
	PID      int
	Stdin    io.Writer
	Stdout   io.Reader
	Status   ModuleStatus
}

// Name returns the attached module identifier from its manifest.
func (m ModuleProcess) Name() string {
	return m.Manifest.Name
}

// ToolSpecs converts manifest-declared tools into the session tool shape.
func (m ModuleProcess) ToolSpecs() []tools.Spec {
	specs := make([]tools.Spec, 0, len(m.Manifest.Tools))
	for _, toolSpec := range m.Manifest.Tools {
		specs = append(specs, tools.Spec{
			Name:        toolSpec.Name,
			Description: toolSpec.Description,
			InputSchema: cloneSchema(toolSpec.InputSchema),
			Source:      m.Manifest.Name,
			Capability:  toolSpec.Capability,
		})
	}

	return specs
}

// Session stores the minimal runtime state for one CLI launch.
type Session struct {
	ID                   string
	WorkspaceRoot        string
	ProviderName         string
	Model                string
	ApprovedCapabilities map[string]CapabilityApproval
	Messages             []Message
	BuiltInTools         []tools.Spec
	ModuleTools          []tools.Spec
	AttachedModules      []ModuleProcess
}

// New constructs a fresh in-memory session with empty approvals and history.
func New(id, workspaceRoot, providerName, model string) *Session {
	return &Session{
		ID:                   id,
		WorkspaceRoot:        workspaceRoot,
		ProviderName:         providerName,
		Model:                model,
		ApprovedCapabilities: make(map[string]CapabilityApproval),
		Messages:             make([]Message, 0),
		BuiltInTools:         make([]tools.Spec, 0),
		ModuleTools:          make([]tools.Spec, 0),
		AttachedModules:      make([]ModuleProcess, 0),
	}
}

// AppendMessage validates and records one message in the session transcript.
func (s *Session) AppendMessage(message Message) error {
	if err := message.Validate(); err != nil {
		return err
	}

	s.Messages = append(s.Messages, message)
	return nil
}

// AppendMessages validates and appends messages in order.
func (s *Session) AppendMessages(messages ...Message) error {
	for _, message := range messages {
		if err := s.AppendMessage(message); err != nil {
			return err
		}
	}

	return nil
}

// Conversation returns a defensive copy of the active transcript.
func (s *Session) Conversation() []Message {
	return append([]Message(nil), s.Messages...)
}

// SetBuiltInTools replaces the built-in tool inventory with a defensive copy.
func (s *Session) SetBuiltInTools(specs []tools.Spec) {
	s.BuiltInTools = cloneToolSpecs(specs)
}

// SetModuleTools replaces the active module tool inventory with a defensive copy.
func (s *Session) SetModuleTools(specs []tools.Spec) {
	s.ModuleTools = cloneToolSpecs(specs)
}

// ActiveTools returns the built-in and module tool sets in session order.
func (s *Session) ActiveTools() []tools.Spec {
	active := make([]tools.Spec, 0, len(s.BuiltInTools)+len(s.ModuleTools))
	active = append(active, cloneToolSpecs(s.BuiltInTools)...)
	active = append(active, cloneToolSpecs(s.ModuleTools)...)
	return active
}

// SetAttachedModules replaces the attached module inventory and rebuilds module tools.
func (s *Session) SetAttachedModules(modules []ModuleProcess) {
	s.AttachedModules = cloneModules(modules)
	s.rebuildModuleTools()
}

// AttachModule inserts or replaces one attached module by manifest name.
func (s *Session) AttachModule(module ModuleProcess) {
	replaced := false
	for idx := range s.AttachedModules {
		if s.AttachedModules[idx].Manifest.Name == module.Manifest.Name {
			s.AttachedModules[idx] = cloneModule(module)
			replaced = true
			break
		}
	}
	if !replaced {
		s.AttachedModules = append(s.AttachedModules, cloneModule(module))
	}
	s.rebuildModuleTools()
}

// DetachModule removes one attached module by manifest name.
func (s *Session) DetachModule(name string) bool {
	for idx := range s.AttachedModules {
		if s.AttachedModules[idx].Manifest.Name == name {
			s.AttachedModules = append(s.AttachedModules[:idx], s.AttachedModules[idx+1:]...)
			s.rebuildModuleTools()
			return true
		}
	}

	return false
}

// Module returns one attached module by manifest name.
func (s *Session) Module(name string) (ModuleProcess, bool) {
	for _, module := range s.AttachedModules {
		if module.Manifest.Name == name {
			return cloneModule(module), true
		}
	}

	return ModuleProcess{}, false
}

// Approve marks a capability as approved for the current session.
func (s *Session) Approve(name string, approvedAt time.Time) error {
	if !tools.IsKnownCapability(name) {
		return fmt.Errorf("unknown capability %q", name)
	}
	if approvedAt.IsZero() {
		approvedAt = time.Now().UTC()
	}

	s.ApprovedCapabilities[name] = CapabilityApproval{
		Name:       name,
		Approved:   true,
		ApprovedAt: approvedAt,
	}

	return nil
}

// Revoke removes a previously granted capability approval.
func (s *Session) Revoke(name string) {
	delete(s.ApprovedCapabilities, name)
}

// Approval returns one session approval record if it exists.
func (s *Session) Approval(name string) (CapabilityApproval, bool) {
	approval, ok := s.ApprovedCapabilities[name]
	return approval, ok
}

// IsApproved reports whether a capability has already been granted.
func (s *Session) IsApproved(name string) bool {
	approval, ok := s.ApprovedCapabilities[name]
	return ok && approval.Approved
}

func (s *Session) rebuildModuleTools() {
	s.ModuleTools = s.ModuleTools[:0]
	for _, module := range s.AttachedModules {
		s.ModuleTools = append(s.ModuleTools, module.ToolSpecs()...)
	}
}

func cloneModules(modules []ModuleProcess) []ModuleProcess {
	cloned := make([]ModuleProcess, 0, len(modules))
	for _, module := range modules {
		cloned = append(cloned, cloneModule(module))
	}

	return cloned
}

func cloneModule(module ModuleProcess) ModuleProcess {
	module.Manifest = cloneManifest(module.Manifest)
	return module
}

func cloneManifest(manifest modules.Manifest) modules.Manifest {
	cloned := modules.Manifest{
		Name:                  manifest.Name,
		Version:               manifest.Version,
		RequestedCapabilities: append([]string(nil), manifest.RequestedCapabilities...),
		Entrypoint:            manifest.Entrypoint,
		Tools:                 make([]modules.Tool, 0, len(manifest.Tools)),
	}

	for _, toolSpec := range manifest.Tools {
		cloned.Tools = append(cloned.Tools, modules.Tool{
			Name:        toolSpec.Name,
			Description: toolSpec.Description,
			InputSchema: cloneSchema(toolSpec.InputSchema),
			Capability:  toolSpec.Capability,
		})
	}

	return cloned
}

func cloneToolSpecs(specs []tools.Spec) []tools.Spec {
	cloned := make([]tools.Spec, 0, len(specs))
	for _, spec := range specs {
		cloned = append(cloned, tools.Spec{
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: cloneSchema(spec.InputSchema),
			Source:      spec.Source,
			Capability:  spec.Capability,
		})
	}

	return cloned
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
