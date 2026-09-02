package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	interactiveArrowUp   = "\x1b[A"
	interactiveArrowDown = "\x1b[B"
)

func interactiveInput(parts ...string) *strings.Reader {
	return strings.NewReader(strings.Join(parts, ""))
}

func interactiveSubmit() string {
	return "\n"
}

func interactiveBackspace() string {
	return "\x7f"
}

func assertOutputContainsAll(t *testing.T, output string, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q: %q", fragment, output)
		}
	}
}

func assertOutputContainsInOrder(t *testing.T, output string, fragments ...string) {
	t.Helper()

	nextIndex := 0
	for _, fragment := range fragments {
		index := strings.Index(output[nextIndex:], fragment)
		if index < 0 {
			t.Fatalf("output missing ordered fragment %q after byte %d: %q", fragment, nextIndex, output)
		}
		nextIndex += index + len(fragment)
	}
}

func assertInteractiveLineRendered(t *testing.T, output, prompt, line string) {
	t.Helper()

	assertOutputContainsAll(t, output, "\r\033[2K"+prompt+line+"\033[s\033[J", "\033[u")
}

type terminalCursor struct {
	row    int
	column int
}

// cursorPositionsForMarkers is a deliberately small terminal-state model for
// the cursor commands emitted by the footer. In particular, DECSTBM (CSI r)
// homes the cursor and changes the usable height, while CSI s/u can restore a
// position which has since been clamped by a terminal resize.
func cursorPositionsForMarkers(output string, initialRow int, markers ...string) map[string]terminalCursor {
	positions := make(map[string]terminalCursor, len(markers))
	row, column, savedRow, savedColumn, height := initialRow, 1, initialRow, 1, initialRow

	for len(output) > 0 {
		if strings.HasPrefix(output, "\033[s") {
			savedRow, savedColumn = row, column
			output = output[len("\033[s"):]
			continue
		}
		if strings.HasPrefix(output, "\033[u") {
			row, column = min(savedRow, height), savedColumn
			output = output[len("\033[u"):]
			continue
		}
		if strings.HasPrefix(output, "\033[") {
			end := strings.IndexAny(output[2:], "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~")
			if end >= 0 {
				end += 2
				params, command := output[2:end], output[end]
				switch command {
				case 'r':
					parts := strings.Split(params, ";")
					if len(parts) == 2 {
						if bottom, err := strconv.Atoi(parts[1]); err == nil && bottom > 0 {
							height = bottom + 1
						}
					}
					row, column = 1, 1
				case 'H':
					parts := strings.Split(params, ";")
					if value, err := strconv.Atoi(parts[0]); err == nil && value > 0 {
						row = min(value, height)
					}
					if len(parts) == 2 {
						if value, err := strconv.Atoi(parts[1]); err == nil && value > 0 {
							column = value
						}
					}
				}
				output = output[end+1:]
				continue
			}
		}

		next := strings.Index(output, "\033")
		segment := output
		if next >= 0 {
			segment = output[:next]
		}
		for _, marker := range markers {
			if strings.Contains(segment, marker) {
				positions[marker] = terminalCursor{row: row, column: column}
			}
		}
		for _, ch := range segment {
			switch ch {
			case '\r':
				column = 1
			case '\n':
				row, column = min(row+1, height), 1
			default:
				column++
			}
		}
		if next < 0 {
			break
		}
		output = output[next:]
	}

	return positions
}

func cursorRowsForMarkers(output string, initialRow int, markers ...string) map[string]int {
	positions := cursorPositionsForMarkers(output, initialRow, markers...)
	rows := make(map[string]int, len(positions))
	for marker, position := range positions {
		rows[marker] = position.row
	}
	return rows
}

