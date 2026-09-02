package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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
	historyMu       sync.Mutex

	outputMu             sync.Mutex
	statusLine           string
	footer               terminalFooter
	terminalSizeProvider terminalSizeProvider
	resizeStop           chan struct{}
	resizeDone           chan struct{}
	resizeStopSignals    func()
	resizeNotifier       func() (<-chan os.Signal, func())
	terminalOutputErr    error
	runCancel            context.CancelCauseFunc
	readingInteractive   bool
	activeApproval       string
	activeApprovalInput  string
	activeStreamLine     string
}

type terminalFooter struct {
	active  bool
	enabled bool
	size    terminalSize
}

type replOutputWriter struct {
	repl *REPL
}

func (w replOutputWriter) Write(data []byte) (int, error) {
	if err := w.repl.writeContent(string(data)); err != nil {
		return 0, err
	}
	return len(data), nil
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

// OutputWriter returns a writer coordinated with interactive footer redraws.
// Command handlers use it so their output cannot interleave with a resize.
func (r *REPL) OutputWriter() io.Writer {
	return replOutputWriter{repl: r}
}

func (r *REPL) recordPromptHistoryLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	r.historyMu.Lock()
	defer r.historyMu.Unlock()

	entry := promptHistoryEntry{
		line:     line,
		sequence: len(r.promptHistory.entries),
		kind:     "prompt",
	}
	if strings.HasPrefix(strings.TrimSpace(line), slashCommandPrefix) {
		entry.kind = "slash"
	}

	r.promptHistory.entries = append(r.promptHistory.entries, entry)
	r.resetPromptHistoryNavigationLocked("")
}

func (r *REPL) resetPromptHistoryNavigation(line string) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.resetPromptHistoryNavigationLocked(line)
}

func (r *REPL) resetPromptHistoryNavigationLocked(line string) {
	r.promptHistory.cursor = len(r.promptHistory.entries)
	r.promptHistory.navigating = false
	r.promptHistory.preservedDraft = ""
	r.promptHistory.visibleLine = line
	r.promptHistory.visibleFromLive = true
}

func (r *REPL) promptHistoryOlder(currentLine string) (string, bool) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
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
	r.setPromptHistoryVisibleLineLocked(line, false)
	return line, true
}

func (r *REPL) promptHistoryNewer() (string, bool) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	if !r.promptHistory.navigating || len(r.promptHistory.entries) == 0 {
		return r.promptHistory.visibleLine, false
	}

	if r.promptHistory.cursor < len(r.promptHistory.entries)-1 {
		r.promptHistory.cursor++
		line := r.promptHistory.entries[r.promptHistory.cursor].line
		r.setPromptHistoryVisibleLineLocked(line, false)
		return line, true
	}

	line := r.promptHistory.preservedDraft
	r.resetPromptHistoryNavigationLocked(line)
	return line, true
}

func (r *REPL) setPromptHistoryVisibleLine(line string, fromLive bool) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.setPromptHistoryVisibleLineLocked(line, fromLive)
}

func (r *REPL) setPromptHistoryVisibleLineLocked(line string, fromLive bool) {
	r.promptHistory.visibleLine = line
	r.promptHistory.visibleFromLive = fromLive
}

func (r *REPL) promptHistoryVisibleLine() string {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	return r.promptHistory.visibleLine
}

