package app

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

func TestSessionRunnerInlineShellApprovalAndSameTurnResume(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntimeWithOptions(RuntimeOptions{
		WorkspaceRoot:  workspace,
		CommandTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntimeWithOptions() error = %v", err)
	}

	sess := session.New("session-1", workspace, "chutes", "model")
	sess.SetBuiltInTools([]tools.Spec{tools.BuiltinShellSpec()})

	client := &stubProviderClient{responses: []provider.Response{
		stubToolCallResponse(``, provider.ToolCall{
			ID:            "call-1",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}, provider.Usage{}),
		stubTextResponse("done", provider.Usage{}),
	}}

	var localMessages []string
	runner, err := NewSessionRunner(SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
		RequestShellApproval: func(_ context.Context, pending session.PendingApprovalRequest) (bool, error) {
			if !strings.Contains(pending.PromptText, `Approve running "pwd"`) {
				t.Fatalf("unexpected prompt: %q", pending.PromptText)
			}
			return true, nil
		},
		WriteLocalMessage: func(_ context.Context, message string) error {
			localMessages = append(localMessages, message)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	if err := runner.RunTurn(context.Background(), "Run pwd."); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(client.requests))
	}
	if len(localMessages) == 0 || !strings.Contains(localMessages[0], `approved shell command "pwd"`) {
		t.Fatalf("local messages = %#v, want approval outcome", localMessages)
	}
	if pending, ok := sess.PendingApproval(); ok {
		t.Fatalf("pending approval still set after approval flow: %+v", pending)
	}
	if !sess.IsShellCommandApproved("pwd", workspace) {
		t.Fatal("approved shell command not cached in session")
	}
}

func TestSessionRunnerDeniedShellApprovalContinuesTurn(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-2", workspace, "chutes", "model")
	sess.SetBuiltInTools([]tools.Spec{tools.BuiltinShellSpec()})

	client := &stubProviderClient{responses: []provider.Response{
		stubToolCallResponse(``, provider.ToolCall{
			ID:            "call-1",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}, provider.Usage{}),
		stubTextResponse("understood", provider.Usage{}),
	}}

	var localMessages []string
	runner, err := NewSessionRunner(SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
		RequestShellApproval: func(context.Context, session.PendingApprovalRequest) (bool, error) {
			return false, nil
		},
		WriteLocalMessage: func(_ context.Context, message string) error {
			localMessages = append(localMessages, message)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	if err := runner.RunTurn(context.Background(), "Run pwd."); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(client.requests))
	}
	if len(localMessages) == 0 || !strings.Contains(localMessages[0], `denied shell command "pwd"`) {
		t.Fatalf("local messages = %#v, want denial outcome", localMessages)
	}
	toolMessageFound := false
	for _, message := range sess.Conversation() {
		if message.Role == session.RoleTool && strings.Contains(message.Content, "shell command denied by user") {
			toolMessageFound = true
		}
	}
	if !toolMessageFound {
		t.Fatal("session transcript missing denied shell tool message")
	}
}

func TestSessionRunnerReusesExactShellApprovalWithoutPrompt(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-3", workspace, "chutes", "model")
	sess.SetBuiltInTools([]tools.Spec{tools.BuiltinShellSpec()})
	sess.ApproveShellCommand("pwd", workspace, time.Time{})

	client := &stubProviderClient{responses: []provider.Response{
		stubToolCallResponse(``, provider.ToolCall{
			ID:            "call-1",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}, provider.Usage{}),
		stubTextResponse("done", provider.Usage{}),
	}}

	var (
		localMessages []string
		promptCount   int
	)
	runner, err := NewSessionRunner(SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
		RequestShellApproval: func(context.Context, session.PendingApprovalRequest) (bool, error) {
			promptCount++
			return false, nil
		},
		WriteLocalMessage: func(_ context.Context, message string) error {
			localMessages = append(localMessages, message)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	if err := runner.RunTurn(context.Background(), "Run pwd."); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if promptCount != 0 {
		t.Fatalf("promptCount = %d, want 0 for reused approval", promptCount)
	}
	foundReuse := false
	for _, message := range localMessages {
		if strings.Contains(message, `reused shell approval for "pwd"`) {
			foundReuse = true
		}
	}
	if !foundReuse {
		t.Fatalf("local messages = %#v, want reused-approval outcome", localMessages)
	}
}

func TestSessionRunnerAggregatesUsageAcrossProviderResponses(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-usage", workspace, "chutes", "model")
	sess.SetBuiltInTools([]tools.Spec{tools.BuiltinShellSpec()})
	sess.ApproveShellCommand("pwd", workspace, time.Time{})

	client := &stubProviderClient{responses: []provider.Response{
		stubToolCallResponse(``, provider.ToolCall{
			ID:            "call-1",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}, provider.Usage{PromptTokens: 120, CompletionTokens: 45, TotalTokens: 165}),
		stubTextResponse("done", provider.Usage{PromptTokens: 30, CompletionTokens: 10, TotalTokens: 40}),
	}}

	runner, err := NewSessionRunner(SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
	})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	if err := runner.RunTurn(context.Background(), "Run pwd."); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	usage := runner.LastTurnUsage()
	if usage.PromptTokens != 150 || usage.CompletionTokens != 55 || usage.TotalTokens != 205 {
		t.Fatalf("LastTurnUsage() = %+v, want prompt=150 completion=55 total=205", usage)
	}
}

type stubProviderClient struct {
	requests  []provider.Request
	responses []provider.Response
}

func (c *stubProviderClient) Chat(_ context.Context, request provider.Request) (<-chan provider.StreamEvent, error) {
	c.requests = append(c.requests, request)
	index := len(c.requests) - 1
	if index >= len(c.responses) {
		return nil, io.EOF
	}

	events := make(chan provider.StreamEvent, 8)
	response := c.responses[index]
	go func() {
		defer close(events)
		events <- provider.StreamEvent{Type: provider.EventResponseStart, ResponseID: "resp", Model: request.Model}
		if response.Message.Content != "" {
			events <- provider.StreamEvent{
				Type:       provider.EventMessageDelta,
				ResponseID: "resp",
				Model:      request.Model,
				Delta: &provider.MessageDelta{
					Role:    provider.RoleAssistant,
					Content: response.Message.Content,
				},
			}
		}
		if len(response.Message.ToolCalls) > 0 {
			deltas := make([]provider.ToolCallDelta, 0, len(response.Message.ToolCalls))
			for idx, call := range response.Message.ToolCalls {
				deltas = append(deltas, provider.ToolCallDelta{
					Index:          idx,
					ID:             call.ID,
					Type:           call.Type,
					Name:           call.Name,
					ArgumentsChunk: call.ArgumentsJSON,
				})
			}
			events <- provider.StreamEvent{
				Type:       provider.EventToolCallDelta,
				ResponseID: "resp",
				Model:      request.Model,
				Delta:      &provider.MessageDelta{Tools: deltas},
			}
		}
		if response.Usage.TotalTokens > 0 || response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
			usage := response.Usage
			events <- provider.StreamEvent{
				Type:       provider.EventUsage,
				ResponseID: "resp",
				Model:      request.Model,
				Usage:      &usage,
			}
		}
		complete := provider.StreamEvent{Type: provider.EventResponseComplete, ResponseID: "resp", Model: request.Model}
		if response.Usage.TotalTokens > 0 || response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
			usage := response.Usage
			complete.Usage = &usage
		}
		events <- complete
	}()

	return events, nil
}

func stubToolCallResponse(content string, call provider.ToolCall, usage provider.Usage) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content,
			ToolCalls: []provider.ToolCall{call},
		},
		Usage: usage,
	}
}

func stubTextResponse(content string, usage provider.Usage) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: content,
		},
		Usage: usage,
	}
}
