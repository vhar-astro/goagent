package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	Text  string
	Usage *TokenUsage
}

// TokenUsage carries normalized prompt/completion token counts for one turn.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type promptHistoryEntry struct {
	line     string
	sequence int
	kind     string
}

type promptHistoryState struct {
	entries         []promptHistoryEntry
	cursor          int
	navigating      bool
	preservedDraft  string
	visibleLine     string
	visibleFromLive bool
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
	reader          *bufio.Reader
	interactive     *bool
	slashSuggester  func(string) []string
	commandExecutor CommandExecutor
	promptSubmitter PromptSubmitter
	promptHistory   promptHistoryState
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

// SetInteractive overrides automatic terminal detection for tests and non-tty flows.
func (r *REPL) SetInteractive(interactive bool) {
	r.interactive = &interactive
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

// SetSlashSuggester wires live slash suggestions for interactive input.
func (r *REPL) SetSlashSuggester(suggester func(string) []string) {
	r.slashSuggester = suggester
}

func (r *REPL) recordPromptHistoryLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}

	entry := promptHistoryEntry{
		line:     line,
		sequence: len(r.promptHistory.entries),
		kind:     "prompt",
	}
	if strings.HasPrefix(strings.TrimSpace(line), slashCommandPrefix) {
		entry.kind = "slash"
	}

	r.promptHistory.entries = append(r.promptHistory.entries, entry)
	r.resetPromptHistoryNavigation("")
}

func (r *REPL) resetPromptHistoryNavigation(line string) {
	r.promptHistory.cursor = len(r.promptHistory.entries)
	r.promptHistory.navigating = false
	r.promptHistory.preservedDraft = ""
	r.promptHistory.visibleLine = line
	r.promptHistory.visibleFromLive = true
}

func (r *REPL) promptHistoryOlder(currentLine string) (string, bool) {
	if len(r.promptHistory.entries) == 0 {
		return currentLine, false
	}

	if !r.promptHistory.navigating {
		r.promptHistory.preservedDraft = currentLine
		r.promptHistory.cursor = len(r.promptHistory.entries) - 1
	} else if r.promptHistory.cursor > 0 {
		r.promptHistory.cursor--
	}

	r.promptHistory.navigating = true
	line := r.promptHistory.entries[r.promptHistory.cursor].line
	r.setPromptHistoryVisibleLine(line, false)
	return line, true
}

func (r *REPL) promptHistoryNewer() (string, bool) {
	if !r.promptHistory.navigating || len(r.promptHistory.entries) == 0 {
		return r.promptHistory.visibleLine, false
	}

	if r.promptHistory.cursor < len(r.promptHistory.entries)-1 {
		r.promptHistory.cursor++
		line := r.promptHistory.entries[r.promptHistory.cursor].line
		r.setPromptHistoryVisibleLine(line, false)
		return line, true
	}

	line := r.promptHistory.preservedDraft
	r.resetPromptHistoryNavigation(line)
	return line, true
}

func (r *REPL) setPromptHistoryVisibleLine(line string, fromLive bool) {
	r.promptHistory.visibleLine = line
	r.promptHistory.visibleFromLive = fromLive
}

// Run executes the interactive read/parse/dispatch/stream loop until EOF, /quit, or cancellation.
func (r *REPL) Run(ctx context.Context) error {
	reader := r.inputReader()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.writePrompt(); err != nil {
			return err
		}

		line, err := r.readInputLine(reader)
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

// PromptApproval displays a local yes/no prompt and reads one reply from stdin.
func (r *REPL) PromptApproval(ctx context.Context, prompt string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, err := io.WriteString(writerOrDiscard(r.Out), prompt); err != nil {
		return false, err
	}

	line, err := readLine(r.inputReader())
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// WriteLocalMessage prints one local CLI-owned message line.
func (r *REPL) WriteLocalMessage(_ context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}

	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}

	_, err := io.WriteString(writerOrDiscard(r.Out), message)
	return err
}

