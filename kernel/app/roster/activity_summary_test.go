// SPDX-License-Identifier: MIT

package roster

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type activityCase struct {
	kind    event.Kind
	subject string
	corr    string
	pl      map[string]any
}

func activityGoldenCases() []activityCase {
	info := event.KindInfo
	long := strings.Repeat("x", 120)
	chain := []any{"m1", "m2"}
	return []activityCase{
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "routing_force_exhausted_detected", "routing_task_type": "code", "routing_force_generation": float64(3)}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "routing_forced_failed_detected"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "routing_unstable_detected", "routing_task_type": long}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "mode": "degraded", "phase": "attempts_exhausted", "self_repair_attempt": float64(2), "self_repair_max_attempts": float64(3)}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "mode": "degraded", "phase": "queued", "reason": "bad config"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "mode": "routing", "phase": "completed", "routing_task_type": "code", "routing_task_model_chain": chain}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "routing_rollback_completed", "routing_task_type": "code", "routing_task_model_chain": chain}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "failed", "error": "boom"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "resolution_failed", "resolution": "force_chain", "reason": "nope"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "delegation_failed", "delegate_to": "helper", "reason": "busy"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "escalation_answered", "resolution": "delegated", "delegate_to": "helper"}},
		{info, "doctor.auto_repair", "", map[string]any{"agent": "a", "phase": "resolution_applied", "resolution": "force_chain", "routing_task_type": "code", "routing_task_model_chain": chain}},
		{info, "agent.repair", "", map[string]any{"agent": "a", "phase": "completed", "applied": []any{"model", "fallbacks"}}},
		{info, "agent.wake", "", map[string]any{"agent": "a", "phase": "requested", "reason": "check", "autonomy_runbook": map[string]any{"trigger_contract": "t", "sleep_contract": "s"}}},
		{info, "agent.wake", "", map[string]any{"agent": "a", "phase": "failed", "error": "no provider"}},
		{event.KindScheduleFired, "schedule.x", "", map[string]any{"agent": "a", "schedule_id": "s1"}},
		{event.KindStandingFired, "standing.x", "", map[string]any{"agent": "a", "trigger_subject": "board.a", "trigger_payload": map[string]any{"from": "ops"}}},
		{event.KindSubAgentSpawned, "", "", map[string]any{"agent": "a", "parent": "boss"}},
		{info, "agent.retire", "", map[string]any{"agent": "a", "reason": "done", "standing_paused": float64(2), "schedules_paused": float64(1)}},
		{info, "agent.revive", "", map[string]any{"agent": "a"}},
		{info, "agent.remove", "", map[string]any{"agent": "a", "standing_removed": float64(1), "skills_archived": float64(2), "workflow_refs_retained": float64(3)}},
		{info, "agent.resolve", "", map[string]any{"agent": "a", "phase": "requested", "resolution": "delegate"}},
		{info, "doctor.auto_repair", "", map[string]any{"target_agent": "a", "agent": "broken", "phase": "escalation_answered", "resolution": "delegated", "delegate_to": "a"}},
		{info, "doctor.auto_repair", "", map[string]any{"target_agent": "a", "agent": "broken", "phase": "escalation_failed", "reason": "x"}},
		{event.KindTaskReceived, "agent.a.task", "run-a", map[string]any{"agent": "a", "intent": "summarize the news " + long}},
		{event.KindTaskCompleted, "", "run-a", map[string]any{"result": "ok"}},
		{event.KindTaskFailed, "", "run-a", map[string]any{"error": "timeout"}},
		{event.KindAgentRetry, "", "run-a", map[string]any{"agent": "a", "next_attempt": float64(2), "max_attempts": float64(4), "reason": "rate limit", "delay_ms": float64(500), "backoff": "exponential", "retry_on": []any{"timeout", "429"}}},
		{event.KindCouncilConvened, "", "run-a", map[string]any{"question": "which plan?"}},
		{event.KindSubAgentSpawned, "", "run-a", map[string]any{"agent": "child"}},
		{event.KindMemoryWritten, "", "", map[string]any{"agent": "a", "subject": "prefs"}},
		{event.KindBoardPosted, "", "", map[string]any{"from": "a", "to": "ops", "text": "help me"}},
		{event.KindRosterUpdated, "", "", map[string]any{"slug": "a", "op": "edit", "reason": "lifecycle completed", "completed_cycles": float64(2), "max_cycles": float64(5)}},
		{event.KindTaskCompleted, "", "run-other", map[string]any{"result": "not mine"}},
		{info, "agent.wake", "", map[string]any{"agent": "b", "phase": "requested"}},
		{event.KindAgentRetry, "", "run-a", map[string]any{"error": "upstream 500"}},
		{event.KindMemoryWritten, "", "", map[string]any{"actor": "a", "action": "stored", "subject": "prefs"}},
		{info, "agent.repair", "", map[string]any{"agent": "a", "phase": "completed", "applied": []any{"model", " ", ""}}},
	}
}

