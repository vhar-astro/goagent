package session

import (
	"testing"
	"time"

	"github.com/vhar-astro/goagent/internal/tools"
)

func TestApproveStoresExplicitApproval(t *testing.T) {
	t.Parallel()

	s := New("session-1", "/workspace", "chutes", "model")
	approvedAt := time.Date(2026, time.June, 5, 12, 0, 0, 0, time.UTC)

	if err := s.Approve(tools.CapabilityWrite, approvedAt); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !s.IsApproved(tools.CapabilityWrite) {
		t.Fatalf("IsApproved(%q) = false, want true", tools.CapabilityWrite)
	}

	approval, ok := s.Approval(tools.CapabilityWrite)
	if !ok {
		t.Fatalf("Approval(%q) not found", tools.CapabilityWrite)
	}
	if approval.Name != tools.CapabilityWrite {
		t.Fatalf("approval.Name = %q, want %q", approval.Name, tools.CapabilityWrite)
	}
	if !approval.Approved {
		t.Fatalf("approval.Approved = false, want true")
	}
	if !approval.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("approval.ApprovedAt = %v, want %v", approval.ApprovedAt, approvedAt)
	}
}

func TestApproveWithZeroTimeSetsTimestamp(t *testing.T) {
	t.Parallel()

	s := New("session-2", "/workspace", "chutes", "model")

	if err := s.Approve(tools.CapabilityModule, time.Time{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	approval, ok := s.Approval(tools.CapabilityModule)
	if !ok {
		t.Fatalf("Approval(%q) not found", tools.CapabilityModule)
	}
	if approval.ApprovedAt.IsZero() {
		t.Fatalf("approval.ApprovedAt = zero time, want generated timestamp")
	}
}

func TestApproveRejectsUnknownCapabilityAndRevokeClearsApproval(t *testing.T) {
	t.Parallel()

	s := New("session-3", "/workspace", "chutes", "model")

	if err := s.Approve("unknown", time.Time{}); err == nil {
		t.Fatal("Approve() error = nil, want unknown capability error")
	}
	if s.IsApproved("unknown") {
		t.Fatal("IsApproved(unknown) = true, want false")
	}

	if err := s.Approve(tools.CapabilityWeb, time.Now().UTC()); err != nil {
		t.Fatalf("Approve(valid) error = %v", err)
	}
	s.Revoke(tools.CapabilityWeb)

	if s.IsApproved(tools.CapabilityWeb) {
		t.Fatalf("IsApproved(%q) = true after revoke, want false", tools.CapabilityWeb)
	}
	if _, ok := s.Approval(tools.CapabilityWeb); ok {
		t.Fatalf("Approval(%q) still present after revoke", tools.CapabilityWeb)
	}
}

func TestApproveRejectsShellCapabilityApproval(t *testing.T) {
	t.Parallel()

	s := New("session-4", "/workspace", "chutes", "model")

	if err := s.Approve(tools.CapabilityShell, time.Time{}); err == nil {
		t.Fatal("Approve(shell) error = nil, want command-specific approval error")
	}
	if s.IsApproved(tools.CapabilityShell) {
		t.Fatal("IsApproved(shell) = true, want false")
	}
}

func TestShellApprovalSignatureAndReuseKey(t *testing.T) {
	t.Parallel()

	command := "go test ./internal/session"
	workdir := "/workspace/internal/session"

	if got := ShellApprovalSignature(command, workdir); got != ShellApprovalSignature(command, workdir) {
		t.Fatal("ShellApprovalSignature() not stable for identical input")
	}
	if ShellApprovalSignature(command, workdir) == ShellApprovalSignature(command+" -run TestOne", workdir) {
		t.Fatal("ShellApprovalSignature() reused key for changed command")
	}
	if ShellApprovalSignature(command, workdir) == ShellApprovalSignature(command, "/workspace") {
		t.Fatal("ShellApprovalSignature() reused key for changed workdir")
	}

	s := New("session-5", "/workspace", "chutes", "model")
	approvedAt := time.Date(2026, time.June, 6, 9, 30, 0, 0, time.UTC)
	approval := s.ApproveShellCommand(command, workdir, approvedAt)

	if !s.IsShellCommandApproved(command, workdir) {
		t.Fatal("IsShellCommandApproved() = false, want true")
	}

	stored, ok := s.ShellApproval(command, workdir)
	if !ok {
		t.Fatal("ShellApproval() missing stored approval")
	}
	if stored != approval {
		t.Fatalf("ShellApproval() = %+v, want %+v", stored, approval)
	}
	if s.IsShellCommandApproved(command+" -run TestOne", workdir) {
		t.Fatal("IsShellCommandApproved() = true for changed command, want false")
	}
	if s.IsShellCommandApproved(command, "/workspace") {
		t.Fatal("IsShellCommandApproved() = true for changed workdir, want false")
	}

	s.RevokeShellCommandApproval(command, workdir)
	if s.IsShellCommandApproved(command, workdir) {
		t.Fatal("IsShellCommandApproved() = true after revoke, want false")
	}
}

func TestPendingShellApprovalLifecycle(t *testing.T) {
	t.Parallel()

	s := New("session-6", "/workspace", "chutes", "model")
	request := PendingApprovalRequest{
		ToolCallID: "call-1",
		ToolName:   "run_shell",
		Command:    "pwd",
		Workdir:    "/workspace",
		PromptText: "Approve running \"pwd\" in /workspace? [y/N]",
	}

	set := s.SetPendingShellApproval(request)
	if set.Status != PendingApprovalStatusPending {
		t.Fatalf("SetPendingShellApproval().Status = %q, want %q", set.Status, PendingApprovalStatusPending)
	}
	if set.Signature != ShellApprovalSignature(request.Command, request.Workdir) {
		t.Fatalf("SetPendingShellApproval().Signature = %q, want signature", set.Signature)
	}

	pending, ok := s.PendingApproval()
	if !ok {
		t.Fatal("PendingApproval() missing request")
	}
	if pending != set {
		t.Fatalf("PendingApproval() = %+v, want %+v", pending, set)
	}

	pending.Status = PendingApprovalStatusDenied
	again, ok := s.PendingApproval()
	if !ok {
		t.Fatal("PendingApproval() missing request after local mutation")
	}
	if again.Status != PendingApprovalStatusPending {
		t.Fatalf("PendingApproval() leaked caller mutation, got %q", again.Status)
	}

	if !s.ClearPendingShellApproval() {
		t.Fatal("ClearPendingShellApproval() = false, want true")
	}
	if _, ok := s.PendingApproval(); ok {
		t.Fatal("PendingApproval() present after clear")
	}
	if s.ClearPendingShellApproval() {
		t.Fatal("ClearPendingShellApproval() = true on empty state, want false")
	}
}

func TestPendingShellApprovalTransitions(t *testing.T) {
	t.Parallel()

	t.Run("approve", func(t *testing.T) {
		t.Parallel()

		s := New("session-7", "/workspace", "chutes", "model")
		request := s.SetPendingShellApproval(PendingApprovalRequest{
			ToolCallID: "call-approve",
			ToolName:   "run_shell",
			Command:    "go test ./internal/session",
			Workdir:    "/workspace",
			PromptText: "Approve?",
		})
		approvedAt := time.Date(2026, time.June, 6, 10, 0, 0, 0, time.UTC)

		approval, ok := s.ApprovePendingShellApproval(approvedAt)
		if !ok {
			t.Fatal("ApprovePendingShellApproval() = false, want true")
		}
		if approval.Signature != request.Signature {
			t.Fatalf("ApprovePendingShellApproval().Signature = %q, want %q", approval.Signature, request.Signature)
		}
		if !approval.ApprovedAt.Equal(approvedAt) {
			t.Fatalf("ApprovePendingShellApproval().ApprovedAt = %v, want %v", approval.ApprovedAt, approvedAt)
		}

		pending, ok := s.PendingApproval()
		if !ok {
			t.Fatal("PendingApproval() missing after approve transition")
		}
		if pending.Status != PendingApprovalStatusApproved {
			t.Fatalf("pending.Status = %q, want %q", pending.Status, PendingApprovalStatusApproved)
		}
		if !s.IsShellCommandApproved(request.Command, request.Workdir) {
			t.Fatal("IsShellCommandApproved() = false after approve transition")
		}
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()

		s := New("session-8", "/workspace", "chutes", "model")
		request := s.SetPendingShellApproval(PendingApprovalRequest{
			ToolCallID: "call-deny",
			ToolName:   "run_shell",
			Command:    "rm -rf /tmp/nope",
			Workdir:    "/workspace",
			PromptText: "Approve?",
		})

		if !s.DenyPendingShellApproval() {
			t.Fatal("DenyPendingShellApproval() = false, want true")
		}

		pending, ok := s.PendingApproval()
		if !ok {
			t.Fatal("PendingApproval() missing after deny transition")
		}
		if pending.Status != PendingApprovalStatusDenied {
			t.Fatalf("pending.Status = %q, want %q", pending.Status, PendingApprovalStatusDenied)
		}
		if s.IsShellCommandApproved(request.Command, request.Workdir) {
			t.Fatal("IsShellCommandApproved() = true after denial, want false")
		}
	})
}
