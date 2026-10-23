// SPDX-License-Identifier: MIT

package standingapi

import (
	"encoding/json"
	"testing"
)

// TestOrderWireShape pins the standing order's wire: the keys the control
// plane serves, the Web UI reads and the store persists are read BY NAME, so a
// renamed key is a silent break for every consumer outside this package — and
// no test in the package would notice, because the package writes and reads
// the same struct.
func TestOrderWireShape(t *testing.T) {
	order := Order{
		ID:            "ord-1",
		Name:          "Nightly digest",
		Enabled:       true,
		Triggers:      []Trigger{{Type: TriggerEvent, Subject: "task.failed"}},
		Observers:     []string{"boss"},
		ScopeEntities: []string{"project:alpha"},
		Initiative:    Initiative{Mode: InitiativeAsk, MaxTrust: "L1"},
		BriefingMin:   "digest",
		BriefingChan:  "ops",
		Plan:          "summarize the failures",
		Agent:         "scribe",
		Assure:        2,
		CooldownSec:   30,
		CreatedMS:     11,
		UpdatedMS:     22,
	}
	raw, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{
		"id", "name", "enabled", "triggers", "observers", "scope_entities",
		"initiative", "briefing_disposition_min", "briefing_channel", "plan",
		"agent", "assure", "cooldown_sec", "created_ms", "updated_ms",
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("the standing order lost %q: %s", key, raw)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the standing order carries %d keys, want %d: %s", len(got), len(want), raw)
	}
	if got["id"] != "ord-1" || got["name"] != "Nightly digest" || got["enabled"] != true {
		t.Errorf("the standing order's own values drifted: %s", raw)
	}
	if got["cooldown_sec"] != float64(30) || got["assure"] != float64(2) {
		t.Errorf("the standing order's numeric values drifted: %s", raw)
	}
}

// TestOrderWireOmitsTheEmptyFields: an unset field stays out of the wire, so
// a persisted order is not full of empty keys.
func TestOrderWireOmitsTheEmptyFields(t *testing.T) {
	raw, err := json.Marshal(Order{ID: "ord-1", Name: "n"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"observers", "scope_entities", "briefing_disposition_min", "briefing_channel", "plan", "agent", "assure", "cooldown_sec"} {
		if _, ok := got[key]; ok {
			t.Errorf("the standing order gained %q: %s", key, raw)
		}
	}
}

// TestTriggerAndInitiativeWireShape: the two nested rows have their own keys.
func TestTriggerAndInitiativeWireShape(t *testing.T) {
	raw, err := json.Marshal(Trigger{Type: TriggerCron, Schedule: "0 * * * *"})
	if err != nil {
		t.Fatalf("marshal the trigger: %v", err)
	}
	var trig map[string]any
	if err := json.Unmarshal(raw, &trig); err != nil {
		t.Fatalf("unmarshal the trigger: %v", err)
	}
	if trig["type"] != "cron" || trig["schedule"] != "0 * * * *" {
		t.Errorf("the trigger's wire drifted: %s", raw)
	}
	if _, ok := trig["subject"]; ok {
		t.Errorf("the trigger gained an empty subject: %s", raw)
	}

	raw, err = json.Marshal(Initiative{Mode: InitiativeActOrAsk})
	if err != nil {
		t.Fatalf("marshal the initiative: %v", err)
	}
	var init map[string]any
	if err := json.Unmarshal(raw, &init); err != nil {
		t.Fatalf("unmarshal the initiative: %v", err)
	}
	if init["mode"] != "act_or_ask" {
		t.Errorf("the initiative's wire drifted: %s", raw)
	}
	for _, key := range []string{"max_trust", "budget_per_run_mc"} {
		if _, ok := init[key]; ok {
			t.Errorf("the initiative gained an empty %q: %s", key, raw)
		}
	}
}
