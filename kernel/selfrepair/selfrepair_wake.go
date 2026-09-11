// SPDX-License-Identifier: MIT

package selfrepair

// Self-repair wake subsystem (M846): the autoWakeManager loop that wakes
// agents on the doctor's behalf, plus the wake-intent/skip-reason helpers.
// Carved out of selfrepair.go during the Day 25 god file split #3 so the
// main file can focus on the coordinator's claim/dispatch/retry path.

import (
	"context"
	"fmt"
	"strings"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/roster"
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

func autoRepairWakeAgent(ctx context.Context, k *kernelruntime.Kernel, cand autoRepairCandidate, msg *board.Message) (autoRepairWakeResult, error) {
	if k == nil {
		return autoRepairWakeResult{}, fmt.Errorf("auto-repair wake requires kernel")
	}
	target := strings.TrimSpace(cand.EscalateTo)
	if target == "" {
		return autoRepairWakeResult{}, nil
	}
	p, ok := k.Roster().Get(target)
	if !ok {
		return autoRepairWakeResult{Target: target, Skipped: "unknown target agent " + target}, nil
	}
	if reason := autoRepairWakeSkipReason(p); reason != "" {
		return autoRepairWakeResult{Target: p.Slug, Skipped: reason}, nil
	}
	corr := k.NewCorrelation()
	rctx := kernelruntime.WithAgentProfile(ctx, p)
	if p.MaxCostMc > 0 {
		rctx = kernelruntime.WithMaxCost(rctx, p.MaxCostMc)
	}
	intent := autoRepairWakeIntent(cand, msg)
	var (
		err    error
		answer string
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = k.RunWithRetry(rctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = k.RunWith(rctx, corr, intent)
	}
	return autoRepairWakeResult{
		Target:      p.Slug,
		Correlation: corr,
		Answer:      answer,
		Resolution:  parseAutoRepairResolution(answer),
		Runbook:     roster.AutonomyRunbook(p),
	}, err
}

func autoRepairWakeSkipReason(p roster.Profile) string {
	if !p.Enabled {
		return "target agent " + p.Slug + " is paused"
	}
	if p.Retired {
		return "target agent " + p.Slug + " is retired"
	}
	if !p.AllowsDirectCall() {
		return "target agent " + p.Slug + " is a managed sub-agent"
	}
	return ""
}

func autoRepairWakeIntent(cand autoRepairCandidate, msg *board.Message) string {
	var b strings.Builder
	b.WriteString("Escalation wake-up.\n")
	b.WriteString("A doctor/self-repair attempt failed for agent ")
	b.WriteString(cand.Slug)
	b.WriteString(".\n")
	if mode := strings.TrimSpace(cand.Mode); mode != "" {
		b.WriteString("Failure mode: ")
		b.WriteString(mode)
		b.WriteString(".\n")
	}
	if reason := strings.TrimSpace(cand.Reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if fp := strings.TrimSpace(cand.Fingerprint); fp != "" {
		b.WriteString("Fingerprint: ")
		b.WriteString(autoRepairClip(fp, 320))
		b.WriteString("\n")
	}
	if msg != nil {
		b.WriteString("Mailbox help request")
		if msg.ID != "" {
			b.WriteString(" id=")
			b.WriteString(msg.ID)
		}
		b.WriteString(" was posted")
		if to := strings.TrimSpace(msg.To); to != "" {
			b.WriteString(" to ")
			b.WriteString(to)
		}
		b.WriteString(".\n")
	}
	if root := autoRepairRootAgent(cand); root != "" {
		b.WriteString("Escalation root agent: ")
		b.WriteString(root)
		b.WriteString(".\n")
	}
	if incidentID := autoRepairIncidentIDValue(cand); incidentID != "" {
		b.WriteString("Incident id: ")
		b.WriteString(incidentID)
		b.WriteString(".\n")
	}
	if cand.ChainDepth > 0 {
		b.WriteString("Escalation chain depth: ")
		b.WriteString(fmt.Sprintf("%d", cand.ChainDepth))
		b.WriteString(".\n")
	}
	switch strings.TrimSpace(cand.Mode) {
	case "routing_forced_exhausted":
		b.WriteString("Routing state: an owner-forced chain has already been retried across multiple forced generations and still remains under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Treat this as an exhausted routing policy. Prefer retire, pause, or delegate unless you have a concrete new force_chain backed by stronger evidence.\n")
	case "routing_forced_failed":
		b.WriteString("Routing state: an owner-forced chain already served its probation window and the same task routing is still under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Choose deliberately between pause, retire, delegate, or a new force_chain decision. Do not answer with handled unless you actually stabilized ownership and routing policy.\n")
	case "routing_unstable":
		b.WriteString("Routing state: a previous routing rewrite rolled back and the task chain still destabilized again.\n")
		b.WriteString("Treat this as an ownership decision, not a routine retry. Prefer pause, retire, delegate, or force_chain when you have a concrete stable route.\n")
	}
	b.WriteString("Take ownership. Inspect the agent's health, mailbox, logs, and repair state; then recover it, pause/retire it, delegate follow-up, or force a stable routing chain when the task routing is clearly wrong.\n")
	b.WriteString("End your final answer with EXACTLY ONE fenced json block so the escalation can be classified deterministically:\n")
	b.WriteString("```json\n")
	b.WriteString("{\"resolution\":\"handled|paused|retired|delegated|blocked|force_chain\",\"summary\":\"short operator-facing closure note\",\"delegate_to\":\"optional target agent slug when delegated\",\"task_type\":\"required when resolution=force_chain\",\"task_model_chain\":[\"required\",\"when\",\"resolution=force_chain\"]}\n")
	b.WriteString("```\n")
	b.WriteString("Use only one resolution value. Omit delegate_to unless resolution is delegated. Use task_type and task_model_chain only when resolution is force_chain.")
	return b.String()
}

func autoRepairMessageID(msg *board.Message) string {
	if msg == nil {
		return ""
	}
	return strings.TrimSpace(msg.ID)
}

func autoRepairErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *autoRepairCoordinator) autoReplyEscalation(mailbox Mailbox, postNotify func(board.Message, string), wake autoRepairWakeResult, msg *board.Message) (*board.Message, error) {
	if mailbox == nil || msg == nil || strings.TrimSpace(msg.ID) == "" || strings.TrimSpace(wake.Target) == "" {
		return nil, nil
	}
	orig, ok := mailbox.Get(msg.ID)
	if !ok {
		return nil, nil
	}
	reply, err := mailbox.Send(board.Message{
		Topic:   orig.Topic,
		From:    wake.Target,
		To:      orig.From,
		ReplyTo: orig.ID,
		Text:    autoRepairWakeReplyText(wake),
	}, c.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if postNotify != nil {
		postNotify(reply, wake.Correlation)
	}
	return &reply, nil
}

func autoRepairWakeReplyText(wake autoRepairWakeResult) string {
	if wake.Resolution != nil {
		var parts []string
		parts = append(parts, "Resolution: "+wake.Resolution.Resolution+".")
		if wake.Resolution.Summary != "" {
			parts = append(parts, wake.Resolution.Summary)
		}
		if wake.Resolution.DelegateTo != "" {
			parts = append(parts, "Delegated to "+wake.Resolution.DelegateTo+".")
		}
		return autoRepairClip(strings.Join(parts, " "), 800)
	}
	answer := strings.TrimSpace(wake.Answer)
	if answer == "" {
		return "Escalation received and handled."
	}
	return autoRepairClip(answer, 800)
}

