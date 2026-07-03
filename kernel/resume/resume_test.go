package resume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func sampleTicket(corr string) *Ticket {
	ceil := 1
	return &Ticket{
		Corr:         corr,
		Intent:       "do the thing",
		AgentSlug:    "researcher",
		Kind:         KindRun,
		TrustCeiling: &ceil,
		MaxCostMc:    5000,
		WakeSource:   "standing",
		Resumable:    true,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "do the thing"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "shell", Input: []byte(`{"cmd":"ls"}`)}}},
			{Role: agent.RoleTool, ToolCallID: "c1", Content: "a\nb\n"},
		},
		Iter: 3,
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	s := newStore(t)
	in := sampleTicket("run-abc")
	if err := s.Put(in); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok, err := s.Get("run-abc")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.Intent != in.Intent || got.AgentSlug != in.AgentSlug || got.Kind != in.Kind {
		t.Fatalf("metadata mismatch: %+v", got)
	}
	if got.TrustCeiling == nil || *got.TrustCeiling != 1 {
		t.Fatalf("trust ceiling not preserved: %v", got.TrustCeiling)
	}
	if len(got.Messages) != 3 || got.Messages[2].ToolCallID != "c1" {
		t.Fatalf("message snapshot not preserved: %+v", got.Messages)
	}
	if got.Iter != 3 {
		t.Fatalf("iter = %d, want 3", got.Iter)
	}
	if got.Status != StatusActive {
		t.Fatalf("status = %q, want active", got.Status)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not stamped")
	}
}

func TestGetMissing(t *testing.T) {
	s := newStore(t)
	_, ok, err := s.Get("run-nope")
	if err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}
}

func TestSnapshotPreservesStatusAndMetadata(t *testing.T) {
	s := newStore(t)
	tk := sampleTicket("run-xyz")
	tk.Messages = nil
	tk.Iter = 0
	if err := s.Put(tk); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.MarkSuspendedAll(); err != nil {
		t.Fatalf("MarkSuspendedAll: %v", err)
	}
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	if err := s.Snapshot("run-xyz", msgs, 7); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	got, _, _ := s.Get("run-xyz")
	if got.Status != StatusSuspended {
		t.Fatalf("snapshot clobbered status: %q", got.Status)
	}
	if got.Intent != "do the thing" {
		t.Fatalf("snapshot clobbered metadata: %q", got.Intent)
	}
	if got.Iter != 7 || len(got.Messages) != 1 {
		t.Fatalf("snapshot not applied: iter=%d msgs=%d", got.Iter, len(got.Messages))
	}
}

func TestSnapshotOnMissingIsNoop(t *testing.T) {
	s := newStore(t)
	if err := s.Snapshot("run-gone", []agent.Message{{Content: "x"}}, 1); err != nil {
		t.Fatalf("Snapshot on missing should be nil, got %v", err)
	}
	if _, ok, _ := s.Get("run-gone"); ok {
		t.Fatalf("Snapshot resurrected a deleted ticket")
	}
}

func TestDeleteIdempotent(t *testing.T) {
	s := newStore(t)
	_ = s.Put(sampleTicket("run-del"))
	if err := s.Delete("run-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("run-del"); err != nil {
		t.Fatalf("second Delete should be nil, got %v", err)
	}
}

func TestMarkSuspendedAll(t *testing.T) {
	s := newStore(t)
	_ = s.Put(sampleTicket("run-1"))
	_ = s.Put(sampleTicket("run-2"))
	n, err := s.MarkSuspendedAll()
	if err != nil || n != 2 {
		t.Fatalf("MarkSuspendedAll n=%d err=%v", n, err)
	}
	// Idempotent: already-suspended tickets aren't re-counted.
	n2, _ := s.MarkSuspendedAll()
	if n2 != 0 {
		t.Fatalf("second MarkSuspendedAll n=%d, want 0", n2)
	}
	for _, corr := range []string{"run-1", "run-2"} {
		got, _, _ := s.Get(corr)
		if got.Status != StatusSuspended {
			t.Fatalf("%s not suspended: %q", corr, got.Status)
		}
	}
}

func TestIncrementAttemptDurable(t *testing.T) {
	s := newStore(t)
	_ = s.Put(sampleTicket("run-att"))
	for want := 1; want <= 3; want++ {
		got, err := s.IncrementAttempt("run-att")
		if err != nil || got != want {
			t.Fatalf("IncrementAttempt got=%d want=%d err=%v", got, want, err)
		}
	}
	// Persisted, not just in memory.
	got, _, _ := s.Get("run-att")
	if got.Attempts != 3 {
		t.Fatalf("attempts not persisted: %d", got.Attempts)
	}
	if _, err := s.IncrementAttempt("run-missing"); err == nil {
		t.Fatalf("IncrementAttempt on missing should error")
	}
}

func TestQuarantineRemovesFromScan(t *testing.T) {
	s := newStore(t)
	_ = s.Put(sampleTicket("run-poison"))
	if err := s.Quarantine("run-poison"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	list, _ := s.List()
	if len(list) != 0 {
		t.Fatalf("quarantined ticket still listed: %d", len(list))
	}
	if _, ok, _ := s.Get("run-poison"); ok {
		t.Fatalf("quarantined ticket still gettable in scan dir")
	}
	// The file survives for postmortem in the quarantine subdir.
	entries, _ := os.ReadDir(s.quarDir)
	if len(entries) != 1 {
		t.Fatalf("quarantine dir has %d files, want 1", len(entries))
	}
	if err := s.Quarantine("run-poison"); err != nil {
		t.Fatalf("Quarantine on already-gone should be nil, got %v", err)
	}
}

func TestListSkipsQuarantineAndSorts(t *testing.T) {
	s := newStore(t)
	_ = s.Put(sampleTicket("run-a"))
	_ = s.Put(sampleTicket("run-b"))
	_ = s.Quarantine("run-b")
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Corr != "run-a" {
		t.Fatalf("List returned %d tickets: %+v", len(list), list)
	}
}

func TestOversizedSnapshotDropped(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "resume"), 4096)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tk := sampleTicket("run-big")
	big := strings.Repeat("x", 8192)
	tk.Messages = []agent.Message{{Role: agent.RoleTool, ToolCallID: "c1", Content: big}}
	if err := s.Put(tk); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, _, _ := s.Get("run-big")
	if !got.SnapshotDropped {
		t.Fatalf("oversized snapshot not dropped")
	}
	if len(got.Messages) != 0 {
		t.Fatalf("messages retained despite drop: %d", len(got.Messages))
	}
	// Dispatch metadata survives so intent-replay still works.
	if got.Intent != "do the thing" {
		t.Fatalf("metadata lost on drop")
	}
}

func TestSafeNameStripsPathSyntax(t *testing.T) {
	if n := safeName("../../etc/passwd"); strings.ContainsAny(n, "/\\") || strings.Contains(n, "..") {
		t.Fatalf("safeName leaked path syntax: %q", n)
	}
}
