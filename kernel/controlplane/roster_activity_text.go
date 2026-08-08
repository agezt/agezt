// SPDX-License-Identifier: MIT

// Event-to-English: the operator-facing copy for an agent's activity timeline.
// This is presentation, not logic — it turns one journal event into the sentence
// the console shows. Kept together and apart from the handlers so a wording
// change never sits in the same file as a policy or teardown change.
// Split from roster.go (refactor Phase 3.5).

package controlplane

import (
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

func agentActivitySummary(e *event.Event, pl map[string]any, slug string, runCorr map[string]bool) (string, bool) {
	if e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		mode := strings.TrimSpace(plString(pl, "mode"))
		phase := strings.TrimSpace(plString(pl, "phase"))
		forceGenSuffix := func() string {
			gen := intNumber(pl["routing_force_generation"])
			if gen > 1 {
				return " gen " + strconv.Itoa(gen)
			}
			return ""
		}
		prefix := "repair"
		if mode == "degraded" {
			prefix = "doctor"
		} else if mode == "routing" {
			prefix = "routing"
		}
		switch phase {
		case "routing_force_exhausted_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "forced chain exhausted for " + truncate(taskType, 40) + forceGenSuffix(), true
			}
			return "forced chain exhausted" + forceGenSuffix(), true
		case "routing_forced_failed_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "forced chain failed for " + truncate(taskType, 40) + forceGenSuffix(), true
			}
			return "forced chain failed after probation" + forceGenSuffix(), true
		case "routing_unstable_detected":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "routing instability detected for " + truncate(taskType, 40), true
			}
			return "routing instability detected", true
		case "attempts_exhausted":
			attempt := plInt(pl, "self_repair_attempt")
			maxAttempts := plInt(pl, "self_repair_max_attempts")
			if attempt > 0 && maxAttempts > 0 {
				return prefix + " attempts exhausted " + strconv.Itoa(attempt) + "/" + strconv.Itoa(maxAttempts), true
			}
			return prefix + " attempts exhausted", true
		case "queued":
			if mode == "degraded" {
				return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
			}
			if mode == "routing" {
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "routing queued for " + truncate(taskType, 40), true
				}
				return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
			}
			issues := len(plStrings(pl, "issues"))
			if issues > 0 {
				return prefix + " queued for " + strconv.Itoa(issues) + " config issue(s)", true
			}
			return prefix + " queued: " + truncate(plString(pl, "reason"), 80), true
		case "routing_rollback_queued":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				return "routing rollback queued for " + truncate(taskType, 40), true
			}
			return "routing rollback queued", true
		case "completed":
			if mode == "routing" {
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "routing rewrote " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
					}
					return "routing rewrote " + truncate(taskType, 30) + " chain", true
				}
			}
			applied := len(plStrings(pl, "applied"))
			if applied > 0 {
				return prefix + " applied " + strconv.Itoa(applied) + " profile change(s)", true
			}
			return prefix + " completed", true
		case "routing_rollback_completed":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
					return "routing rolled back " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
				}
				return "routing rolled back " + truncate(taskType, 30) + " chain", true
			}
			return "routing rollback completed", true
		case "failed":
			if err := strings.TrimSpace(plString(pl, "error")); err != "" {
				return prefix + " failed: " + truncate(err, 80), true
			}
			return prefix + " failed", true
		case "routing_rollback_failed":
			if err := strings.TrimSpace(plString(pl, "error")); err != "" {
				return "routing rollback failed: " + truncate(err, 80), true
			}
			return "routing rollback failed", true
		case "resolution_failed":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "resolution " + res + " failed: " + truncate(plString(pl, "reason"), 80), true
			}
			return "resolution follow-up failed: " + truncate(plString(pl, "reason"), 80), true
		case "delegation_queued":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "manager delegated escalation to " + truncate(to, 60), true
			}
			return "delegation queued", true
		case "delegation_woke":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "delegated wake launched for " + truncate(to, 60), true
			}
			return "delegated wake launched", true
		case "delegation_failed":
			if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
				return "delegated wake failed for " + truncate(to, 60) + ": " + truncate(plString(pl, "reason"), 80), true
			}
			return "delegated wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "escalation_answered":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "manager delegated escalation to " + truncate(to, 60), true
				}
				return "manager delegated escalation", true
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "manager forced " + truncate(taskType, 30) + " to " + truncate(chain, 80) + forceGenSuffix(), true
					}
					return "manager forced " + truncate(taskType, 30) + " chain" + forceGenSuffix(), true
				}
				return "manager forced a routing chain" + forceGenSuffix(), true
			case "retired":
				return "manager retired the agent after escalation", true
			case "paused":
				return "manager paused the agent after escalation", true
			case "blocked":
				return "manager marked escalation blocked", true
			default:
				return "manager answered escalation", true
			}
		case "resolution_applied":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "manager applied forced " + truncate(taskType, 30) + " to " + truncate(chain, 80) + forceGenSuffix(), true
					}
					return "manager applied forced " + truncate(taskType, 30) + " chain" + forceGenSuffix(), true
				}
				return "manager applied a forced routing chain" + forceGenSuffix(), true
			case "retired":
				return "manager retirement was applied", true
			case "paused":
				return "manager pause was applied", true
			default:
				return "manager resolution applied", true
			}
		default:
			if phase != "" {
				return prefix + " " + phase, true
			}
		}
	}
	if e.Subject == "agent.repair" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			return "operator requested a governed repair run", true
		case "completed":
			if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
				if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
					return "operator rewrote " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
				}
				return "operator rewrote " + truncate(taskType, 30) + " chain", true
			}
			if applied := len(plStrings(pl, "applied")); applied > 0 {
				return "operator repair applied " + strconv.Itoa(applied) + " profile change(s)", true
			}
			return "operator repair completed", true
		case "failed":
			return "operator repair failed: " + truncate(plString(pl, "error"), 80), true
		}
	}
	if e.Subject == "agent.wake" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			if reason := strings.TrimSpace(plString(pl, "reason")); reason != "" {
				return joinActivityParts("operator wake requested: "+truncate(reason, 80), contract), true
			}
			return joinActivityParts("operator wake requested", contract), true
		case "completed":
			return joinActivityParts("operator wake completed", contract), true
		case "failed":
			return joinActivityParts("operator wake failed: "+truncate(plString(pl, "error"), 80), contract), true
		}
	}
	if e.Kind == event.KindScheduleFired && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		label := "schedule wake fired"
		if id := strings.TrimSpace(plString(pl, "schedule_id")); id != "" {
			label += ": " + truncate(id, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Kind == event.KindStandingFired && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		// A board.* trigger subject is the mailbox-wake route: name the message
		// (and sender) that woke the agent rather than the standing order id.
		if isMailboxWakeSubject(plString(pl, "trigger_subject")) {
			tp, _ := pl["trigger_payload"].(map[string]any)
			label := "mailbox wake fired"
			if from := strings.TrimSpace(plString(tp, "from")); from != "" {
				label += ": from " + truncate(from, 60)
			} else if id := strings.TrimSpace(plString(tp, "id")); id != "" {
				label += ": " + truncate(id, 60)
			}
			return joinActivityParts(label, contract), true
		}
		label := "standing wake fired"
		// standing.fired addresses the order by "id"/"name" (not "standing_id").
		if id := strings.TrimSpace(firstNonEmpty(plString(pl, "standing_id"), plString(pl, "id"))); id != "" {
			label += ": " + truncate(id, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Kind == event.KindSubAgentSpawned && plString(pl, "agent") == slug {
		contract := wakeRunbookActivitySuffix(pl)
		label := "delegated wake fired"
		if by := strings.TrimSpace(plString(pl, "delegated_by")); by != "" {
			label += ": by " + truncate(by, 60)
		}
		return joinActivityParts(label, contract), true
	}
	if e.Subject == "agent.retire" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		parts := []string{"operator retired the agent"}
		if reason := strings.TrimSpace(plString(pl, "reason")); reason != "" {
			parts = append(parts, "reason: "+truncate(reason, 80))
		}
		if paused := pausedTriggerSummary(pl); paused != "" {
			parts = append(parts, paused)
		}
		return strings.Join(parts, " · "), true
	}
	if e.Subject == "agent.revive" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		if paused := pausedTriggerSummary(pl); paused != "" {
			return "operator revived the agent · " + paused, true
		}
		return "operator revived the agent", true
	}
	if e.Subject == "agent.remove" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		if cleanup := removalCleanupSummary(pl); cleanup != "" {
			return "operator removed the agent · " + cleanup, true
		}
		return "operator removed the agent", true
	}
	if e.Subject == "agent.resolve" && e.Kind == event.KindInfo && plString(pl, "agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "requested":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "operator requested resolution " + res, true
			}
			return "operator requested a resolution", true
		case "completed":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					if chain := strings.Join(plStrings(pl, "routing_task_model_chain"), " -> "); chain != "" {
						return "operator forced " + truncate(taskType, 30) + " to " + truncate(chain, 80), true
					}
				}
				return "operator forced a routing chain", true
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "operator delegated incident to " + truncate(to, 60), true
				}
				return "operator delegated incident", true
			case "paused":
				return "operator paused the agent", true
			case "retired":
				return "operator retired the agent", true
			default:
				return "operator resolution completed", true
			}
		case "failed":
			if res := strings.TrimSpace(plString(pl, "resolution")); res != "" {
				return "operator resolution " + res + " failed: " + truncate(plString(pl, "reason"), 80), true
			}
			return "operator resolution failed: " + truncate(plString(pl, "reason"), 80), true
		}
	}
	if e.Subject == "doctor.auto_repair" && e.Kind == event.KindInfo && plString(pl, "target_agent") == slug {
		switch phase := strings.TrimSpace(plString(pl, "phase")); phase {
		case "escalation_answered":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "delegated":
				if to := strings.TrimSpace(plString(pl, "delegate_to")); to != "" {
					return "delegated escalation for " + truncate(plString(pl, "agent"), 60) + " to " + truncate(to, 40), true
				}
				return "delegated escalation for " + truncate(plString(pl, "agent"), 60), true
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "forced routing chain for " + truncate(plString(pl, "agent"), 60) + " on " + truncate(taskType, 30), true
				}
				return "forced routing chain for " + truncate(plString(pl, "agent"), 60), true
			case "retired":
				return "retired " + truncate(plString(pl, "agent"), 60) + " after escalation", true
			case "paused":
				return "paused " + truncate(plString(pl, "agent"), 60) + " after escalation", true
			case "blocked":
				return "marked escalation blocked for " + truncate(plString(pl, "agent"), 60), true
			default:
				return "answered escalation for " + truncate(plString(pl, "agent"), 60), true
			}
		case "escalation_woke":
			return joinActivityParts("accepted escalation wake for "+truncate(plString(pl, "agent"), 60), wakeRunbookActivitySuffix(pl)), true
		case "escalation_skipped":
			return "skipped escalation wake: " + truncate(plString(pl, "reason"), 80), true
		case "escalation_failed":
			return "escalation wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "delegation_queued":
			return "received delegated escalation for " + truncate(plString(pl, "agent"), 60), true
		case "delegation_woke":
			return joinActivityParts("accepted delegated escalation for "+truncate(plString(pl, "agent"), 60), wakeRunbookActivitySuffix(pl)), true
		case "delegation_failed":
			return "delegated escalation wake failed: " + truncate(plString(pl, "reason"), 80), true
		case "resolution_applied":
			switch res := strings.TrimSpace(plString(pl, "resolution")); res {
			case "force_chain":
				if taskType := strings.TrimSpace(plString(pl, "routing_task_type")); taskType != "" {
					return "applied forced routing chain for " + truncate(plString(pl, "agent"), 60) + " on " + truncate(taskType, 30), true
				}
				return "applied forced routing chain for " + truncate(plString(pl, "agent"), 60), true
			case "retired":
				return "applied retirement for " + truncate(plString(pl, "agent"), 60), true
			case "paused":
				return "applied pause for " + truncate(plString(pl, "agent"), 60), true
			default:
				return "applied resolution for " + truncate(plString(pl, "agent"), 60), true
			}
		}
	}
	switch e.Kind {
	case event.KindTaskReceived:
		if plString(pl, "agent") == slug {
			return "started a run: " + truncate(plString(pl, "intent"), 100), true
		}
	case event.KindTaskCompleted:
		if runCorr[e.CorrelationID] {
			return "completed a run", true
		}
	case event.KindTaskFailed:
		if runCorr[e.CorrelationID] {
			r := plString(pl, "reason")
			if r == "" {
				r = "failed"
			}
			return "run failed: " + truncate(r, 80), true
		}
	case event.KindAgentRetry:
		if runCorr[e.CorrelationID] || plString(pl, "agent") == slug {
			attempt := plInt(pl, "next_attempt")
			maxAttempts := plInt(pl, "max_attempts")
			reason := firstNonEmpty(plString(pl, "reason"), plString(pl, "error"))
			policy := agentRetryPolicySummary(pl)
			if attempt > 0 && maxAttempts > 0 {
				return "retrying run: attempt " + strconv.Itoa(attempt) + "/" + strconv.Itoa(maxAttempts) + " after " + truncate(reason, 80) + policy, true
			}
			return "retrying run: " + truncate(reason, 80) + policy, true
		}
	case event.KindCouncilConvened:
		if runCorr[e.CorrelationID] {
			return "consulted the council: " + truncate(plString(pl, "question"), 100), true
		}
	case event.KindSubAgentSpawned:
		// The agent delegated (its run spawned a sub-agent), or it WAS the named
		// sub-agent that ran.
		if runCorr[e.CorrelationID] {
			return "delegated to a sub-agent: " + truncate(plString(pl, "agent"), 60), true
		}
		if plString(pl, "agent") == slug {
			return "ran as a delegated sub-agent", true
		}
	case event.KindMemoryWritten:
		if plString(pl, "actor") == slug {
			return "memory " + plString(pl, "action") + ": " + truncate(plString(pl, "subject"), 80), true
		}
	case event.KindBoardPosted:
		if plString(pl, "from") == slug {
			if to := plString(pl, "to"); to != "" {
				return "messaged " + to, true
			}
			return "posted to the board: " + truncate(plString(pl, "topic"), 60), true
		}
	case event.KindRosterUpdated:
		if plString(pl, "slug") == slug {
			a := plString(pl, "action")
			if a == "" {
				a = "updated"
			}
			if a == "lifecycle_cycle_completed" {
				completed := plInt(pl, "completed_cycles")
				max := plInt(pl, "max_cycles")
				if max > 0 {
					return "completed lifecycle cycle " + strconv.Itoa(completed) + "/" + strconv.Itoa(max), true
				}
				return "completed lifecycle cycle " + strconv.Itoa(completed), true
			}
			if a == "retired" {
				reason := strings.TrimSpace(plString(pl, "reason"))
				if strings.Contains(strings.ToLower(reason), "completed") {
					if reason != "" {
						return "lifecycle retired the agent: " + truncate(reason, 100), true
					}
					return "lifecycle retired the agent", true
				}
				if reason != "" {
					return "profile retired: " + truncate(reason, 100), true
				}
				return "profile retired", true
			}
			if a == "revived" {
				return "profile revived", true
			}
			return "profile " + a, true
		}
	}
	return "", false
}

