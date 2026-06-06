package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

var (
	ErrExecutorSessionRequired     = errors.New("executor session is required")
	ErrExecutorWorkspaceRequired   = errors.New("executor runtime workspace root is required")
	ErrExecutorToolCallIDRequired  = errors.New("tool call id is required")
	ErrExecutorToolNameRequired    = errors.New("tool name is required")
	ErrExecutorUnsupportedToolType = errors.New("unsupported tool type")
	ErrExecutorUnknownTool         = errors.New("tool is not active in this session")
	ErrExecutorUnsupportedTool     = errors.New("tool is not supported by the built-in executor")
)

// ToolExecutionStatus describes how one tool call ended.
type ToolExecutionStatus string

const (
	ToolExecutionSucceeded        ToolExecutionStatus = "ok"
	ToolExecutionFailed           ToolExecutionStatus = "error"
	ToolExecutionApprovalRequired ToolExecutionStatus = "approval_required"
)

var approvalRequiredCapabilities = map[string]struct{}{
	tools.CapabilityWrite:  {},
	tools.CapabilityWeb:    {},
	tools.CapabilityModule: {},
}

// ToolExecutionSummary carries deterministic fields that the CLI can print.
type ToolExecutionSummary struct {
	ToolName   string
	Capability string
	Status     ToolExecutionStatus
	Fields     map[string]string
}

// String formats the summary as stable key=value pairs for CLI output.
func (s ToolExecutionSummary) String() string {
	parts := []string{
		"tool=" + quoteSummaryValue(s.ToolName),
		"status=" + quoteSummaryValue(string(s.Status)),
	}
	if s.Capability != "" {
		parts = append(parts, "capability="+quoteSummaryValue(s.Capability))
	}

	if len(s.Fields) == 0 {
		return strings.Join(parts, " ")
	}

	keys := make([]string, 0, len(s.Fields))
	for key := range s.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		parts = append(parts, key+"="+quoteSummaryValue(s.Fields[key]))
	}

	return strings.Join(parts, " ")
}

// ToolExecutionResult stores the structured outcome for one tool call.
type ToolExecutionResult struct {
	ToolCallID          string
	ToolName            string
	Source              string
	Capability          string
	Status              ToolExecutionStatus
	Summary             ToolExecutionSummary
	Output              string
	OutputOriginalBytes int
	OutputTruncated     bool
	OutputDroppedBytes  int
	ApprovalCommand     string
	PendingApproval     *session.PendingApprovalRequest
	ApprovalReused      bool
}

// Successful reports whether the tool call completed without errors.
func (r ToolExecutionResult) Successful() bool {
	return r.Status == ToolExecutionSucceeded
}

// RequiresApproval reports whether the tool call was skipped pending approval.
func (r ToolExecutionResult) RequiresApproval() bool {
	return r.Status == ToolExecutionApprovalRequired
}

// ToolMessage converts the executor result into a session tool message.
func (r ToolExecutionResult) ToolMessage() session.Message {
	return session.Message{
		Role:       session.RoleTool,
		Content:    r.Output,
		ToolCallID: r.ToolCallID,
		ToolName:   r.ToolName,
	}
}

// Event converts the executor result into the event shape already used by the session runner.
func (r ToolExecutionResult) Event() ToolExecutionEvent {
	return ToolExecutionEvent{
		ToolCallID: r.ToolCallID,
		ToolName:   r.ToolName,
		Source:     r.Source,
		Capability: r.Capability,
		Output: LimitedText{
			Text:          r.Output,
			OriginalBytes: r.OutputOriginalBytes,
			Truncated:     r.OutputTruncated,
		},
		Failed: r.Status != ToolExecutionSucceeded,
	}
}

// Executor dispatches the built-in tool set while enforcing capability approvals.
type Executor struct {
	runtime Runtime
	session *session.Session
}

// NewExecutor creates a tool executor for one live session.
func NewExecutor(runtime Runtime, sess *session.Session) (*Executor, error) {
	if sess == nil {
		return nil, ErrExecutorSessionRequired
	}
	if strings.TrimSpace(runtime.WorkspaceRoot()) == "" {
		return nil, ErrExecutorWorkspaceRequired
	}

	return &Executor{
		runtime: runtime,
		session: sess,
	}, nil
}