// Run executes the interactive read/parse/dispatch/stream loop until EOF, /quit, or cancellation.
func (r *REPL) Run(ctx context.Context) (runErr error) {
	if r.isInteractive() {
		runCtx, cancel := context.WithCancelCause(ctx)
		ctx = runCtx
		r.setRunCancel(cancel)
		defer func() {
			r.setRunCancel(nil)
			cancel(nil)
		}()
		if err := r.startTerminalFooter(); err != nil {
			return err
		}
		defer func() {
			if err := r.stopTerminalFooter(); runErr == nil && err != nil {
				runErr = err
			}
		}()
	}

	reader := r.inputReader()

	for {
		if err := r.terminalError(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.writePrompt(); err != nil {
			return err
		}

		line, err := r.readInputLine(ctx, reader)
		if terminalErr := r.terminalError(); terminalErr != nil {
			return terminalErr
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
	r.outputMu.Lock()
	r.activeApproval = prompt
	r.activeApprovalInput = ""
	err := r.writeContentLocked(prompt)
	r.outputMu.Unlock()
	if err != nil {
		return false, err
	}
	defer func() {
		r.outputMu.Lock()
		r.activeApproval = ""
		r.activeApprovalInput = ""
		r.outputMu.Unlock()
	}()

	var line string
	if r.isInteractive() {
		line, err = r.readInteractiveApproval(ctx, r.inputReader())
	} else {
		line, err = readLine(r.inputReader())
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if err := ctx.Err(); err != nil {
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

	return r.writeContent(message)
}

func (r *REPL) updateStatusLine(message string) error {
	message = strings.TrimSpace(message)
	r.outputMu.Lock()
	defer r.outputMu.Unlock()

	r.statusLine = message
	if r.footer.enabled {
		return r.renderFooterLocked()
	}
	if r.isInteractive() && message != "" {
		return r.writeContentLocked(message + "\n")
	}
	return nil
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
	r.outputMu.Lock()
	r.activeStreamLine = ""
	r.outputMu.Unlock()
	defer func() {
		r.outputMu.Lock()
		r.activeStreamLine = ""
		r.outputMu.Unlock()
	}()

	wroteText := false
	endedWithNewline := false
	var usage *TokenUsage

	for {
		chunk, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			if wroteText && !endedWithNewline {
				if writeErr := r.writeContent("\n"); writeErr != nil {
					return writeErr
				}
			}
			if usage != nil {
				summary := formatTokenUsageSummary(*usage)
				if summary == "" {
					return nil
				}
				if r.isInteractive() {
					if err := r.updateStatusLine(summary); err != nil {
						return err
					}
				} else if err := r.WriteLocalMessage(ctx, summary); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			if wroteText && !endedWithNewline {
				if writeErr := r.writeContent("\n"); writeErr != nil {
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
		if err := r.writeAssistantText(chunk.Text); err != nil {
			return err
		}
		wroteText = true
		endedWithNewline = strings.HasSuffix(chunk.Text, "\n")
	}
}

func (r *REPL) writeAssistantText(text string) error {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	parts := strings.Split(text, "\n")
	if len(parts) == 1 {
		r.activeStreamLine += text
	} else {
		r.activeStreamLine = parts[len(parts)-1]
	}
	return r.writeContentLocked(text)
}

func formatTokenUsageSummary(usage TokenUsage) string {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 && usage.TotalTokens <= 0 {
		return ""
	}

	return fmt.Sprintf("[Tokens: %d input, %d output]", usage.InputTokens, usage.OutputTokens)
}

func (r *REPL) readInputLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	if !r.isInteractive() {
		return readLine(reader)
	}

	return r.readInteractiveLine(ctx, reader)
}

func (r *REPL) readInteractiveLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	restore, err := r.enterRawMode()
	if err != nil {
		return "", err
	}
	defer restore()
	r.setReadingInteractive(true)
	defer r.setReadingInteractive(false)

	line := ""
	r.setPromptHistoryVisibleLine(line, true)

	for {
		ch, _, err := r.readInteractiveRune(ctx, reader)
		if err != nil {
			if clearErr := r.clearInteractiveBlock(); clearErr != nil {
				return line, clearErr
			}
			return line, err
		}

		switch ch {
		case '\n':
			if err := r.clearInteractiveBlock(); err != nil {
				return "", err
			}
			if err := r.writeContent("\n"); err != nil {
				return "", err
			}
			return line, nil
		case '\r':
			continue
		case 0x1b:
			updatedLine, handled, err := r.readInteractiveEscapeSequence(ctx, reader, line)
			if err != nil {
				if clearErr := r.clearInteractiveBlock(); clearErr != nil {
					return "", clearErr
				}
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

func (r *REPL) readInteractiveEscapeSequence(ctx context.Context, reader *bufio.Reader, currentLine string) (string, bool, error) {
	next, _, err := r.readInteractiveRune(ctx, reader)
	if err != nil {
		return "", false, err
	}
	if next != '[' {
		return currentLine, false, nil
	}

	code, _, err := r.readInteractiveRune(ctx, reader)
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

func (r *REPL) readInteractiveApproval(ctx context.Context, reader *bufio.Reader) (string, error) {
	restore, err := r.enterRawMode()
	if err != nil {
		return "", err
	}
	defer restore()

	line := ""
	for {
		ch, _, err := r.readInteractiveRune(ctx, reader)
		if err != nil {
			return line, err
		}

		switch ch {
		case '\n':
			if err := r.writeContent("\n"); err != nil {
				return "", err
			}
			return line, nil
		case '\r':
			continue
		case 0x7f, '\b':
			text := []rune(line)
			if len(text) > 0 {
				line = string(text[:len(text)-1])
			}
		default:
			line += string(ch)
		}

		r.outputMu.Lock()
		r.activeApprovalInput = line
		err = r.writeContentLocked("\r\033[2K" + r.activeApproval + line)
		r.outputMu.Unlock()
		if err != nil {
			return "", err
		}
	}
}

func (r *REPL) readInteractiveRune(ctx context.Context, reader *bufio.Reader) (rune, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if file := r.inputFile(); file != nil && reader.Buffered() == 0 {
		if err := waitForTerminalInput(ctx, file); err != nil {
			return 0, 0, err
		}
	}
	if err := r.terminalError(); err != nil {
		return 0, 0, err
	}
	ch, size, err := reader.ReadRune()
	if err != nil {
		return 0, 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if err := r.terminalError(); err != nil {
		return 0, 0, err
	}
	return ch, size, nil
}

func (r *REPL) renderInteractiveLine(line string) error {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.renderInteractiveLineLocked(line)
}

func (r *REPL) renderInteractiveLineLocked(line string) error {
	var rendered strings.Builder
	rendered.WriteString("\r\033[2K")
	rendered.WriteString(r.Prompt)
	rendered.WriteString(line)
	rendered.WriteString("\033[s\033[J")
	suggestions := r.slashSuggestions(line)
	for _, suggestion := range suggestions {
		rendered.WriteString("\n")
		rendered.WriteString(suggestion)
	}
	rendered.WriteString("\033[u")
	return r.writeContentLocked(rendered.String())
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

func (r *REPL) clearInteractiveBlock() error {
	return r.writeContent("\033[s\033[J\033[u")
}

func (r *REPL) writePrompt() error {
	if r.isInteractive() {
		r.setPromptHistoryVisibleLine("", true)
		return r.renderInteractiveLine("")
	}

	return r.writeContent(r.Prompt)
}

func (r *REPL) writeError(err error) error {
	if err == nil {
		return nil
	}

	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	_, writeErr := fmt.Fprintf(writerOrDiscard(r.Err), "goagent: %v\n", err)
	return writeErr
}

func (r *REPL) writeContent(text string) error {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.writeContentLocked(text)
}

func (r *REPL) writeContentLocked(text string) error {
	if text == "" {
		return nil
	}
	if _, err := io.WriteString(writerOrDiscard(r.Out), text); err != nil {
		return err
	}

	// Interactive redraws use ESC[J to clear suggestion lines. Redraw the
	// footer after every coordinated content write so that sequence can never
	// erase a pinned token status.
	if r.footer.enabled {
		return r.renderFooterLocked()
	}
	return nil
}

func (r *REPL) startTerminalFooter() error {
	r.outputMu.Lock()
	if r.footer.active {
		r.outputMu.Unlock()
		return nil
	}
	r.footer.active = true
	err := r.configureFooterLocked(false)
	r.outputMu.Unlock()
	if err != nil {
		return errors.Join(err, r.stopTerminalFooter())
	}

	if r.outputFile() == nil && r.resizeNotifier == nil {
		return nil
	}

	notifier := r.resizeNotifier
	if notifier == nil {
		notifier = newResizeNotifier
	}
	resizeSignals, stopSignals := notifier()
	if resizeSignals == nil {
		return nil
	}
	r.resizeStop = make(chan struct{})
	r.resizeDone = make(chan struct{})
	r.resizeStopSignals = stopSignals
	go func() {
		defer close(r.resizeDone)
		for {
			select {
			case <-r.resizeStop:
				return
			case <-resizeSignals:
				_ = r.handleTerminalResize()
			}
		}
	}()
	return nil
}

func (r *REPL) stopTerminalFooter() error {
	// Stop and join before changing terminal state so a late SIGWINCH cannot
	// repaint a footer after cleanup.
	if r.resizeStop != nil {
		if r.resizeStopSignals != nil {
			r.resizeStopSignals()
		}
		close(r.resizeStop)
		<-r.resizeDone
		r.resizeStop = nil
		r.resizeDone = nil
		r.resizeStopSignals = nil
	}

	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	if !r.footer.active {
		return nil
	}
	var cleanupErr error
	if r.footer.enabled {
		_, cleanupErr = io.WriteString(writerOrDiscard(r.Out), fmt.Sprintf("\033[s\033[%d;1H\033[2K\033[r\033[u\r\n", r.footer.size.Height))
	} else {
		_, cleanupErr = io.WriteString(writerOrDiscard(r.Out), "\r\n")
	}
	r.footer = terminalFooter{}
	return cleanupErr
}

func (r *REPL) handleTerminalResize() error {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	if !r.footer.active {
		return nil
	}
	if err := r.configureFooterLocked(true); err != nil {
		r.terminalOutputErr = err
		if r.runCancel != nil {
			r.runCancel(err)
		}
		if r.resizeStopSignals != nil {
			r.resizeStopSignals()
		}
		// The writer may already be unusable, but restoring the terminal is
		// still worthwhile. Do it here because the foreground read can remain
		// blocked until the user next types; do not create a competing reader.
		_, _ = io.WriteString(writerOrDiscard(r.Out), "\033[r\r\n")
		r.footer = terminalFooter{}
		return err
	}
	return nil
}

func (r *REPL) configureFooterLocked(redraw bool) error {
	previous := r.footer
	size, usable := r.lookupTerminalSize()
	shrink := redraw && previous.enabled && usable && (size.Height < previous.size.Height || size.Width < previous.size.Width)
	if shrink {
		if err := r.clearActiveContentLocked(); err != nil {
			return err
		}
	}
	if previous.enabled {
		if _, err := io.WriteString(writerOrDiscard(r.Out), fmt.Sprintf("\033[s\033[%d;1H\033[2K\033[r\033[u", previous.size.Height)); err != nil {
			return err
		}
	}

	r.footer.enabled = usable && size.Width >= 1 && size.Height >= 3
	r.footer.size = size
	if !r.footer.enabled {
		if previous.enabled && r.statusLine != "" {
			if _, err := io.WriteString(writerOrDiscard(r.Out), "\r\n"+r.statusLine+"\n"); err != nil {
				return err
			}
			if r.readingInteractive {
				return r.renderInteractiveLineLocked(r.promptHistoryVisibleLine())
			}
		}
		return nil
	}

	if redraw {
		// DECSTBM homes the cursor, and terminals can clamp a saved cursor to
		// the new physical bottom before SIGWINCH is delivered. Re-anchor at the
		// final scrollable row so no subsequent content can collide with the
		// reserved footer row.
		sequence := fmt.Sprintf("\033[s\033[1;%dr\033[u", size.Height-1)
		if shrink {
			sequence += fmt.Sprintf("\033[%d;1H", size.Height-1)
		}
		if _, err := io.WriteString(writerOrDiscard(r.Out), sequence); err != nil {
			return err
		}
	} else {
		// First scroll the shell's current command into the transcript. Without
		// this, clearing the new footer row would erase whatever was previously
		// printed at the terminal bottom. Then anchor the REPL at the final
		// scrollable row, not row 1 over the transcript.
		if _, err := io.WriteString(writerOrDiscard(r.Out), fmt.Sprintf("\r\n\033[1;%dr\033[%d;1H", size.Height-1, size.Height-1)); err != nil {
			return err
		}
	}
	if !shrink {
		return r.renderFooterLocked()
	}
	if r.readingInteractive {
		return r.renderInteractiveLineLocked(r.promptHistoryVisibleLine())
	}
	if r.activeApproval != "" {
		return r.writeContentLocked("\r\033[2K" + r.activeApproval + r.activeApprovalInput)
	}
	if r.activeStreamLine != "" {
		return r.writeContentLocked("\r\033[2K" + r.activeStreamLine)
	}
	return r.renderFooterLocked()
}

func (r *REPL) clearActiveContentLocked() error {
	if r.readingInteractive {
		_, err := io.WriteString(writerOrDiscard(r.Out), "\r\033[2K\033[s\033[J\033[u")
		return err
	}
	if r.activeApproval != "" || r.activeStreamLine != "" {
		_, err := io.WriteString(writerOrDiscard(r.Out), "\r\033[2K")
		return err
	}
	return nil
}

func (r *REPL) terminalError() error {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.terminalOutputErr
}

func (r *REPL) setReadingInteractive(reading bool) {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	r.readingInteractive = reading
}

func (r *REPL) setRunCancel(cancel context.CancelCauseFunc) {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	r.runCancel = cancel
}

func (r *REPL) lookupTerminalSize() (terminalSize, bool) {
	provider := r.terminalSizeProvider
	if provider == nil {
		provider = currentTerminalSize
	}
	return provider(r.outputFile())
}

func (r *REPL) outputFile() *os.File {
	file, _ := r.Out.(*os.File)
	return file
}

func (r *REPL) inputFile() *os.File {
	file, _ := r.In.(*os.File)
	return file
}

func (r *REPL) renderFooterLocked() error {
	if !r.footer.enabled {
		return nil
	}

	// Reserve one column where possible: writing exactly to the final column can
	// auto-wrap on many terminals. A one-column footer temporarily disables
	// autowrap so it can still show a visible truncated marker safely.
	width := r.footer.size.Width - 1
	oneColumn := r.footer.size.Width == 1
	if oneColumn {
		width = 1
	}
	status := truncateTerminalText(r.statusLine, width)
	prefix, suffix := "", ""
	if oneColumn {
		prefix, suffix = "\033[?7l", "\033[?7h"
	}
	_, err := io.WriteString(writerOrDiscard(r.Out), fmt.Sprintf("\033[s\033[%d;1H\033[2K%s%s%s\033[u", r.footer.size.Height, prefix, status, suffix))
	return err
}

func truncateTerminalText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
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
