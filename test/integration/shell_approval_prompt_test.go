package integration

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vhar-astro/goagent/internal/app"
	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

func TestInlineShellApprovalPromptScenario(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := app.NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-1", workspace, "chutes", "model")
	sess.SetBuiltInTools([]tools.Spec{tools.BuiltinShellSpec()})

	client := &integrationStubClient{responses: []provider.Response{
		integrationToolCallResponse(provider.ToolCall{
			ID:            "call-1",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}),
		integrationTextResponse("done"),
	}}

	var (
		promptCount   int
		localMessages []string
	)
	runner, err := app.NewSessionRunner(app.SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
		RequestShellApproval: func(_ context.Context, pending session.PendingApprovalRequest) (bool, error) {
			promptCount++
			if !strings.Contains(pending.PromptText, `Approve running "pwd"`) {
				t.Fatalf("unexpected prompt text: %q", pending.PromptText)
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
	if promptCount != 1 {
		t.Fatalf("promptCount = %d, want 1", promptCount)
	}
	if len(localMessages) == 0 || !strings.Contains(localMessages[0], `approved shell command "pwd"`) {
		t.Fatalf("localMessages = %#v, want approval outcome", localMessages)
	}
}

type integrationStubClient struct {
	requests  []provider.Request
	responses []provider.Response
}

func (c *integrationStubClient) Chat(_ context.Context, request provider.Request) (<-chan provider.StreamEvent, error) {
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

func integrationToolCallResponse(call provider.ToolCall) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:      provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{call},
		},
	}
}

func integrationTextResponse(content string) provider.Response {
	return provider.Response{
		Message: provider.Message{
			Role:    provider.RoleAssistant,
			Content: content,
		},
	}
}