func agentRetryPolicySummary(pl map[string]any) string {
	var bits []string
	if delay := plInt(pl, "delay_ms"); delay > 0 {
		bits = append(bits, "delay "+strconv.Itoa(delay)+"ms")
	}
	if backoff := strings.TrimSpace(plString(pl, "backoff")); backoff != "" {
		bits = append(bits, "backoff "+backoff)
	}
	if retryOn := plStrings(pl, "retry_on"); len(retryOn) > 0 {
		bits = append(bits, "retry_on "+strings.Join(retryOn, ","))
	}
	if len(bits) == 0 {
		return ""
	}
	return " (" + strings.Join(bits, "; ") + ")"
}

func pausedTriggerSummary(pl map[string]any) string {
	var bits []string
	if n := plInt(pl, "standing_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" standing paused")
	}
	if n := plInt(pl, "schedules_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" schedules paused")
	}
	return strings.Join(bits, ", ")
}

func removalCleanupSummary(pl map[string]any) string {
	var bits []string
	fields := []struct {
		key   string
		label string
	}{
		{"standing_removed", "standing removed"},
		{"schedules_removed", "schedules removed"},
		{"memories_forgotten", "private memories forgotten"},
		{"authored_memories_forgotten", "authored memories forgotten"},
		{"skills_archived", "skills archived"},
		{"configs_deleted", "configs deleted"},
		{"configs_access_pruned", "shared config access pruned"},
		{"workspaces_deleted", "workspaces deleted"},
		{"subagents_retired", "sub-agents retired"},
		{"mailbox_messages_retained", "mailbox/audit messages retained"},
		{"workflow_refs_retained", "workflow refs retained"},
		{"subagent_workflow_refs_retained", "sub-agent workflow refs retained"},
	}
	for _, f := range fields {
		if n := plInt(pl, f.key); n > 0 {
			bits = append(bits, strconv.Itoa(n)+" "+f.label)
		}
	}
	return strings.Join(bits, ", ")
}