// Execute dispatches one provider tool call against the built-in tool set.
func (e *Executor) Execute(ctx context.Context, call provider.ToolCall) (ToolExecutionResult, error) {
	if e == nil || e.session == nil {
		return ToolExecutionResult{}, ErrExecutorSessionRequired
	}

	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		return ToolExecutionResult{}, ErrExecutorToolCallIDRequired
	}

	toolName := strings.TrimSpace(call.Name)
	if toolName == "" {
		return ToolExecutionResult{}, ErrExecutorToolNameRequired
	}

	if call.Type != "" && call.Type != provider.ToolTypeFunction {
		return e.failureResult(callID, toolName, tools.SourceBuiltin, "", map[string]string{
			"error": fmt.Sprintf("%v: %q", ErrExecutorUnsupportedToolType, call.Type),
		}, fmt.Errorf("%w %q", ErrExecutorUnsupportedToolType, call.Type)), nil
	}

	spec, ok := e.lookupTool(toolName)
	if !ok {
		return e.failureResult(callID, toolName, tools.SourceBuiltin, "", map[string]string{
			"error": ErrExecutorUnknownTool.Error(),
		}, fmt.Errorf("%w %q", ErrExecutorUnknownTool, toolName)), nil
	}

	if command, needed := e.approvalCommand(spec.Capability); needed {
		return e.approvalRequiredResult(callID, spec, command), nil
	}

	switch spec.Name {
	case tools.ReadFileToolName:
		return e.executeReadFile(callID, spec, call.ArgumentsJSON), nil
	case tools.WriteFileToolName:
		return e.executeWriteFile(callID, spec, call.ArgumentsJSON), nil
	case tools.ToolNameRunShell:
		return e.executeShell(ctx, callID, spec, call.ArgumentsJSON), nil
	case tools.ToolNameFetchURL:
		return e.executeWebFetch(ctx, callID, spec, call.ArgumentsJSON), nil
	default:
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"error":  ErrExecutorUnsupportedTool.Error(),
			"source": defaultToolSource(spec.Source),
		}, fmt.Errorf("%w %q", ErrExecutorUnsupportedTool, spec.Name)), nil
	}
}

// ExecuteCalls runs tool calls in order and stops only on structural executor errors.
func (e *Executor) ExecuteCalls(ctx context.Context, calls []provider.ToolCall) ([]ToolExecutionResult, error) {
	results := make([]ToolExecutionResult, 0, len(calls))
	for _, call := range calls {
		result, err := e.Execute(ctx, call)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (e *Executor) executeReadFile(callID string, spec tools.Spec, rawArgs string) ToolExecutionResult {
	req, err := parseReadFileArgs(rawArgs)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"error": "decode read_file arguments",
		}, fmt.Errorf("decode read_file arguments: %w", err))
	}

	result, err := tools.ReadFile(e.runtime, req)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"path":  req.Path,
			"error": err.Error(),
		}, err)
	}

	fields := map[string]string{
		"bytes": strconv.Itoa(result.OriginalBytes),
		"path":  result.Path,
	}
	if result.Truncated {
		fields["truncated"] = "true"
		fields["dropped_bytes"] = strconv.Itoa(result.DroppedBytes)
	}

	return e.successResult(callID, spec, fields, formatReadFileOutput(result))
}

func (e *Executor) executeWriteFile(callID string, spec tools.Spec, rawArgs string) ToolExecutionResult {
	req, err := parseWriteFileArgs(rawArgs)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"error": "decode write_file arguments",
		}, fmt.Errorf("decode write_file arguments: %w", err))
	}

	result, err := tools.WriteFile(e.runtime, req)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"path":  req.Path,
			"error": err.Error(),
		}, err)
	}

	action := "overwrote"
	if result.Created {
		action = "created"
	}

	return e.successResult(callID, spec, map[string]string{
		"action":        action,
		"bytes_written": strconv.Itoa(result.BytesWritten),
		"path":          result.Path,
	}, formatWriteFileOutput(result))
}