// TestActivitySummaryGoldenBranches pins one event per summary branch family so
// later rewrites of the moved text cannot drift silently.
func TestActivitySummaryGoldenBranches(t *testing.T) {
	want := []struct {
		ok   bool
		text string
	}{
		{true, "forced chain exhausted for code gen 3"},
		{true, "forced chain failed after probation"},
		{true, "routing instability detected for xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx…"},
		{true, "doctor attempts exhausted 2/3"},
		{true, "doctor queued: bad config"},
		{true, "routing rewrote code to m1 -> m2"},
		{true, "routing rolled back code to m1 -> m2"},
		{true, "repair failed: boom"},
		{true, "resolution force_chain failed: nope"},
		{true, "delegated wake failed for helper: busy"},
		{true, "manager delegated escalation to helper"},
		{true, "manager applied forced code to m1 -> m2"},
		{true, "operator repair applied 2 profile change(s)"},
		{true, "operator wake requested: check · contract t/s"},
		{true, "operator wake failed: no provider"},
		{true, "schedule wake fired: s1"},
		{true, "mailbox wake fired: from ops"},
		{true, "delegated wake fired"},
		{true, "operator retired the agent · reason: done · 2 standing paused, 1 schedules paused"},
		{true, "operator revived the agent"},
		{true, "operator removed the agent · 1 standing removed, 2 skills archived, 3 workflow refs retained"},
		{true, "operator requested resolution delegate"},
		{true, "delegated escalation for broken to a"},
		{true, "escalation wake failed: x"},
		{true, "started a run: summarize the news xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx…"},
		{true, "completed a run"},
		{true, "run failed: failed"},
		{true, "retrying run: attempt 2/4 after rate limit (delay 500ms; backoff exponential; retry_on timeout,429)"},
		{true, "consulted the council: which plan?"},
		{true, "delegated to a sub-agent: child"},
		{false, ""},
		{true, "messaged ops"},
		{true, "profile updated"},
		{false, ""},
		{false, ""},
		{true, "retrying run: upstream 500"},
		{true, "memory stored: prefs"},
		{true, "operator repair applied 1 profile change(s)"},
	}
	cases := activityGoldenCases()
	if len(cases) != len(want) {
		t.Fatal(len(cases), len(want))
	}
	for i, c := range cases {
		got, ok := ActivitySummary(&event.Event{Kind: c.kind, Subject: c.subject, CorrelationID: c.corr}, c.pl, "a", map[string]bool{"run-a": true})
		if got != want[i].text || ok != want[i].ok {
			t.Errorf("case %d %s/%s: got %q %v, want %q %v", i, c.kind, c.subject, got, ok, want[i].text, want[i].ok)
		}
	}
	for subject, want := range map[string]bool{"board": true, "board.a": true, "boardx": false, "": false, "agent.board": false} {
		if IsMailboxWakeSubject(subject) != want {
			t.Error("mailbox subject", subject)
		}
	}
}
