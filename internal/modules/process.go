package modules

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	messageTypeInit   = "init"
	messageTypeReady  = "ready"
	messageTypeCall   = "call"
	messageTypeResult = "result"
	messageTypeError  = "error"
)

var (
	// ErrProcessStopped reports that the module process is no longer available.
	ErrProcessStopped = errors.New("module process is stopped")
	// ErrProtocolViolation reports an invalid NDJSON protocol message.
	ErrProtocolViolation = errors.New("module protocol violation")
)

var processCallCounter uint64

// StartOptions captures the host context needed for module startup.
type StartOptions struct {
	SessionID     string
	WorkspaceRoot string
	Env           []string
}

// Process manages one attached local module process and its NDJSON protocol.
type Process struct {
	manifest Manifest
	path     string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytesBuffer

	reader *bufio.Reader
	writer *json.Encoder

	mu      sync.Mutex
	waitMu  sync.Mutex
	waitErr error
	waited  bool
	stopped bool
	status  string
}

// CallResult is one successful module tool response.
type CallResult struct {
	ID      string
	Tool    string
	Content string
}

// ProtocolError keeps the failing message context for protocol problems.
type ProtocolError struct {
	Message string
	Cause   error
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CallError reports a module-declared tool failure.
type CallError struct {
	ID      string
	Tool    string
	Message string
}

func (e *CallError) Error() string {
	if e == nil {
		return ""
	}
	if e.Tool == "" {
		return e.Message
	}
	return fmt.Sprintf("module tool %q failed: %s", e.Tool, e.Message)
}

type initRequest struct {
	Type          string `json:"type"`
	SessionID     string `json:"session_id"`
	WorkspaceRoot string `json:"workspace_root"`
}

type readyResponse struct {
	Type    string `json:"type"`
	Module  string `json:"module"`
	Version string `json:"version"`
}

type callRequest struct {
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

type resultResponse struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Content string `json:"content"`
}

type errorResponse struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

// StartProcess launches the local module executable, performs the init/ready
// handshake, and returns a live NDJSON protocol client.
func StartProcess(ctx context.Context, moduleDir string, manifest Manifest, options StartOptions) (*Process, error) {
	modulePath, err := filepath.Abs(strings.TrimSpace(moduleDir))
	if err != nil {
		return nil, fmt.Errorf("resolve module directory: %w", err)
	}
	if modulePath == "" {
		return nil, fmt.Errorf("resolve module directory: empty path")
	}

	workspaceRoot, err := filepath.Abs(strings.TrimSpace(options.WorkspaceRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root is required")
	}

	entrypointPath, err := resolveEntrypoint(modulePath, manifest.Entrypoint)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, entrypointPath)
	cmd.Dir = modulePath
	if len(options.Env) > 0 {
		cmd.Env = append(os.Environ(), options.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open module stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("open module stdout: %w", err)
	}

	stderr := &bytesBuffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("start module process: %w", err)
	}

	process := &Process{
		manifest: manifest,
		path:     modulePath,
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		reader:   bufio.NewReader(stdout),
		writer:   json.NewEncoder(stdin),
		status:   "starting",
	}

	if err := process.initialize(ctx, options); err != nil {
		process.status = "failed"
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = process.Stop(stopCtx)
		return nil, err
	}

	process.status = "ready"
	return process, nil
}

// Manifest returns the validated manifest associated with this process.
func (p *Process) Manifest() Manifest {
	if p == nil {
		return Manifest{}
	}
	return cloneProcessManifest(p.manifest)
}

// Path returns the absolute module directory path.
func (p *Process) Path() string {
	if p == nil {
		return ""
	}
	return p.path
}

// PID returns the running OS process id, or zero if unavailable.
func (p *Process) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Status reports the current process lifecycle status.
func (p *Process) Status() string {
	if p == nil {
		return ""
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// Call executes one module-exported tool over the NDJSON protocol.
func (p *Process) Call(ctx context.Context, tool string, arguments map[string]any) (CallResult, error) {
	if p == nil {
		return CallResult{}, ErrProcessStopped
	}
	if strings.TrimSpace(tool) == "" {
		return CallResult{}, fmt.Errorf("tool name is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped || p.status == "stopped" {
		return CallResult{}, ErrProcessStopped
	}
	if p.status != "ready" {
		return CallResult{}, fmt.Errorf("module process is not ready")
	}

	callID := nextCallID()
	if arguments == nil {
		arguments = map[string]any{}
	}

	if err := p.writeMessage(ctx, callRequest{
		Type:      messageTypeCall,
		ID:        callID,
		Tool:      tool,
		Arguments: arguments,
	}); err != nil {
		p.status = "failed"
		return CallResult{}, err
	}

	payload, err := p.readRawMessage(ctx)
	if err != nil {
		p.status = "failed"
		return CallResult{}, err
	}

	messageType, err := messageTypeFromPayload(payload)
	if err != nil {
		p.status = "failed"
		return CallResult{}, err
	}

	switch messageType {
	case messageTypeResult:
		var response resultResponse
		if err := decodePayload(payload, &response); err != nil {
			p.status = "failed"
			return CallResult{}, err
		}
		if response.ID != callID {
			p.status = "failed"
			return CallResult{}, protocolErrorf("module result id %q does not match request %q", response.ID, callID)
		}

		return CallResult{
			ID:      response.ID,
			Tool:    tool,
			Content: response.Content,
		}, nil
	case messageTypeError:
		var response errorResponse
		if err := decodePayload(payload, &response); err != nil {
			p.status = "failed"
			return CallResult{}, err
		}
		if response.ID != callID {
			p.status = "failed"
			return CallResult{}, protocolErrorf("module error id %q does not match request %q", response.ID, callID)
		}

		return CallResult{}, &CallError{
			ID:      response.ID,
			Tool:    tool,
			Message: response.Message,
		}
	default:
		p.status = "failed"
		return CallResult{}, protocolErrorf("unexpected module response type %q for tool call", messageType)
	}
}

// Stop closes the module client transport and waits for the process to exit.
// If the process does not stop before the context is done, it is killed.
func (p *Process) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return p.wait(ctx)
	}
	p.stopped = true
	p.status = "stopped"

	stdin := p.stdin
	process := (*os.Process)(nil)
	if p.cmd != nil {
		process = p.cmd.Process
	}
	p.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if process != nil {
		_ = process.Signal(os.Interrupt)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.wait(context.Background())
	}()

	select {
	case err := <-done:
		return normalizeExitError(err)
	case <-ctx.Done():
		if process != nil {
			_ = process.Signal(syscall.SIGTERM)
		}

		killDone := make(chan error, 1)
		go func() {
			killDone <- p.wait(context.Background())
		}()

		if process != nil {
			_ = process.Kill()
		}

		select {
		case err := <-killDone:
			if err == nil {
				return ctx.Err()
			}
			return errors.Join(ctx.Err(), normalizeExitError(err))
		default:
			return ctx.Err()
		}
	}
}

func (p *Process) initialize(ctx context.Context, options StartOptions) error {
	request := initRequest{
		Type:          messageTypeInit,
		SessionID:     strings.TrimSpace(options.SessionID),
		WorkspaceRoot: options.WorkspaceRoot,
	}
	if request.SessionID == "" {
		request.SessionID = nextCallID()
	}

	if err := p.writeMessage(ctx, request); err != nil {
		return err
	}

	payload, err := p.readRawMessage(ctx)
	if err != nil {
		return err
	}

	messageType, err := messageTypeFromPayload(payload)
	if err != nil {
		return err
	}
	if messageType != messageTypeReady {
		return protocolErrorf("expected ready response during init, got %q", messageType)
	}

	var response readyResponse
	if err := decodePayload(payload, &response); err != nil {
		return err
	}
	if strings.TrimSpace(response.Module) == "" {
		return protocolErrorf("ready response is missing module name")
	}
	if response.Module != p.manifest.Name {
		return protocolErrorf("ready response module %q does not match manifest %q", response.Module, p.manifest.Name)
	}
	if strings.TrimSpace(response.Version) == "" {
		return protocolErrorf("ready response is missing version")
	}
	if response.Version != p.manifest.Version {
		return protocolErrorf("ready response version %q does not match manifest %q", response.Version, p.manifest.Version)
	}

	return nil
}

func (p *Process) wait(ctx context.Context) error {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()

	if p.waited {
		return p.waitErr
	}

	done := make(chan error, 1)
	go func() {
		if p.cmd == nil {
			done <- nil
			return
		}
		done <- p.cmd.Wait()
	}()

	select {
	case err := <-done:
		p.waited = true
		p.waitErr = err
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
		return p.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Process) writeMessage(ctx context.Context, message any) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.writer.Encode(message)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("write module request: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Process) readRawMessage(ctx context.Context) (json.RawMessage, error) {
	type readResult struct {
		line []byte
		err  error
	}

	resultCh := make(chan readResult, 1)
	go func() {
		line, err := p.reader.ReadBytes('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			if errors.Is(result.err, io.EOF) && len(result.line) == 0 {
				return nil, fmt.Errorf("read module response: %w", io.EOF)
			}
			if result.err != nil && !errors.Is(result.err, io.EOF) {
				return nil, fmt.Errorf("read module response: %w", result.err)
			}
		}

		line := strings.TrimSpace(string(result.line))
		if line == "" {
			return nil, protocolErrorf("module sent an empty response line")
		}

		return json.RawMessage(line), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func resolveEntrypoint(moduleDir, entrypoint string) (string, error) {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return "", fmt.Errorf("entrypoint is required")
	}

	resolved := entrypoint
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(moduleDir, resolved)
	}

	resolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve module entrypoint: %w", err)
	}

	rel, err := filepath.Rel(moduleDir, resolved)
	if err != nil {
		return "", fmt.Errorf("compare entrypoint to module directory: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("entrypoint must stay inside module directory")
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat module entrypoint: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("module entrypoint %q is a directory", resolved)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("module entrypoint %q is not executable", resolved)
	}

	return resolved, nil
}

func messageTypeFromPayload(payload json.RawMessage) (string, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", &ProtocolError{
			Message: "decode module response envelope",
			Cause:   errors.Join(ErrProtocolViolation, err),
		}
	}
	if strings.TrimSpace(envelope.Type) == "" {
		return "", protocolErrorf("module response is missing type")
	}

	return envelope.Type, nil
}

func decodePayload[T any](payload json.RawMessage, target *T) error {
	if err := json.Unmarshal(payload, target); err != nil {
		return &ProtocolError{
			Message: "decode module response",
			Cause:   errors.Join(ErrProtocolViolation, err),
		}
	}

	return nil
}

func protocolErrorf(format string, args ...any) error {
	return &ProtocolError{
		Message: fmt.Sprintf(format, args...),
		Cause:   ErrProtocolViolation,
	}
}

func nextCallID() string {
	id := atomic.AddUint64(&processCallCounter, 1)
	return fmt.Sprintf("call-%d-%d", time.Now().UTC().UnixNano(), id)
}

func normalizeExitError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}

	return err
}

func cloneProcessManifest(manifest Manifest) Manifest {
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
			InputSchema: cloneProcessSchema(toolSpec.InputSchema),
			Capability:  toolSpec.Capability,
		})
	}

	return cloned
}

func cloneProcessSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = value
	}

	return cloned
}

type bytesBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *bytesBuffer) String() string {
	if b == nil {
		return ""
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.buf...))
}