func visibleMarkerCount(output string, initialRow int, marker string) int {
	lines := map[int][]rune{}
	row, column, savedRow, savedColumn, height := initialRow, 1, initialRow, 1, initialRow
	for len(output) > 0 {
		if strings.HasPrefix(output, "\033[s") {
			savedRow, savedColumn, output = row, column, output[len("\033[s"):]
			continue
		}
		if strings.HasPrefix(output, "\033[u") {
			row, column, output = min(savedRow, height), savedColumn, output[len("\033[u"):]
			continue
		}
		if strings.HasPrefix(output, "\033[") {
			end := strings.IndexAny(output[2:], "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~")
			if end >= 0 {
				end += 2
				params, command := output[2:end], output[end]
				switch command {
				case 'r':
					parts := strings.Split(params, ";")
					if len(parts) == 2 {
						if bottom, err := strconv.Atoi(parts[1]); err == nil && bottom > 0 {
							height = bottom + 1
						}
					}
					row, column = 1, 1
				case 'H':
					parts := strings.Split(params, ";")
					if value, err := strconv.Atoi(parts[0]); err == nil && value > 0 {
						row = min(value, height)
					}
					if len(parts) == 2 {
						if value, err := strconv.Atoi(parts[1]); err == nil && value > 0 {
							column = value
						}
					}
				case 'K':
					lines[row] = nil
				}
				output = output[end+1:]
				continue
			}
		}
		next := strings.Index(output, "\033")
		segment := output
		if next >= 0 {
			segment = output[:next]
		}
		for _, ch := range segment {
			switch ch {
			case '\r':
				column = 1
			case '\n':
				row, column = min(row+1, height), 1
			default:
				line := lines[row]
				for len(line) < column {
					line = append(line, ' ')
				}
				line[column-1] = ch
				lines[row] = line
				column++
			}
		}
		if next < 0 {
			break
		}
		output = output[next:]
	}

	count := 0
	for _, line := range lines {
		count += strings.Count(string(line), marker)
	}
	return count
}

type fixedAssistantStream struct{}

func (fixedAssistantStream) Recv(context.Context) (AssistantChunk, error) {
	return AssistantChunk{}, io.EOF
}

