// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChangelog_IncludesAnomalyAutoHalt: the anomaly auto-halt circuit breaker
// (SPEC-06 §5) is a material system change — the daemon halted ITSELF — and must
// appear in the SPEC-08 §4.2 system timeline alongside the halt it triggers, with
// its reason, so an operator can see WHY the system stopped (not just that it
// did). Before, only the `halt` symptom was surfaced, never the anomaly cause.
func TestChangelog_IncludesAnomalyAutoHalt(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	reason := "tool-call rate anomaly: 200 tool calls within 10s exceeds ceiling 120"
	ev, err := k.Bus().Publish(event.Spec{
		Subject: "system.anomaly", Kind: event.KindAnomalyDetected, Actor: "anomaly",
		Payload: map[string]any{"signal": "tool_call_rate", "count": 200, "ceiling": 120, "reason": reason},
	})
	if err != nil {
		t.Fatalf("publish anomaly: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdChangelog, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	entries, _ := res["entries"].([]any)
	var found map[string]any
	for _, raw := range entries {
		m, _ := raw.(map[string]any)
		if kind, _ := m["kind"].(string); kind == "system.anomaly" {
			found = m
		}
	}
	if found == nil {
		t.Fatalf("system.anomaly not in the changelog timeline: %v", entries)
	}
	if got, _ := found["label"].(string); got != "anomaly auto-halt" {
		t.Errorf("label = %q want 'anomaly auto-halt'", got)
	}
	if got, _ := found["detail"].(string); got != reason {
		t.Errorf("detail = %q want the anomaly reason %q", got, reason)
	}
	if got, _ := found["event_id"].(string); got != ev.ID {
		t.Errorf("event_id = %q want %q (so `agt why` can explain it)", got, ev.ID)
	}
}

// TestChangelog_FiltersToMaterialChanges — the system changelog folds only
// material-change kinds (skill lifecycle, policy change, …) and ignores routine
// events (task.received), newest-first, each labeled and carrying its event id
// (M133, SPEC-08 §4.2).
func TestChangelog_FiltersToMaterialChanges(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	pub := func(kind event.Kind, payload map[string]any) string {
		t.Helper()
		e, err := k.Bus().Publish(event.Spec{
			Subject: "x", Kind: kind, Actor: "test", Payload: payload,
		})
		if err != nil {
			t.Fatalf("Publish %s: %v", kind, err)
		}
		return e.ID
	}

	// Routine noise (must NOT appear) + two material changes (must appear).
	pub(event.KindTaskReceived, map[string]any{"intent": "noise"})
	pub(event.KindPolicyChanged, map[string]any{"change": "shell→ASK"})
	skillID := pub(event.KindSkillPromoted, map[string]any{"skill_id": "summarize-v2"})

	res, err := c.Call(context.Background(), controlplane.CmdChangelog, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	entries, _ := res["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %d want 2 (noise filtered out): %v", len(entries), entries)
	}

	// Newest-first: skill.promoted (last published) is entry 0.
	top, _ := entries[0].(map[string]any)
	if got, _ := top["kind"].(string); got != "skill.promoted" {
		t.Errorf("entry[0].kind = %q want skill.promoted (newest-first)", got)
	}
	if got, _ := top["label"].(string); got != "skill promoted" {
		t.Errorf("entry[0].label = %q want 'skill promoted'", got)
	}
	if got, _ := top["detail"].(string); got != "summarize-v2" {
		t.Errorf("entry[0].detail = %q want summarize-v2", got)
	}
	if got, _ := top["event_id"].(string); got != skillID {
		t.Errorf("entry[0].event_id = %q want %q (so `agt why` works)", got, skillID)
	}

	// The policy change is the second entry; task.received appears nowhere.
	for _, raw := range entries {
		m, _ := raw.(map[string]any)
		if kind, _ := m["kind"].(string); kind == "task.received" {
			t.Errorf("task.received must not appear in the system changelog")
		}
	}
}
