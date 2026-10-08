// SPDX-License-Identifier: MIT

package controlplane

// Operator incident-resolution journal lookups behind the resolve operation's
// ports: force generation, routing-chain exhaustion and lineage matching
// (M833/M846). Carved out during the Day 145 god file split.

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

func latestOperatorForceGeneration(k *runtime.Kernel, slug, taskType string) int {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" {
		return 0
	}
	best := 0
	_ = k.Journal().Range(func(e *event.Event) error {
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

func latestExhaustedRoutingChain(k *runtime.Kernel, slug string, lineage operatorWakeLineage, taskType string) []string {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" || !lineage.hasAny() {
		return nil
	}
	var bestChain []string
	var bestSeq int64
	_ = k.Journal().Range(func(e *event.Event) error {
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

func incidentLineageMatchesPayload(lineage operatorWakeLineage, pl map[string]any) bool {
	if !lineage.hasAny() {
		return false
	}
	payloadIDs := []string{
		strings.TrimSpace(plString(pl, "incident_id")),
		strings.TrimSpace(plString(pl, "root_incident_id")),
		strings.TrimSpace(plString(pl, "parent_incident_id")),
	}
	return incidentIDInSlice(lineage.incidentID, payloadIDs) ||
		incidentIDInSlice(lineage.rootIncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.parentIncidentID, payloadIDs)
}

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

func (l operatorWakeLineage) hasAny() bool {
	return strings.TrimSpace(l.incidentID) != "" ||
		strings.TrimSpace(l.rootIncidentID) != "" ||
		strings.TrimSpace(l.parentIncidentID) != ""
}
