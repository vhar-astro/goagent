package cli

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/vhar-astro/goagent/internal/tools"
)

const slashCommandPrefix = "/"

// SlashCommandSpec captures the local slash-command metadata used for parsing and discovery.
type SlashCommandSpec struct {
	Name        string
	Usage       string
	Description string
}

var slashCommandCatalog = []SlashCommandSpec{
	{
		Name:        "approve",
		Usage:       "/approve write|web|module",
		Description: "Approve one risky non-shell capability for this session.",
	},
	{
		Name:        "attach",
		Usage:       "/attach PATH",
		Description: "Attach a local module directory.",
	},
	{
		Name:        "detach",
		Usage:       "/detach NAME",
		Description: "Detach an attached module.",
	},
	{
		Name:        "provider",
		Usage:       "/provider NAME",
		Description: "Switch the active provider profile.",
	},
	{
		Name:        "tools",
		Usage:       "/tools",
		Description: "Show the active built-in and module tools.",
	},
	{
		Name:        "quit",
		Usage:       "/quit",
		Description: "Exit the interactive session.",
	},
}

var slashCommandCatalogByName = func() map[string]SlashCommandSpec {
	byName := make(map[string]SlashCommandSpec, len(slashCommandCatalog))
	for _, spec := range slashCommandCatalog {
		byName[spec.Name] = spec
	}

	return byName
}()

var slashCommandParsers = map[string]func(string, []string) (SlashCommand, error){
	"approve":  parseApproveCommand,
	"attach":   parseAttachCommand,
	"detach":   parseDetachCommand,
	"provider": parseProviderCommand,
	"tools":    parseToolsCommand,
	"quit":     parseQuitCommand,
}

var approvableCapabilities = map[string]struct{}{
	tools.CapabilityWrite:  {},
	tools.CapabilityWeb:    {},
	tools.CapabilityModule: {},
}

// ParsedInput separates ordinary prompt text from local slash commands.
type ParsedInput struct {
	Raw     string
	Text    string
	Command SlashCommand
}

// IsCommand reports whether the parsed input carries a slash command.
func (p ParsedInput) IsCommand() bool {
	return p.Command != nil
}

// SlashCommand is the typed local control command understood by the CLI.
type SlashCommand interface {
	CommandName() string
}

// ApproveCommand grants one risky session capability.
type ApproveCommand struct {
	Capability string
}

// CommandName returns the slash command keyword.
func (ApproveCommand) CommandName() string {
	return "approve"
}

// AttachCommand requests activation of one local module directory.
type AttachCommand struct {
	Path string
}

// CommandName returns the slash command keyword.
func (AttachCommand) CommandName() string {
	return "attach"
}

// DetachCommand requests removal of one attached module.
type DetachCommand struct {
	Name string
}

// CommandName returns the slash command keyword.
func (DetachCommand) CommandName() string {
	return "detach"
}

// ProviderCommand requests a runtime provider profile switch.
type ProviderCommand struct {
	Name string
}

// CommandName returns the slash command keyword.
func (ProviderCommand) CommandName() string {
	return "provider"
}

// ToolsCommand requests the currently active built-in and module tools.
type ToolsCommand struct{}

// CommandName returns the slash command keyword.
func (ToolsCommand) CommandName() string {
	return "tools"
}

// QuitCommand requests a clean shutdown of the interactive session.
type QuitCommand struct{}

// CommandName returns the slash command keyword.
func (QuitCommand) CommandName() string {
	return "quit"
}

// CommandParseError captures a user-actionable CLI parsing failure.
type CommandParseError struct {
	Command string
	Input   string
	Message string
	Usage   string
}

// Error formats the parsing failure for direct CLI display.
func (e *CommandParseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Usage != "" {
		return fmt.Sprintf("%s (usage: %s)", e.Message, e.Usage)
	}

	return e.Message
}

// CommandDispatchError captures a local dispatch failure before handler logic runs.
type CommandDispatchError struct {
	Command string
	Message string
}

// Error formats the dispatch failure for direct CLI display.
func (e *CommandDispatchError) Error() string {
	if e == nil {
		return ""
	}
	if e.Command == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Command, e.Message)
}

// CommandHandlers holds the typed slash-command callbacks that the REPL can wire in.
type CommandHandlers struct {
	Approve  func(context.Context, ApproveCommand) error
	Attach   func(context.Context, AttachCommand) error
	Detach   func(context.Context, DetachCommand) error
	Provider func(context.Context, ProviderCommand) error
	Tools    func(context.Context, ToolsCommand) error
	Quit     func(context.Context, QuitCommand) error
}

// Dispatcher routes typed slash commands to the configured handlers.
type Dispatcher struct {
	handlers CommandHandlers
}

// NewDispatcher builds a slash-command dispatcher from the supplied handlers.
func NewDispatcher(handlers CommandHandlers) *Dispatcher {
	return &Dispatcher{handlers: handlers}
}

