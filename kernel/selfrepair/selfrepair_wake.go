// SPDX-License-Identifier: MIT

package selfrepair

import (
	"context"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)


func (c *autoRepairCoordinator) autoWakeManager(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate, msg *board.Message, mailboxErr error) {
	res, err := autoRepairWakeAgent(ctx, k, cand, msg)
	if err != nil {
		publishAutoRepair(b, res.Correlation, map[string]any{
			"phase":                    "escalation_failed",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"reason":                   err.Error(),
			"mailbox_error":            autoRepairErrString(mailboxErr),
			"mailbox_message_id":       autoRepairMessageID(msg),
		})
		return
	}
	if res.Skipped != "" {
		publishAutoRepair(b, "", map[string]any{
			"phase":                    "escalation_skipped",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"reason":                   res.Skipped,
			"mailbox_error":            autoRepairErrString(mailboxErr),
			"mailbox_message_id":       autoRepairMessageID(msg),
		})
		return
	}
	if res.Target == "" {
		return
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":                    "escalation_woke",
		"agent":                    cand.Slug,
		"mode":                     cand.Mode,
		"root_agent":               autoRepairRootAgent(cand),
		"chain_depth":              cand.ChainDepth,
		"incident_id":              autoRepairIncidentIDValue(cand),
		"root_incident_id":         autoRepairRootChainID(cand),
		"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
		"target_agent":             res.Target,
		"target_correlation":       res.Correlation,
		"fingerprint":              cand.Fingerprint,
		"self_repair_attempt":      cand.SelfRepairAttempt,
		"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
		"mailbox_error":            autoRepairErrString(mailboxErr),
		"mailbox_message_id":       autoRepairMessageID(msg),
		"wake_source":              "doctor",
		"autonomy_runbook":         res.Runbook,
	})
	if reply, err := c.autoReplyEscalation(mailbox, postNotify, res, msg); err == nil && reply != nil {
		payload := map[string]any{
			"phase":                    "escalation_answered",
			"agent":                    cand.Slug,
			"mode":                     cand.Mode,
			"root_agent":               autoRepairRootAgent(cand),
			"chain_depth":              cand.ChainDepth,
			"incident_id":              autoRepairIncidentIDValue(cand),
			"root_incident_id":         autoRepairRootChainID(cand),
			"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
			"target_agent":             res.Target,
			"target_correlation":       res.Correlation,
			"answer":                   autoRepairClip(res.Answer, 800),
			"fingerprint":              cand.Fingerprint,
			"self_repair_attempt":      cand.SelfRepairAttempt,
			"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
			"mailbox_message_id":       autoRepairMessageID(msg),
			"reply_message_id":         strings.TrimSpace(reply.ID),
		}
		if res.Resolution != nil {
			payload["resolution"] = res.Resolution.Resolution
			payload["resolution_summary"] = res.Resolution.Summary
			if res.Resolution.DelegateTo != "" {
				payload["delegate_to"] = res.Resolution.DelegateTo
			}
			if res.Resolution.TaskType != "" {
				payload["routing_task_type"] = res.Resolution.TaskType
			}
			if len(res.Resolution.TaskModelChain) > 0 {
				payload["routing_task_model_chain"] = res.Resolution.TaskModelChain
			}
		}
		publishAutoRepair(b, res.Correlation, payload)
		outcome, err := c.applyAutoRepairResolution(ctx, k, src, mailbox, postNotify, cand, res)
		if err != nil {
			fail := map[string]any{
				"phase":                    "resolution_failed",
				"agent":                    cand.Slug,
				"mode":                     cand.Mode,
				"root_agent":               autoRepairRootAgent(cand),
				"chain_depth":              cand.ChainDepth,
				"incident_id":              autoRepairIncidentIDValue(cand),
				"root_incident_id":         autoRepairRootChainID(cand),
				"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
				"target_agent":             res.Target,
				"target_correlation":       res.Correlation,
				"fingerprint":              cand.Fingerprint,
				"self_repair_attempt":      cand.SelfRepairAttempt,
				"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
				"reason":                   err.Error(),
				"mailbox_message_id":       autoRepairMessageID(msg),
			}
			if res.Resolution != nil {
				fail["resolution"] = res.Resolution.Resolution
				fail["resolution_summary"] = res.Resolution.Summary
				if res.Resolution.DelegateTo != "" {
					fail["delegate_to"] = res.Resolution.DelegateTo
				}
				if res.Resolution.TaskType != "" {
					fail["routing_task_type"] = res.Resolution.TaskType
				}
				if len(res.Resolution.TaskModelChain) > 0 {
					fail["routing_task_model_chain"] = res.Resolution.TaskModelChain
				}
			}
			publishAutoRepair(b, res.Correlation, fail)
		} else if outcome != nil {
			applied := map[string]any{
				"phase":                    strutil.FirstNonEmpty(outcome.Phase, "resolution_applied"),
				"agent":                    cand.Slug,
				"mode":                     cand.Mode,
				"root_agent":               autoRepairRootAgent(cand),
				"chain_depth":              cand.ChainDepth,
				"incident_id":              autoRepairIncidentIDValue(cand),
				"root_incident_id":         autoRepairRootChainID(cand),
				"parent_incident_id":       strings.TrimSpace(cand.ParentHopID),
				"target_agent":             res.Target,
				"target_correlation":       res.Correlation,
				"fingerprint":              cand.Fingerprint,
				"self_repair_attempt":      cand.SelfRepairAttempt,
				"self_repair_max_attempts": cand.SelfRepairMaxAttempts,
				"resolution":               res.Resolution.Resolution,
				"resolution_summary":       res.Resolution.Summary,
				"mailbox_message_id":       autoRepairMessageID(msg),
			}
			if res.Resolution.DelegateTo != "" {
				applied["delegate_to"] = res.Resolution.DelegateTo
			}
			if outcome.RoutingTaskType != "" {
				applied["routing_task_type"] = outcome.RoutingTaskType
			}
			if len(outcome.RoutingTaskModelChain) > 0 {
				applied["routing_task_model_chain"] = outcome.RoutingTaskModelChain
			}
			if len(outcome.PreviousRoutingTaskModelChain) > 0 {
				applied["previous_routing_task_model_chain"] = outcome.PreviousRoutingTaskModelChain
			}
			if outcome.RoutingForceGeneration > 0 {
				applied["routing_force_generation"] = outcome.RoutingForceGeneration
			}
			if outcome.PreviousRoutingForceGeneration > 0 {
				applied["previous_routing_force_generation"] = outcome.PreviousRoutingForceGeneration
			}
			publishAutoRepair(b, res.Correlation, applied)
		}
	}
}

