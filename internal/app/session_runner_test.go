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
		}),
		stubTextResponse("done"),
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
		}),
		stubTextResponse("understood"),
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
		}),
		stubTextResponse("done"),
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
		events <- provider.StreamEvent{Type: provider.EventResponseComplete, ResponseID: "resp", Model: request.Model}
	}()

	return events, nil
}

func stubToolCallResponse(content string, call provider.ToolCall) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content,
			ToolCalls: []provider.ToolCall{call},
		},
	}
}

func stubTextResponse(content string) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: content,
		},
	}
}
