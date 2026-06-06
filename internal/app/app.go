package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vhar-astro/goagent/internal/cli"
	"github.com/vhar-astro/goagent/internal/config"
	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

// Runner is the minimal session-loop contract the entrypoint can delegate to.
type Runner interface {
	Run(context.Context) error
}

// Options captures the launch-scoped values the application needs to start.
type Options struct {
	ConfigPath    string
	WorkspaceRoot string
	ProviderName  string
	Model         string
}

// BootstrapOptions captures the CLI-provided inputs needed to construct an App.
type BootstrapOptions struct {
	ConfigPath        string
	WorkspaceOverride string
	ProviderOverride  string
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
}

// App wires together launch options and the interactive runner.
type App struct {
	options Options
	config  config.Config
	profile config.ProviderProfile
	session *session.Session
	repl    *cli.REPL
	runner  Runner
}

// New constructs the minimal application shell used by the CLI entrypoint.
func New(options Options) *App {
	return &App{options: options}
}

// Options returns the launch settings currently attached to the application.
func (a *App) Options() Options {
	return a.options
}

// Config returns the loaded runtime config attached to the application.
func (a *App) Config() config.Config {
	return a.config
}

// ProviderProfile returns the resolved provider configuration for this launch.
func (a *App) ProviderProfile() config.ProviderProfile {
	return a.profile
}

// Session returns the in-memory session container for this launch.
func (a *App) Session() *session.Session {
	return a.session
}

// REPL returns the terminal shell bound to this application.
func (a *App) REPL() *cli.REPL {
	return a.repl
}

// SetRunner attaches the interactive runner that will power the live session.
func (a *App) SetRunner(runner Runner) {
	a.runner = runner
}

// Run executes the configured runner when one has been attached.
func (a *App) Run(ctx context.Context) error {
	if a.runner == nil {
		return nil
	}

	return a.runner.Run(ctx)
}

// Bootstrap loads config, resolves the launch settings, and wires the base
// interactive session path for the CLI entrypoint.
func Bootstrap(options BootstrapOptions) (*App, error) {
	configPath, err := config.ResolveConfigPath(options.ConfigPath)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	workspaceRoot, err := cfg.ResolveWorkspace(options.WorkspaceOverride)
	if err != nil {
		return nil, err
	}

	providerName, err := cfg.ResolveProviderName(options.ProviderOverride)
	if err != nil {
		return nil, err
	}

	profile, ok := cfg.Provider(providerName)
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", providerName)
	}

	runtime, err := runtimeFromConfig(workspaceRoot, cfg)
	if err != nil {
		return nil, err
	}

	terminalOut := writerOrDefault(options.Stdout, os.Stdout)
	terminalErr := writerOrDefault(options.Stderr, os.Stderr)

	application := New(Options{
		ConfigPath:    configPath,
		WorkspaceRoot: workspaceRoot,
		ProviderName:  providerName,
		Model:         profile.Model,
	})
	application.config = cfg
	application.profile = profile
	application.session = session.New(newSessionID(), workspaceRoot, providerName, profile.Model)
	if err := registerBuiltinTools(application.session); err != nil {
		return nil, err
	}
	application.repl = cli.NewREPL(
		readerOrDefault(options.Stdin, os.Stdin),
		terminalOut,
		terminalErr,
	)
	application.repl.SetSlashSuggester(func(line string) []string {
		matches := cli.SuggestSlashCommands(line)
		suggestions := make([]string, 0, len(matches))
		for _, match := range matches {
			suggestions = append(suggestions, match.Usage+" - "+match.Description)
		}
		return suggestions
	})
	application.repl.SetPromptSubmitter(&sessionPromptSubmitter{
		session: application.session,
		runtime: runtime,
		repl:    application.repl,
		stream:  cfg.Streaming,
		clientFactory: func() (provider.Client, error) {
			return newProviderClient(profile)
		},
	})
	application.repl.SetCommandHandlers(newBaseCommandHandlers(application.session, terminalOut))
	application.SetRunner(application.repl)

	return application, nil
}

func newSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UTC().UnixNano())
}

func runtimeFromConfig(workspaceRoot string, cfg config.Config) (Runtime, error) {
	return NewRuntimeWithOptions(RuntimeOptions{
		WorkspaceRoot:   workspaceRoot,
		CommandTimeout:  time.Duration(cfg.Timeouts.ShellSeconds) * time.Second,
		WebTimeout:      time.Duration(cfg.Timeouts.HTTPSeconds) * time.Second,
		ToolOutputLimit: cfg.OutputLimits.ToolBytes,
	})
}

func registerBuiltinTools(sess *session.Session) error {
	registry := tools.NewRegistry()
	if err := registry.RegisterBuiltins(builtinToolSpecs()); err != nil {
		return fmt.Errorf("register built-in tools: %w", err)
	}

	sess.SetBuiltInTools(registry.Builtins())
	return nil
}

func builtinToolSpecs() []tools.Spec {
	specs := append([]tools.Spec(nil), tools.FileToolSpecs()...)
	specs = append(specs, tools.BuiltinShellSpec(), tools.BuiltinWebSpec())
	return specs
}

