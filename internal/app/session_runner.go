package app

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

const defaultSystemPrompt = "You are goagent, a minimal coding agent. Be concise. Use tools only when needed. Stay inside the current workspace for file and shell work. Use fetch_url only for explicit http or https URLs."

var (
	errSessionRequired  = errors.New("session is required")
	errProviderRequired = errors.New("provider client is required")
	errInputReaderUnset = errors.New("session input reader is not configured")
)

// SessionInputFunc reads the next natural-language input for one session turn.
type SessionInputFunc func(context.Context) (string, error)

// AssistantChunkWriterFunc consumes streamed assistant text chunks.
type AssistantChunkWriterFunc func(context.Context, string) error

// ToolResultWriterFunc consumes normalized tool execution summaries.
type ToolResultWriterFunc func(context.Context, ToolExecutionEvent) error

// ShellApprovalPromptFunc asks the user to approve one pending shell command.
type ShellApprovalPromptFunc func(context.Context, session.PendingApprovalRequest) (bool, error)

// LocalMessageWriterFunc prints one CLI-owned local message.
type LocalMessageWriterFunc func(context.Context, string) error

// ToolExecutionEvent captures one executed tool call and its reinjected payload.
type ToolExecutionEvent struct {
	ToolCallID string
	ToolName   string
	Source     string
	Capability string
	Output     LimitedText
	Failed     bool
}

// SessionRunnerOptions configures one in-memory conversation loop.
type SessionRunnerOptions struct {
	Session              *session.Session
	Client               provider.Client
	Runtime              *Runtime
	ReadInput            SessionInputFunc
	WriteAssistantChunk  AssistantChunkWriterFunc
	WriteToolResult      ToolResultWriterFunc
	RequestShellApproval ShellApprovalPromptFunc
	WriteLocalMessage    LocalMessageWriterFunc
	SystemPrompt         string
	Stream               bool
}

// SessionRunner coordinates user turns, provider streaming, and tool execution.
type SessionRunner struct {
	session              *session.Session
	client               provider.Client
	runtime              Runtime
	executor             *Executor
	readInput            SessionInputFunc
	writeAssistantChunk  AssistantChunkWriterFunc
	writeToolResult      ToolResultWriterFunc
	requestShellApproval ShellApprovalPromptFunc
	writeLocalMessage    LocalMessageWriterFunc
	systemPrompt         string
	stream               bool
	transcript           []provider.Message
	lastTurnUsage        provider.Usage
}

