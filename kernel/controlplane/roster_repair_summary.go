// SPDX-License-Identifier: MIT

package controlplane

// Agent repair summary helpers (M846): the journal-derived per-agent
// repair summaries consumed by the roster list view. Carved out of
// roster.go during the Day 24 god file split #9.

import (
	"encoding/json"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"os"
	"strings"
	"time"
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
