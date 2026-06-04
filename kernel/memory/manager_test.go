// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// fixedNow pins the manager clock in tests so recency and ids-by-time are
// deterministic.
var fixedNow = time.Unix(1_700_000_000, 0).UTC()

func newTestManager(t *testing.T) (*Manager, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	m := NewManager(s, b)
	// Pin the clock for deterministic recency/ids-by-time behaviour.
	m.now = func() time.Time { return fixedNow }
	t.Cleanup(func() { b.Close(); j.Close() })
	return m, j
}

func countKind(t *testing.T, j *journal.Journal, k event.Kind) int {
	t.Helper()
	n := 0
	if err := j.Range(func(e *event.Event) error {
		if e.Kind == k {
			n++
		}
		return nil
	}); err != nil {
		t.Fatalf("range: %v", err)
	}
	return n
}

func TestRememberCreatesAndJournals(t *testing.T) {
	m, j := newTestManager(t)
	rec, created, err := m.Remember("corr-1", RememberSpec{Type: TypeFact, Subject: "lictor", Content: "Agezt is a Go agentic OS"})
	if err != nil || !created {
		t.Fatalf("remember: created=%v err=%v", created, err)
	}
	if rec.SourceEvent == "" {
		t.Fatal("created record must carry provenance (source_event)")
	}
	if countKind(t, j, event.KindMemoryWritten) != 1 {
		t.Fatal("expected one memory.written event")
	}
}

func TestRememberDedupReinforces(t *testing.T) {
	m, _ := newTestManager(t)
	r1, c1, _ := m.Remember("c", RememberSpec{Subject: "s", Content: "same", Confidence: 0.5})
	r2, c2, _ := m.Remember("c", RememberSpec{Subject: "s", Content: "same", Confidence: 0.5})
	if !c1 || c2 {
		t.Fatalf("first create=%v (want true), second create=%v (want false)", c1, c2)
	}
	if r1.ID != r2.ID {
		t.Fatal("identical content must dedupe onto same id")
	}
	if r2.Confidence <= r1.Confidence {
		t.Fatalf("reinforce should strengthen confidence: %v -> %v", r1.Confidence, r2.Confidence)
	}
	all, _ := m.All()
	if len(all) != 1 {
		t.Fatalf("dedup must not create a second record, got %d", len(all))
	}
}

func TestForgetTombstonesAndExcludesFromActive(t *testing.T) {
	m, j := newTestManager(t)
	rec, _, _ := m.Remember("c", RememberSpec{Subject: "s", Content: "forget me"})
	ok, err := m.Forget("c", rec.ID)
	if err != nil || !ok {
		t.Fatalf("forget: ok=%v err=%v", ok, err)
	}
	active, _ := m.Active()
	if len(active) != 0 {
		t.Fatal("forgotten record must not appear in Active")
	}
	// Still on disk (reversibility) and still gettable.
	if _, found, _ := m.Get(rec.ID); !found {
		t.Fatal("forgotten record must remain stored")
	}
	if countKind(t, j, event.KindMemoryForgotten) != 1 {
		t.Fatal("expected one memory.forgotten event")
	}
	// Forgetting an unknown id is a clean false, not an error.
	if ok, err := m.Forget("c", "nope"); ok || err != nil {
		t.Fatalf("forget unknown: ok=%v err=%v", ok, err)
	}
}

func TestRecallJournalsWhenMatched(t *testing.T) {
	m, j := newTestManager(t)
	_, _, _ = m.Remember("c", RememberSpec{Subject: "agezt", Content: "agezt journals everything"})
	hits, err := m.Recall("run-1", "agezt", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("recall: hits=%d err=%v", len(hits), err)
	}
	if countKind(t, j, event.KindMemoryRetrieved) != 1 {
		t.Fatal("a matched recall must journal memory.retrieved")
	}
	// A miss journals nothing (avoids noise).
	if _, err := m.Recall("run-1", "nonexistent-topic", 5); err != nil {
		t.Fatal(err)
	}
	if countKind(t, j, event.KindMemoryRetrieved) != 1 {
		t.Fatal("a zero-hit recall must not journal")
	}
}

