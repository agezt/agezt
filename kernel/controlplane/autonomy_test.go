// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func ev(kind event.Kind, payload map[string]any) *event.Event {
	b, _ := json.Marshal(payload)
	return &event.Event{Kind: kind, Payload: b}
}

func TestAutonomyDetail(t *testing.T) {
	cases := []struct {
		name string
		e    *event.Event
		want string
	}{
		{"schedule intent", ev(event.KindScheduleFired, map[string]any{"intent": "digest the inbox"}), "digest the inbox"},
		{"standing name", ev(event.KindStandingCreated, map[string]any{"name": "watch CI"}), "watch CI"},
		{"skill name", ev(event.KindSkillPromoted, map[string]any{"name": "diagnose-ci", "id": "abc"}), "diagnose-ci"},
		{"skill id fallback", ev(event.KindSkillCreated, map[string]any{"id": "deadbeef"}), "deadbeef"},
		{"assure complete", ev(event.KindAssureVerdict, map[string]any{"complete": true}), "complete: true"},
		{"assure gap", ev(event.KindAssureVerdict, map[string]any{"complete": false, "gap": "no tests"}), "gap: no tests"},
		{"briefing subject", ev(event.KindBriefingSent, map[string]any{"subject": "morning digest"}), "morning digest"},
		{"board topic+from", ev(event.KindBoardPosted, map[string]any{"topic": "acil-mudahale", "from": "watcher"}), "acil-mudahale · from watcher"},
		{"board topic only", ev(event.KindBoardPosted, map[string]any{"topic": "general"}), "general"},
		{"delegation named+by+task", ev(event.KindSubAgentSpawned, map[string]any{"agent": "worker", "delegated_by": "lead", "task": "summarize logs"}), "worker · by lead · summarize logs"},
		{"delegation named only", ev(event.KindSubAgentSpawned, map[string]any{"agent": "worker"}), "worker"},
		{
			"doctor unstable routing detected",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "routing_unstable", "phase": "routing_unstable_detected",
					"routing_task_type": "code",
				}),
			},
			"builder · unstable routing detected for code",
		},
		{
			"doctor forced chain exhausted detected",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "routing_forced_exhausted", "phase": "routing_force_exhausted_detected",
					"routing_task_type": "code", "routing_force_generation": 3,
				}),
			},
			"builder · forced chain exhausted for code · gen 3",
		},
		{
			"doctor forced chain failed detected",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "routing_forced_failed", "phase": "routing_forced_failed_detected",
					"routing_task_type": "code",
				}),
			},
			"builder · forced chain failed for code",
		},
		{
			"doctor queued",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "misconfigured", "phase": "queued",
					"issues": []string{"AGEZT_MAX_ITER: must be an integer"},
				}),
			},
			"builder · AGEZT_MAX_ITER: must be an integer",
		},
		{
			"doctor completed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "degraded", "phase": "completed",
					"applied": []string{"model", "config_overrides"},
				}),
			},
			"builder · applied model, config_overrides",
		},
		{
			"doctor routing completed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "routing", "phase": "completed",
					"routing_task_type":        "code",
					"routing_task_model_chain": []string{"gpt-4.1", "deepseek-chat"},
				}),
			},
			"builder · rewrote code chain to gpt-4.1 → deepseek-chat",
		},
		{
			"doctor routing rollback completed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "routing", "phase": "routing_rollback_completed",
					"routing_task_type":        "code",
					"routing_task_model_chain": []string{"gpt-5", "gpt-4.1"},
				}),
			},
			"builder · rolled back code chain to gpt-5 → gpt-4.1",
		},
		{
			"doctor failed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "mode": "degraded", "phase": "failed", "error": "provider timeout",
				}),
			},
			"builder · provider timeout",
		},
		{
			"doctor escalation woke",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "escalation_woke", "target_agent": "lead",
				}),
			},
			"builder · woke lead",
		},
		{
			"doctor escalation answered",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "escalation_answered", "target_agent": "lead",
				}),
			},
			"builder · answered by lead",
		},
		{
			"doctor escalation delegated",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "escalation_answered", "target_agent": "lead",
					"resolution": "delegated", "resolution_summary": "needs infra owner review", "delegate_to": "infra-lead",
				}),
			},
			"builder · delegated by lead to infra-lead: needs infra owner review",
		},
		{
			"doctor resolution applied force chain",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "resolution_applied", "target_agent": "lead",
					"resolution": "force_chain", "routing_task_type": "code", "routing_task_model_chain": []string{"gpt-5", "gpt-4.1"},
					"routing_force_generation": 2,
				}),
			},
			"builder · applied forced code chain by lead to gpt-5 → gpt-4.1 · gen 2",
		},
		{
			"doctor resolution failed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "resolution_failed", "resolution": "retired", "reason": "permission denied",
				}),
			},
			"builder · retired failed: permission denied",
		},
		{
			"doctor delegation woke",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "delegation_woke", "delegate_to": "infra-lead",
				}),
			},
			"builder · woke delegated agent infra-lead",
		},
		{
			"doctor delegation failed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "doctor.auto_repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "delegation_failed", "delegate_to": "infra-lead", "reason": "target agent infra-lead is paused",
				}),
			},
			"builder · delegated wake infra-lead failed: target agent infra-lead is paused",
		},
		{
			"operator repair requested",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "agent.repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "requested", "reason": "manual rerun from incident",
				}),
			},
			"builder · operator repair requested: manual rerun from incident",
		},
		{
			"operator repair completed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "agent.repair",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "completed", "applied": []string{"model", "config_overrides"},
				}),
			},
			"builder · applied model, config_overrides",
		},
		{
			"operator wake failed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "agent.wake",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "failed", "error": "provider timeout",
				}),
			},
			"builder · provider timeout",
		},
		{
			"operator resolution completed",
			&event.Event{
				Kind:    event.KindInfo,
				Subject: "agent.resolve",
				Payload: mustJSON(map[string]any{
					"agent": "builder", "phase": "completed", "resolution": "force_chain",
					"routing_task_type": "code", "routing_task_model_chain": []string{"gpt-5", "gpt-4.1"},
				}),
			},
			"builder · operator forced code chain to gpt-5 → gpt-4.1",
		},
		{"empty payload", &event.Event{Kind: event.KindScheduleFired}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := autonomyDetail(c.e); got != c.want {
				t.Errorf("autonomyDetail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAutonomyMeta_DoctorAutoRepair(t *testing.T) {
	cases := []struct {
		name      string
		payload   map[string]any
		wantCat   string
		wantTitle string
	}{
		{"queued unstable routing", map[string]any{"mode": "routing_unstable", "phase": "routing_unstable_detected"}, "doctor", "an unstable routing escalation was queued"},
		{"queued forced-chain exhaustion", map[string]any{"mode": "routing_forced_exhausted", "phase": "routing_force_exhausted_detected"}, "doctor", "a forced-chain exhaustion escalation was queued"},
		{"queued forced-chain failure", map[string]any{"mode": "routing_forced_failed", "phase": "routing_forced_failed_detected"}, "doctor", "a forced-chain failure escalation was queued"},
		{"queued config", map[string]any{"mode": "misconfigured", "phase": "queued"}, "doctor", "a config repair was queued"},
		{"queued routing", map[string]any{"mode": "routing", "phase": "queued"}, "doctor", "a routing repair was queued"},
		{"queued routing rollback", map[string]any{"mode": "routing", "phase": "routing_rollback_queued"}, "doctor", "a routing rollback was queued"},
		{"completed degraded", map[string]any{"mode": "degraded", "phase": "completed"}, "doctor", "a doctor run repaired an agent"},
		{"completed routing", map[string]any{"mode": "routing", "phase": "completed"}, "doctor", "a routing repair rewrote a chain"},
		{"completed routing rollback", map[string]any{"mode": "routing", "phase": "routing_rollback_completed"}, "doctor", "a routing rollback restored a chain"},
		{"failed degraded", map[string]any{"mode": "degraded", "phase": "failed"}, "doctor", "a doctor run failed"},
		{"failed routing", map[string]any{"mode": "routing", "phase": "failed"}, "doctor", "a routing repair failed"},
		{"failed routing rollback", map[string]any{"mode": "routing", "phase": "routing_rollback_failed"}, "doctor", "a routing rollback failed"},
		{"escalation woke", map[string]any{"phase": "escalation_woke"}, "doctor", "an owner agent was woken"},
		{"escalation answered", map[string]any{"phase": "escalation_answered"}, "doctor", "an owner agent answered the escalation"},
		{"resolution applied", map[string]any{"phase": "resolution_applied"}, "doctor", "an owner resolution was applied"},
		{"resolution failed", map[string]any{"phase": "resolution_failed"}, "doctor", "a resolution follow-up failed"},
		{"delegation queued", map[string]any{"phase": "delegation_queued"}, "doctor", "a delegated follow-up was queued"},
		{"delegation woke", map[string]any{"phase": "delegation_woke"}, "doctor", "a delegated agent was woken"},
		{"delegation failed", map[string]any{"phase": "delegation_failed"}, "doctor", "a delegated wake failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cat, title, ok := autonomyMeta(&event.Event{Kind: event.KindInfo, Subject: "doctor.auto_repair", Payload: mustJSON(c.payload)})
			if !ok || cat != c.wantCat || title != c.wantTitle {
				t.Fatalf("autonomyMeta = (%q, %q, %v), want (%q, %q, true)", cat, title, ok, c.wantCat, c.wantTitle)
			}
		})
	}
}

func TestAutonomyMeta_OperatorActions(t *testing.T) {
	cases := []struct {
		name      string
		subject   string
		payload   map[string]any
		wantCat   string
		wantTitle string
	}{
		{"repair requested", "agent.repair", map[string]any{"phase": "requested"}, "doctor", "an operator repair was requested"},
		{"repair completed", "agent.repair", map[string]any{"phase": "completed"}, "doctor", "an operator repair completed"},
		{"repair failed", "agent.repair", map[string]any{"phase": "failed"}, "doctor", "an operator repair failed"},
		{"wake requested", "agent.wake", map[string]any{"phase": "requested"}, "doctor", "an operator wake was requested"},
		{"wake completed", "agent.wake", map[string]any{"phase": "completed"}, "doctor", "an operator wake completed"},
		{"wake failed", "agent.wake", map[string]any{"phase": "failed"}, "doctor", "an operator wake failed"},
		{"resolve requested", "agent.resolve", map[string]any{"phase": "requested"}, "doctor", "an operator resolution was requested"},
		{"resolve completed", "agent.resolve", map[string]any{"phase": "completed"}, "doctor", "an operator resolution completed"},
		{"resolve failed", "agent.resolve", map[string]any{"phase": "failed"}, "doctor", "an operator resolution failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cat, title, ok := autonomyMeta(&event.Event{Kind: event.KindInfo, Subject: c.subject, Payload: mustJSON(c.payload)})
			if !ok || cat != c.wantCat || title != c.wantTitle {
				t.Fatalf("autonomyMeta = (%q, %q, %v), want (%q, %q, true)", cat, title, ok, c.wantCat, c.wantTitle)
			}
		})
	}
}

