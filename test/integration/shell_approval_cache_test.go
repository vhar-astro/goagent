package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/vhar-astro/goagent/internal/app"
	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

func TestExactShellApprovalReuseAndFreshPromptScenario(t *testing.T) {
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
		integrationTextResponse("first done"),
		integrationToolCallResponse(provider.ToolCall{
			ID:            "call-2",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd"}`,
		}),
		integrationTextResponse("second done"),
		integrationToolCallResponse(provider.ToolCall{
			ID:            "call-3",
			Type:          provider.ToolTypeFunction,
			Name:          tools.ToolNameRunShell,
			ArgumentsJSON: `{"command":"pwd -P"}`,
		}),
		integrationTextResponse("third done"),
	}}

	var (
		promptCount   int
		localMessages []string
	)
	runner, err := app.NewSessionRunner(app.SessionRunnerOptions{
		Session: sess,
		Client:  client,
		Runtime: &runtime,
		RequestShellApproval: func(context.Context, session.PendingApprovalRequest) (bool, error) {
			promptCount++
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

	for _, prompt := range []string{"Run pwd.", "Run pwd again.", "Run pwd -P."} {
		if err := runner.RunTurn(context.Background(), prompt); err != nil {
			t.Fatalf("RunTurn(%q) error = %v", prompt, err)
		}
	}

	if promptCount != 2 {
		t.Fatalf("promptCount = %d, want 2 (first command and changed args)", promptCount)
	}

	foundReuse := false
	for _, message := range localMessages {
		if strings.Contains(message, `reused shell approval for "pwd"`) {
			foundReuse = true
		}
	}
	if !foundReuse {
		t.Fatalf("localMessages = %#v, want reused-approval outcome", localMessages)
	}
}