type sliceAssistantStream struct {
	chunks []AssistantChunk
	index  int
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (s *sliceAssistantStream) Recv(context.Context) (AssistantChunk, error) {
	if s.index >= len(s.chunks) {
		return AssistantChunk{}, io.EOF
	}

	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func TestREPLInteractiveSlashSuggestions(t *testing.T) {
	t.Parallel()

	var (
		out     bytes.Buffer
		errOut  bytes.Buffer
		handled []string
	)

	repl := NewREPL(interactiveInput("/tools", interactiveSubmit()), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetSlashSuggester(func(line string) []string {
		matches := SuggestSlashCommands(line)
		items := make([]string, 0, len(matches))
		for _, match := range matches {
			items = append(items, match.Usage+" - "+match.Description)
		}
		return items
	})
	repl.SetCommandExecutor(CommandExecutorFunc(func(_ context.Context, input ParsedInput) error {
		handled = append(handled, input.Command.CommandName())
		return nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(handled) != 1 || handled[0] != "tools" {
		t.Fatalf("handled commands = %#v, want [tools]", handled)
	}

	rendered := out.String()
	assertInteractiveLineRendered(t, rendered, repl.Prompt, "/tools")
	assertOutputContainsInOrder(t, rendered,
		"\r\033[2K"+repl.Prompt+"/tools\033[s\033[J",
		"\n/tools - Show the active built-in and module tools.",
		"\033[u",
	)
}

func TestREPLNonInteractiveKeepsLineBufferedFallback(t *testing.T) {
	t.Parallel()

	var (
		out     bytes.Buffer
		errOut  bytes.Buffer
		handled int
	)

	repl := NewREPL(strings.NewReader("/tools\n"), &out, &errOut)
	repl.SetInteractive(false)
	repl.SetSlashSuggester(func(line string) []string {
		matches := SuggestSlashCommands(line)
		items := make([]string, 0, len(matches))
		for _, match := range matches {
			items = append(items, match.Usage+" - "+match.Description)
		}
		return items
	})
	repl.SetCommandExecutor(CommandExecutorFunc(func(_ context.Context, input ParsedInput) error {
		handled++
		return nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if handled != 1 {
		t.Fatalf("handled = %d, want 1", handled)
	}
	if strings.Contains(out.String(), "/tools - Show the active built-in and module tools.") {
		t.Fatalf("non-interactive output unexpectedly rendered slash suggestions: %q", out.String())
	}
}

func TestREPLPromptApproval(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	repl := NewREPL(strings.NewReader("yes\n"), &out, &out)
	approved, err := repl.PromptApproval(context.Background(), `Approve running "pwd" in /workspace? [y/N] `)
	if err != nil {
		t.Fatalf("PromptApproval() error = %v", err)
	}
	if !approved {
		t.Fatal("PromptApproval() = false, want true")
	}
	if !strings.Contains(out.String(), `Approve running "pwd" in /workspace? [y/N] `) {
		t.Fatalf("PromptApproval() output missing prompt: %q", out.String())
	}
}

func TestREPLPromptHistoryRecallOlderEntries(t *testing.T) {
	t.Parallel()

	var (
		out         bytes.Buffer
		errOut      bytes.Buffer
		submissions []string
	)

	repl := NewREPL(interactiveInput(
		"first prompt", interactiveSubmit(),
		"second prompt", interactiveSubmit(),
		interactiveArrowUp,
		interactiveArrowUp,
		interactiveArrowUp,
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		submissions = append(submissions, prompt)
		return fixedAssistantStream{}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := strings.Join(submissions, "|"), "first prompt|second prompt|first prompt"; got != want {
		t.Fatalf("submitted prompts = %q, want %q", got, want)
	}

	rendered := out.String()
	assertOutputContainsInOrder(t, rendered,
		"\r\033[2K"+repl.Prompt+"second prompt\033[s\033[J",
		"\r\033[2K"+repl.Prompt+"first prompt\033[s\033[J",
	)
}

func TestREPLPromptHistoryEmptyHistoryDoesNotCorruptDraft(t *testing.T) {
	t.Parallel()

	var (
		out         bytes.Buffer
		errOut      bytes.Buffer
		submissions []string
	)

	repl := NewREPL(interactiveInput(
		interactiveArrowUp,
		"draft survives",
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		submissions = append(submissions, prompt)
		return fixedAssistantStream{}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := strings.Join(submissions, "|"), "draft survives"; got != want {
		t.Fatalf("submitted prompts = %q, want %q", got, want)
	}
	assertInteractiveLineRendered(t, out.String(), repl.Prompt, "draft survives")
}

func TestREPLPromptHistoryRestoresDraftAtNewestBoundary(t *testing.T) {
	t.Parallel()

	var (
		out         bytes.Buffer
		errOut      bytes.Buffer
		submissions []string
	)

	repl := NewREPL(interactiveInput(
		"alpha", interactiveSubmit(),
		"beta", interactiveSubmit(),
		"draft that should come back",
		interactiveArrowUp,
		interactiveArrowDown,
		interactiveArrowDown,
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		submissions = append(submissions, prompt)
		return fixedAssistantStream{}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := strings.Join(submissions, "|"), "alpha|beta|draft that should come back"; got != want {
		t.Fatalf("submitted prompts = %q, want %q", got, want)
	}

	rendered := out.String()
	assertOutputContainsInOrder(t, rendered,
		"\r\033[2K"+repl.Prompt+"draft that should come back\033[s\033[J",
		"\r\033[2K"+repl.Prompt+"beta\033[s\033[J",
		"\r\033[2K"+repl.Prompt+"draft that should come back\033[s\033[J",
	)
}

func TestREPLPromptHistoryRecalledSlashCommandStaysLocal(t *testing.T) {
	t.Parallel()

	var (
		out      bytes.Buffer
		errOut   bytes.Buffer
		commands []string
	)

	repl := NewREPL(interactiveInput(
		"/tools", interactiveSubmit(),
		interactiveArrowUp,
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetSlashSuggester(func(line string) []string {
		matches := SuggestSlashCommands(line)
		items := make([]string, 0, len(matches))
		for _, match := range matches {
			items = append(items, match.Usage+" - "+match.Description)
		}
		return items
	})
	repl.SetCommandExecutor(CommandExecutorFunc(func(_ context.Context, input ParsedInput) error {
		commands = append(commands, input.Command.CommandName())
		return nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := strings.Join(commands, "|"), "tools|tools"; got != want {
		t.Fatalf("handled commands = %q, want %q", got, want)
	}
	assertOutputContainsInOrder(t, out.String(),
		"\n/tools - Show the active built-in and module tools.",
		"\r\033[2K"+repl.Prompt+"/tools\033[s\033[J",
		"\n/tools - Show the active built-in and module tools.",
	)
}

func TestREPLPromptHistoryEditedResubmissionPreservesEarlierEntries(t *testing.T) {
	t.Parallel()

	var (
		out         bytes.Buffer
		errOut      bytes.Buffer
		submissions []string
	)

	repl := NewREPL(interactiveInput(
		"original prompt", interactiveSubmit(),
		interactiveArrowUp,
		" updated",
		interactiveSubmit(),
		interactiveArrowUp,
		interactiveArrowUp,
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		submissions = append(submissions, prompt)
		return fixedAssistantStream{}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := strings.Join(submissions, "|"), "original prompt|original prompt updated|original prompt"; got != want {
		t.Fatalf("submitted prompts = %q, want %q", got, want)
	}

	assertOutputContainsInOrder(t, out.String(),
		"\r\033[2K"+repl.Prompt+"original prompt updated\033[s\033[J",
		"\r\033[2K"+repl.Prompt+"original prompt\033[s\033[J",
	)
}

func TestREPLPromptHistoryBackspaceEditsRecalledLongLine(t *testing.T) {
	t.Parallel()

	var (
		out         bytes.Buffer
		errOut      bytes.Buffer
		submissions []string
	)

	longLine := strings.Repeat("history-segment-", 8)

	repl := NewREPL(interactiveInput(
		longLine, interactiveSubmit(),
		interactiveArrowUp,
		interactiveBackspace(),
		"X",
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		submissions = append(submissions, prompt)
		return fixedAssistantStream{}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantEdited := longLine[:len(longLine)-1] + "X"
	if got, want := strings.Join(submissions, "|"), longLine+"|"+wantEdited; got != want {
		t.Fatalf("submitted prompts = %q, want %q", got, want)
	}
	assertInteractiveLineRendered(t, out.String(), repl.Prompt, wantEdited)
}

func TestREPLPrintsTokenUsageAfterAssistantStream(t *testing.T) {
	t.Parallel()

	var (
		out    bytes.Buffer
		errOut bytes.Buffer
	)

	repl := NewREPL(strings.NewReader("hello\n"), &out, &errOut)
	repl.SetInteractive(false)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		if prompt != "hello" {
			t.Fatalf("prompt = %q, want hello", prompt)
		}

		return &sliceAssistantStream{chunks: []AssistantChunk{
			{Text: "assistant reply"},
			{Usage: &TokenUsage{InputTokens: 120, OutputTokens: 45, TotalTokens: 165}},
		}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertOutputContainsInOrder(t, out.String(),
		"> assistant reply\n",
		"[Tokens: 120 input, 45 output]\n",
	)
}

func setTerminalSize(repl *REPL, size *terminalSize) {
	repl.terminalSizeProvider = func(*os.File) (terminalSize, bool) {
		return *size, true
	}
}

func TestREPLInteractivePinsTokenUsageToTerminalFooter(t *testing.T) {
	t.Parallel()

	var (
		out    bytes.Buffer
		errOut bytes.Buffer
	)

	repl := NewREPL(interactiveInput(
		"hello",
		interactiveSubmit(),
	), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		if prompt != "hello" {
			t.Fatalf("prompt = %q, want hello", prompt)
		}

		return &sliceAssistantStream{chunks: []AssistantChunk{
			{Text: "assistant reply"},
			{Usage: &TokenUsage{InputTokens: 120, OutputTokens: 45, TotalTokens: 165}},
		}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	rendered := out.String()
	assertOutputContainsInOrder(t, rendered,
		"\033[1;23r",
		"\033[24;1H\033[2K",
		"\r\033[2K"+repl.Prompt+"hello\033[s\033[J",
		"assistant reply",
		"\033[24;1H\033[2K[Tokens: 120 input, 45 output]",
		"\033[24;1H\033[2K\033[r",
	)
	if strings.Contains(rendered, "\n[Tokens: 120 input, 45 output]\n") {
		t.Fatalf("interactive output rendered token usage only as trailing message: %q", rendered)
	}
	if strings.Contains(rendered, repl.Prompt+" [Tokens:") {
		t.Fatalf("interactive output attached token usage to the prompt: %q", rendered)
	}
}

func TestREPLInteractiveFooterPersistsUntilNewUsageArrives(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(interactiveInput("first", interactiveSubmit(), "second", interactiveSubmit()), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)

	turn := 0
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(_ context.Context, prompt string) (AssistantStream, error) {
		turn++
		if prompt == "first" {
			return &sliceAssistantStream{chunks: []AssistantChunk{{Text: "first reply"}, {Usage: &TokenUsage{InputTokens: 10, OutputTokens: 2}}}}, nil
		}
		return &sliceAssistantStream{chunks: []AssistantChunk{{Text: "second reply"}, {Usage: &TokenUsage{InputTokens: 20, OutputTokens: 3}}}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn != 2 {
		t.Fatalf("turns = %d, want 2", turn)
	}
	assertOutputContainsInOrder(t, out.String(),
		"\033[24;1H\033[2K[Tokens: 10 input, 2 output]",
		"second reply",
		"\033[24;1H\033[2K[Tokens: 20 input, 3 output]",
	)
}

func TestREPLInteractiveWithoutTerminalGeometryFallsBackToStandaloneUsage(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(interactiveInput("hello", interactiveSubmit()), &out, &errOut)
	repl.SetInteractive(true)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(context.Context, string) (AssistantStream, error) {
		return &sliceAssistantStream{chunks: []AssistantChunk{{Text: "reply"}, {Usage: &TokenUsage{InputTokens: 3, OutputTokens: 1}}}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "reply\n[Tokens: 3 input, 1 output]\n") {
		t.Fatalf("interactive geometry fallback did not print standalone usage: %q", out.String())
	}
}

func TestREPLInteractiveFooterReconfiguresOnResizeAndTruncates(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)

	repl.startTerminalFooter()
	defer repl.stopTerminalFooter()
	if err := repl.updateStatusLine("[Tokens: 120 input, 45 output]"); err != nil {
		t.Fatalf("updateStatusLine() error = %v", err)
	}

	size = terminalSize{Width: 15, Height: 30}
	repl.handleTerminalResize()
	size = terminalSize{Width: 15, Height: 2}
	repl.handleTerminalResize()
	size = terminalSize{Width: 80, Height: 24}
	repl.handleTerminalResize()

	rendered := out.String()
	assertOutputContainsInOrder(t, rendered,
		"\033[1;23r",
		"\033[24;1H\033[2K[Tokens: 120 input, 45 output]",
		"\033[24;1H\033[2K\033[r",
		"\033[1;29r",
		"\033[30;1H\033[2K[Tokens: 120 …",
		"\033[30;1H\033[2K\033[r",
		"\033[1;23r",
		"\033[24;1H\033[2K[Tokens: 120 input, 45 output]",
	)
	if got := truncateTerminalText(strings.Repeat("x", 15), 15); len([]rune(got)) != 15 {
		t.Fatalf("truncated text width = %d, want 15", len([]rune(got)))
	}
}

func TestREPLFooterAttachAndResizePreserveContentCursor(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()

	if err := repl.writeContent("before-resize"); err != nil {
		t.Fatalf("writeContent(before) error = %v", err)
	}
	size = terminalSize{Width: 80, Height: 30}
	if err := repl.handleTerminalResize(); err != nil {
		t.Fatalf("handleTerminalResize() error = %v", err)
	}
	if _, err := repl.OutputWriter().Write([]byte("after-resize")); err != nil {
		t.Fatalf("OutputWriter().Write() error = %v", err)
	}

	size = terminalSize{Width: 80, Height: 20}
	if err := repl.handleTerminalResize(); err != nil {
		t.Fatalf("handleTerminalResize(shrink) error = %v", err)
	}
	if _, err := repl.OutputWriter().Write([]byte("after-shrink")); err != nil {
		t.Fatalf("OutputWriter().Write(shrink) error = %v", err)
	}

	if !strings.Contains(out.String(), "\r\n\033[1;23r") {
		t.Fatalf("initial attach did not scroll the shell transcript before reserving the footer: %q", out.String())
	}
	rows := cursorRowsForMarkers(out.String(), 24, "before-resize", "after-resize", "after-shrink")
	if rows["before-resize"] != 23 {
		t.Fatalf("initial content row = %d, want 23", rows["before-resize"])
	}
	if rows["after-resize"] != 23 {
		t.Fatalf("expanded content row = %d, want preserved row 23", rows["after-resize"])
	}
	if rows["after-shrink"] != 19 {
		t.Fatalf("shrunken content row = %d, want safe row 19", rows["after-shrink"])
	}
}

func TestREPLResizeRedrawsApprovalAndPartialStreamAtSafeCursor(t *testing.T) {
	t.Parallel()

	t.Run("approval", func(t *testing.T) {
		var out, errOut bytes.Buffer
		repl := NewREPL(strings.NewReader(""), &out, &errOut)
		repl.SetInteractive(true)
		size := terminalSize{Width: 80, Height: 24}
		setTerminalSize(repl, &size)
		if err := repl.startTerminalFooter(); err != nil {
			t.Fatalf("startTerminalFooter() error = %v", err)
		}
		defer repl.stopTerminalFooter()
		repl.outputMu.Lock()
		repl.activeApproval = "Approve command? [y/N] "
		repl.activeApprovalInput = "ye"
		repl.outputMu.Unlock()
		size = terminalSize{Width: 80, Height: 20}
		if err := repl.handleTerminalResize(); err != nil {
			t.Fatalf("handleTerminalResize() error = %v", err)
		}
		position := cursorPositionsForMarkers(out.String(), 24, "Approve command? [y/N] ye")["Approve command? [y/N] ye"]
		if position != (terminalCursor{row: 19, column: 1}) {
			t.Fatalf("approval cursor = %+v, want row 19 column 1", position)
		}
		if count := visibleMarkerCount(out.String(), 24, "Approve command? [y/N] ye"); count != 1 {
			t.Fatalf("visible approval copies = %d, want 1", count)
		}
	})

	t.Run("partial stream", func(t *testing.T) {
		var out, errOut bytes.Buffer
		repl := NewREPL(strings.NewReader(""), &out, &errOut)
		repl.SetInteractive(true)
		size := terminalSize{Width: 80, Height: 24}
		setTerminalSize(repl, &size)
		if err := repl.startTerminalFooter(); err != nil {
			t.Fatalf("startTerminalFooter() error = %v", err)
		}
		defer repl.stopTerminalFooter()
		if err := repl.writeAssistantText("partial assistant text"); err != nil {
			t.Fatalf("writeAssistantText() error = %v", err)
		}
		size = terminalSize{Width: 80, Height: 20}
		if err := repl.handleTerminalResize(); err != nil {
			t.Fatalf("handleTerminalResize() error = %v", err)
		}
		position := cursorPositionsForMarkers(out.String(), 24, "partial assistant text")["partial assistant text"]
		if position != (terminalCursor{row: 19, column: 1}) {
			t.Fatalf("partial stream cursor = %+v, want row 19 column 1", position)
		}
		if count := visibleMarkerCount(out.String(), 24, "partial assistant text"); count != 1 {
			t.Fatalf("visible partial stream copies = %d, want 1", count)
		}
	})
}

func TestREPLPromptHistoryAndResizeAreSynchronized(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()
	repl.recordPromptHistoryLine("first")
	repl.recordPromptHistoryLine("second")
	repl.setReadingInteractive(true)
	defer repl.setReadingInteractive(false)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		line := "draft"
		for range 100 {
			line, _ = repl.promptHistoryOlder(line)
			repl.setPromptHistoryVisibleLine(line, true)
			line, _ = repl.promptHistoryNewer()
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			if err := repl.handleTerminalResize(); err != nil {
				t.Errorf("handleTerminalResize() error = %v", err)
			}
		}
	}()
	wg.Wait()
}

func TestREPLInteractiveFooterSurvivesHistoryAndSuggestions(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(interactiveInput(
		"hello", interactiveSubmit(),
		interactiveArrowUp,
		strings.Repeat("\b", len("hello")),
		"/",
	), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	repl.SetSlashSuggester(func(string) []string { return []string{"/tools - Show tools"} })
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(context.Context, string) (AssistantStream, error) {
		return &sliceAssistantStream{chunks: []AssistantChunk{{Usage: &TokenUsage{InputTokens: 8, OutputTokens: 2}}}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertOutputContainsInOrder(t, out.String(),
		"\033[24;1H\033[2K[Tokens: 8 input, 2 output]",
		"/tools - Show tools",
		"\033[24;1H\033[2K[Tokens: 8 input, 2 output]",
	)
}

func TestREPLInteractiveFooterSurvivesApprovalPrompt(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	repl := NewREPL(strings.NewReader("yes\n"), &out, &out)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()
	if err := repl.updateStatusLine("[Tokens: 8 input, 2 output]"); err != nil {
		t.Fatalf("updateStatusLine() error = %v", err)
	}

	approved, err := repl.PromptApproval(context.Background(), "Approve command? [y/N] ")
	if err != nil || !approved {
		t.Fatalf("PromptApproval() = (%t, %v), want (true, nil)", approved, err)
	}
	assertOutputContainsInOrder(t, out.String(),
		"Approve command? [y/N] ",
		"\033[24;1H\033[2K[Tokens: 8 input, 2 output]",
	)
}

func TestREPLFooterAvoidsAttachmentAtOneColumn(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 1, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()
	if err := repl.updateStatusLine("[Tokens: 8 input, 2 output]"); err != nil {
		t.Fatalf("updateStatusLine() error = %v", err)
	}
	if !strings.Contains(out.String(), "\033[1;23r") {
		t.Fatalf("one-column terminal did not reserve a footer row: %q", out.String())
	}
	if strings.Contains(out.String(), "[Tokens: 8 input, 2 output]\n") {
		t.Fatalf("one-column terminal wrapped usage instead of truncating it: %q", out.String())
	}
	if !strings.Contains(out.String(), "\033[?7l…\033[?7h") {
		t.Fatalf("one-column terminal did not render a visible no-wrap marker: %q", out.String())
	}
}

func TestREPLFooterToFallbackRendersStatusOnceAndRedrawsInput(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()
	repl.setPromptHistoryVisibleLine("draft", true)
	repl.setReadingInteractive(true)
	defer repl.setReadingInteractive(false)
	if err := repl.updateStatusLine("[Tokens: 8 input, 2 output]"); err != nil {
		t.Fatalf("updateStatusLine() error = %v", err)
	}

	size = terminalSize{Width: 80, Height: 2}
	if err := repl.handleTerminalResize(); err != nil {
		t.Fatalf("handleTerminalResize() error = %v", err)
	}
	if err := repl.handleTerminalResize(); err != nil {
		t.Fatalf("repeated fallback resize error = %v", err)
	}

	rendered := out.String()
	if count := strings.Count(rendered, "[Tokens: 8 input, 2 output]\n"); count != 1 {
		t.Fatalf("fallback status count = %d, want 1: %q", count, rendered)
	}
	if !strings.Contains(rendered, "\r\033[2K"+repl.Prompt+"draft\033[s\033[J") {
		t.Fatalf("fallback did not redraw active input: %q", rendered)
	}
}

func TestREPLFooterSerializesConcurrentWriterAndResize(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()
	if err := repl.updateStatusLine("[Tokens: 8 input, 2 output]"); err != nil {
		t.Fatalf("updateStatusLine() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := repl.OutputWriter().Write([]byte("command output\n")); err != nil {
				t.Errorf("OutputWriter().Write() error = %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := repl.handleTerminalResize(); err != nil {
				t.Errorf("handleTerminalResize() error = %v", err)
			}
		}()
	}
	wg.Wait()
	if got := strings.Count(out.String(), "command output\n"); got != 20 {
		t.Fatalf("coordinated command output count = %d, want 20", got)
	}
}

func TestREPLRetainsTerminalControlWriteErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("terminal write failed")
	var errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), failingWriter{err: wantErr}, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)

	err := repl.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want terminal write error", err)
	}
}

func TestREPLRetainsResizeFooterWriteErrors(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader(""), &out, &errOut)
	repl.SetInteractive(true)
	size := terminalSize{Width: 80, Height: 24}
	setTerminalSize(repl, &size)
	if err := repl.startTerminalFooter(); err != nil {
		t.Fatalf("startTerminalFooter() error = %v", err)
	}
	defer repl.stopTerminalFooter()

	wantErr := errors.New("resize write failed")
	repl.Out = failingWriter{err: wantErr}
	if err := repl.handleTerminalResize(); !errors.Is(err, wantErr) {
		t.Fatalf("handleTerminalResize() error = %v, want resize write error", err)
	}
	if err := repl.terminalError(); !errors.Is(err, wantErr) {
		t.Fatalf("terminalError() = %v, want resize write error", err)
	}
}

func TestREPLInteractiveFooterCleansUpOnAllExitPaths(t *testing.T) {
	t.Parallel()

	readErr := errors.New("terminal read failed")
	for _, tt := range []struct {
		name    string
		ctx     func() context.Context
		in      io.Reader
		wantErr error
	}{
		{name: "eof", ctx: context.Background, in: interactiveInput()},
		{name: "quit", ctx: context.Background, in: interactiveInput("/quit", interactiveSubmit())},
		{name: "handled error", ctx: context.Background, in: interactiveInput("prompt", interactiveSubmit())},
		{name: "read error", ctx: context.Background, in: failingReader{err: readErr}, wantErr: readErr},
		{name: "cancellation", ctx: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, in: interactiveInput(), wantErr: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			repl := NewREPL(tt.in, &out, &errOut)
			repl.SetInteractive(true)
			size := terminalSize{Width: 80, Height: 24}
			setTerminalSize(repl, &size)

			err := repl.Run(tt.ctx())
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(out.String(), "\033[24;1H\033[2K\033[r") {
				t.Fatalf("footer was not cleared and scroll region restored: %q", out.String())
			}
			if !strings.HasSuffix(out.String(), "\033[u\r\n") {
				t.Fatalf("footer cleanup did not leave a newline for the shell: %q", out.String())
			}
		})
	}
}

func TestREPLNonInteractiveTokenUsageHasNoTerminalControlSequences(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	repl := NewREPL(strings.NewReader("hello\n"), &out, &errOut)
	repl.SetInteractive(false)
	repl.SetPromptSubmitter(PromptSubmitterFunc(func(context.Context, string) (AssistantStream, error) {
		return &sliceAssistantStream{chunks: []AssistantChunk{{Usage: &TokenUsage{InputTokens: 1, OutputTokens: 1}}}}, nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(out.String(), "\033") {
		t.Fatalf("non-interactive output contains terminal control sequence: %q", out.String())
	}
}
