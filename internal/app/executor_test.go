package app

import (
	"context"
	"testing"
	"time"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
	"github.com/vhar-astro/goagent/internal/tools"
)

func TestExecutorRunShellRequiresInlineApproval(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-1", workspace, "chutes", "model")
	executor, err := NewExecutor(runtime, sess)
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), provider.ToolCall{
		ID:            "call-1",
		Type:          provider.ToolTypeFunction,
		Name:          tools.ToolNameRunShell,
		ArgumentsJSON: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.RequiresApproval() {
		t.Fatalf("RequiresApproval() = false, want true: %+v", result)
	}
	if result.PendingApproval == nil {
		t.Fatal("PendingApproval = nil, want populated request")
	}
	if _, ok := sess.PendingApproval(); !ok {
		t.Fatal("PendingApproval() missing request in session")
	}
}

func TestExecutorRunShellReusesOnlyExactApprovedCommand(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	sess := session.New("session-2", workspace, "chutes", "model")
	sess.ApproveShellCommand("pwd", workspace, time.Time{})

	executor, err := NewExecutor(runtime, sess)
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	same, err := executor.Execute(context.Background(), provider.ToolCall{
		ID:            "call-same",
		Type:          provider.ToolTypeFunction,
		Name:          tools.ToolNameRunShell,
		ArgumentsJSON: `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatalf("Execute(same) error = %v", err)
	}
	if same.RequiresApproval() {
		t.Fatalf("same command still required approval: %+v", same)
	}
	if !same.ApprovalReused {
		t.Fatal("ApprovalReused = false, want true for exact approved command")
	}

	changed, err := executor.Execute(context.Background(), provider.ToolCall{
		ID:            "call-changed",
		Type:          provider.ToolTypeFunction,
		Name:          tools.ToolNameRunShell,
		ArgumentsJSON: `{"command":"pwd -P"}`,
	})
	if err != nil {
		t.Fatalf("Execute(changed) error = %v", err)
	}
	if !changed.RequiresApproval() {
		t.Fatalf("changed command should require fresh approval: %+v", changed)
	}
}