// ParseInput turns one raw REPL line into either text or a typed slash command.
func ParseInput(line string) (ParsedInput, error) {
	parsed := ParsedInput{Raw: line}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return parsed, nil
	}
	if !strings.HasPrefix(trimmed, slashCommandPrefix) {
		parsed.Text = trimmed
		return parsed, nil
	}

	command, err := ParseSlashCommand(trimmed)
	if err != nil {
		return ParsedInput{}, err
	}

	parsed.Command = command
	return parsed, nil
}

// ParseSlashCommand parses and validates one slash command line.
func ParseSlashCommand(line string) (SlashCommand, error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, slashCommandPrefix) {
		return nil, &CommandParseError{
			Input:   line,
			Message: "slash commands must start with '/'",
		}
	}

	tokens, err := splitCommandTokens(strings.TrimPrefix(trimmed, slashCommandPrefix))
	if err != nil {
		return nil, &CommandParseError{
			Input:   line,
			Message: err.Error(),
			Usage:   availableSlashCommandsText(),
		}
	}
	if len(tokens) == 0 {
		return nil, &CommandParseError{
			Input:   line,
			Message: "slash command is empty",
			Usage:   availableSlashCommandsText(),
		}
	}

	name := tokens[0]
	args := tokens[1:]

	parser, ok := slashCommandParsers[name]
	if !ok {
		return nil, &CommandParseError{
			Command: name,
			Input:   line,
			Message: fmt.Sprintf("unknown slash command %q", name),
			Usage:   availableSlashCommandsText(),
		}
	}

	return parser(line, args)
}

// LookupSlashCommandSpec returns the command catalog entry for one slash command.
func LookupSlashCommandSpec(name string) (SlashCommandSpec, bool) {
	spec, ok := slashCommandCatalogByName[name]
	return spec, ok
}

// AvailableSlashCommandsText returns the catalog usage summary for user-facing help text.
func AvailableSlashCommandsText() string {
	return availableSlashCommandsText()
}

// SuggestSlashCommands returns prefix-filtered command metadata for slash discovery.
func SuggestSlashCommands(prefix string) []SlashCommandSpec {
	return suggestSlashCommands(prefix)
}

// DispatchParsedInput sends a parsed slash command to the configured handler set.
func (d *Dispatcher) DispatchParsedInput(ctx context.Context, input ParsedInput) error {
	if !input.IsCommand() {
		return &CommandDispatchError{Message: "parsed input does not contain a slash command"}
	}

	return d.Dispatch(ctx, input.Command)
}

// Dispatch routes one typed slash command to the matching handler.
func (d *Dispatcher) Dispatch(ctx context.Context, command SlashCommand) error {
	switch typed := command.(type) {
	case nil:
		return &CommandDispatchError{Message: "slash command is nil"}
	case ApproveCommand:
		return d.dispatchApprove(ctx, typed)
	case *ApproveCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "approve", Message: "slash command is nil"}
		}
		return d.dispatchApprove(ctx, *typed)
	case AttachCommand:
		return d.dispatchAttach(ctx, typed)
	case *AttachCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "attach", Message: "slash command is nil"}
		}
		return d.dispatchAttach(ctx, *typed)
	case DetachCommand:
		return d.dispatchDetach(ctx, typed)
	case *DetachCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "detach", Message: "slash command is nil"}
		}
		return d.dispatchDetach(ctx, *typed)
	case ProviderCommand:
		return d.dispatchProvider(ctx, typed)
	case *ProviderCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "provider", Message: "slash command is nil"}
		}
		return d.dispatchProvider(ctx, *typed)
	case ToolsCommand:
		return d.dispatchTools(ctx, typed)
	case *ToolsCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "tools", Message: "slash command is nil"}
		}
		return d.dispatchTools(ctx, *typed)
	case QuitCommand:
		return d.dispatchQuit(ctx, typed)
	case *QuitCommand:
		if typed == nil {
			return &CommandDispatchError{Command: "quit", Message: "slash command is nil"}
		}
		return d.dispatchQuit(ctx, *typed)
	default:
		return &CommandDispatchError{
			Command: command.CommandName(),
			Message: "unsupported slash command type",
		}
	}
}

func (d *Dispatcher) dispatchApprove(ctx context.Context, command ApproveCommand) error {
	if d == nil || d.handlers.Approve == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Approve(ctx, command)
}

func (d *Dispatcher) dispatchAttach(ctx context.Context, command AttachCommand) error {
	if d == nil || d.handlers.Attach == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Attach(ctx, command)
}

func (d *Dispatcher) dispatchDetach(ctx context.Context, command DetachCommand) error {
	if d == nil || d.handlers.Detach == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Detach(ctx, command)
}

func (d *Dispatcher) dispatchProvider(ctx context.Context, command ProviderCommand) error {
	if d == nil || d.handlers.Provider == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Provider(ctx, command)
}

