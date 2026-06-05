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

	if err := s.Approve(tools.CapabilityShell, time.Time{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	approval, ok := s.Approval(tools.CapabilityShell)
	if !ok {
		t.Fatalf("Approval(%q) not found", tools.CapabilityShell)
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
