package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const ToolNameRunShell = "run_shell"

var (
	ErrShellCommandRequired  = errors.New("shell command is required")
	ErrShellWorkdirInvalid   = errors.New("shell workdir must be an existing directory")
	ErrShellExecutionTimeout = errors.New("shell command timed out")
)

// ShellInput is the built-in shell tool payload.
type ShellInput struct {
	Command string `json:"command"`
	Workdir string `json:"workdir,omitempty"`
}

// ShellResult captures one completed shell-tool execution.
type ShellResult struct {
	Command  string
	Workdir  string
	ExitCode int
	TimedOut bool
	Output   LimitedText
}

// BuiltinShellSpec describes the default workspace-scoped shell tool.
func BuiltinShellSpec() Spec {
	return Spec{
		Name:        ToolNameRunShell,
		Description: "Run a shell command inside the current workspace boundary.",
		Capability:  CapabilityShell,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute.",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Optional working directory inside the workspace. Defaults to the workspace root.",
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
	}
}

// ParseShellInputJSON decodes one provider tool-call argument payload.
func ParseShellInputJSON(raw string) (ShellInput, error) {
	var input ShellInput
	if err := decodeJSONArguments(raw, &input); err != nil {
		return ShellInput{}, fmt.Errorf("decode shell arguments: %w", err)
	}

	return input, nil
}

// ExecuteShell runs a shell command within the workspace boundary and returns
// its combined stdout/stderr output with session truncation applied.
func ExecuteShell(ctx context.Context, runtime shellRuntime, input ShellInput) (ShellResult, error) {
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return ShellResult{}, ErrShellCommandRequired
	}

	workdir, err := ResolveShellWorkdir(runtime, input.Workdir)
	if err != nil {
		return ShellResult{}, err
	}

	cmdCtx, cancel := runtime.WithCommandTimeout(ctx)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-lc", command)
	cmd.Dir = workdir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	runErr := cmd.Run()

	result := ShellResult{
		Command: command,
		Workdir: workdir,
		Output:  limitText(output.String(), runtime.CommandOutputLimit()),
	}

	if runErr == nil {
		return result.withExitCode(0), nil
	}

	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExitCode = -1
		return result, fmt.Errorf("%w after %s for command %q in %q", ErrShellExecutionTimeout, runtime.CommandTimeout(), command, workdir)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return result.withExitCode(exitErr.ExitCode()), nil
	}

	return result.withExitCode(-1), fmt.Errorf("run shell command %q in %q: %w", command, workdir, runErr)
}

// Success reports whether the command completed with exit code 0.
func (r ShellResult) Success() bool {
	return !r.TimedOut && r.ExitCode == 0
}

// ToolOutput formats the shell result for reinjection into session context.
func (r ShellResult) ToolOutput() string {
	var builder strings.Builder

	builder.WriteString("command: ")
	builder.WriteString(r.Command)
	builder.WriteString("\nworkdir: ")
	builder.WriteString(r.Workdir)
	builder.WriteString("\nexit_code: ")
	builder.WriteString(fmt.Sprintf("%d", r.ExitCode))

	if r.TimedOut {
		builder.WriteString("\ntimed_out: true")
	}

	builder.WriteString("\noutput:\n")
	builder.WriteString(r.Output.Text)

	return builder.String()
}

func (r ShellResult) withExitCode(exitCode int) ShellResult {
	r.ExitCode = exitCode
	return r
}

// ResolveShellWorkdir validates and resolves one shell workdir inside the workspace.
func ResolveShellWorkdir(runtime shellRuntime, requested string) (string, error) {
	workdir := requested
	if strings.TrimSpace(workdir) == "" {
		workdir = "."
	}

	resolved, err := runtime.ResolvePath(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve shell workdir %q: %w", workdir, err)
	}

	info, statErr := os.Stat(resolved)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("resolve shell workdir %q: %w", workdir, ErrShellWorkdirInvalid)
		}
		return "", fmt.Errorf("stat shell workdir %q: %w", resolved, statErr)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate shell workdir %q: %w", resolved, ErrShellWorkdirInvalid)
	}

	return resolved, nil
}

// ShellApprovalSignature scopes one approval to exact command text in one resolved workdir.
func ShellApprovalSignature(command, workdir string) string {
	return strings.TrimSpace(command) + "\x00" + strings.TrimSpace(workdir)
}

func decodeJSONArguments(raw string, target any) error {
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

// LimitedText describes a possibly truncated tool payload without depending on
// the higher-level app package.
type LimitedText struct {
	Text          string
	OriginalBytes int
	Truncated     bool
}

type shellRuntime interface {
	ResolvePath(string) (string, error)
	WithCommandTimeout(context.Context) (context.Context, context.CancelFunc)
	CommandTimeout() time.Duration
	CommandOutputLimit() int
}

func limitText(text string, limit int) LimitedText {
	if limit <= 0 {
		return LimitedText{Text: text, OriginalBytes: len(text)}
	}

	originalBytes := len(text)
	if originalBytes <= limit {
		return LimitedText{Text: text, OriginalBytes: originalBytes}
	}

	marker := ""
	contentLimit := limit
	if limit > len(truncatedMarker) {
		marker = truncatedMarker
		contentLimit = limit - len(marker)
	}

	prefix := utf8SafePrefix(text, contentLimit)
	return LimitedText{
		Text:          prefix + marker,
		OriginalBytes: originalBytes,
		Truncated:     true,
	}
}

func utf8SafePrefix(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}

	cut := limit
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}

	return text[:cut]
}

const truncatedMarker = "\n...[truncated]"