func newProviderClient(profile config.ProviderProfile) (provider.Client, error) {
	apiKey := strings.TrimSpace(os.Getenv(profile.APIKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("provider %q requires environment variable %s", profile.Name, profile.APIKeyEnv)
	}

	client, err := provider.NewClient(provider.HTTPClientConfig{
		BaseURL:      profile.BaseURL,
		APIKey:       apiKey,
		ExtraHeaders: profile.ExtraHeaders,
	})
	if err != nil {
		return nil, fmt.Errorf("configure provider %q: %w", profile.Name, err)
	}

	return client, nil
}

func newBaseCommandHandlers(sess *session.Session, out io.Writer) cli.CommandHandlers {
	return cli.CommandHandlers{
		Approve: func(_ context.Context, command cli.ApproveCommand) error {
			if sess.IsApproved(command.Capability) {
				_, err := fmt.Fprintf(out, "capability %q is already approved for this session\n", command.Capability)
				return err
			}
			if err := sess.Approve(command.Capability, time.Time{}); err != nil {
				return err
			}

			_, err := fmt.Fprintf(out, "approved capability %q for this session\n", command.Capability)
			return err
		},
		Attach: func(context.Context, cli.AttachCommand) error {
			return fmt.Errorf("/attach is not implemented yet; module support lands in User Story 2")
		},
		Detach: func(context.Context, cli.DetachCommand) error {
			return fmt.Errorf("/detach is not implemented yet; module support lands in User Story 2")
		},
		Provider: func(context.Context, cli.ProviderCommand) error {
			return fmt.Errorf("/provider is not implemented yet at runtime; relaunch with --provider until User Story 3 lands")
		},
		Tools: func(_ context.Context, _ cli.ToolsCommand) error {
			specs := sess.ActiveTools()
			if len(specs) == 0 {
				_, err := io.WriteString(out, "active tools: none\n")
				return err
			}

			if _, err := io.WriteString(out, "active tools:\n"); err != nil {
				return err
			}
			for _, spec := range specs {
				source := strings.TrimSpace(spec.Source)
				if source == "" {
					source = tools.SourceBuiltin
				}
				if _, err := fmt.Fprintf(out, "- %s [%s/%s]\n", spec.Name, source, spec.Capability); err != nil {
					return err
				}
			}
			return nil
		},
		Quit: func(context.Context, cli.QuitCommand) error {
			return nil
		},
	}
}

type sessionPromptSubmitter struct {
	session       *session.Session
	runtime       Runtime
	repl          *cli.REPL
	stream        bool
	clientFactory func() (provider.Client, error)

	mu     sync.Mutex
	client provider.Client
}

func (s *sessionPromptSubmitter) SubmitPrompt(ctx context.Context, prompt string) (cli.AssistantStream, error) {
	client, err := s.providerClient()
	if err != nil {
		return nil, err
	}

	stream := newAssistantChunkStream()
	runner, err := NewSessionRunner(SessionRunnerOptions{
		Session: s.session,
		Client:  client,
		Runtime: &s.runtime,
		Stream:  s.stream,
		WriteAssistantChunk: func(ctx context.Context, chunk string) error {
			return stream.Send(ctx, chunk)
		},
		WriteToolResult: func(ctx context.Context, event ToolExecutionEvent) error {
			if s.repl == nil {
				return nil
			}
			status := "ok"
			if event.Failed {
				status = "error"
			}
			return s.repl.WriteLocalMessage(ctx, fmt.Sprintf("tool=%s status=%s capability=%s", event.ToolName, status, event.Capability))
		},
		RequestShellApproval: func(ctx context.Context, pending session.PendingApprovalRequest) (bool, error) {
			if s.repl == nil {
				return false, errors.New("repl is not configured")
			}
			return s.repl.PromptApproval(ctx, pending.PromptText)
		},
		WriteLocalMessage: func(ctx context.Context, message string) error {
			if s.repl == nil {
				return nil
			}
			return s.repl.WriteLocalMessage(ctx, message)
		},
	})
	if err != nil {
		return nil, err
	}

	go func() {
		stream.Finish(runner.RunTurn(ctx, prompt))
	}()

	return stream, nil
}

func (s *sessionPromptSubmitter) providerClient() (provider.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		return s.client, nil
	}

	client, err := s.clientFactory()
	if err != nil {
		return nil, err
	}

	s.client = client
	return s.client, nil
}

type assistantChunkStream struct {
	items chan assistantChunkItem
	once  sync.Once
}

type assistantChunkItem struct {
	chunk cli.AssistantChunk
	err   error
	done  bool
}

func newAssistantChunkStream() *assistantChunkStream {
	return &assistantChunkStream{
		items: make(chan assistantChunkItem, 16),
	}
}

func (s *assistantChunkStream) Recv(ctx context.Context) (cli.AssistantChunk, error) {
	select {
	case <-ctx.Done():
		return cli.AssistantChunk{}, ctx.Err()
	case item, ok := <-s.items:
		if !ok || item.done {
			if item.err != nil {
				return cli.AssistantChunk{}, item.err
			}
			return cli.AssistantChunk{}, io.EOF
		}
		return item.chunk, nil
	}
}

func (s *assistantChunkStream) Send(ctx context.Context, chunk string) error {
	if chunk == "" {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.items <- assistantChunkItem{chunk: cli.AssistantChunk{Text: chunk}}:
		return nil
	}
}

func (s *assistantChunkStream) Finish(err error) {
	s.once.Do(func() {
		s.items <- assistantChunkItem{err: err, done: true}
		close(s.items)
	})
}

func readerOrDefault(value, fallback io.Reader) io.Reader {
	if value != nil {
		return value
	}

	return fallback
}

func writerOrDefault(value, fallback io.Writer) io.Writer {
	if value != nil {
		return value
	}

	return fallback
}