// NewSessionRunner constructs a session runner around the normalized runtime types.
func NewSessionRunner(options SessionRunnerOptions) (*SessionRunner, error) {
	if options.Session == nil {
		return nil, errSessionRequired
	}
	if options.Client == nil {
		return nil, errProviderRequired
	}

	runtime := options.Runtime
	if runtime == nil {
		created, err := NewRuntime(options.Session.WorkspaceRoot)
		if err != nil {
			return nil, err
		}
		runtime = &created
	}

	executor, err := NewExecutor(*runtime, options.Session)
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(options.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	runner := &SessionRunner{
		session:              options.Session,
		client:               options.Client,
		runtime:              *runtime,
		executor:             executor,
		readInput:            options.ReadInput,
		writeAssistantChunk:  options.WriteAssistantChunk,
		writeToolResult:      options.WriteToolResult,
		requestShellApproval: options.RequestShellApproval,
		writeLocalMessage:    options.WriteLocalMessage,
		systemPrompt:         systemPrompt,
		stream:               true,
		transcript:           make([]provider.Message, 0, len(options.Session.Messages)),
	}
	if !options.Stream {
		runner.stream = false
	}

	for _, message := range options.Session.Conversation() {
		runner.transcript = append(runner.transcript, providerMessageFromSession(message))
	}

	return runner, nil
}

// Run processes natural-language session turns until input ends or the context stops.
func (r *SessionRunner) Run(ctx context.Context) error {
	if r.readInput == nil {
		return errInputReaderUnset
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		input, err := r.readInput(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if strings.TrimSpace(input) == "" {
			continue
		}

		if err := r.RunTurn(ctx, input); err != nil {
			return err
		}
	}
}

// RunTurn executes one natural-language user turn through the provider/tool loop.
func (r *SessionRunner) RunTurn(ctx context.Context, input string) error {
	text := strings.TrimSpace(input)
	if text == "" {
		return nil
	}

	r.lastTurnUsage = provider.Usage{}

	userMessage := provider.Message{
		Role:    provider.RoleUser,
		Content: text,
	}
	r.transcript = append(r.transcript, userMessage)
	if err := r.session.AppendMessage(session.Message{
		Role:    session.RoleUser,
		Content: text,
	}); err != nil {
		return err
	}

	for {
		response, err := r.requestAssistantTurn(ctx)
		if err != nil {
			return err
		}
		r.lastTurnUsage = addUsage(r.lastTurnUsage, response.Usage)

		if response.Message.Role == "" {
			response.Message.Role = provider.RoleAssistant
		}

		r.transcript = append(r.transcript, cloneProviderMessage(response.Message))
		if err := r.appendAssistantSummary(response.Message); err != nil {
			return err
		}

		if len(response.Message.ToolCalls) == 0 {
			return nil
		}

		toolMessages, err := r.executeToolCalls(ctx, response.Message.ToolCalls)
		if err != nil {
			return err
		}
		r.transcript = append(r.transcript, toolMessages...)
	}
}

// LastTurnUsage returns the aggregated token usage from the most recent turn.
func (r *SessionRunner) LastTurnUsage() provider.Usage {
	return *cloneUsage(&r.lastTurnUsage)
}

func (r *SessionRunner) requestAssistantTurn(ctx context.Context) (provider.Response, error) {
	request := provider.Request{
		Model:         r.session.Model,
		Messages:      r.providerMessages(),
		Tools:         buildProviderTools(r.session.ActiveTools()),
		Stream:        r.stream,
		StreamOptions: &provider.StreamOptions{IncludeUsage: true},
	}
	if len(request.Tools) > 0 {
		request.ToolChoice = &provider.ToolChoice{Mode: provider.ToolChoiceAuto}
	}

	events, err := r.client.Chat(ctx, request)
	if err != nil {
		return provider.Response{}, err
	}

	return r.collectAssistantResponse(ctx, events)
}

func (r *SessionRunner) providerMessages() []provider.Message {
	messages := make([]provider.Message, 0, len(r.transcript)+1)
	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: r.systemPrompt,
	})
	for _, message := range r.transcript {
		messages = append(messages, cloneProviderMessage(message))
	}

	return messages
}

func (r *SessionRunner) collectAssistantResponse(ctx context.Context, events <-chan provider.StreamEvent) (provider.Response, error) {
	response := provider.Response{
		Message: provider.Message{
			Role: provider.RoleAssistant,
		},
	}

	var (
		builders          []toolCallBuilder
		sawCompletion     bool
		sawAssistantEvent bool
	)

	for {
		select {
		case <-ctx.Done():
			return provider.Response{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				response.Message.ToolCalls = finalizeToolCalls(builders)
				return response, nil
			}

			switch event.Type {
			case provider.EventResponseStart:
				response.ID = event.ResponseID
				response.Model = event.Model
			case provider.EventMessageDelta:
				if event.Delta == nil {
					continue
				}
				sawAssistantEvent = true
				if event.Delta.Role != "" {
					response.Message.Role = event.Delta.Role
				}
				if event.Delta.Content != "" {
					response.Message.Content += event.Delta.Content
					if err := r.emitAssistantChunk(ctx, event.Delta.Content); err != nil {
						return provider.Response{}, err
					}
				}
			case provider.EventToolCallDelta:
				if event.Delta == nil {
					continue
				}
				sawAssistantEvent = true
				builders = mergeToolCallBuilders(builders, event.Delta.Tools)
			case provider.EventUsage:
				if event.Usage != nil {
					response.Usage = *cloneUsage(event.Usage)
				}
			case provider.EventResponseComplete:
				response.ID = firstNonEmpty(response.ID, event.ResponseID)
				response.Model = firstNonEmpty(response.Model, event.Model)
				response.FinishReason = event.FinishReason
				if event.Usage != nil {
					response.Usage = *cloneUsage(event.Usage)
				}
				sawCompletion = true
			case provider.EventError:
				if event.Err != nil {
					return provider.Response{}, event.Err
				}
			}

			if sawCompletion {
				response.Message.ToolCalls = finalizeToolCalls(builders)
				return response, nil
			}
			if !sawAssistantEvent {
				continue
			}
		}
	}
}

