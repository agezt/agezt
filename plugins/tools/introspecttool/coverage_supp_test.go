// SPDX-License-Identifier: MIT

package introspecttool

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/standing"
)

func TestErrResult_Format_OverseerLike(t *testing.T) {
	r := errResult("not available yet")
	if !r.IsError {
		t.Error("errResult should mark IsError")
	}
	if !strings.HasPrefix(r.Output, "introspect: ") {
		t.Errorf("errResult output = %q, want introspect: prefix", r.Output)
	}
}

func TestStandingView_TriggerWithEmptySubjectAndSchedule(t *testing.T) {
	o := standingView(standing.Order{
		ID: "o1", Name: "trigger-test", Enabled: true,
		Triggers: []standing.Trigger{
			{Type: standing.TriggerEvent}, // no Subject
			{Type: standing.TriggerCron},   // no Schedule
		},
		Initiative: standing.Initiative{Mode: standing.InitiativeAsk},
		Plan:       "react",
	})
	trigs := o["triggers"].([]map[string]any)
	if len(trigs) != 2 {
		t.Fatalf("triggers = %d, want 2", len(trigs))
	}
	// Both should be present without extra fields.
	if trigs[0]["type"] != string(standing.TriggerEvent) {
		t.Error("trigger 0 type mismatch")
	}
	if trigs[1]["type"] != string(standing.TriggerCron) {
		t.Error("trigger 1 type mismatch")
	}
}

func TestNext_NonInt64ReturnsZero(t *testing.T) {
	got := next(map[string]any{"next_run_unix": float64(42)})
	if got != 0 {
		t.Errorf("next(float64) = %d, want 0", got)
	}
}

func TestScheduleView_AssureIntType(t *testing.T) {
	v := scheduleView(cadence.Entry{
		ID: "s-assure", Intent: "test", Mode: cadence.ModeOnce,
		Source: "operator", Enabled: true, NextRunUnix: 1000,
		Assure: 3,
	})
	// Assure is int, not int64 — verify the map carries the right type.
	if v["assure"] != 3 {
		t.Errorf("scheduleView assure = %v (type %T), want 3", v["assure"], v["assure"])
	}
}
