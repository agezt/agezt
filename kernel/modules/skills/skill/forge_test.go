// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/plugins/providers/mock"
)

var fixedNow = time.Unix(1_700_000_000, 0).UTC()

func newTestForge(t *testing.T) (*Forge, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("skill.Open: %v", err)
	}
	f := NewForge(s, b)
	f.now = func() time.Time { return fixedNow }
	t.Cleanup(func() { b.Close(); j.Close() })
	return f, j
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

func TestCreateDraftJournals(t *testing.T) {
	f, j := newTestForge(t)
	sk, created, err := f.Create("c", CreateSpec{Name: "diagnose-ci", Description: "fix CI", Body: "do the thing"})
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if sk.Status != StatusDraft {
		t.Errorf("new skill should be draft, got %s", sk.Status)
	}
	if sk.SourceEvent == "" {
		t.Error("created skill must carry provenance")
	}
	if countKind(t, j, event.KindSkillCreated) != 1 {
		t.Error("expected 1 skill.created event")
	}
}

func TestPromoteChainAndIllegalSkip(t *testing.T) {
	f, j := newTestForge(t)
	sk, _, _ := f.Create("c", CreateSpec{Name: "s", Body: "b"})

	st, err := f.Promote("c", sk.ID) // draft → shadow
	if err != nil || st != StatusShadow {
		t.Fatalf("promote 1: status=%s err=%v", st, err)
	}
	st, err = f.Promote("c", sk.ID) // shadow → active
	if err != nil || st != StatusActive {
		t.Fatalf("promote 2: status=%s err=%v", st, err)
	}
	if countKind(t, j, event.KindSkillPromoted) != 2 {
		t.Error("expected 2 skill.promoted events")
	}
	// Active has no further promotion.
	if _, err := f.Promote("c", sk.ID); err == nil {
		t.Error("promoting an active skill should be illegal")
	}
}

func TestQuarantineThenReactivate(t *testing.T) {
	f, _ := newTestForge(t)
	sk, _, _ := f.Create("c", CreateSpec{Name: "s", Body: "b"})
	_, _ = f.Promote("c", sk.ID)
	_, _ = f.Promote("c", sk.ID) // active
	if err := f.Quarantine("c", sk.ID, "regression"); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	got, _, _ := f.Get(sk.ID)
	if got.Status != StatusQuarantined {
		t.Fatalf("status = %s want quarantined", got.Status)
	}
	// quarantined → active is legal (un-quarantine).
	if st, err := f.Promote("c", sk.ID); err != nil || st != StatusActive {
		t.Errorf("un-quarantine failed: status=%s err=%v", st, err)
	}
}

func TestRevertArchivesAndRestoresParent(t *testing.T) {
	f, j := newTestForge(t)
	// v1 active.
	v1, _, _ := f.Create("c", CreateSpec{Name: "deploy", Body: "v1 steps"})
	_, _ = f.Promote("c", v1.ID)
	_, _ = f.Promote("c", v1.ID) // active

	// v2: same name, new body → lineage includes v1.
	v2, _, _ := f.Create("c", CreateSpec{Name: "deploy", Body: "v2 steps"})
	if len(v2.Lineage) == 0 || v2.Lineage[len(v2.Lineage)-1] != v1.ID {
		t.Fatalf("v2 lineage should include v1: %+v", v2.Lineage)
	}
	_, _ = f.Promote("c", v2.ID)
	_, _ = f.Promote("c", v2.ID) // v2 active

	restored, err := f.Revert("c", v2.ID)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if restored != v1.ID {
		t.Errorf("revert should restore v1, got %q", restored)
	}
	got2, _, _ := f.Get(v2.ID)
	if got2.Status != StatusArchived {
		t.Errorf("reverted skill should be archived, got %s", got2.Status)
	}
	got1, _, _ := f.Get(v1.ID)
	if got1.Status != StatusActive {
		t.Errorf("parent should be re-activated, got %s", got1.Status)
	}
	if countKind(t, j, event.KindSkillReverted) != 1 {
		t.Error("expected 1 skill.reverted event")
	}
}

func TestActivateRanksActiveAndJournals(t *testing.T) {
	f, j := newTestForge(t)
	sk, _, _ := f.Create("c", CreateSpec{Name: "diagnose-ci", Description: "diagnose failing CI", Triggers: []string{"ci"}, Body: "b"})
	_, _ = f.Promote("c", sk.ID)
	_, _ = f.Promote("c", sk.ID) // active

	hits, err := f.Activate("run-1", "my CI is failing", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("activate: hits=%d err=%v", len(hits), err)
	}
	if countKind(t, j, event.KindSkillActivated) != 1 {
		t.Error("activation with hits should journal skill.activated")
	}
	got, _, _ := f.Get(sk.ID)
	if got.Metrics.Uses != 1 {
		t.Errorf("activation should bump uses, got %d", got.Metrics.Uses)
	}
	// A draft never activates.
	draft, _, _ := f.Create("c", CreateSpec{Name: "other", Description: "deploy stuff", Body: "b2"})
	if h2, _ := f.Activate("run-2", "deploy stuff now", 5); len(h2) != 0 {
		t.Errorf("draft must not activate, got %+v", h2)
	}
	_ = draft
}

func TestProposeCreatesDraft(t *testing.T) {
	f, _ := newTestForge(t)
	prov := mock.New(mock.FinalText(`{"skill":{"name":"restart-service","description":"restart a crashed service","triggers":["ops"],"body":"1. find pid 2. kill 3. relaunch","tools":["shell"]}}`))
	ids, err := f.Propose(context.Background(), "c", prov, "model", "the service crashed", "ran shell commands")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 proposed skill, got %d", len(ids))
	}
	sk, _, _ := f.Get(ids[0])
	if sk.Status != StatusDraft || sk.Name != "restart-service" {
		t.Errorf("proposed skill wrong: %+v", sk)
	}
}

func TestProposeDeclineYieldsNothing(t *testing.T) {
	f, _ := newTestForge(t)
	prov := mock.New(mock.FinalText(`{"skill":null}`))
	ids, err := f.Propose(context.Background(), "c", prov, "model", "trivial", "did nothing special")
	if err != nil || len(ids) != 0 {
		t.Errorf("decline should yield no skill: ids=%v err=%v", ids, err)
	}
}

func TestNilBusStoreOnly(t *testing.T) {
	s, _ := Open(t.TempDir())
	f := NewForge(s, nil)
	f.now = func() time.Time { return fixedNow }
	if _, _, err := f.Create("", CreateSpec{Name: "x", Body: "b"}); err != nil {
		t.Fatalf("create without bus should work: %v", err)
	}
}