func TestAutonomyMeta_SubAgentSpawnedNamedOnly(t *testing.T) {
	// A NAMED delegation (ran AS a roster agent) is identity-level autonomy.
	cat, title, ok := autonomyMeta(ev(event.KindSubAgentSpawned, map[string]any{"agent": "worker", "task": "x"}))
	if !ok || cat != "delegation" || title != "a sub-agent was delegated" {
		t.Fatalf("named delegation meta = (%q, %q, %v), want delegation included", cat, title, ok)
	}
	// An anonymous fan-out spawn is firehose noise — excluded from the feed.
	if _, _, ok := autonomyMeta(ev(event.KindSubAgentSpawned, map[string]any{"task": "x"})); ok {
		t.Fatal("anonymous sub-agent spawn should be excluded from the autonomy feed")
	}
}

func TestAutonomyKinds_ExcludesReactiveNoise(t *testing.T) {
	// Reactive plumbing must NOT be in the curated feed.
	for _, k := range []event.Kind{event.KindLLMRequest, event.KindToolInvoked, event.KindTaskReceived} {
		if _, ok := autonomyKinds[k]; ok {
			t.Errorf("%q should be excluded from the autonomy feed", k)
		}
	}
	// Self-directed milestones must be present.
	for _, k := range []event.Kind{event.KindScheduleFired, event.KindSkillCreated, event.KindAssureVerdict} {
		if _, ok := autonomyKinds[k]; !ok {
			t.Errorf("%q should be in the autonomy feed", k)
		}
	}
}

func TestClipDetail(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	if got := clipDetail(string(long)); len([]rune(got)) != 120 {
		t.Errorf("clipDetail length = %d, want 120", len([]rune(got)))
	}
	if clipDetail("short") != "short" {
		t.Error("short strings should pass through unchanged")
	}
}

func mustJSON(v map[string]any) []byte {
	b, _ := json.Marshal(v)
	return b
}