func (e *Executor) executeShell(ctx context.Context, callID string, spec tools.Spec, rawArgs string) ToolExecutionResult {
	input, err := tools.ParseShellInputJSON(rawArgs)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"error": "decode run_shell arguments",
		}, err)
	}

	workdir, err := tools.ResolveShellWorkdir(e.runtime, input.Workdir)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"command": input.Command,
			"error":   err.Error(),
		}, err)
	}

	signature := tools.ShellApprovalSignature(input.Command, workdir)
	if !e.session.IsShellCommandApproved(input.Command, workdir) {
		pending := session.PendingApprovalRequest{
			ToolCallID: callID,
			ToolName:   spec.Name,
			Command:    input.Command,
			Workdir:    workdir,
			Signature:  signature,
			PromptText: fmt.Sprintf("Approve running %q in %s? [y/N] ", input.Command, workdir),
			Status:     session.PendingApprovalStatusPending,
		}
		e.session.SetPendingShellApproval(pending)
		return e.pendingApprovalResult(spec, pending)
	}

	result, execErr := tools.ExecuteShell(ctx, e.runtime, input)
	fields := map[string]string{
		"command":   input.Command,
		"exit_code": strconv.Itoa(result.ExitCode),
		"workdir":   result.Workdir,
	}
	if result.TimedOut {
		fields["timed_out"] = "true"
	}

	if execErr != nil {
		fields["error"] = execErr.Error()
		failed := e.failureResult(callID, spec.Name, spec.Source, spec.Capability, fields, execErr)
		failed.ApprovalReused = true
		return failed
	}

	succeeded := e.successResult(callID, spec, fields, result.ToolOutput())
	succeeded.ApprovalReused = true
	return succeeded
}

func (e *Executor) executeWebFetch(ctx context.Context, callID string, spec tools.Spec, rawArgs string) ToolExecutionResult {
	input, err := tools.ParseWebFetchInputJSON(rawArgs)
	if err != nil {
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, map[string]string{
			"error": "decode fetch_url arguments",
		}, err)
	}

	result, execErr := tools.ExecuteWebFetch(ctx, e.runtime, input)
	fields := map[string]string{
		"url": input.URL,
	}
	if result.StatusCode != 0 {
		fields["status_code"] = strconv.Itoa(result.StatusCode)
	}
	if result.ContentType != "" {
		fields["content_type"] = result.ContentType
	}

	if execErr != nil {
		fields["error"] = execErr.Error()
		return e.failureResult(callID, spec.Name, spec.Source, spec.Capability, fields, execErr)
	}

	if result.FinalURL != "" && result.FinalURL != result.URL {
		fields["final_url"] = result.FinalURL
	}

	return e.successResult(callID, spec, fields, result.ToolOutput())
}

func (e *Executor) successResult(callID string, spec tools.Spec, fields map[string]string, output string) ToolExecutionResult {
	return e.buildResult(callID, spec.Name, spec.Source, spec.Capability, ToolExecutionSucceeded, fields, output, "")
}

func (e *Executor) pendingApprovalResult(spec tools.Spec, pending session.PendingApprovalRequest) ToolExecutionResult {
	return ToolExecutionResult{
		ToolCallID:      pending.ToolCallID,
		ToolName:        spec.Name,
		Source:          defaultToolSource(spec.Source),
		Capability:      spec.Capability,
		Status:          ToolExecutionApprovalRequired,
		Summary:         ToolExecutionSummary{ToolName: spec.Name, Capability: spec.Capability, Status: ToolExecutionApprovalRequired, Fields: map[string]string{"command": pending.Command, "workdir": pending.Workdir}},
		PendingApproval: &pending,
	}
}

func (e *Executor) approvalRequiredResult(callID string, spec tools.Spec, command string) ToolExecutionResult {
	return e.buildResult(callID, spec.Name, spec.Source, spec.Capability, ToolExecutionApprovalRequired, map[string]string{
		"approve_command": command,
	}, formatErrorOutput(fmt.Errorf("approval required for capability %q; run %s and retry the action", spec.Capability, command)), command)
}

func (e *Executor) failureResult(callID, toolName, source, capability string, fields map[string]string, execErr error, outputOverride ...string) ToolExecutionResult {
	output := ""
	if len(outputOverride) > 0 {
		output = outputOverride[0]
	} else if execErr != nil {
		output = formatErrorOutput(execErr)
	}

	return e.buildResult(callID, toolName, source, capability, ToolExecutionFailed, fields, output, "")
}

