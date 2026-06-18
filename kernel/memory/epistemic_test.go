// SPDX-License-Identifier: MIT

package memory

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryExpirationBarsRecallButRetainsRecord(t *testing.T) {
	m, _ := newTestManager(t)
	start := fixedNow
	m.now = func() time.Time { return start }
	rec, _, err := m.Remember("c", RememberSpec{
		Subject:    "weather",
		Content:    "it is raining",
		Evidence:   EvidenceObserved,
		HalfLifeMS: int64(time.Hour / time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}

	m.now = func() time.Time { return start.Add(2 * time.Hour) }
	hits, err := m.Recall("c", "raining", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expired memory must not be recalled, got %d hit(s)", len(hits))
	}
	if got, found, _ := m.Get(rec.ID); !found || !got.Expired(m.now().UnixMilli()) {
		t.Fatalf("expired record should remain stored and marked expired: found=%v rec=%+v", found, got)
	}
}

func TestRememberReconstructsSuspendedRecord(t *testing.T) {
	m, _ := newTestManager(t)
	rec, _, err := m.Remember("c", RememberSpec{Subject: "deploy", Content: "prod is in eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := m.Suspend("c", rec.ID, "contradiction")
	if err != nil || !ok {
		t.Fatalf("suspend: ok=%v err=%v", ok, err)
	}
	if hits, _ := m.Recall("c", "prod", 10); len(hits) != 0 {
		t.Fatalf("suspended memory must not be recalled, got %d hit(s)", len(hits))
	}
	rec2, created, err := m.Remember("c", RememberSpec{Subject: "deploy", Content: "prod is in eu-west-1"})
	if err != nil || created {
		t.Fatalf("reinforce suspended: created=%v err=%v", created, err)
	}
	if rec2.Suspended() {
		t.Fatal("reinforcement should reconstruct/unsuspend the record")
	}
}

func TestAuditReportsExpiredSuspendedAndContradictions(t *testing.T) {
	m, _ := newTestManager(t)
	now := fixedNow
	m.now = func() time.Time { return now }
	expired, _, _ := m.Remember("c", RememberSpec{
		Subject: "old", Content: "stale", HalfLifeMS: int64(time.Millisecond / time.Millisecond),
	})
	suspended, _, _ := m.Remember("c", RememberSpec{Subject: "thin", Content: "uncertain"})
	_, _ = m.Suspend("c", suspended.ID, "needs source")
	_, _, _ = m.Remember("c", RememberSpec{Type: TypeFact, Subject: "region", Content: "prod is eu-west-1"})
	_, _, _ = m.Remember("c", RememberSpec{Type: TypeFact, Subject: "region", Content: "prod is us-east-1"})

	m.now = func() time.Time { return now.Add(time.Second) }
	report, err := m.Audit()
	if err != nil {
		t.Fatal(err)
	}
	if report.Expired != 1 || report.ExpiredIDs[0] != expired.ID {
		t.Fatalf("expired report = %+v, want %s", report, expired.ID)
	}
	if report.Suspended != 1 || report.SuspendedIDs[0] != suspended.ID {
		t.Fatalf("suspended report = %+v, want %s", report, suspended.ID)
	}
	if report.ContradictionLoad != 1 || len(report.Contradictions) != 1 {
		t.Fatalf("contradiction report = %+v, want one competing pair", report)
	}
}

func TestAutomaticLowValueMemoryRejected(t *testing.T) {
	m, _ := newTestManager(t)
	_, _, err := m.Remember("c", RememberSpec{
		Subject: "test",
		Content: "ran go test ./... and it passed",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
	})
	if err == nil {
		t.Fatal("agent execution logs must not enter long-term memory")
	}

	if _, _, err := m.Remember("c", RememberSpec{
		Subject: "project",
		Content: "Agezt is a Go agentic OS",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
	}); err != nil {
		t.Fatalf("durable agent fact should be retained: %v", err)
	}

	if _, _, err := m.Remember("c", RememberSpec{
		Subject: "city",
		Content: "Tehran is a city in Iran",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
	}); err != nil {
		t.Fatalf("durable fact containing 'ran ' should not be mistaken for an execution log: %v", err)
	}
}

func TestSystemAgentLogMemoryRejected(t *testing.T) {
	m, _ := newTestManager(t)
	logs := []RememberSpec{
		{
			Type:    TypeSummary,
			Subject: "health",
			Content: "SUMMARY: health sweep complete; daemon healthy; no action",
			Tags:    map[string]string{"source": "agent"},
			Actor:   "guardian-health",
		},
		{
			Type:    TypeObservation,
			Subject: "runs",
			Content: "OBSERVATION: active runs checked via introspect; no issues",
			Tags:    map[string]string{"source": "agent"},
			Actor:   "health-sentinel",
		},
		{
			Type:    TypeSummary,
			Subject: "health",
			Content: "SUMMARY: daemon healthy; no action",
			Tags:    map[string]string{"source": "agent", "scope": "guardian-health"},
		},
	}
	for _, spec := range logs {
		if _, _, err := m.Remember("c", spec); err == nil {
			t.Fatalf("system agent log memory should be rejected: %+v", spec)
		} else if !strings.Contains(err.Error(), "system_agent_") {
			t.Fatalf("system agent log rejection reason should be explicit, got %v", err)
		}
	}
	if got := m.Count(); got != 0 {
		t.Fatalf("rejected system logs must not dirty the store, Count=%d", got)
	}

	if _, _, err := m.Remember("c", RememberSpec{
		Type:    TypeFact,
		Subject: "routing",
		Content: "Provider routing uses the task model chain setting",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "guardian-routing",
	}); err != nil {
		t.Fatalf("durable system-agent fact should still be retainable: %v", err)
	}
}

func TestCleanLowValueHardDeletesBacklog(t *testing.T) {
	m, _ := newTestManager(t)
	low, _, err := m.Remember("seed", RememberSpec{
		Subject: "test",
		Content: "ran go test ./... and it passed",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
		Force:   true, // simulate old backlog from before write-time filtering
	})
	if err != nil {
		t.Fatal(err)
	}
	keep, _, err := m.Remember("seed", RememberSpec{
		Subject: "project",
		Content: "Agezt uses Go for the kernel",
		Tags:    map[string]string{"source": "agent"},
		Actor:   "agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := m.CleanLowValue("c", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Rejected != 1 || report.Removed != 0 {
		t.Fatalf("dry-run report = %+v, want one rejected and none removed", report)
	}
	if _, found, _ := m.Get(low.ID); !found {
		t.Fatal("dry-run must not delete low-value record")
	}

	report, err = m.CleanLowValue("c", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Removed != 1 {
		t.Fatalf("execute removed = %d, want 1 (%+v)", report.Removed, report)
	}
	if _, found, _ := m.Get(low.ID); found {
		t.Fatal("low-value backlog record should be hard-deleted")
	}
	if got, found, _ := m.Get(keep.ID); !found || got.Tombstoned {
		t.Fatal("durable fact should survive clean")
	}
}
