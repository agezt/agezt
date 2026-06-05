// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestParseShadowVerdict(t *testing.T) {
	cases := map[string]bool{
		"YES":                        true,
		"yes":                        true,
		"Yes, it would have helped":  true,
		"true":                       true,
		"NO":                         false,
		"no, the run already did it": false,
		"maybe":                      false,
		"":                           false,
		"\n\nYES\n":                  true,
		"I think no":                 false, // first word is "i", not affirmative
	}
	for in, want := range cases {
		if got := parseShadowVerdict(in); got != want {
			t.Errorf("parseShadowVerdict(%q) = %v, want %v", in, got, want)
		}
	}
}

// shadowSkill creates a draft and promotes it to shadow (not active).
func shadowSkill(t *testing.T, f *Forge, name, desc string) string {
	t.Helper()
	sk, _, err := f.Create("c", CreateSpec{Name: name, Description: desc, Triggers: []string{"ci"}, Body: "instructions for " + name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.Promote("c", sk.ID); err != nil { // draft → shadow
		t.Fatalf("promote→shadow: %v", err)
	}
	return sk.ID
}

func TestRecordShadowOutcome_BumpsCountersAndJournals(t *testing.T) {
	f, j := newTestForge(t)
	id := shadowSkill(t, f, "diagnose", "diagnose a red ci build")

	f.RecordShadowOutcome("run-1", id, true)
	f.RecordShadowOutcome("run-2", id, false)

	sk, _, _ := f.Get(id)
	if sk.Metrics.ShadowEvals != 2 || sk.Metrics.ShadowWins != 1 {
		t.Errorf("metrics evals=%d wins=%d, want 2/1", sk.Metrics.ShadowEvals, sk.Metrics.ShadowWins)
	}
	if n := countKind(t, j, event.KindSkillShadowEval); n != 2 {
		t.Errorf("skill.shadow_evaluated events = %d, want 2", n)
	}
}

// TestRecordShadowOutcome_OnlyShadowSkills: an active (or draft) skill is never
// credited with a shadow outcome — the gate reads shadow evidence only.
func TestRecordShadowOutcome_OnlyShadowSkills(t *testing.T) {
	f, _ := newTestForge(t)
	id := activeSkill(t, f, "live") // promoted all the way to active
	f.RecordShadowOutcome("run", id, true)
	sk, _, _ := f.Get(id)
	if sk.Metrics.ShadowEvals != 0 {
		t.Errorf("active skill got %d shadow evals, want 0 (shadow-only)", sk.Metrics.ShadowEvals)
	}
}

// TestShadowEvaluate_JudgesRelevantShadowSkill: a shadow skill relevant to the
// intent is judged by the provider; a YES verdict records a win + event.
func TestShadowEvaluate_JudgesRelevantShadowSkill(t *testing.T) {
	f, j := newTestForge(t)
	id := shadowSkill(t, f, "diagnose-ci", "diagnose a failing ci build")
	// An irrelevant shadow skill that shouldn't match the intent tokens.
	_ = shadowSkill(t, f, "watering", "water the office plants")

	prov := mock.New(mock.FinalText("YES"))
	if err := f.ShadowEvaluate(context.Background(), "run-9", prov, "m", "the ci build is failing", "fixed the build", 5); err != nil {
		t.Fatalf("ShadowEvaluate: %v", err)
	}
	sk, _, _ := f.Get(id)
	if sk.Metrics.ShadowEvals != 1 || sk.Metrics.ShadowWins != 1 {
		t.Errorf("relevant shadow skill metrics evals=%d wins=%d, want 1/1", sk.Metrics.ShadowEvals, sk.Metrics.ShadowWins)
	}
	// The shadow_evaluated event carries the verdict + the run correlation.
	corr, helped := lastShadowEval(j)
	if corr != "run-9" || !helped {
		t.Errorf("shadow_evaluated event corr=%q helped=%v, want run-9/true", corr, helped)
	}
}

// TestShadowEvaluate_NoProviderErrors: a nil provider is a clear error (the
// caller should not invoke it without one).
func TestShadowEvaluate_NoProviderErrors(t *testing.T) {
	f, _ := newTestForge(t)
	if err := f.ShadowEvaluate(context.Background(), "c", nil, "m", "x", "y", 1); err == nil {
		t.Error("ShadowEvaluate with a nil provider should error")
	}
}

func lastShadowEval(j interface {
	Range(func(*event.Event) error) error
}) (corr string, helped bool) {
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillShadowEval {
			var p struct {
				Helped bool `json:"helped"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			helped = p.Helped
			corr = e.CorrelationID
		}
		return nil
	})
	return corr, helped
}
