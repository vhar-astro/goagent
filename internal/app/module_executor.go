package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

const (
	ModuleRequestTypeCall    = "call"
	ModuleResponseTypeResult = "result"
	ModuleResponseTypeError  = "error"
)

var (
	ErrModuleProtocolClientRequired = errors.New("module protocol client is required")
	ErrModuleNotAttached            = errors.New("module is not attached")
	ErrModuleNameRequired           = errors.New("module name is required")
	ErrModuleResponseTypeInvalid    = errors.New("module response type is invalid")
	ErrModuleResponseIDMismatch     = errors.New("module response id does not match request")
	ErrModuleResponseMessageEmpty   = errors.New("module error response message is required")
	ErrModuleArgumentsObjectOnly    = errors.New("module tool arguments must be a JSON object")
)

// ModuleToolRequest is the app-owned NDJSON request shape for one module call.
type ModuleToolRequest struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

// ModuleToolResponse is the app-owned NDJSON response shape for one module call.
type ModuleToolResponse struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
}

// ModuleProtocolClient abstracts the module process transport for future
// stdio-backed implementations without forcing that implementation into the
// session executor today.
type ModuleProtocolClient interface {
	Call(context.Context, session.ModuleProcess, ModuleToolRequest) (ModuleToolResponse, error)
	Detach(context.Context, session.ModuleProcess) error
}

// ModuleRegistry resolves module-owned tools and attached modules for the live
// session. The initial implementation adapts the current in-memory session
// model, while leaving room for a richer attached-module registry later.
type ModuleRegistry interface {
	LookupModuleTool(name string) (tools.Spec, bool)
	LookupModule(name string) (session.ModuleProcess, bool)
	DetachModule(name string) bool
}

// ModuleDetachResult reports the session-visible result of one detach request.
type ModuleDetachResult struct {
	ModuleName       string
	RemovedToolNames []string
	Detached         bool
	CleanupError     error
}

// SessionModuleRegistry adapts the current session tool/module state to the
// module-registry contract used by the app executor.
type SessionModuleRegistry struct {
	session *session.Session
}

// NewSessionModuleRegistry creates a session-backed module registry adapter.
func NewSessionModuleRegistry(sess *session.Session) *SessionModuleRegistry {
	return &SessionModuleRegistry{session: sess}
}

// LookupModuleTool returns an active module-owned tool by name.
func (r *SessionModuleRegistry) LookupModuleTool(name string) (tools.Spec, bool) {
	if r == nil || r.session == nil {
		return tools.Spec{}, false
	}

	for _, spec := range r.session.ModuleTools {
		if spec.Name == name {
			spec.Source = defaultToolSource(spec.Source)
			return spec, true
		}
	}

	return tools.Spec{}, false
}

// LookupModule returns one attached module by manifest name.
func (r *SessionModuleRegistry) LookupModule(name string) (session.ModuleProcess, bool) {
	if r == nil || r.session == nil {
		return session.ModuleProcess{}, false
	}

	return r.session.Module(name)
}

// DetachModule removes one attached module from the session state.
func (r *SessionModuleRegistry) DetachModule(name string) bool {
	if r == nil || r.session == nil {
		return false
	}

	return r.session.DetachModule(name)
}

// ExecuteModuleCall routes one provider tool call to an attached module when
// the tool is owned by a module. It reports handled=false for built-in or
// unknown tools so the caller can fall back to the built-in executor path.
func (e *Executor) ExecuteModuleCall(ctx context.Context, client ModuleProtocolClient, call provider.ToolCall) (ToolExecutionResult, bool, error) {
	if e == nil || e.session == nil {
		return ToolExecutionResult{}, true, ErrExecutorSessionRequired
	}

	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		return ToolExecutionResult{}, true, ErrExecutorToolCallIDRequired
	}

	toolName := strings.TrimSpace(call.Name)
	if toolName == "" {
		return ToolExecutionResult{}, true, ErrExecutorToolNameRequired
	}

	registry := NewSessionModuleRegistry(e.session)
	spec, ok := registry.LookupModuleTool(toolName)
	if !ok {
		return ToolExecutionResult{}, false, nil
	}

	if call.Type != "" && call.Type != provider.ToolTypeFunction {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  fmt.Sprintf("%v: %q", ErrExecutorUnsupportedToolType, call.Type),
		}, fmt.Errorf("%w %q", ErrExecutorUnsupportedToolType, call.Type)), true, nil
	}

	module, ok := registry.LookupModule(spec.Source)
	if !ok {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  ErrModuleNotAttached.Error(),
		}, fmt.Errorf("%w %q", ErrModuleNotAttached, spec.Source)), true, nil
	}

	if command, capability, needed, err := e.moduleApprovalCommand(module, spec); err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  err.Error(),
		}, err), true, nil
	} else if needed {
		return e.moduleApprovalRequiredResult(callID, spec, capability, command), true, nil
	}

	if client == nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  ErrModuleProtocolClientRequired.Error(),
		}, ErrModuleProtocolClientRequired), true, nil
	}

	request, err := newModuleToolRequest(call)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  err.Error(),
		}, err), true, nil
	}

	response, err := client.Call(ctx, module, request)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": spec.Source,
			"error":  err.Error(),
		}, err), true, nil
	}

	return e.moduleResponseResult(callID, spec, module, response), true, nil
}

