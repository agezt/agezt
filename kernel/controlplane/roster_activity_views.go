// SPDX-License-Identifier: MIT

package controlplane

// Agent repair view helpers: agentRepairContractView +
// agentRepairNextActionView + repairEscalationOwner +
// repairDecisionDetail. Carved out of roster_activity.go during
// the Day 198 god-file split so the main file can stay focused on
// the handleAgentActivity + handleAgentRepairStatus handlers.
// Public API unchanged.

import (
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

func agentRepairContractView(p roster.Profile, cooldown time.Duration) map[string]any {
	retryAttempts := 1
	retryBackoff := "none"
	retryOn := []string{"error", "timeout"}
	if p.RetryPolicy != nil {
		if p.RetryPolicy.MaxAttempts > 0 {
			retryAttempts = p.RetryPolicy.MaxAttempts
		}
		if strings.TrimSpace(p.RetryPolicy.Backoff) != "" {
			retryBackoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		}
		if len(p.RetryPolicy.RetryOn) > 0 {
			retryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		}
	}
	selfRepairEnabled := p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
	selfRepairMax := 0
	escalateTo := ""
	if p.SelfRepairPolicy != nil {
		selfRepairMax = p.SelfRepairPolicy.MaxAttempts
		escalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	doctor := ""
	failureThreshold := 0
	if p.HealthPolicy != nil {
		doctor = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
		failureThreshold = p.HealthPolicy.FailureThreshold
	}
	return map[string]any{
		"retry_attempts":       retryAttempts,
		"retry_backoff":        retryBackoff,
		"retry_on":             retryOn,
		"doctor_agent":         doctor,
		"failure_threshold":    failureThreshold,
		"self_repair_enabled":  selfRepairEnabled,
		"self_repair_attempts": selfRepairMax,
		"escalate_to":          escalateTo,
		"cooldown_sec":         int(cooldown / time.Second),
		"authority_boundary":   "agent identity owns retry, doctor, self-repair and escalation; schedules/workflows only wake this contract",
	}
}

func agentRepairNextActionView(p roster.Profile, rows, inflight []agentRepairRow, nowMS int64) map[string]any {
	action := "manual_repair"
	label := "manual repair"
	detail := "no autonomous repair is currently queued"
	tone := "muted"
	if p.Retired {
		return map[string]any{"action": "revive_required", "label": "revive required", "detail": "graveyard agent cannot repair until revived", "tone": "muted"}
	}
	if !p.Enabled {
		return map[string]any{"action": "resume_required", "label": "resume required", "detail": "paused agent cannot repair until resumed", "tone": "warn"}
	}
	if len(inflight) > 0 {
		row := inflight[0]
		return map[string]any{
			"action":         "wait_inflight",
			"label":          "repair in flight",
			"detail":         repairDecisionDetail(row, "doctor/self-repair run is already queued"),
			"tone":           "accent",
			"correlation_id": row.CorrelationID,
			"fingerprint":    row.Fingerprint,
			"phase":          row.Phase,
		}
	}
	var latest agentRepairRow
	if len(rows) > 0 {
		latest = rows[0]
		if latest.NextEligibleMS > nowMS {
			return map[string]any{
				"action":           "cooldown",
				"label":            "cooldown active",
				"detail":           repairDecisionDetail(latest, "wait before another autonomous repair attempt"),
				"tone":             "warn",
				"next_eligible_ms": latest.NextEligibleMS,
				"phase":            latest.Phase,
				"fingerprint":      latest.Fingerprint,
			}
		}
		switch strings.TrimSpace(latest.Phase) {
		case "attempts_exhausted", "resolution_failed", "routing_rollback_failed", "failed":
			target := firstNonEmpty(strings.TrimSpace(latest.DelegateTo), repairEscalationOwner(p))
			if target != "" {
				return map[string]any{
					"action":      "escalate_owner",
					"label":       "escalate owner",
					"detail":      repairDecisionDetail(latest, "self-repair failed; owner should take over"),
					"tone":        "bad",
					"delegate_to": target,
					"phase":       latest.Phase,
				}
			}
			return map[string]any{
				"action": "operator_resolution",
				"label":  "operator resolution",
				"detail": repairDecisionDetail(latest, "repair failed and no owner escalation target is configured"),
				"tone":   "bad",
				"phase":  latest.Phase,
			}
		}
	}
	if p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled {
		action = "run_self_repair"
		label = "self-repair eligible"
		detail = "next failure can trigger autonomous self-repair"
		tone = "good"
	} else if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		action = "doctor_monitor"
		label = "doctor monitoring"
		detail = "doctor can queue repair after health threshold"
		tone = "good"
	}
	if latest.Phase != "" {
		detail = repairDecisionDetail(latest, detail)
	}
	return map[string]any{"action": action, "label": label, "detail": detail, "tone": tone}
}

func repairEscalationOwner(p roster.Profile) string {
	if p.SelfRepairPolicy != nil && strings.TrimSpace(p.SelfRepairPolicy.EscalateTo) != "" {
		return strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	return firstNonEmpty(strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent))
}

func repairDecisionDetail(row agentRepairRow, fallback string) string {
	parts := []string{fallback}
	if row.Mode != "" {
		parts = append(parts, "mode "+row.Mode)
	}
	if row.Phase != "" {
		parts = append(parts, "phase "+row.Phase)
	}
	if row.Fingerprint != "" {
		parts = append(parts, "fingerprint "+row.Fingerprint)
	}
	if row.Reason != "" {
		parts = append(parts, row.Reason)
	} else if row.Error != "" {
		parts = append(parts, row.Error)
	}
	if row.SelfRepairAttempt > 0 && row.SelfRepairMaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt %d/%d", row.SelfRepairAttempt, row.SelfRepairMaxAttempts))
	}
	return strings.Join(parts, " · ")
}