func (e *Executor) buildResult(callID, toolName, source, capability string, status ToolExecutionStatus, fields map[string]string, output, approvalCommand string) ToolExecutionResult {
	limited := e.runtime.LimitToolOutput(output)
	summaryFields := cloneStringMap(fields)
	if limited.Truncated {
		if summaryFields == nil {
			summaryFields = make(map[string]string)
		}
		summaryFields["tool_output_bytes"] = strconv.Itoa(limited.OriginalBytes)
		summaryFields["tool_output_truncated"] = "true"
	}

	return ToolExecutionResult{
		ToolCallID:          callID,
		ToolName:            toolName,
		Source:              defaultToolSource(source),
		Capability:          capability,
		Status:              status,
		Summary:             ToolExecutionSummary{ToolName: toolName, Capability: capability, Status: status, Fields: summaryFields},
		Output:              limited.Text,
		OutputOriginalBytes: limited.OriginalBytes,
		OutputTruncated:     limited.Truncated,
		OutputDroppedBytes:  limited.DroppedBytes(),
		ApprovalCommand:     approvalCommand,
	}
}

func (e *Executor) approvalCommand(capability string) (string, bool) {
	if _, ok := approvalRequiredCapabilities[capability]; !ok {
		return "", false
	}
	if e.session.IsApproved(capability) {
		return "", false
	}

	return "/approve " + capability, true
}

func (e *Executor) lookupTool(name string) (tools.Spec, bool) {
	for _, spec := range e.session.ActiveTools() {
		if spec.Name == name {
			spec.Source = defaultToolSource(spec.Source)
			return spec, true
		}
	}

	spec, ok := builtinToolCatalog()[name]
	if !ok {
		return tools.Spec{}, false
	}

	return spec, true
}

func builtinToolCatalog() map[string]tools.Spec {
	specs := tools.FileToolSpecs()
	specs = append(specs, tools.BuiltinShellSpec(), tools.BuiltinWebSpec())

	catalog := make(map[string]tools.Spec, len(specs))
	for _, spec := range specs {
		spec.Source = defaultToolSource(spec.Source)
		catalog[spec.Name] = spec
	}

	return catalog
}

func defaultToolSource(source string) string {
	if strings.TrimSpace(source) == "" {
		return tools.SourceBuiltin
	}

	return source
}

func parseReadFileArgs(raw string) (tools.ReadFileRequest, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := decodeExecutorJSON(raw, &payload); err != nil {
		return tools.ReadFileRequest{}, err
	}

	return tools.ReadFileRequest{
		Path: payload.Path,
	}, nil
}

func parseWriteFileArgs(raw string) (tools.WriteFileRequest, error) {
	var payload struct {
		Path          string  `json:"path"`
		Content       *string `json:"content"`
		CreateParents bool    `json:"create_parents"`
	}
	if err := decodeExecutorJSON(raw, &payload); err != nil {
		return tools.WriteFileRequest{}, err
	}

	return tools.WriteFileRequest{
		Path:          payload.Path,
		Content:       payload.Content,
		CreateParents: payload.CreateParents,
	}, nil
}

func decodeExecutorJSON(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return errors.New("unexpected trailing JSON content")
}

func formatReadFileOutput(result tools.ReadFileResult) string {
	var builder strings.Builder

	builder.WriteString("path: ")
	builder.WriteString(result.Path)
	builder.WriteString("\nbytes: ")
	builder.WriteString(strconv.Itoa(result.OriginalBytes))
	if result.Truncated {
		builder.WriteString("\ntruncated: true")
		builder.WriteString("\ndropped_bytes: ")
		builder.WriteString(strconv.Itoa(result.DroppedBytes))
	}
	builder.WriteString("\ncontent:\n")
	builder.WriteString(result.Content)

	return builder.String()
}

func formatWriteFileOutput(result tools.WriteFileResult) string {
	status := "overwritten"
	if result.Created {
		status = "created"
	}

	return fmt.Sprintf("path: %s\nstatus: %s\nbytes_written: %d", result.Path, status, result.BytesWritten)
}

func formatErrorOutput(err error) string {
	if err == nil {
		return ""
	}

	return "error: " + err.Error()
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}

func quoteSummaryValue(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return `""`
	}
	if strings.ContainsAny(normalized, " =\"") {
		return strconv.Quote(normalized)
	}

	return normalized
}
