// SPDX-License-Identifier: MIT

package controlplane

// Agent repair summary helpers (M846): the journal-derived per-agent
// repair summaries consumed by the roster list view. Carved out of
// roster.go during the Day 24 god file split #9.

import (
	"encoding/json"
	"os"
	"strings"
	"time"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
)

func agentAutoRepairCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"))
	if raw == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func (s *Server) agentRepairSummaries() map[string]agentRepairSummary {
	cooldown := agentAutoRepairCooldown()
	latestBySlug := map[string]agentRepairRow{}
	latestBySlugFingerprint := map[string]map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := plString(pl, "agent")
		if strings.TrimSpace(slug) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			Issues:                         plStrings(pl, "issues"),
			Applied:                        plStrings(pl, "applied"),
			Answer:                         plString(pl, "answer"),
			Error:                          plString(pl, "error"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			MailboxMessage:                 plString(pl, "mailbox_message_id"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := latestBySlug[slug]; !ok || row.Seq > cur.Seq {
			latestBySlug[slug] = row
		}
		if row.Fingerprint != "" {
			if latestBySlugFingerprint[slug] == nil {
				latestBySlugFingerprint[slug] = map[string]agentRepairRow{}
			}
			if cur, ok := latestBySlugFingerprint[slug][row.Fingerprint]; !ok || row.Seq > cur.Seq {
				latestBySlugFingerprint[slug][row.Fingerprint] = row
			}
		}
		return nil
	})
	out := map[string]agentRepairSummary{}
	for slug, latest := range latestBySlug {
		sum := agentRepairSummary{Latest: latest, HasLatest: true}
		for _, row := range latestBySlugFingerprint[slug] {
			if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
				sum.InflightCount++
			}
		}
		out[slug] = sum
	}
	return out
}

func repairPhaseLabel(mode, phase string) string {
	mode = strings.TrimSpace(mode)
	switch strings.TrimSpace(phase) {
	case "routing_forced_failed_detected":
		return "forced chain failed"
	case "routing_force_exhausted_detected":
		return "forced chain exhausted"
	case "routing_unstable_detected":
		return "unstable routing"
	case "attempts_exhausted":
		return "repair exhausted"
	case "queued":
		if mode == "routing_unstable" {
			return "unstable routing"
		}
		if mode == "degraded" {
			return "doctor queued"
		}
		if mode == "routing" {
			return "routing queued"
		}
		return "repair queued"
	case "routing_rollback_queued":
		return "rollback queued"
	case "completed":
		if mode == "degraded" {
			return "doctor repaired"
		}
		if mode == "routing" {
			return "routing stabilized"
		}
		return "repaired"
	case "routing_rollback_completed":
		return "rolled back"
	case "failed":
		if mode == "degraded" {
			return "doctor failed"
		}
		if mode == "routing" {
			return "routing failed"
		}
		return "repair failed"
	case "routing_rollback_failed":
		return "rollback failed"
	case "escalation_answered":
		return "manager answered"
	case "resolution_applied":
		return "manager applied"
	case "escalation_woke":
		return "manager woke"
	case "escalation_skipped":
		return "wake skipped"
	case "escalation_failed":
		return "wake failed"
	case "resolution_failed":
		return "resolution failed"
	case "delegation_queued":
		return "delegation queued"
	case "delegation_woke":
		return "delegation woke"
	case "delegation_failed":
		return "delegation failed"
	default:
		if strings.TrimSpace(phase) == "" {
			return "idle"
		}
		return phase
	}
}

func repairRowsView(rows []agentRepairRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, repairRowView(row))
	}
	return out
}

func repairRowView(row agentRepairRow) map[string]any {
	return map[string]any{
		"seq":                               row.Seq,
		"ts_unix_ms":                        row.TSUnixMS,
		"correlation_id":                    row.CorrelationID,
		"mode":                              row.Mode,
		"phase":                             row.Phase,
		"reason":                            row.Reason,
		"fingerprint":                       row.Fingerprint,
		"self_repair_attempt":               row.SelfRepairAttempt,
		"self_repair_max_attempts":          row.SelfRepairMaxAttempts,
		"issues":                            row.Issues,
		"applied":                           row.Applied,
		"answer":                            row.Answer,
		"error":                             row.Error,
		"target_agent":                      row.TargetAgent,
		"target_correlation":                row.TargetCorr,
		"mailbox_message_id":                row.MailboxMessage,
		"resolution":                        row.Resolution,
		"resolution_summary":                row.ResolutionSummary,
		"delegate_to":                       row.DelegateTo,
		"delegated_by":                      row.DelegatedBy,
		"root_agent":                        row.RootAgent,
		"chain_depth":                       row.ChainDepth,
		"incident_id":                       row.IncidentID,
		"root_incident_id":                  row.RootIncidentID,
		"parent_incident_id":                row.ParentIncidentID,
		"next_eligible_ms":                  row.NextEligibleMS,
		"routing_task_type":                 row.RoutingTaskType,
		"routing_task_model_chain":          row.RoutingTaskModelChain,
		"previous_routing_task_model_chain": row.PreviousRoutingTaskModelChain,
		"routing_force_generation":          row.RoutingForceGeneration,
		"previous_routing_force_generation": row.PreviousRoutingForceGeneration,
	}
}

