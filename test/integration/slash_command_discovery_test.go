package integration

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/vhar-astro/goagent/internal/cli"
)

func TestInteractiveSlashCommandDiscoveryScenario(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	repl := cli.NewREPL(strings.NewReader("/tools\n"), &out, &out)
	repl.SetInteractive(true)
	repl.SetSlashSuggester(func(line string) []string {
		matches := cli.SuggestSlashCommands(line)
		suggestions := make([]string, 0, len(matches))
		for _, match := range matches {
			suggestions = append(suggestions, match.Usage+" - "+match.Description)
		}
		return suggestions
	})
	repl.SetCommandExecutor(cli.CommandExecutorFunc(func(context.Context, cli.ParsedInput) error {
		return nil
	}))

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "/tools - Show the active built-in and module tools.") {
		t.Fatalf("interactive output missing slash discovery text: %q", out.String())
	}
}