func (r *REPL) handleLine(ctx context.Context, line string) (stop bool, handled bool, err error) {
	if r.isInteractive() {
		r.recordPromptHistoryLine(line)
	}

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
	var usage *TokenUsage

	for {
		chunk, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			if wroteText && !endedWithNewline {
				if _, writeErr := io.WriteString(writerOrDiscard(r.Out), "\n"); writeErr != nil {
					return writeErr
				}
			}
			if usage != nil {
				if err := r.WriteLocalMessage(ctx, formatTokenUsageSummary(*usage)); err != nil {
					return err
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
		if chunk.Usage != nil {
			usage = chunk.Usage
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

func formatTokenUsageSummary(usage TokenUsage) string {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 && usage.TotalTokens <= 0 {
		return ""
	}

	return fmt.Sprintf("[Tokens: %d input, %d output]", usage.InputTokens, usage.OutputTokens)
}

func (r *REPL) readInputLine(reader *bufio.Reader) (string, error) {
	if !r.isInteractive() {
		return readLine(reader)
	}

	return r.readInteractiveLine(reader)
}

func (r *REPL) readInteractiveLine(reader *bufio.Reader) (string, error) {
	restore, err := r.enterRawMode()
	if err != nil {
		return "", err
	}
	defer restore()

	line := ""
	r.setPromptHistoryVisibleLine(line, true)

	for {
		ch, _, err := reader.ReadRune()
		if err != nil {
			r.clearInteractiveBlock()
			return line, err
		}

		switch ch {
		case '\n':
			r.clearInteractiveBlock()
			if _, err := io.WriteString(writerOrDiscard(r.Out), "\n"); err != nil {
				return "", err
			}
			return line, nil
		case '\r':
			continue
		case 0x1b:
			updatedLine, handled, err := r.readInteractiveEscapeSequence(reader, line)
			if err != nil {
				return "", err
			}
			if !handled {
				continue
			}
			line = updatedLine
		case 0x7f, '\b':
			text := []rune(line)
			if len(text) > 0 {
				line = string(text[:len(text)-1])
			}
			r.setPromptHistoryVisibleLine(line, true)
		default:
			line += string(ch)
			r.setPromptHistoryVisibleLine(line, true)
		}

		if err := r.renderInteractiveLine(line); err != nil {
			return "", err
		}
	}
}

func (r *REPL) readInteractiveEscapeSequence(reader *bufio.Reader, currentLine string) (string, bool, error) {
	next, _, err := reader.ReadRune()
	if err != nil {
		return "", false, err
	}
	if next != '[' {
		return currentLine, false, nil
	}

	code, _, err := reader.ReadRune()
	if err != nil {
		return "", false, err
	}

	switch code {
	case 'A':
		line, changed := r.promptHistoryOlder(currentLine)
		return line, changed, nil
	case 'B':
		line, changed := r.promptHistoryNewer()
		return line, changed, nil
	default:
		return currentLine, false, nil
	}
}

func (r *REPL) renderInteractiveLine(line string) error {
	out := writerOrDiscard(r.Out)
	if _, err := io.WriteString(out, "\r\033[2K"+r.Prompt+line+"\033[s\033[J"); err != nil {
		return err
	}

	suggestions := r.slashSuggestions(line)
	for _, suggestion := range suggestions {
		if _, err := io.WriteString(out, "\n"+suggestion); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, "\033[u"); err != nil {
		return err
	}

	return nil
}

func (r *REPL) slashSuggestions(line string) []string {
	if r.slashSuggester == nil {
		return nil
	}
	if !strings.HasPrefix(line, slashCommandPrefix) {
		return nil
	}

	return r.slashSuggester(line)
}

func (r *REPL) clearInteractiveBlock() {
	_, _ = io.WriteString(writerOrDiscard(r.Out), "\033[s\033[J\033[u")
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

func (r *REPL) inputReader() *bufio.Reader {
	if r.reader == nil {
		r.reader = bufio.NewReader(readerOrDiscard(r.In))
	}

	return r.reader
}

func (r *REPL) isInteractive() bool {
	if r.interactive != nil {
		return *r.interactive
	}

	file, ok := r.In.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func (r *REPL) enterRawMode() (func(), error) {
	file, ok := r.In.(*os.File)
	if !ok {
		return func() {}, nil
	}

	state, err := makeRawTTY(file)
	if err != nil {
		return nil, err
	}

	return func() {
		_ = restoreTTY(file, state)
	}, nil
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
