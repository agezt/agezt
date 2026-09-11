// SPDX-License-Identifier: MIT

package selfrepair

// Self-repair apply pipeline (M846): the wake-result -> profile mutation
// half of the resolution cycle, including the delegated-resolution text
// formatters and the incident-id helpers. Carved out of selfrepair.go
// during the Day 25 god file split #4 so the main file can focus on the
// claim/dispatch/retry path.

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func (c *autoRepairCoordinator) applyDelegatedResolution(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate, wake autoRepairWakeResult) error {
	target := strings.TrimSpace(wake.Resolution.DelegateTo)
	if target == "" {
		return fmt.Errorf("delegated resolution is missing delegate_to")
	}
	if target == cand.Slug {
		return fmt.Errorf("delegated resolution points back to the broken agent %s", cand.Slug)
	}
	if strings.TrimSpace(wake.Target) != "" && target == strings.TrimSpace(wake.Target) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", wake.Target)
	}
	if mailbox == nil {
		return fmt.Errorf("delegated resolution requires mailbox access")
	}
	msg, err := mailbox.HelpRequest(strings.TrimSpace(wake.Target), target, autoRepairDelegationText(cand, wake), c.now().UnixMilli())
	if err != nil {
		return err
	}
	if postNotify != nil {
		postNotify(msg, wake.Correlation)
	}
	childIncidentID := autoRepairChildIncidentID(cand, target, c.now())
	publishAutoRepair(b, wake.Correlation, map[string]any{
		"phase":              "delegation_queued",
		"agent":              cand.Slug,
		"mode":               cand.Mode,
		"root_agent":         autoRepairRootAgent(cand),
		"chain_depth":        cand.ChainDepth + 1,
		"incident_id":        childIncidentID,
		"root_incident_id":   autoRepairRootChainID(cand),
		"parent_incident_id": autoRepairIncidentIDValue(cand),
		"target_agent":       target,
		"delegated_by":       strings.TrimSpace(wake.Target),
		"target_correlation": wake.Correlation,
		"fingerprint":        cand.Fingerprint,
		"mailbox_message_id": strings.TrimSpace(msg.ID),
		"resolution":         wake.Resolution.Resolution,
		"resolution_summary": wake.Resolution.Summary,
		"delegate_to":        target,
	})
	res, wakeErr := autoRepairWakeAgent(ctx, k, autoRepairCandidate{
		Slug:        cand.Slug,
		Mode:        cand.Mode,
		Reason:      autoRepairDelegationReason(cand, wake),
		Fingerprint: cand.Fingerprint,
		EscalateTo:  target,
		RootAgent:   autoRepairRootAgent(cand),
		ChainDepth:  cand.ChainDepth + 1,
		IncidentID:  childIncidentID,
		RootChainID: autoRepairRootChainID(cand),
		ParentHopID: autoRepairIncidentIDValue(cand),
	}, &msg)
	if wakeErr != nil {
		publishAutoRepair(b, res.Correlation, map[string]any{
			"phase":              "delegation_failed",
			"agent":              cand.Slug,
			"mode":               cand.Mode,
			"root_agent":         autoRepairRootAgent(cand),
			"chain_depth":        cand.ChainDepth + 1,
			"incident_id":        childIncidentID,
			"root_incident_id":   autoRepairRootChainID(cand),
			"parent_incident_id": autoRepairIncidentIDValue(cand),
			"target_agent":       target,
			"delegated_by":       strings.TrimSpace(wake.Target),
			"target_correlation": res.Correlation,
			"fingerprint":        cand.Fingerprint,
			"reason":             wakeErr.Error(),
			"mailbox_message_id": strings.TrimSpace(msg.ID),
			"resolution":         wake.Resolution.Resolution,
			"resolution_summary": wake.Resolution.Summary,
			"delegate_to":        target,
		})
		return wakeErr
	}
	if res.Skipped != "" {
		publishAutoRepair(b, "", map[string]any{
			"phase":              "delegation_failed",
			"agent":              cand.Slug,
			"mode":               cand.Mode,
			"root_agent":         autoRepairRootAgent(cand),
			"chain_depth":        cand.ChainDepth + 1,
			"incident_id":        childIncidentID,
			"root_incident_id":   autoRepairRootChainID(cand),
			"parent_incident_id": autoRepairIncidentIDValue(cand),
			"target_agent":       target,
			"delegated_by":       strings.TrimSpace(wake.Target),
			"target_correlation": res.Correlation,
			"fingerprint":        cand.Fingerprint,
			"reason":             res.Skipped,
			"mailbox_message_id": strings.TrimSpace(msg.ID),
			"resolution":         wake.Resolution.Resolution,
			"resolution_summary": wake.Resolution.Summary,
			"delegate_to":        target,
		})
		return nil
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":              "delegation_woke",
		"agent":              cand.Slug,
		"mode":               cand.Mode,
		"root_agent":         autoRepairRootAgent(cand),
		"chain_depth":        cand.ChainDepth + 1,
		"incident_id":        childIncidentID,
		"root_incident_id":   autoRepairRootChainID(cand),
		"parent_incident_id": autoRepairIncidentIDValue(cand),
		"target_agent":       target,
		"delegated_by":       strings.TrimSpace(wake.Target),
		"target_correlation": res.Correlation,
		"fingerprint":        cand.Fingerprint,
		"mailbox_message_id": strings.TrimSpace(msg.ID),
		"resolution":         wake.Resolution.Resolution,
		"resolution_summary": wake.Resolution.Summary,
		"delegate_to":        target,
		"wake_source":        "doctor",
		"autonomy_runbook":   res.Runbook,
	})
	return nil
}

