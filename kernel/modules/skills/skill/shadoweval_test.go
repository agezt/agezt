// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"encoding/json"
	"strings"
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

// TestRecordShadowOutcome_AutoPromotesAfterWins: a shadow skill that crosses the
// win count + rate threshold is promoted to active automatically, journaling
// skill.promoted with an auto-promote reason carrying the run correlation.
func TestRecordShadowOutcome_AutoPromotesAfterWins(t *testing.T) {
	f, j := newTestForge(t)
	id := shadowSkill(t, f, "diagnose", "diagnose a red ci build")

	// Two wins: below the min-win threshold (3) → still shadow.
	f.RecordShadowOutcome("run-1", id, true)
	f.RecordShadowOutcome("run-2", id, true)
	if got := statusOf(t, f, id); got != StatusShadow {
		t.Fatalf("after 2 wins status=%s, want still shadow", got)
	}
	// Third win crosses 3 wins @ 100% rate → auto-promoted to active.
	f.RecordShadowOutcome("run-3", id, true)
	if got := statusOf(t, f, id); got != StatusActive {
		t.Fatalf("after 3 wins status=%s, want active", got)
	}
	corr, from, to, reason := lastPromoteEvent(j)
	if from != "shadow" || to != "active" {
		t.Errorf("promote event %s→%s, want shadow→active", from, to)
	}
	if !strings.Contains(reason, "auto-promote") {
		t.Errorf("promote reason = %q, want an auto-promote reason", reason)
	}
	if corr != "run-3" {
		t.Errorf("promote correlation = %q, want run-3", corr)
	}
}

// TestRecordShadowOutcome_NoPromoteWhenMixedVerdicts: a shadow skill judged
// unhelpful as often as helpful is below the rate threshold and stays shadow.
func TestRecordShadowOutcome_NoPromoteWhenMixedVerdicts(t *testing.T) {
	f, _ := newTestForge(t)
	id := shadowSkill(t, f, "marginal", "a marginal ci helper")
	f.RecordShadowOutcome("r", id, true)
	f.RecordShadowOutcome("r", id, false)
	f.RecordShadowOutcome("r", id, true)
	f.RecordShadowOutcome("r", id, false) // 2/4 = 50%... wins=2 < min 3 anyway
	if got := statusOf(t, f, id); got != StatusShadow {
		t.Errorf("status=%s, want shadow (below win threshold)", got)
	}
}

// TestRecordShadowOutcome_AutoPromoteDisabled: SetAutoPromote(0,…) disables it.
func TestRecordShadowOutcome_AutoPromoteDisabled(t *testing.T) {
	f, _ := newTestForge(t)
	f.SetAutoPromote(0, 0)
	id := shadowSkill(t, f, "stuck", "a stuck shadow skill")
	for i := 0; i < 6; i++ {
		f.RecordShadowOutcome("r", id, true)
	}
	if got := statusOf(t, f, id); got != StatusShadow {
		t.Errorf("status=%s, want shadow (auto-promote disabled)", got)
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
