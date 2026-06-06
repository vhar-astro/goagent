package cli

import (
	"strings"
	"testing"
)

func TestLookupSlashCommandSpec(t *testing.T) {
	t.Parallel()

	spec, ok := LookupSlashCommandSpec("approve")
	if !ok {
		t.Fatal("expected approve command spec")
	}
	if spec.Usage != "/approve write|web|module" {
		t.Fatalf("unexpected approve usage: %q", spec.Usage)
	}
	if spec.Description == "" {
		t.Fatal("expected approve description")
	}
	if strings.Contains(spec.Usage, "shell") {
		t.Fatalf("approve usage should not mention shell: %q", spec.Usage)
	}
}

func TestAvailableSlashCommandsText(t *testing.T) {
	t.Parallel()

	text := AvailableSlashCommandsText()
	for _, want := range []string{
		"/approve write|web|module",
		"/attach PATH",
		"/detach NAME",
		"/provider NAME",
		"/tools",
		"/quit",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("available commands text missing %q from %q", want, text)
		}
	}
	if strings.Contains(text, "shell") {
		t.Fatalf("available commands text should not mention shell: %q", text)
	}
}

func TestSuggestSlashCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		wants  []string
		length int
	}{
		{
			name:   "root query returns full catalog",
			query:  "/",
			length: len(slashCommandCatalog),
			wants:  []string{"approve", "tools", "quit"},
		},
		{
			name:   "prefix query narrows catalog",
			query:  "/to",
			length: 1,
			wants:  []string{"tools"},
		},
		{
			name:   "query ignores trailing arguments",
			query:  "/provider openrouter",
			length: 1,
			wants:  []string{"provider"},
		},
		{
			name:   "unknown prefix returns no matches",
			query:  "/zzz",
			length: 0,
			wants:  nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matches := SuggestSlashCommands(tt.query)
			if len(matches) != tt.length {
				t.Fatalf("expected %d matches, got %d", tt.length, len(matches))
			}

			for _, want := range tt.wants {
				found := false
				for _, match := range matches {
					if match.Name == want {
						found = true
						if match.Description == "" {
							t.Fatalf("expected description for %q", want)
						}
						break
					}
				}
				if !found {
					t.Fatalf("expected match %q in %#v", want, matches)
				}
			}
		})
	}
}

func TestParseSlashCommandApproveRejectsShell(t *testing.T) {
	t.Parallel()

	_, err := ParseSlashCommand("/approve shell")
	if err == nil {
		t.Fatal("expected parse error")
	}

	parseErr, ok := err.(*CommandParseError)
	if !ok {
		t.Fatalf("expected CommandParseError, got %T", err)
	}
	if parseErr.Command != "approve" {
		t.Fatalf("unexpected command name: %q", parseErr.Command)
	}
	if parseErr.Usage != "/approve write|web|module" {
		t.Fatalf("unexpected usage: %q", parseErr.Usage)
	}
	if !strings.Contains(parseErr.Error(), `unsupported approval capability "shell"`) {
		t.Fatalf("unexpected error text: %q", parseErr.Error())
	}
}

func TestParseSlashCommandUnknownCommandIncludesCatalogUsage(t *testing.T) {
	t.Parallel()

	_, err := ParseSlashCommand("/wat")
	if err == nil {
		t.Fatal("expected parse error")
	}

	parseErr, ok := err.(*CommandParseError)
	if !ok {
		t.Fatalf("expected CommandParseError, got %T", err)
	}
	if !strings.Contains(parseErr.Error(), `unknown slash command "wat"`) {
		t.Fatalf("unexpected error text: %q", parseErr.Error())
	}
	if !strings.Contains(parseErr.Error(), "/approve write|web|module") {
		t.Fatalf("expected available commands in error text: %q", parseErr.Error())
	}
	if strings.Contains(parseErr.Error(), "shell") {
		t.Fatalf("error text should not mention shell approval: %q", parseErr.Error())
	}
}

func TestParseInputRecognizesSlashCommand(t *testing.T) {
	t.Parallel()

	parsed, err := ParseInput(` /attach "module dir" `)
	if err != nil {
		t.Fatalf("ParseInput returned error: %v", err)
	}
	if parsed.Text != "" {
		t.Fatalf("expected empty text for slash command, got %q", parsed.Text)
	}

	command, ok := parsed.Command.(AttachCommand)
	if !ok {
		t.Fatalf("expected AttachCommand, got %T", parsed.Command)
	}
	if command.Path != "module dir" {
		t.Fatalf("unexpected attach path: %q", command.Path)
	}
}
