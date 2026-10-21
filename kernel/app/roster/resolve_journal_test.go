// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// resolveEvent builds one journal event for the operator resolve reads.
func resolveEvent(seq int64, subject string, payload map[string]any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{
		Seq: seq, Kind: event.KindInfo, Subject: subject, TSUnixMS: seq * 1000, Payload: raw,
	}
}

func TestLatestOperatorForceGeneration(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "resolution_applied",
			"routing_task_type": "code", "routing_force_generation": 2,
		}),
		resolveEvent(2, "agent.resolve", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "completed",
			"routing_task_type": "code", "routing_force_generation": 5,
		}),
	}
	got := LatestOperatorForceGeneration(journalOf(events...), "ops", "code")
	if got != 5 {
		t.Errorf("force generation = %d, want the latest applied one", got)
	}
}

func TestLatestOperatorForceGeneration_IgnoresOtherAgentsAndTaskTypes(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "other", "resolution": "force_chain", "phase": "resolution_applied",
			"routing_task_type": "code", "routing_force_generation": 9,
		}),
		resolveEvent(2, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "resolution_applied",
			"routing_task_type": "other", "routing_force_generation": 8,
		}),
		resolveEvent(3, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "escalate", "phase": "resolution_applied",
			"routing_task_type": "code", "routing_force_generation": 7,
		}),
		resolveEvent(4, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "queued",
			"routing_task_type": "code", "routing_force_generation": 6,
		}),
		resolveEvent(5, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "routing_force_applied",
			"routing_task_type": "code", "routing_force_generation": 4,
		}),
	}
	if got := LatestOperatorForceGeneration(journalOf(events...), "ops", "code"); got != 0 {
		t.Errorf("force generation = %d, want none", got)
	}
}

func TestLatestOperatorForceGeneration_EmptyInputs(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "ops", "resolution": "force_chain", "phase": "resolution_applied",
			"routing_task_type": "code", "routing_force_generation": 3,
		}),
	}
	if got := LatestOperatorForceGeneration(nil, "ops", "code"); got != 0 {
		t.Errorf("no journal = %d, want none", got)
	}
	if got := LatestOperatorForceGeneration(journalOf(events...), "", "code"); got != 0 {
		t.Errorf("no agent = %d, want none", got)
	}
	if got := LatestOperatorForceGeneration(journalOf(events...), "ops", ""); got != 0 {
		t.Errorf("no task type = %d, want none", got)
	}
}

func TestLatestExhaustedRoutingChain(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "code", "incident_id": "i-1",
			"routing_task_model_chain": []any{"a", "b"},
		}),
		resolveEvent(2, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "code", "incident_id": "i-2",
			"routing_task_model_chain": []any{"c"},
		}),
	}
	chain := LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{IncidentID: "i-2"}, "code")
	if len(chain) != 1 || chain[0] != "c" {
		t.Errorf("chain = %v, want the latest one for the incident", chain)
	}
	// The lineage match is case-insensitive and matches any of the three ids.
	chain = LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{RootIncidentID: "I-1"}, "code")
	if len(chain) != 2 || chain[0] != "a" {
		t.Errorf("chain = %v, want the root-incident match", chain)
	}
	chain = LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{ParentIncidentID: "i-1"}, "code")
	if len(chain) != 2 || chain[0] != "a" {
		t.Errorf("chain = %v, want the parent-incident match", chain)
	}
}

func TestLatestExhaustedRoutingChain_IgnoresOtherIncidents(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "code", "incident_id": "i-1",
			"routing_task_model_chain": []any{"a"},
		}),
	}
	if chain := LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{IncidentID: "other"}, "code"); chain != nil {
		t.Errorf("chain = %v, want none for another incident", chain)
	}
	// A lineage with no incident at all matches nothing.
	if chain := LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{}, "code"); chain != nil {
		t.Errorf("chain = %v, want none without a lineage", chain)
	}
}

func TestLatestExhaustedRoutingChain_IgnoresOtherPhasesAgentsAndTaskTypes(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "other", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "code", "incident_id": "i",
			"routing_task_model_chain": []any{"a"},
		}),
		resolveEvent(2, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "completed",
			"routing_task_type": "code", "incident_id": "i",
			"routing_task_model_chain": []any{"b"},
		}),
		resolveEvent(3, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "other", "incident_id": "i",
			"routing_task_model_chain": []any{"c"},
		}),
	}
	if chain := LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{IncidentID: "i"}, "code"); chain != nil {
		t.Errorf("chain = %v, want none", chain)
	}
}

func TestLatestExhaustedRoutingChain_EmptyInputs(t *testing.T) {
	events := []*event.Event{
		resolveEvent(1, "doctor.auto_repair", map[string]any{
			"agent": "ops", "phase": "routing_force_exhausted_detected",
			"routing_task_type": "code", "incident_id": "i",
			"routing_task_model_chain": []any{"a"},
		}),
	}
	if chain := LatestExhaustedRoutingChain(nil, "ops", IncidentLineage{IncidentID: "i"}, "code"); chain != nil {
		t.Errorf("no journal = %v, want none", chain)
	}
	if chain := LatestExhaustedRoutingChain(journalOf(events...), "", IncidentLineage{IncidentID: "i"}, "code"); chain != nil {
		t.Errorf("no agent = %v, want none", chain)
	}
	if chain := LatestExhaustedRoutingChain(journalOf(events...), "ops", IncidentLineage{IncidentID: "i"}, ""); chain != nil {
		t.Errorf("no task type = %v, want none", chain)
	}
}

func TestIncidentLineageHasAny(t *testing.T) {
	if (IncidentLineage{}).hasAny() {
		t.Error("an empty lineage names no incident")
	}
	if !(IncidentLineage{IncidentID: "i"}).hasAny() {
		t.Error("an incident id is a lineage")
	}
	if !(IncidentLineage{RootIncidentID: "r"}).hasAny() {
		t.Error("a root incident id is a lineage")
	}
	if !(IncidentLineage{ParentIncidentID: "p"}).hasAny() {
		t.Error("a parent incident id is a lineage")
	}
	if (IncidentLineage{IncidentID: "   "}).hasAny() {
		t.Error("a blank incident id is not a lineage")
	}
}
