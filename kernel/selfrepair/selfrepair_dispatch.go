// SPDX-License-Identifier: MIT

package selfrepair

// Self-repair coordinator dispatch + lifecycle hooks (M846): the main
// dispatch loop that routes candidates to wake/escalate/apply, plus the
// release/publishAutoRepair lifecycle endpoints. Carved out of
// selfrepair.go during the Day 25 god file split #6 so the main file can
// focus on the wire-up + claim path.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

func autoRepairClip(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func autoRepairQueuedPhase(cand autoRepairCandidate) string {
	if cand.SelfRepairExhausted {
		return "attempts_exhausted"
	}
	if cand.Mode == "routing_forced_failed" {
		return "routing_forced_failed_detected"
	}
	if cand.Mode == "routing_forced_exhausted" {
		return "routing_force_exhausted_detected"
	}
	if cand.Mode == "routing_unstable" {
		return "routing_unstable_detected"
	}
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_queued"
	}
	return "queued"
}

func autoRepairCompletedPhase(cand autoRepairCandidate) string {
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_completed"
	}
	return "completed"
}

func autoRepairFailedPhase(cand autoRepairCandidate) string {
	if cand.RoutingRollbackTaskType != "" {
		return "routing_rollback_failed"
	}
	return "failed"
}

func (c *autoRepairCoordinator) dispatch(ctx context.Context, k *kernelruntime.Kernel, b *bus.Bus, src autoRepairSource, mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate) {
	// Panic firewall (WF-001). dispatch is launched with a bare `go` from the
	// coordinator loop, so nothing above can recover it — and it drives a full
	// governed run: provider calls, tools, plugin subprocesses, MCP servers, then
	// a mailbox post over the network. Any of those can panic, and a repair
	// attempt taking the daemon down is the worst possible failure mode for the
	// component whose entire job is keeping the fleet healthy.
	//
	// Registered BEFORE the release defer so it runs last, after the slug lock is
	// returned: a contained panic must not also leak the in-flight claim and
	// wedge every future repair of this agent.
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "auto-repair for %q panicked: %v\n", cand.Slug, r)
		if b != nil {
			_, _ = b.Publish(event.Spec{
				Subject: autoRepairPulseSubject,
				Kind:    event.KindSelfRepairPanic,
				Actor:   "selfrepair",
				Payload: map[string]any{
					"agent":       cand.Slug,
					"mode":        cand.Mode,
					"incident_id": autoRepairIncidentIDValue(cand),
					"panic":       fmt.Sprintf("%v", r),
				},
			})
		}
	}()
	defer c.release(cand.Slug)
	if cand.SelfRepairExhausted {
		err := fmt.Errorf("self-repair attempts exhausted (%d/%d)", cand.SelfRepairAttempt, cand.SelfRepairMaxAttempts)
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, err)
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	if cand.Mode == "routing_unstable" || cand.Mode == "routing_forced_failed" || cand.Mode == "routing_forced_exhausted" {
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, errors.New(cand.Reason))
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	var (
		res overseertool.RepairResult
		err error
	)
	if cand.RoutingRollbackTaskType != "" && len(cand.RoutingRollbackToChain) > 0 {
		rb, ok := src.(autoRepairRoutingRollbacker)
		if !ok {
			err = fmt.Errorf("routing rollback is not supported by the active repair source")
		} else {
			res, err = rb.RollbackRouting(cand.Slug, cand.RoutingRollbackTaskType, cand.RoutingRollbackToChain, cand.Reason)
		}
	} else {
		res, err = src.RepairAgent(cand.Slug, cand.Reason)
	}
	if err != nil {
		publishAutoRepair(b, "", map[string]any{
			"phase":                             autoRepairFailedPhase(cand),
			"agent":                             cand.Slug,
			"mode":                              cand.Mode,
			"issues":                            cand.Issues,
			"reason":                            cand.Reason,
			"fingerprint":                       cand.Fingerprint,
			"self_repair_attempt":               cand.SelfRepairAttempt,
			"self_repair_max_attempts":          cand.SelfRepairMaxAttempts,
			"error":                             err.Error(),
			"routing_task_type":                 strutil.FirstNonEmpty(cand.RoutingRollbackTaskType, res.RoutingTaskType),
			"routing_task_model_chain":          strutil.FirstNonEmptySlice(cand.RoutingRollbackToChain, res.RoutingTaskModelChain),
			"previous_routing_task_model_chain": strutil.FirstNonEmptySlice(cand.RoutingRollbackFromChain, res.PreviousRoutingTaskModelChain),
			"incident_id":                       autoRepairIncidentIDValue(cand),
			"root_incident_id":                  autoRepairRootChainID(cand),
			"parent_incident_id":                strings.TrimSpace(cand.ParentHopID),
		})
		msg, mailboxErr := c.autoEscalate(mailbox, postNotify, cand, err)
		c.autoWakeManager(ctx, k, b, src, mailbox, postNotify, cand, msg, mailboxErr)
		return
	}
	publishAutoRepair(b, res.Correlation, map[string]any{
		"phase":                             autoRepairCompletedPhase(cand),
		"agent":                             cand.Slug,
		"mode":                              cand.Mode,
		"issues":                            cand.Issues,
		"reason":                            cand.Reason,
		"fingerprint":                       cand.Fingerprint,
		"self_repair_attempt":               cand.SelfRepairAttempt,
		"self_repair_max_attempts":          cand.SelfRepairMaxAttempts,
		"applied":                           res.Applied,
		"routing_task_type":                 res.RoutingTaskType,
		"routing_task_model_chain":          res.RoutingTaskModelChain,
		"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
		"answer":                            res.Answer,
		"incident_id":                       autoRepairIncidentIDValue(cand),
		"root_incident_id":                  autoRepairRootChainID(cand),
		"parent_incident_id":                strings.TrimSpace(cand.ParentHopID),
	})
}

func (c *autoRepairCoordinator) autoEscalate(mailbox Mailbox, postNotify func(board.Message, string), cand autoRepairCandidate, repairErr error) (*board.Message, error) {
	if mailbox == nil {
		return nil, nil
	}
	target := strings.TrimSpace(cand.EscalateTo)
	msg, err := mailbox.HelpRequest(cand.EscalateFrom, target, autoRepairEscalationText(cand, repairErr), c.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if postNotify != nil {
		postNotify(msg, "")
	}
	return &msg, nil
}

func autoRepairEscalationText(cand autoRepairCandidate, repairErr error) string {
	target := strings.TrimSpace(cand.Slug)
	mode := strings.TrimSpace(cand.Mode)
	text := "Doctor self-repair failed"
	if mode == "degraded" {
		text = "Doctor recovery failed"
	}
	if target != "" {
		text += " for agent " + target
	}
	if reason := autoRepairClip(cand.Reason, 220); reason != "" {
		text += ". Reason: " + reason
	}
	if repairErr != nil {
		text += ". Error: " + autoRepairClip(repairErr.Error(), 220)
	}
	if cand.Fingerprint != "" {
		text += ". Fingerprint: " + autoRepairClip(cand.Fingerprint, 160)
	}
	return text
}

func (c *autoRepairCoordinator) release(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, slug)
}

func publishAutoRepair(b *bus.Bus, corr string, payload map[string]any) {
	if b == nil {
		return
	}
	_, _ = b.Publish(event.Spec{
		Subject:       autoRepairEventSubject,
		Kind:          event.KindInfo,
		Actor:         "kernel",
		CorrelationID: corr,
		Payload:       payload,
	})
}