func TestSupersedeLinksOld(t *testing.T) {
	m, j := newTestManager(t)
	old, _, _ := m.Remember("c", RememberSpec{Subject: "v", Content: "version 1"})
	newRec, err := m.Supersede("c", old.ID, RememberSpec{Subject: "v", Content: "version 2"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := m.Get(old.ID)
	if got.SupersededBy != newRec.ID {
		t.Fatalf("old.SupersededBy = %q, want %q", got.SupersededBy, newRec.ID)
	}
	active, _ := m.Active()
	if len(active) != 1 || active[0].ID != newRec.ID {
		t.Fatal("only the new record should be active")
	}
	if countKind(t, j, event.KindMemorySuperseded) != 1 {
		t.Fatal("expected one memory.superseded event")
	}
}

func TestRememberValidatesTypeAndContent(t *testing.T) {
	m, _ := newTestManager(t)
	if _, _, err := m.Remember("c", RememberSpec{Subject: "s", Content: "  "}); err != ErrEmptyContent {
		t.Fatalf("empty content: %v", err)
	}
	if _, _, err := m.Remember("c", RememberSpec{Type: "BOGUS", Subject: "s", Content: "x"}); err == nil {
		t.Fatal("invalid type must error")
	}
}

func TestMemoryToolRememberRecallForget(t *testing.T) {
	m, _ := newTestManager(t)
	tool := m.Tool()
	if tool.Definition().Name != "memory" {
		t.Fatalf("tool name = %q", tool.Definition().Name)
	}
	ctx := WithCorrelation(context.Background(), "run-9")

	in, _ := json.Marshal(map[string]any{"action": "remember", "subject": "proj", "content": "agezt remembers facts"})
	res, _ := tool.Invoke(ctx, in)
	if res.IsError {
		t.Fatalf("remember errored: %s", res.Output)
	}

	in, _ = json.Marshal(map[string]any{"action": "recall", "query": "agezt"})
	res, _ = tool.Invoke(ctx, in)
	if res.IsError || res.Output == "no relevant memory found" {
		t.Fatalf("recall should find the stored fact: %s", res.Output)
	}

	// Tool writes are tagged source=agent.
	all, _ := m.All()
	if len(all) != 1 || all[0].Tags["source"] != "agent" {
		t.Fatalf("tool write should be tagged source=agent: %+v", all)
	}

	in, _ = json.Marshal(map[string]any{"action": "bogus"})
	if res, _ := tool.Invoke(ctx, in); !res.IsError {
		t.Fatal("unknown action must be an error result")
	}
}

// fakeDistiller is a provider that returns a fixed JSON facts payload.
type fakeDistiller struct{ body string }

func (f fakeDistiller) Name() string { return "fake" }
func (f fakeDistiller) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return &agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: f.body}, StopReason: agent.StopEndTurn}, nil
}

func TestDistillStoresExtractedFacts(t *testing.T) {
	m, _ := newTestManager(t)
	prov := fakeDistiller{body: `Sure! {"facts":[{"subject":"lictor","content":"repo is the agezt monorepo","type":"FACT"},{"subject":"","content":"  ","type":"FACT"}]}`}
	ids, err := m.Distill(context.Background(), "run-d", prov, "model", "what is this?", "ran ls; it is a go repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 stored fact (blank one skipped), got %d", len(ids))
	}
	all, _ := m.All()
	if all[0].Tags["source"] != "distill" {
		t.Fatalf("distilled record must be tagged source=distill: %+v", all[0])
	}
}

func TestDistillNonJSONIsNoOp(t *testing.T) {
	m, _ := newTestManager(t)
	ids, err := m.Distill(context.Background(), "c", fakeDistiller{body: "I have no idea, sorry."}, "m", "intent", "transcript")
	if err != nil || len(ids) != 0 {
		t.Fatalf("non-JSON distill should be a clean no-op: ids=%d err=%v", len(ids), err)
	}
}

func TestDistillInvalidTypeCoercedToSummary(t *testing.T) {
	m, _ := newTestManager(t)
	// A model returning a non-canonical type string must not drop the fact — it is
	// stored as a SUMMARY rather than rejected (Distill is lossy-tolerant of the
	// model's type vocabulary).
	prov := fakeDistiller{body: `{"facts":[{"subject":"x","content":"a distilled note","type":"BOGUS_TYPE"}]}`}
	ids, err := m.Distill(context.Background(), "run-t", prov, "model", "intent", "transcript")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 stored fact, got %d", len(ids))
	}
	rec, ok, _ := m.Get(ids[0])
	if !ok {
		t.Fatal("stored record not found")
	}
	if rec.Type != TypeSummary {
		t.Errorf("invalid type should coerce to %q, got %q", TypeSummary, rec.Type)
	}
}

func TestDistillMalformedJSONObjectIsNoOp(t *testing.T) {
	m, _ := newTestManager(t)
	// Braces present but their contents aren't valid JSON — parseDistill must fail
	// closed (a clean no-op), distinct from the no-braces non-JSON case. Guards the
	// brace-scan from feeding garbage into json.Unmarshal and storing nothing/half.
	ids, err := m.Distill(context.Background(), "c", fakeDistiller{body: "here you go: {facts: not, valid json,,}"}, "m", "i", "t")
	if err != nil || len(ids) != 0 {
		t.Fatalf("malformed JSON object should be a clean no-op: ids=%d err=%v", len(ids), err)
	}
	if n := m.Count(); n != 0 {
		t.Errorf("nothing should be stored on malformed distill, Count=%d", n)
	}
}