func (r *SessionRunner) executeToolCalls(ctx context.Context, calls []provider.ToolCall) ([]provider.Message, error) {
	messages := make([]provider.Message, 0, len(calls))
	for _, call := range calls {
		result, err := r.executor.Execute(ctx, call)
		if err != nil {
			return nil, err
		}

		if result.RequiresApproval() {
			result, err = r.handleApprovalRequired(ctx, call, result)
			if err != nil {
				return nil, err
			}
		}

		if err := r.emitToolResult(ctx, result.Event()); err != nil {
			return nil, err
		}
		if err := r.emitShellApprovalOutcome(ctx, result); err != nil {
			return nil, err
		}

		sessionMessage := result.ToolMessage()
		if err := r.session.AppendMessage(sessionMessage); err != nil {
			return nil, err
		}

		messages = append(messages, providerMessageFromSession(sessionMessage))
	}

	return messages, nil
}

func (r *SessionRunner) handleApprovalRequired(ctx context.Context, call provider.ToolCall, result ToolExecutionResult) (ToolExecutionResult, error) {
	if result.PendingApproval == nil {
		return result, nil
	}
	if r.requestShellApproval == nil {
		return ToolExecutionResult{}, errors.New("shell approval prompt is not configured")
	}

	approved, err := r.requestShellApproval(ctx, *result.PendingApproval)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if approved {
		if _, ok := r.session.ApprovePendingShellApproval(time.Time{}); !ok {
			return ToolExecutionResult{}, errors.New("pending shell approval disappeared before approval")
		}
		pending, _ := r.session.PendingApproval()
		if r.writeLocalMessage != nil {
			if err := r.writeLocalMessage(ctx, "approved shell command "+strconv.Quote(pending.Command)+" in "+pending.Workdir); err != nil {
				return ToolExecutionResult{}, err
			}
		}
		r.session.ClearPendingShellApproval()
		executed, err := r.executor.Execute(ctx, call)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		executed.ApprovalReused = false
		return executed, nil
	}

	r.session.DenyPendingShellApproval()
	pending, _ := r.session.PendingApproval()
	if r.writeLocalMessage != nil {
		if err := r.writeLocalMessage(ctx, "denied shell command "+strconv.Quote(pending.Command)+" in "+pending.Workdir); err != nil {
			return ToolExecutionResult{}, err
		}
	}
	r.session.ClearPendingShellApproval()

	return ToolExecutionResult{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Source:     tools.SourceBuiltin,
		Capability: tools.CapabilityShell,
		Status:     ToolExecutionFailed,
		Summary: ToolExecutionSummary{
			ToolName:   call.Name,
			Capability: tools.CapabilityShell,
			Status:     ToolExecutionFailed,
			Fields: map[string]string{
				"command": pending.Command,
				"workdir": pending.Workdir,
				"error":   "shell command denied by user",
			},
		},
		Output: formatErrorOutput(errors.New("shell command denied by user")),
	}, nil
}

func (r *SessionRunner) emitShellApprovalOutcome(ctx context.Context, result ToolExecutionResult) error {
	if r.writeLocalMessage == nil || result.ToolName != tools.ToolNameRunShell {
		return nil
	}

	switch {
	case result.Status == ToolExecutionSucceeded && result.ApprovalReused:
		command := result.Summary.Fields["command"]
		workdir := result.Summary.Fields["workdir"]
		if command == "" || workdir == "" {
			return nil
		}
		return r.writeLocalMessage(ctx, "reused shell approval for "+strconv.Quote(command)+" in "+workdir)
	default:
		return nil
	}
}

func (r *SessionRunner) appendAssistantSummary(message provider.Message) error {
	summary := strings.TrimSpace(message.Content)
	if len(message.ToolCalls) > 0 {
		toolSummary := summarizeToolCalls(message.ToolCalls)
		switch {
		case summary == "":
			summary = toolSummary
		case toolSummary != "":
			summary += "\n" + toolSummary
		}
	}
	if summary == "" {
		return nil
	}

	return r.session.AppendMessage(session.Message{
		Role:    session.RoleAssistant,
		Content: summary,
	})
}

func (r *SessionRunner) emitAssistantChunk(ctx context.Context, chunk string) error {
	if r.writeAssistantChunk == nil || chunk == "" {
		return nil
	}

	return r.writeAssistantChunk(ctx, chunk)
}

