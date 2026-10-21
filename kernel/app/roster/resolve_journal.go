// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

// ResolveJournal is the journal the operator resolution reads its incident
// state from.
type ResolveJournal func(func(*event.Event) error) error

// LatestOperatorForceGeneration reads the journal for the latest applied
// force-chain generation of an agent's task type. It is the generation the
// operator's next force-chain resolution will supersede.
func LatestOperatorForceGeneration(journal ResolveJournal, slug, taskType string) int {
	if journal == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" {
		return 0
	}
	best := 0
	_ = journal(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(plString(pl, "agent")) != slug || strings.TrimSpace(plString(pl, "resolution")) != "force_chain" {
			return nil
		}
		phase := strings.TrimSpace(plString(pl, "phase"))
		if phase != "resolution_applied" && phase != "completed" {
			return nil
		}
		if strings.TrimSpace(plString(pl, "routing_task_type")) != taskType {
			return nil
		}
		if gen := intNumber(pl["routing_force_generation"]); gen > best {
			best = gen
		}
		return nil
	})
	return best
}

// LatestExhaustedRoutingChain reads the journal for the latest chain the
// doctor found exhausted for an agent, a task type and an incident lineage.
// The operator's force-chain resolution must not repeat it.
func LatestExhaustedRoutingChain(journal ResolveJournal, slug string, lineage IncidentLineage, taskType string) []string {
	if journal == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" || !lineage.hasAny() {
		return nil
	}
	var bestChain []string
	var bestSeq int64
	_ = journal(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != "doctor.auto_repair" {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "agent")), slug) {
			return nil
		}
		if strings.TrimSpace(plString(pl, "phase")) != "routing_force_exhausted_detected" {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "routing_task_type")), taskType) {
			return nil
		}
		if !incidentLineageMatchesPayload(lineage, pl) || e.Seq <= bestSeq {
			return nil
		}
		bestSeq = e.Seq
		bestChain = plStrings(pl, "routing_task_model_chain")
		return nil
	})
	return append([]string(nil), bestChain...)
}

// incidentLineageMatchesPayload reports whether the payload belongs to the
// incident the operator is resolving: any of the lineage's ids is among the
// payload's incident ids.
func incidentLineageMatchesPayload(lineage IncidentLineage, pl map[string]any) bool {
	if !lineage.hasAny() {
		return false
	}
	payloadIDs := []string{
		strings.TrimSpace(plString(pl, "incident_id")),
		strings.TrimSpace(plString(pl, "root_incident_id")),
		strings.TrimSpace(plString(pl, "parent_incident_id")),
	}
	return incidentIDInSlice(lineage.IncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.RootIncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.ParentIncidentID, payloadIDs)
}

// incidentIDInSlice reports whether the id is among the items, ignoring case
// and surrounding blanks.
func incidentIDInSlice(id string, items []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(id, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

// hasAny reports whether the lineage names an incident at all.
func (l IncidentLineage) hasAny() bool {
	return strings.TrimSpace(l.IncidentID) != "" ||
		strings.TrimSpace(l.RootIncidentID) != "" ||
		strings.TrimSpace(l.ParentIncidentID) != ""
}
