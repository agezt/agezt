// SPDX-License-Identifier: MIT

package cadenceapi

import (
	"encoding/json"
	"testing"
)

// TestEntryWireShape pins the schedule entry's wire: the keys the control
// plane serves, the Web UI reads and the store persists are read BY NAME, so a
// renamed key is a silent break for every consumer outside this package — and
// no test in the package would notice, because the package writes and reads
// the same struct.
func TestEntryWireShape(t *testing.T) {
	entry := Entry{
		ID:          "s-1",
		Intent:      "morning brief",
		Mode:        ModeDaily,
		IntervalSec: 60,
		AtMinutes:   9 * 60,
		EndMinutes:  17 * 60,
		Days:        AllDays,
		TZ:          "Europe/Istanbul",
		Model:       "deepseek-chat",
		Agent:       "scribe",
		Target:      TargetWorkflow,
		Workflow:    "flow-1",
		SystemTask:  SystemTaskCatalogSync,
		Tool:        "fetch",
		Payload:     json.RawMessage(`{"k":"v"}`),
		Source:      SourceOperator,
		Enabled:     true,
		CreatedUnix: 11,
		LastRunUnix: 22,
		NextRunUnix: 33,
		Fires:       44,
		Assure:      2,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{
		"id", "intent", "mode", "interval_sec", "at_minutes", "end_minutes",
		"days", "tz", "model", "agent", "target", "workflow", "system_task",
		"tool", "payload", "source", "enabled", "created_unix", "last_run_unix",
		"next_run_unix", "fires", "assure",
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("the schedule entry lost %q: %s", key, raw)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the schedule entry carries %d keys, want %d: %s", len(got), len(want), raw)
	}
	if got["id"] != "s-1" || got["intent"] != "morning brief" || got["enabled"] != true {
		t.Errorf("the schedule entry's own values drifted: %s", raw)
	}
	if got["next_run_unix"] != float64(33) || got["fires"] != float64(44) {
		t.Errorf("the schedule entry's numeric values drifted: %s", raw)
	}
}

// TestEntryWireOmitsTheEmptyFields: an unset field stays out of the wire, so
// a persisted entry is not full of empty keys.
func TestEntryWireOmitsTheEmptyFields(t *testing.T) {
	raw, err := json.Marshal(Entry{ID: "s-1", Intent: "x", Source: SourceOperator})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"mode", "interval_sec", "at_minutes", "end_minutes", "days", "tz", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "last_run_unix", "fires", "assure"} {
		if _, ok := got[key]; ok {
			t.Errorf("the schedule entry gained %q: %s", key, raw)
		}
	}
	// The keys that are always present.
	for _, key := range []string{"id", "intent", "source", "enabled", "created_unix", "next_run_unix"} {
		if _, ok := got[key]; !ok {
			t.Errorf("the schedule entry lost %q: %s", key, raw)
		}
	}
}