// DetachModule removes one attached module from the active session, using the
// provided protocol client for best-effort transport/process cleanup.
func (e *Executor) DetachModule(ctx context.Context, client ModuleProtocolClient, moduleName string) (ModuleDetachResult, error) {
	if e == nil || e.session == nil {
		return ModuleDetachResult{}, ErrExecutorSessionRequired
	}

	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return ModuleDetachResult{}, ErrModuleNameRequired
	}

	registry := NewSessionModuleRegistry(e.session)
	module, ok := registry.LookupModule(moduleName)
	if !ok {
		return ModuleDetachResult{ModuleName: moduleName}, fmt.Errorf("%w %q", ErrModuleNotAttached, moduleName)
	}

	cleanupErr := detachModuleResources(ctx, client, module)
	toolNames := append([]string(nil), module.Manifest.ToolNames()...)
	detached := registry.DetachModule(moduleName)

	result := ModuleDetachResult{
		ModuleName:       moduleName,
		RemovedToolNames: toolNames,
		Detached:         detached,
		CleanupError:     cleanupErr,
	}
	if !detached {
		if cleanupErr != nil {
			return result, errors.Join(fmt.Errorf("%w %q", ErrModuleNotAttached, moduleName), cleanupErr)
		}
		return result, fmt.Errorf("%w %q", ErrModuleNotAttached, moduleName)
	}
	if cleanupErr != nil {
		return result, cleanupErr
	}

	return result, nil
}

func (e *Executor) moduleApprovalCommand(module session.ModuleProcess, spec tools.Spec) (string, string, bool, error) {
	capabilities := make([]string, 0, len(module.Manifest.RequestedCapabilities)+2)
	capabilities = append(capabilities, tools.CapabilityModule)
	if spec.Capability != "" {
		capabilities = append(capabilities, spec.Capability)
	}
	capabilities = append(capabilities, module.Manifest.RequestedCapabilities...)

	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}

		if !tools.IsKnownCapability(capability) {
			return "", "", false, fmt.Errorf("module %q requests unknown capability %q", module.Name(), capability)
		}
		if command, needed := e.approvalCommand(capability); needed {
			return command, capability, true, nil
		}
	}

	return "", "", false, nil
}

func (e *Executor) moduleApprovalRequiredResult(callID string, spec tools.Spec, capability, command string) ToolExecutionResult {
	return e.buildResult(callID, spec.Name, spec.Source, capability, ToolExecutionApprovalRequired, map[string]string{
		"module":          defaultToolSource(spec.Source),
		"approve_command": command,
	}, formatErrorOutput(fmt.Errorf("approval required for module %q capability %q; run %s and retry the action", defaultToolSource(spec.Source), capability, command)), command)
}

func (e *Executor) moduleResponseResult(callID string, spec tools.Spec, module session.ModuleProcess, response ModuleToolResponse) ToolExecutionResult {
	if responseID := strings.TrimSpace(response.ID); responseID != "" && responseID != callID {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module":      module.Name(),
			"response_id": responseID,
			"error":       ErrModuleResponseIDMismatch.Error(),
		}, fmt.Errorf("%w: call=%q response=%q", ErrModuleResponseIDMismatch, callID, responseID))
	}

	switch strings.TrimSpace(response.Type) {
	case ModuleResponseTypeResult:
		return e.successResult(callID, spec, map[string]string{
			"module": module.Name(),
		}, formatModuleResultOutput(module.Name(), spec.Name, response.Content))
	case ModuleResponseTypeError:
		message := strings.TrimSpace(response.Message)
		if message == "" {
			return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
				"module": module.Name(),
				"error":  ErrModuleResponseMessageEmpty.Error(),
			}, ErrModuleResponseMessageEmpty)
		}
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module": module.Name(),
			"error":  message,
		}, errors.New(message), formatModuleErrorOutput(module.Name(), spec.Name, message))
	default:
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"module":        module.Name(),
			"response_type": response.Type,
			"error":         ErrModuleResponseTypeInvalid.Error(),
		}, fmt.Errorf("%w %q", ErrModuleResponseTypeInvalid, response.Type))
	}
}

func newModuleToolRequest(call provider.ToolCall) (ModuleToolRequest, error) {
	arguments, err := moduleArgumentsJSON(call.ArgumentsJSON)
	if err != nil {
		return ModuleToolRequest{}, err
	}

	return ModuleToolRequest{
		Type:      ModuleRequestTypeCall,
		ID:        strings.TrimSpace(call.ID),
		Tool:      strings.TrimSpace(call.Name),
		Arguments: arguments,
	}, nil
}

func moduleArgumentsJSON(raw string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return json.RawMessage(`{}`), nil
	}

	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("decode module tool arguments: %w", err)
	}
	if _, ok := payload.(map[string]any); !ok {
		return nil, ErrModuleArgumentsObjectOnly
	}

	return json.RawMessage(trimmed), nil
}

func detachModuleResources(ctx context.Context, client ModuleProtocolClient, module session.ModuleProcess) error {
	if client != nil {
		if err := client.Detach(ctx, module); err != nil {
			return err
		}
	}

	return closeModuleIO(module)
}

func closeModuleIO(module session.ModuleProcess) error {
	var errs []error

	if closer, ok := module.Stdin.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close stdin for module %q: %w", module.Name(), err))
		}
	}
	if closer, ok := module.Stdout.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close stdout for module %q: %w", module.Name(), err))
		}
	}

	return errors.Join(errs...)
}

func formatModuleResultOutput(moduleName, toolName, content string) string {
	var builder strings.Builder

	builder.WriteString("module: ")
	builder.WriteString(moduleName)
	builder.WriteString("\ntool: ")
	builder.WriteString(toolName)
	builder.WriteString("\ncontent:\n")
	builder.WriteString(content)

	return builder.String()
}

func formatModuleErrorOutput(moduleName, toolName, message string) string {
	return fmt.Sprintf("error: module %q tool %q failed: %s", moduleName, toolName, message)
}
