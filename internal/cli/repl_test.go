package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestREPLInteractiveSlashSuggestions(t *testing.T) {
	t.Parallel()

	var (
		out     bytes.Buffer
		errOut  bytes.Buffer
		handled []string
	)

	repl := NewREPL(strings.NewReader("/tools\n"), &out, &errOut)
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
	if !strings.Contains(rendered, "/tools - Show the active built-in and module tools.") {
		t.Fatalf("interactive output missing slash suggestion: %q", rendered)
	}
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