func (r *SessionRunner) emitToolResult(ctx context.Context, event ToolExecutionEvent) error {
	if r.writeToolResult == nil {
		return nil
	}

	return r.writeToolResult(ctx, event)
}

func buildProviderTools(specs []tools.Spec) []provider.Tool {
	if len(specs) == 0 {
		return nil
	}

	converted := make([]provider.Tool, 0, len(specs))
	for _, spec := range specs {
		converted = append(converted, provider.Tool{
			Type:        provider.ToolTypeFunction,
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: cloneSchema(spec.InputSchema),
		})
	}

	return converted
}

func providerMessageFromSession(message session.Message) provider.Message {
	return provider.Message{
		Role:       provider.Role(message.Role),
		Content:    message.Content,
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
	}
}

func cloneProviderMessage(message provider.Message) provider.Message {
	cloned := provider.Message{
		Role:       message.Role,
		Content:    message.Content,
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
	}
	if len(message.ToolCalls) > 0 {
		cloned.ToolCalls = append([]provider.ToolCall(nil), message.ToolCalls...)
	}

	return cloned
}

func cloneUsage(usage *provider.Usage) *provider.Usage {
	if usage == nil {
		return nil
	}

	cloned := *usage
	if usage.PromptTokenDetails != nil {
		details := *usage.PromptTokenDetails
		cloned.PromptTokenDetails = &details
	}
	if usage.CompletionDetails != nil {
		details := *usage.CompletionDetails
		cloned.CompletionDetails = &details
	}

	return &cloned
}

func addUsage(total, next provider.Usage) provider.Usage {
	total.PromptTokens += next.PromptTokens
	total.CompletionTokens += next.CompletionTokens
	total.TotalTokens += next.TotalTokens

	if next.PromptTokenDetails != nil {
		if total.PromptTokenDetails == nil {
			total.PromptTokenDetails = &provider.PromptTokenDetails{}
		}
		total.PromptTokenDetails.CachedTokens += next.PromptTokenDetails.CachedTokens
		total.PromptTokenDetails.AudioTokens += next.PromptTokenDetails.AudioTokens
	}

	if next.CompletionDetails != nil {
		if total.CompletionDetails == nil {
			total.CompletionDetails = &provider.CompletionTokenDetails{}
		}
		total.CompletionDetails.ReasoningTokens += next.CompletionDetails.ReasoningTokens
		total.CompletionDetails.AudioTokens += next.CompletionDetails.AudioTokens
		total.CompletionDetails.AcceptedPredictionTokens += next.CompletionDetails.AcceptedPredictionTokens
		total.CompletionDetails.RejectedPredictionTokens += next.CompletionDetails.RejectedPredictionTokens
	}

	return total
}

func summarizeToolCalls(calls []provider.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}

	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			name = "unknown_tool"
		}
		parts = append(parts, name)
	}

	return "tool_calls: " + strings.Join(parts, ", ")
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cloned := make(map[string]any, len(schema))
	for key, value := range schema {
		cloned[key] = value
	}

	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

type toolCallBuilder struct {
	ID            string
	Type          provider.ToolType
	Name          string
	ArgumentsJSON strings.Builder
}

func mergeToolCallBuilders(builders []toolCallBuilder, deltas []provider.ToolCallDelta) []toolCallBuilder {
	for _, delta := range deltas {
		index := delta.Index
		for len(builders) <= index {
			builders = append(builders, toolCallBuilder{})
		}

		builder := &builders[index]
		if delta.ID != "" {
			builder.ID = delta.ID
		}
		if delta.Type != "" {
			builder.Type = delta.Type
		}
		if delta.Name != "" {
			builder.Name = delta.Name
		}
		if delta.ArgumentsChunk != "" {
			builder.ArgumentsJSON.WriteString(delta.ArgumentsChunk)
		}
	}

	return builders
}

func finalizeToolCalls(builders []toolCallBuilder) []provider.ToolCall {
	if len(builders) == 0 {
		return nil
	}

	calls := make([]provider.ToolCall, 0, len(builders))
	for _, builder := range builders {
		call := provider.ToolCall{
			ID:            builder.ID,
			Type:          builder.Type,
			Name:          builder.Name,
			ArgumentsJSON: builder.ArgumentsJSON.String(),
		}
		if call.Type == "" {
			call.Type = provider.ToolTypeFunction
		}
		calls = append(calls, call)
	}

	return calls
}
