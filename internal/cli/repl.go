package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const defaultPrompt = "> "

// CommandExecutor is the REPL-facing slash-command dispatch contract.
type CommandExecutor interface {
	DispatchParsedInput(context.Context, ParsedInput) error
}

// AssistantStream yields assistant output chunks until io.EOF.
type AssistantStream interface {
	Recv(context.Context) (AssistantChunk, error)
}

// AssistantChunk carries one streamed assistant text fragment.
type AssistantChunk struct {
	Text string
}

// PromptSubmitter turns one natural-language REPL line into a streamed response.
type PromptSubmitter interface {
	SubmitPrompt(context.Context, string) (AssistantStream, error)
}

// CommandExecutorFunc adapts a function to the CommandExecutor interface.
type CommandExecutorFunc func(context.Context, ParsedInput) error

// DispatchParsedInput forwards to the wrapped function.
func (f CommandExecutorFunc) DispatchParsedInput(ctx context.Context, input ParsedInput) error {
	return f(ctx, input)
}

// PromptSubmitterFunc adapts a function to the PromptSubmitter interface.
type PromptSubmitterFunc func(context.Context, string) (AssistantStream, error)

// SubmitPrompt forwards to the wrapped function.
func (f PromptSubmitterFunc) SubmitPrompt(ctx context.Context, prompt string) (AssistantStream, error) {
	return f(ctx, prompt)
}

// REPL owns the terminal streams and prompt used by the interactive session.
type REPL struct {
	In              io.Reader
	Out             io.Writer
	Err             io.Writer
	Prompt          string
	commandExecutor CommandExecutor
	promptSubmitter PromptSubmitter
}

// NewREPL builds the minimal terminal shell for a single interactive launch.
func NewREPL(in io.Reader, out, err io.Writer) *REPL {
	return &REPL{
		In:     in,
		Out:    out,
		Err:    err,
		Prompt: defaultPrompt,
	}
}

// SetPrompt updates the visible prompt prefix for future reads.
func (r *REPL) SetPrompt(prompt string) {
	if prompt == "" {
		r.Prompt = defaultPrompt
		return
	}

	r.Prompt = prompt
}

// SetDispatcher wires slash-command handling into the REPL loop.
func (r *REPL) SetDispatcher(dispatcher *Dispatcher) {
	r.commandExecutor = dispatcher
}

// SetCommandExecutor wires a generic parsed-command executor into the REPL loop.
func (r *REPL) SetCommandExecutor(executor CommandExecutor) {
	r.commandExecutor = executor
}

// SetCommandHandlers builds and installs a dispatcher from typed command handlers.
func (r *REPL) SetCommandHandlers(handlers CommandHandlers) {
	r.commandExecutor = NewDispatcher(handlers)
}

// SetPromptSubmitter wires natural-language prompt handling into the REPL loop.
func (r *REPL) SetPromptSubmitter(submitter PromptSubmitter) {
	r.promptSubmitter = submitter
}

// Run executes the interactive read/parse/dispatch/stream loop until EOF, /quit, or cancellation.
func (r *REPL) Run(ctx context.Context) error {
	reader := bufio.NewReader(readerOrDiscard(r.In))

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.writePrompt(); err != nil {
			return err
		}

		line, err := readLine(reader)
		switch {
		case errors.Is(err, io.EOF) && strings.TrimSpace(line) == "":
			return nil
		case err != nil && !errors.Is(err, io.EOF):
			return err
		}

		if stop, handled, handleErr := r.handleLine(ctx, line); handleErr != nil {
			if writeErr := r.writeError(handleErr); writeErr != nil {
				return writeErr
			}
		} else if stop {
			return nil
		} else if handled {
			// already processed
		}

		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func (r *REPL) handleLine(ctx context.Context, line string) (stop bool, handled bool, err error) {
	parsed, err := ParseInput(line)
	if err != nil {
		return false, false, err
	}
	if parsed.IsCommand() {
		if isQuitCommand(parsed.Command) {
			if r.commandExecutor == nil {
				return true, true, nil
			}
			if err := r.commandExecutor.DispatchParsedInput(ctx, parsed); err != nil {
				return false, true, err
			}
			return true, true, nil
		}
		if r.commandExecutor == nil {
			return false, true, missingHandlerError(parsed.Command.CommandName())
		}
		return false, true, r.commandExecutor.DispatchParsedInput(ctx, parsed)
	}
	if strings.TrimSpace(parsed.Text) == "" {
		return false, true, nil
	}
	if r.promptSubmitter == nil {
		return false, true, errors.New("prompt submitter is not configured")
	}

	assistantStream, err := r.promptSubmitter.SubmitPrompt(ctx, parsed.Text)
	if err != nil {
		return false, true, err
	}
	if assistantStream == nil {
		return false, true, errors.New("prompt submitter returned a nil assistant stream")
	}

	return false, true, r.renderAssistantStream(ctx, assistantStream)
}

func (r *REPL) renderAssistantStream(ctx context.Context, stream AssistantStream) error {
	wroteText := false
	endedWithNewline := false

	for {
		chunk, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			if wroteText && !endedWithNewline {
				if _, writeErr := io.WriteString(writerOrDiscard(r.Out), "\n"); writeErr != nil {
					return writeErr
				}
			}
			return nil
		}
		if err != nil {
			if wroteText && !endedWithNewline {
				if _, writeErr := io.WriteString(writerOrDiscard(r.Out), "\n"); writeErr != nil {
					return writeErr
				}
			}
			return err
		}
		if chunk.Text == "" {
			continue
		}
		if _, err := io.WriteString(writerOrDiscard(r.Out), chunk.Text); err != nil {
			return err
		}
		wroteText = true
		endedWithNewline = strings.HasSuffix(chunk.Text, "\n")
	}
}

func (r *REPL) writePrompt() error {
	_, err := io.WriteString(writerOrDiscard(r.Out), r.Prompt)
	return err
}

func (r *REPL) writeError(err error) error {
	if err == nil {
		return nil
	}

	_, writeErr := fmt.Fprintf(writerOrDiscard(r.Err), "goagent: %v\n", err)
	return writeErr
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	return line, err
}

func isQuitCommand(command SlashCommand) bool {
	switch command.(type) {
	case QuitCommand, *QuitCommand:
		return true
	default:
		return false
	}
}

func readerOrDiscard(reader io.Reader) io.Reader {
	if reader != nil {
		return reader
	}

	return strings.NewReader("")
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}

	return io.Discard
}