func (d *Dispatcher) dispatchTools(ctx context.Context, command ToolsCommand) error {
	if d == nil || d.handlers.Tools == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Tools(ctx, command)
}

func (d *Dispatcher) dispatchQuit(ctx context.Context, command QuitCommand) error {
	if d == nil || d.handlers.Quit == nil {
		return missingHandlerError(command.CommandName())
	}

	return d.handlers.Quit(ctx, command)
}

func parseApproveCommand(line string, args []string) (SlashCommand, error) {
	capability, err := requireSingleArgument(line, "approve", args)
	if err != nil {
		return nil, err
	}
	if _, ok := approvableCapabilities[capability]; !ok {
		return nil, &CommandParseError{
			Command: "approve",
			Input:   line,
			Message: fmt.Sprintf("unsupported approval capability %q", capability),
			Usage:   usageFor("approve"),
		}
	}

	return ApproveCommand{Capability: capability}, nil
}

func parseAttachCommand(line string, args []string) (SlashCommand, error) {
	path, err := requireSingleArgument(line, "attach", args)
	if err != nil {
		return nil, err
	}

	return AttachCommand{Path: path}, nil
}

func parseDetachCommand(line string, args []string) (SlashCommand, error) {
	name, err := requireSingleArgument(line, "detach", args)
	if err != nil {
		return nil, err
	}

	return DetachCommand{Name: name}, nil
}

func parseProviderCommand(line string, args []string) (SlashCommand, error) {
	name, err := requireSingleArgument(line, "provider", args)
	if err != nil {
		return nil, err
	}

	return ProviderCommand{Name: name}, nil
}

func parseToolsCommand(line string, args []string) (SlashCommand, error) {
	if err := requireNoArguments(line, "tools", args); err != nil {
		return nil, err
	}

	return ToolsCommand{}, nil
}

func parseQuitCommand(line string, args []string) (SlashCommand, error) {
	if err := requireNoArguments(line, "quit", args); err != nil {
		return nil, err
	}

	return QuitCommand{}, nil
}

func requireSingleArgument(line, name string, args []string) (string, error) {
	if len(args) != 1 {
		return "", argumentCountError(line, name, "exactly one argument")
	}
	if strings.TrimSpace(args[0]) == "" {
		return "", &CommandParseError{
			Command: name,
			Input:   line,
			Message: "argument cannot be empty",
			Usage:   usageFor(name),
		}
	}

	return args[0], nil
}

func requireNoArguments(line, name string, args []string) error {
	if len(args) == 0 {
		return nil
	}

	return argumentCountError(line, name, "no arguments")
}

func argumentCountError(line, name, want string) error {
	return &CommandParseError{
		Command: name,
		Input:   line,
		Message: fmt.Sprintf("%s expects %s", slashCommandPrefix+name, want),
		Usage:   usageFor(name),
	}
}

func usageFor(name string) string {
	spec, ok := LookupSlashCommandSpec(name)
	if !ok {
		return ""
	}

	return spec.Usage
}

func availableSlashCommandsText() string {
	usages := make([]string, 0, len(slashCommandCatalog))
	for _, spec := range slashCommandCatalog {
		usages = append(usages, spec.Usage)
	}

	return strings.Join(usages, ", ")
}

func suggestSlashCommands(prefix string) []SlashCommandSpec {
	query := strings.TrimSpace(prefix)
	query = strings.TrimPrefix(query, slashCommandPrefix)
	if cut := strings.IndexFunc(query, unicode.IsSpace); cut >= 0 {
		query = query[:cut]
	}

	matches := make([]SlashCommandSpec, 0, len(slashCommandCatalog))
	for _, spec := range slashCommandCatalog {
		if query != "" && !strings.HasPrefix(spec.Name, query) {
			continue
		}
		matches = append(matches, spec)
	}

	return matches
}

func missingHandlerError(name string) error {
	return &CommandDispatchError{
		Command: name,
		Message: "handler is not configured",
	}
}

func splitCommandTokens(input string) ([]string, error) {
	var (
		tokens  []string
		current strings.Builder
		quoted  rune
		escaped bool
		inToken bool
	)

	flush := func() {
		if !inToken {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
		inToken = false
	}

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			inToken = true
			continue
		}

		if quoted != 0 {
			switch r {
			case '\\':
				escaped = true
			case quoted:
				quoted = 0
			default:
				current.WriteRune(r)
			}
			inToken = true
			continue
		}

		switch {
		case unicode.IsSpace(r):
			flush()
		case r == '\\':
			escaped = true
			inToken = true
		case r == '\'' || r == '"':
			quoted = r
			inToken = true
		default:
			current.WriteRune(r)
			inToken = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("slash command has a dangling escape sequence")
	}
	if quoted != 0 {
		return nil, fmt.Errorf("slash command has an unterminated quoted argument")
	}

	flush()
	return tokens, nil
}
