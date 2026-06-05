package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ModuleAttacher activates one local module directory for the current session.
type ModuleAttacher interface {
	AttachModule(context.Context, string) (ModuleAttachResult, error)
}

// ModuleDetacher removes one active module from the current session.
type ModuleDetacher interface {
	DetachModule(context.Context, string) (ModuleDetachResult, error)
}

// ModuleLifecycle bundles the attach and detach operations used by the CLI.
type ModuleLifecycle interface {
	ModuleAttacher
	ModuleDetacher
}

// ModuleAttachResult describes the visible outcome of one /attach operation.
type ModuleAttachResult struct {
	Name                  string
	Path                  string
	RequestedCapabilities []string
	ToolNames             []string
}

// ModuleDetachResult describes the visible outcome of one /detach operation.
type ModuleDetachResult struct {
	Name      string
	ToolNames []string
}

// NewModuleAttachHandler adapts a module attacher to the CLI command contract.
func NewModuleAttachHandler(out io.Writer, attacher ModuleAttacher) func(context.Context, AttachCommand) error {
	writer := writerOrDiscard(out)

	return func(ctx context.Context, command AttachCommand) error {
		if attacher == nil {
			return &CommandDispatchError{
				Command: command.CommandName(),
				Message: "module attacher is not configured",
			}
		}

		result, err := attacher.AttachModule(ctx, command.Path)
		if err != nil {
			return &CommandDispatchError{
				Command: command.CommandName(),
				Message: err.Error(),
			}
		}

		return writeAttachResult(writer, command, result)
	}
}

// NewModuleDetachHandler adapts a module detacher to the CLI command contract.
func NewModuleDetachHandler(out io.Writer, detacher ModuleDetacher) func(context.Context, DetachCommand) error {
	writer := writerOrDiscard(out)

	return func(ctx context.Context, command DetachCommand) error {
		if detacher == nil {
			return &CommandDispatchError{
				Command: command.CommandName(),
				Message: "module detacher is not configured",
			}
		}

		result, err := detacher.DetachModule(ctx, command.Name)
		if err != nil {
			return &CommandDispatchError{
				Command: command.CommandName(),
				Message: err.Error(),
			}
		}

		return writeDetachResult(writer, command, result)
	}
}

// ConfigureModuleCommandHandlers installs /attach and /detach handlers onto an existing set.
func ConfigureModuleCommandHandlers(handlers *CommandHandlers, out io.Writer, lifecycle ModuleLifecycle) {
	if handlers == nil {
		return
	}

	handlers.Attach = NewModuleAttachHandler(out, lifecycle)
	handlers.Detach = NewModuleDetachHandler(out, lifecycle)
}

func writeAttachResult(out io.Writer, command AttachCommand, result ModuleAttachResult) error {
	name := strings.TrimSpace(result.Name)
	if name == "" {
		name = strings.TrimSpace(command.Path)
	}

	if _, err := fmt.Fprintf(out, "attached module %q", name); err != nil {
		return err
	}
	if path := strings.TrimSpace(result.Path); path != "" {
		if _, err := fmt.Fprintf(out, " from %s", path); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, "\n"); err != nil {
		return err
	}

	if len(result.RequestedCapabilities) > 0 {
		if _, err := fmt.Fprintf(out, "requested capabilities: %s\n", strings.Join(result.RequestedCapabilities, ", ")); err != nil {
			return err
		}
	}
	if len(result.ToolNames) > 0 {
		if _, err := fmt.Fprintf(out, "activated tools: %s\n", strings.Join(result.ToolNames, ", ")); err != nil {
			return err
		}
	}

	return nil
}

func writeDetachResult(out io.Writer, command DetachCommand, result ModuleDetachResult) error {
	name := strings.TrimSpace(result.Name)
	if name == "" {
		name = strings.TrimSpace(command.Name)
	}

	if _, err := fmt.Fprintf(out, "detached module %q\n", name); err != nil {
		return err
	}
	if len(result.ToolNames) > 0 {
		if _, err := fmt.Fprintf(out, "removed tools: %s\n", strings.Join(result.ToolNames, ", ")); err != nil {
			return err
		}
	}

	return nil
}
