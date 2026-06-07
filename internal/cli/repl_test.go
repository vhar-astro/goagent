package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
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

type fixedAssistantStream struct{}

func (fixedAssistantStream) Recv(context.Context) (AssistantChunk, error) {
	return AssistantChunk{}, io.EOF
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
