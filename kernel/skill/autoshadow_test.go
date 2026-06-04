// SPDX-License-Identifier: MIT

package skill

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// wellFormedSpec is a draft that passes the shadow-test: substantive body plus a
// trigger (so it is retrievable).
func wellFormedSpec(name string) CreateSpec {
	return CreateSpec{
		Name:        name,
		Description: "diagnose a red build and propose a fix",
		Triggers:    []string{"ci", "build-failure"},
		Body:        "1. read the failing job log\n2. find the first error\n3. propose a fix",
	}
}

func TestShadowTest(t *testing.T) {
	cases := []struct {
		name string
		sk   Skill
		want bool
	}{
		{"well-formed", Skill{Description: "d", Triggers: []string{"t"}, Body: strings.Repeat("x", 32)}, true},
		{"triggers-only", Skill{Triggers: []string{"t"}, Body: strings.Repeat("x", 32)}, true},
		{"description-only", Skill{Description: "does a thing", Body: strings.Repeat("x", 32)}, true},
		{"empty-body", Skill{Description: "d", Triggers: []string{"t"}, Body: ""}, false},
		{"short-body", Skill{Description: "d", Triggers: []string{"t"}, Body: "too short"}, false},
		{"no-retrieval-surface", Skill{Body: strings.Repeat("x", 32)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := ShadowTest(c.sk)
			if got != c.want {
				t.Errorf("ShadowTest = %v (reason %q), want %v", got, reason, c.want)
			}
			if !got && reason == "" {
				t.Error("a failing shadow-test must give a reason")
			}
		})
	}
}

// TestAutoShadow_StagesWellFormedDraft: with auto-shadow on, a freshly-created
// well-formed draft is advanced to shadow on creation, journaling skill.promoted
// with the auto reason; Create returns the post-staging status.
func TestAutoShadow_StagesWellFormedDraft(t *testing.T) {
	f, j := newTestForge(t)
	f.SetAutoShadow(true)

	sk, created, err := f.Create("run-7", wellFormedSpec("diagnose-ci"))
	if err != nil || !created {
		t.Fatalf("Create: created=%v err=%v", created, err)
	}
	if sk.Status != StatusShadow {
		t.Fatalf("auto-shadow should stage the draft to shadow, got %s", sk.Status)
	}
	if got := statusOf(t, f, sk.ID); got != StatusShadow {
		t.Errorf("stored status = %s, want shadow", got)
	}
	// The promotion event carries the gate reason and the creating run's corr.
	corr, from, to, reason := lastPromoteEvent(j)
	if from != "draft" || to != "shadow" {
		t.Errorf("promote event from=%s to=%s, want draft→shadow", from, to)
	}
	if !strings.Contains(reason, "auto-shadow") {
		t.Errorf("promote reason = %q, want an auto-shadow reason", reason)
	}
	if corr != "run-7" {
		t.Errorf("promote correlation = %q, want run-7", corr)
	}
}

// TestAutoShadow_RejectsDraftFailingShadowTest: a draft that can't be retrieved
// (no description/triggers) fails the shadow-test and stays a draft even with
// auto-shadow on.
func TestAutoShadow_RejectsDraftFailingShadowTest(t *testing.T) {
	f, _ := newTestForge(t)
	f.SetAutoShadow(true)

	sk, _, err := f.Create("c", CreateSpec{Name: "orphan", Body: strings.Repeat("x", 40)}) // no desc/triggers
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sk.Status != StatusDraft {
		t.Errorf("a draft failing the shadow-test must stay draft, got %s", sk.Status)
	}
}

// TestAutoShadow_DisabledLeavesDraft: off by default — Create leaves a well-formed
// draft as a draft.
func TestAutoShadow_DisabledLeavesDraft(t *testing.T) {
	f, _ := newTestForge(t)
	sk, _, err := f.Create("c", wellFormedSpec("manual"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sk.Status != StatusDraft {
		t.Errorf("auto-shadow off → status should be draft, got %s", sk.Status)
	}
}

// lastPromoteEvent returns the fields of the last skill.promoted event.
func lastPromoteEvent(j interface {
	Range(func(*event.Event) error) error
}) (corr, from, to, reason string) {
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillPromoted {
			var p struct {
				From, To, Reason string
			}
			_ = json.Unmarshal(e.Payload, &p)
			from, to, reason = p.From, p.To, p.Reason
			corr = e.CorrelationID
		}
		return nil
	})
	return corr, from, to, reason
}