func autoRepairDelegationText(cand autoRepairCandidate, wake autoRepairWakeResult) string {
	var parts []string
	parts = append(parts, "Escalated responsibility for agent "+cand.Slug+".")
	switch cand.Mode {
	case "degraded":
		parts = append(parts, "This is a degraded-doctor recovery follow-up.")
	case "routing":
		parts = append(parts, "This is a routing-repair follow-up.")
	case "routing_forced_exhausted":
		parts = append(parts, "This is a forced-chain-exhausted follow-up after multiple owner-forced generations still failed.")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			parts = append(parts, "Forced task type: "+taskType+".")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			parts = append(parts, "Forced chain: "+strings.Join(cand.RoutingRollbackToChain, " → ")+".")
		}
	case "routing_unstable":
		parts = append(parts, "This is an unstable-routing follow-up after rollback pressure returned.")
	case "routing_forced_failed":
		parts = append(parts, "This is a forced-chain-failed follow-up after owner probation expired.")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			parts = append(parts, "Forced task type: "+taskType+".")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			parts = append(parts, "Forced chain: "+strings.Join(cand.RoutingRollbackToChain, " → ")+".")
		}
	default:
		parts = append(parts, "This is a config-repair follow-up.")
	}
	if reason := autoRepairClip(cand.Reason, 220); reason != "" {
		parts = append(parts, "Original reason: "+reason)
	}
	if wake.Resolution != nil && wake.Resolution.Summary != "" {
		parts = append(parts, "Owner note: "+autoRepairClip(wake.Resolution.Summary, 220))
	}
	return strings.Join(parts, " ")
}

func autoRepairDelegationReason(cand autoRepairCandidate, wake autoRepairWakeResult) string {
	base := "delegated escalation follow-up for agent " + cand.Slug
	if wake.Resolution != nil && wake.Resolution.Summary != "" {
		return base + ": " + autoRepairClip(wake.Resolution.Summary, 220)
	}
	if cand.Reason != "" {
		return base + ": " + autoRepairClip(cand.Reason, 220)
	}
	return base
}

func autoRepairRootAgent(cand autoRepairCandidate) string {
	root := strings.TrimSpace(cand.RootAgent)
	if root != "" {
		return root
	}
	return strings.TrimSpace(cand.Slug)
}

func autoRepairIncidentID(root, fingerprint string, now time.Time) string {
	root = autoRepairIDPart(root)
	if root == "" {
		root = "agent"
	}
	fp := autoRepairIDPart(fingerprint)
	if fp == "" {
		fp = "repair"
	}
	return fmt.Sprintf("%s-%d-%s", root, now.UnixMilli(), fp)
}

func autoRepairChildIncidentID(cand autoRepairCandidate, target string, now time.Time) string {
	fp := strings.TrimSpace(cand.Fingerprint)
	if target = strings.TrimSpace(target); target != "" {
		fp += " " + target
	}
	return autoRepairIncidentID(autoRepairRootAgent(cand), fp, now)
}

func autoRepairIDPart(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-', r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 32 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func autoRepairIncidentIDValue(cand autoRepairCandidate) string {
	return strings.TrimSpace(cand.IncidentID)
}

func autoRepairRootChainID(cand autoRepairCandidate) string {
	if root := strings.TrimSpace(cand.RootChainID); root != "" {
		return root
	}
	return autoRepairIncidentIDValue(cand)
}

