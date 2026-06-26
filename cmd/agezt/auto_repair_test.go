// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

func TestAutoRepairShouldHandle(t *testing.T) {
	if !autoRepairShouldHandle(mustEvent(t, autoRepairPulseSubject, map[string]any{"kind": "reaper_candidates"})) {
		t.Fatal("reaper delta event should be handled")
	}
	if autoRepairShouldHandle(mustEvent(t, autoRepairPulseSubject, map[string]any{"error": "observer failed"})) {
		t.Fatal("observer error event should be ignored")
	}
	if autoRepairShouldHandle(mustEvent(t, "pulse.observer.system:disk", map[string]any{"kind": "disk_low"})) {
		t.Fatal("other pulse subjects must be ignored")
	}
}

func TestAutoRepairCoordinatorClaim_EligibleOnly(t *testing.T) {
	managed := false
	coord := newAutoRepairCoordinator(time.Hour)
	rep := kernelruntime.ReaperReport{
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{
			{Slug: "builder", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
			{Slug: "guardian", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
			{Slug: "paused", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
			{Slug: "retired", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
			{Slug: "managed", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
			{Slug: "manual", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
		},
	}
	got := coord.claim(nil, rep, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "guardian", Enabled: true, System: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "paused", Enabled: false, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "retired", Enabled: true, Retired: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "managed", Enabled: true, DirectCallable: &managed, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "manual", Enabled: true},
	})
	if len(got) != 1 || got[0].Slug != "builder" {
		t.Fatalf("claimed = %+v, want only builder", got)
	}
}

func TestAutoRepairCoordinatorClaim_DedupesByFingerprintCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return now }
	profiles := []roster.Profile{{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}}
	report := func(issue string) kernelruntime.ReaperReport {
		return kernelruntime.ReaperReport{
			MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{{Slug: "builder", Issues: []string{issue}}},
		}
	}

	if got := coord.claim(nil, report("AGEZT_MAX_ITER: must be integer"), profiles); len(got) != 1 {
		t.Fatalf("first claim = %+v, want 1 candidate", got)
	}
	coord.release("builder")
	if got := coord.claim(nil, report("AGEZT_MAX_ITER: must be integer"), profiles); len(got) != 0 {
		t.Fatalf("duplicate claim inside cooldown = %+v, want none", got)
	}
	if got := coord.claim(nil, report("AGEZT_MODEL: must be non-empty"), profiles); len(got) != 1 {
		t.Fatalf("changed fingerprint should bypass cooldown, got %+v", got)
	}
	coord.release("builder")
	now = now.Add(2 * time.Hour)
	if got := coord.claim(nil, report("AGEZT_MODEL: must be non-empty"), profiles); len(got) != 1 {
		t.Fatalf("post-cooldown claim = %+v, want 1 candidate", got)
	}
}

func TestAutoRepairCoordinatorClaim_ExhaustsAfterProfileMaxAttempts(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return now }
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	fp := autoRepairFingerprint("misconfigured", "AGEZT_MAX_ITER: must be integer")
	publishAutoRepair(k.Bus(), "corr-old", map[string]any{
		"phase":       "queued",
		"agent":       "builder",
		"mode":        "misconfigured",
		"fingerprint": fp,
		"reason":      "old repair attempt",
	})

	got := coord.claim(k, kernelruntime.ReaperReport{
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{{
			Slug:   "builder",
			Issues: []string{"AGEZT_MAX_ITER: must be integer"},
		}},
	}, []roster.Profile{{
		Slug:             "builder",
		Enabled:          true,
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, MaxAttempts: 1, EscalateTo: "lead"},
	}})
	if len(got) != 1 {
		t.Fatalf("claim = %+v, want one exhausted candidate", got)
	}
	if !got[0].SelfRepairExhausted || got[0].SelfRepairAttempt != 1 || got[0].SelfRepairMaxAttempts != 1 {
		t.Fatalf("candidate did not carry exhausted attempt budget: %+v", got[0])
	}
	if phase := autoRepairQueuedPhase(got[0]); phase != "attempts_exhausted" {
		t.Fatalf("queued phase = %q, want attempts_exhausted", phase)
	}
}

func TestAutoRepairDispatch_ExhaustedAttemptsEscalatesWithoutRepairRun(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	src := &countingAutoRepairSource{}
	box := &fakeAutoRepairMailbox{}
	cand := autoRepairCandidate{
		Slug:                  "builder",
		Mode:                  "misconfigured",
		Fingerprint:           "fp-1",
		Reason:                "invalid override; self-repair attempts exhausted (1/1)",
		SelfRepairAttempt:     1,
		SelfRepairMaxAttempts: 1,
		SelfRepairExhausted:   true,
		EscalateTo:            "lead",
		EscalateFrom:          "system:doctor",
		IncidentID:            "inc-1",
		RootChainID:           "inc-1",
	}

	coord.dispatch(context.Background(), k, k.Bus(), src, box, nil, cand)

	if src.calls != 0 {
		t.Fatalf("RepairAgent calls = %d, want 0 after exhausted attempts", src.calls)
	}
	if len(box.calls) != 1 {
		t.Fatalf("mailbox calls = %+v, want one escalation", box.calls)
	}
	if !strings.Contains(box.calls[0].text, "attempts exhausted") {
		t.Fatalf("escalation text did not explain exhaustion: %q", box.calls[0].text)
	}
}

func TestAutoRepairCoordinatorClaim_DegradedEligible(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		DegradedAgents: []kernelruntime.DegradedAgent{
			{Slug: "builder", Failures: 3, Window: 5, Threshold: 3, LastReason: "provider timeout"},
			{Slug: "guardian", Failures: 2, Window: 3, Threshold: 2, LastReason: "boom"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "guardian", Enabled: true, System: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Slug != "builder" || got[0].Mode != "degraded" {
		t.Fatalf("degraded claim = %+v, want only degraded builder", got)
	}
	if got[0].Issues != nil {
		t.Fatalf("degraded candidate should not carry config issues: %+v", got[0])
	}
}

func TestAutoRepairCoordinatorClaim_RoutingEligible(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "builder", Count: 3, Threshold: 3, WindowSec: 3600, LastFailedModel: "gpt-5", LastNextModel: "gpt-4.1", LastReason: "provider timeout"},
			{Slug: "guardian", Count: 3, Threshold: 3, WindowSec: 3600, LastFailedModel: "gpt-5", LastNextModel: "gpt-4.1", LastReason: "provider timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "guardian", Enabled: true, System: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Slug != "builder" || got[0].Mode != "routing" {
		t.Fatalf("routing claim = %+v, want only routing builder", got)
	}
}

func TestAutoRepairCoordinatorClaim_RetryPressureEligible(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RetryPressure: []kernelruntime.RetryPressureAgent{
			{Slug: "builder", Count: 3, Threshold: 3, WindowSec: 3600, LastReason: "timeout", NextAttempt: 2, MaxAttempts: 3},
			{Slug: "guardian", Count: 3, Threshold: 3, WindowSec: 3600, LastReason: "timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
		{Slug: "guardian", Enabled: true, System: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Slug != "builder" || got[0].Mode != "retry_pressure" {
		t.Fatalf("retry-pressure claim = %+v, want only retry-pressure builder", got)
	}
	if !strings.Contains(got[0].Reason, "3 whole-run retry decision") || !strings.Contains(got[0].Reason, "attempt 2/3") {
		t.Fatalf("retry-pressure reason = %q", got[0].Reason)
	}
}

func TestAutoRepairCoordinatorClaim_MisconfiguredBeatsDegradedSameAgent(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{
			{Slug: "builder", Issues: []string{"AGEZT_MAX_ITER: must be integer"}},
		},
		DegradedAgents: []kernelruntime.DegradedAgent{
			{Slug: "builder", Failures: 3, Window: 5, Threshold: 3, LastReason: "provider timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Mode != "misconfigured" {
		t.Fatalf("combined claim = %+v, want one misconfigured candidate", got)
	}
}

func TestAutoRepairCoordinatorClaim_RoutingBeatsDegradedSameAgent(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "builder", Count: 3, Threshold: 3, WindowSec: 3600, LastFailedModel: "gpt-5", LastNextModel: "gpt-4.1", LastReason: "provider timeout"},
		},
		DegradedAgents: []kernelruntime.DegradedAgent{
			{Slug: "builder", Failures: 3, Window: 5, Threshold: 3, LastReason: "provider timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Mode != "routing" {
		t.Fatalf("combined claim = %+v, want one routing candidate", got)
	}
}

func TestAutoRepairCoordinatorClaim_RoutingRecurrenceQueuesRollback(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	k := newAutoRepairKernel(t, newRoutingProvider(map[string][]string{
		"code": {"gpt-4.1", "deepseek-chat"},
	}))
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return now }
	publishAutoRepair(k.Bus(), "", map[string]any{
		"phase":                             "completed",
		"agent":                             "builder",
		"mode":                              "routing",
		"fingerprint":                       "routing-fp-1",
		"routing_task_type":                 "code",
		"routing_task_model_chain":          []string{"gpt-4.1", "deepseek-chat"},
		"previous_routing_task_model_chain": []string{"gpt-5", "gpt-4.1"},
	})
	got := coord.claim(k, kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "builder", Count: 3, Threshold: 3, WindowSec: 3600, TaskType: "code", LastFailedModel: "gpt-4.1", LastNextModel: "deepseek-chat", LastReason: "provider timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, TaskType: "code", SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 {
		t.Fatalf("claim = %+v, want one candidate", got)
	}
	if got[0].RoutingRollbackTaskType != "code" {
		t.Fatalf("rollback task type = %q, want code", got[0].RoutingRollbackTaskType)
	}
	if strings.Join(got[0].RoutingRollbackFromChain, ",") != "gpt-4.1,deepseek-chat" {
		t.Fatalf("rollback from chain = %+v", got[0].RoutingRollbackFromChain)
	}
	if strings.Join(got[0].RoutingRollbackToChain, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("rollback to chain = %+v", got[0].RoutingRollbackToChain)
	}
	if !strings.Contains(got[0].Reason, "rolling back to gpt-5 -> gpt-4.1") {
		t.Fatalf("rollback reason = %q", got[0].Reason)
	}
}

func TestAutoRepairCoordinatorClaim_RoutingUnstableBeatsRoutingRepair(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RoutingUnstable: []kernelruntime.RoutingUnstableAgent{
			{Slug: "builder", Count: 1, Threshold: 1, WindowSec: 3600, TaskType: "code", CurrentChain: []string{"gpt-5", "gpt-4.1"}, PreviousChain: []string{"gpt-4.1", "deepseek-chat"}, LastReason: "provider timeout"},
		},
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "builder", Count: 3, Threshold: 3, WindowSec: 3600, TaskType: "code", LastFailedModel: "gpt-5", LastNextModel: "gpt-4.1", LastReason: "provider timeout"},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, TaskType: "code", SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Mode != "routing_unstable" {
		t.Fatalf("claim = %+v, want one routing_unstable candidate", got)
	}
}

func TestAutoRepairCoordinatorClaim_RoutingForcedFailedBeatsRoutingRepair(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RoutingForcedFailed: []kernelruntime.RoutingForcedFailedAgent{
			{
				Slug:           "builder",
				Count:          4,
				Threshold:      3,
				WindowSec:      3600,
				TaskType:       "code",
				ForcedChain:    []string{"gpt-5", "gpt-4.1"},
				LastReason:     "provider timeout",
				LastFallbackMS: time.Now().UnixMilli(),
			},
		},
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{
				Slug:            "builder",
				Count:           4,
				Threshold:       3,
				WindowSec:       3600,
				TaskType:        "code",
				LastFailedModel: "gpt-5",
				LastNextModel:   "gpt-4.1",
				LastReason:      "provider timeout",
			},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, TaskType: "code", SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Mode != "routing_forced_failed" {
		t.Fatalf("claim = %+v, want one routing_forced_failed candidate", got)
	}
	if got[0].RoutingRollbackTaskType != "code" || strings.Join(got[0].RoutingRollbackToChain, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("forced failed candidate chain = %+v", got[0])
	}
}

func TestAutoRepairCoordinatorClaim_RoutingForcedExhaustedBeatsForcedFailedAndRoutingRepair(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	got := coord.claim(nil, kernelruntime.ReaperReport{
		RoutingForcedExhausted: []kernelruntime.RoutingForcedExhaustedAgent{
			{
				Slug:            "builder",
				Count:           4,
				Threshold:       3,
				WindowSec:       3600,
				TaskType:        "code",
				ForcedChain:     []string{"gpt-5", "gpt-4.1"},
				ForceGeneration: 2,
				LastReason:      "provider timeout",
			},
		},
		RoutingForcedFailed: []kernelruntime.RoutingForcedFailedAgent{
			{
				Slug:            "builder",
				Count:           4,
				Threshold:       3,
				WindowSec:       3600,
				TaskType:        "code",
				ForcedChain:     []string{"gpt-5", "gpt-4.1"},
				ForceGeneration: 1,
				LastReason:      "provider timeout",
			},
		},
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{
				Slug:            "builder",
				Count:           4,
				Threshold:       3,
				WindowSec:       3600,
				TaskType:        "code",
				LastFailedModel: "gpt-5",
				LastNextModel:   "gpt-4.1",
				LastReason:      "provider timeout",
			},
		},
	}, []roster.Profile{
		{Slug: "builder", Enabled: true, TaskType: "code", SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}},
	})
	if len(got) != 1 || got[0].Mode != "routing_forced_exhausted" {
		t.Fatalf("claim = %+v, want one routing_forced_exhausted candidate", got)
	}
}

func TestAutoRepairEscalationTarget_PrefersParentThenOwner(t *testing.T) {
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "worker", ParentAgent: "lead", OwnerAgent: "boss"}); got != "lead" {
		t.Fatalf("target = %q, want lead", got)
	}
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "worker", OwnerAgent: "boss"}); got != "boss" {
		t.Fatalf("target = %q, want boss", got)
	}
	if got := autoRepairEscalationTarget(roster.Profile{Slug: "worker", ParentAgent: "worker", OwnerAgent: "worker"}); got != "" {
		t.Fatalf("self-target = %q, want blank", got)
	}
}

func TestAutoRepairDispatch_FailedRepairPostsHelpToManager(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	coord.inflight["builder"] = struct{}{}
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	src := fakeAutoRepairSource{err: errors.New("provider timeout")}
	box := &fakeAutoRepairMailbox{}
	var notified []board.Message

	coord.dispatch(context.Background(), nil, nil, src, box, func(m board.Message, _ string) { notified = append(notified, m) }, autoRepairCandidate{
		Slug:         "builder",
		Mode:         "degraded",
		Fingerprint:  "fp-1",
		Reason:       "deterministic auto-repair: degraded by failures",
		EscalateTo:   "lead",
		EscalateFrom: "guardian-doctor",
	})

	if len(box.calls) != 1 {
		t.Fatalf("mailbox calls = %d, want 1", len(box.calls))
	}
	call := box.calls[0]
	if call.from != "guardian-doctor" || call.to != "lead" {
		t.Fatalf("mailbox call = %+v, want guardian-doctor -> lead", call)
	}
	if len(notified) != 1 || notified[0].To != "lead" || !notified[0].Help {
		t.Fatalf("notified = %+v, want one help message to lead", notified)
	}
	if _, busy := coord.inflight["builder"]; busy {
		t.Fatal("dispatch should release inflight marker")
	}
}

func TestAutoRepairDispatch_RoutingRollbackPublishesRollbackCompletion(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	coord.inflight["builder"] = struct{}{}
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	k := newAutoRepairKernel(t, newRoutingProvider(map[string][]string{
		"code": {"gpt-4.1", "deepseek-chat"},
	}))
	sub, err := k.Bus().Subscribe(autoRepairEventSubject, 16)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()
	src := fakeAutoRepairSource{
		rollback: overseertool.RepairResult{
			Agent:                         "builder",
			Applied:                       []string{"task_model_chain"},
			RoutingTaskType:               "code",
			RoutingTaskModelChain:         []string{"gpt-5", "gpt-4.1"},
			PreviousRoutingTaskModelChain: []string{"gpt-4.1", "deepseek-chat"},
			Answer:                        "rolled back code chain",
		},
	}
	coord.dispatch(context.Background(), k, k.Bus(), src, nil, nil, autoRepairCandidate{
		Slug:                     "builder",
		Mode:                     "routing",
		Fingerprint:              "routing-fp-1",
		Reason:                   "recurring routing pressure",
		RoutingRollbackTaskType:  "code",
		RoutingRollbackFromChain: []string{"gpt-4.1", "deepseek-chat"},
		RoutingRollbackToChain:   []string{"gpt-5", "gpt-4.1"},
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev := <-sub.C:
			var payload map[string]any
			if json.Unmarshal(ev.Payload, &payload) != nil {
				continue
			}
			if payload["phase"] != "routing_rollback_completed" {
				continue
			}
			if payload["routing_task_type"] != "code" {
				t.Fatalf("payload task type = %v", payload["routing_task_type"])
			}
			prev, _ := payload["previous_routing_task_model_chain"].([]any)
			if len(prev) != 2 {
				t.Fatalf("payload previous chain = %+v", payload["previous_routing_task_model_chain"])
			}
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Fatal("did not observe routing_rollback_completed")
}

func TestAutoRepairWakeSkipReason(t *testing.T) {
	managed := false
	cases := []struct {
		name string
		p    roster.Profile
		want string
	}{
		{name: "paused", p: roster.Profile{Slug: "lead"}, want: "target agent lead is paused"},
		{name: "retired", p: roster.Profile{Slug: "lead", Enabled: true, Retired: true}, want: "target agent lead is retired"},
		{name: "managed", p: roster.Profile{Slug: "lead", Enabled: true, DirectCallable: &managed}, want: "target agent lead is a managed sub-agent"},
		{name: "eligible", p: roster.Profile{Slug: "lead", Enabled: true}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoRepairWakeSkipReason(tc.p); got != tc.want {
				t.Fatalf("skip reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAutoRepairWakeIntent_IncludesMailboxMessageID(t *testing.T) {
	intent := autoRepairWakeIntent(autoRepairCandidate{
		Slug:        "builder",
		Mode:        "misconfigured",
		Reason:      "invalid overrides",
		Fingerprint: "fp-1",
	}, &board.Message{ID: "msg-1", To: "lead"})
	for _, want := range []string{"builder", "misconfigured", "msg-1", "lead"} {
		if !strings.Contains(intent, want) {
			t.Fatalf("intent %q missing %q", intent, want)
		}
	}
}

func TestAutoRepairWakeIntent_ForcedChainFailureCarriesRoutingPlaybook(t *testing.T) {
	intent := autoRepairWakeIntent(autoRepairCandidate{
		Slug:                    "builder",
		Mode:                    "routing_forced_failed",
		Reason:                  "owner-forced chain stayed under pressure",
		Fingerprint:             "fp-routing-force",
		RoutingRollbackTaskType: "code",
		RoutingRollbackToChain:  []string{"gpt-5", "gpt-4.1"},
	}, &board.Message{ID: "msg-1", To: "lead"})
	for _, want := range []string{
		"owner-forced chain already served its probation window",
		"Forced task type: code.",
		"Forced chain: gpt-5 → gpt-4.1.",
		"Choose deliberately between pause, retire, delegate, or a new force_chain decision.",
	} {
		if !strings.Contains(intent, want) {
			t.Fatalf("intent %q missing %q", intent, want)
		}
	}
}

func TestAutoRepairWakeIntent_ForcedChainExhaustionCarriesEscalationPlaybook(t *testing.T) {
	intent := autoRepairWakeIntent(autoRepairCandidate{
		Slug:                    "builder",
		Mode:                    "routing_forced_exhausted",
		Reason:                  "owner-forced chain exhausted after repeated generations",
		Fingerprint:             "fp-routing-exhausted",
		RoutingRollbackTaskType: "code",
		RoutingRollbackToChain:  []string{"gpt-5", "gpt-4.1"},
	}, &board.Message{ID: "msg-1", To: "lead"})
	for _, want := range []string{
		"owner-forced chain has already been retried across multiple forced generations",
		"Forced task type: code.",
		"Forced chain: gpt-5 → gpt-4.1.",
		"Treat this as an exhausted routing policy.",
	} {
		if !strings.Contains(intent, want) {
			t.Fatalf("intent %q missing %q", intent, want)
		}
	}
}

func TestAutoRepairDelegationText_ForcedChainFailureCarriesForcedChainContext(t *testing.T) {
	text := autoRepairDelegationText(
		autoRepairCandidate{
			Slug:                    "builder",
			Mode:                    "routing_forced_failed",
			Reason:                  "owner-forced chain stayed under routing pressure after probation",
			RoutingRollbackTaskType: "code",
			RoutingRollbackToChain:  []string{"gpt-5", "gpt-4.1"},
		},
		autoRepairWakeResult{
			Target: "lead",
			Resolution: &autoRepairResolution{
				Resolution: "delegated",
				Summary:    "infra owner should take this chain",
				DelegateTo: "infra-lead",
			},
		},
	)
	for _, want := range []string{
		"forced-chain-failed follow-up after owner probation expired.",
		"Forced task type: code.",
		"Forced chain: gpt-5 → gpt-4.1.",
		"Owner note: infra owner should take this chain",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("delegation text %q missing %q", text, want)
		}
	}
}

func TestAutoReplyEscalation_PostsThreadedReply(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	box := &fakeAutoRepairMailbox{
		msgs: map[string]board.Message{
			"m1": {ID: "m1", Topic: "help", From: "guardian-doctor", To: "lead", Text: "need help", Help: true},
		},
	}
	var notified []board.Message
	reply, err := coord.autoReplyEscalation(box, func(m board.Message, _ string) { notified = append(notified, m) }, autoRepairWakeResult{
		Target:      "lead",
		Correlation: "corr-1",
		Answer:      "I inspected the builder and applied a fix.",
		Resolution:  &autoRepairResolution{Resolution: "handled", Summary: "I inspected the builder and applied a fix."},
	}, &board.Message{ID: "m1"})
	if err != nil {
		t.Fatalf("autoReplyEscalation: %v", err)
	}
	if reply == nil || reply.ReplyTo != "m1" || reply.From != "lead" || reply.To != "guardian-doctor" {
		t.Fatalf("reply = %+v", reply)
	}
	if !strings.Contains(reply.Text, "Resolution: handled.") || !strings.Contains(reply.Text, "applied a fix") {
		t.Fatalf("reply text = %q", reply.Text)
	}
	if len(notified) != 1 || notified[0].ReplyTo != "m1" {
		t.Fatalf("notified = %+v", notified)
	}
}

func TestParseAutoRepairResolution_UsesLastJsonBlock(t *testing.T) {
	got := parseAutoRepairResolution("text\n```json\n{\"resolution\":\"delegated\",\"summary\":\"needs infra owner\",\"delegate_to\":\"infra-lead\"}\n```")
	if got == nil {
		t.Fatal("parseAutoRepairResolution returned nil")
	}
	if got.Resolution != "delegated" || got.Summary != "needs infra owner" || got.DelegateTo != "infra-lead" {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestParseAutoRepairResolution_ForceChainRequiresTaskTypeAndChain(t *testing.T) {
	got := parseAutoRepairResolution("```json\n{\"resolution\":\"force_chain\",\"summary\":\"lock it\",\"task_type\":\"code\",\"task_model_chain\":[\"gpt-5\",\"gpt-4.1\"]}\n```")
	if got == nil {
		t.Fatal("parseAutoRepairResolution returned nil")
	}
	if got.Resolution != "force_chain" || got.TaskType != "code" || strings.Join(got.TaskModelChain, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("resolution = %+v", got)
	}
	if bad := parseAutoRepairResolution("```json\n{\"resolution\":\"force_chain\",\"summary\":\"lock it\",\"task_type\":\"code\"}\n```"); bad != nil {
		t.Fatalf("force_chain without chain should be rejected: %+v", bad)
	}
}

func TestParseAutoRepairResolution_RejectsUnknownResolution(t *testing.T) {
	if got := parseAutoRepairResolution("```json\n{\"resolution\":\"whatever\",\"summary\":\"x\"}\n```"); got != nil {
		t.Fatalf("unexpected resolution = %+v", got)
	}
}

func TestApplyAutoRepairResolution_PausesAgent(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	coord := newAutoRepairCoordinator(time.Hour)
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, nil, nil, nil, autoRepairCandidate{Slug: "builder"}, autoRepairWakeResult{
		Target:     "lead",
		Resolution: &autoRepairResolution{Resolution: "paused", Summary: "stop it for now"},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome == nil || outcome.Phase != "resolution_applied" {
		t.Fatalf("pause outcome = %+v", outcome)
	}
	got, ok := k.Roster().Get("builder")
	if !ok || got.Enabled {
		t.Fatalf("profile after pause = %+v", got)
	}
}

func TestApplyAutoRepairResolution_RetiresAgent(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	coord := newAutoRepairCoordinator(time.Hour)
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, nil, nil, nil, autoRepairCandidate{Slug: "builder"}, autoRepairWakeResult{
		Target:     "lead",
		Resolution: &autoRepairResolution{Resolution: "retired", Summary: "mission finished"},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome == nil || outcome.Phase != "resolution_applied" {
		t.Fatalf("retire outcome = %+v", outcome)
	}
	got, ok := k.Roster().Get("builder")
	if !ok || !got.Retired || got.RetiredReason != "mission finished" {
		t.Fatalf("profile after retire = %+v", got)
	}
}

func TestApplyAutoRepairResolution_DelegatesViaMailbox(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}); err != nil {
		t.Fatalf("add builder: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "infra-lead", Enabled: true}); err != nil {
		t.Fatalf("add infra-lead: %v", err)
	}
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	box := &fakeAutoRepairMailbox{}
	var notified []board.Message
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, nil, box, func(m board.Message, _ string) { notified = append(notified, m) }, autoRepairCandidate{
		Slug:   "builder",
		Mode:   "degraded",
		Reason: "repeated provider failure",
	}, autoRepairWakeResult{
		Target:      "lead",
		Correlation: "corr-owner-1",
		Resolution:  &autoRepairResolution{Resolution: "delegated", Summary: "needs infra owner review", DelegateTo: "infra-lead"},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome != nil {
		t.Fatalf("delegation should not return direct outcome: %+v", outcome)
	}
	if len(box.calls) != 1 {
		t.Fatalf("mailbox calls = %d, want 1", len(box.calls))
	}
	call := box.calls[0]
	if call.from != "lead" || call.to != "infra-lead" {
		t.Fatalf("delegation mailbox call = %+v", call)
	}
	if !strings.Contains(call.text, "needs infra owner review") || !strings.Contains(call.text, "builder") {
		t.Fatalf("delegation text = %q", call.text)
	}
	if len(notified) != 1 || notified[0].To != "infra-lead" || !notified[0].Help {
		t.Fatalf("notified = %+v", notified)
	}
	tail, err := k.Journal().Tail(20)
	if err != nil {
		t.Fatalf("journal tail: %v", err)
	}
	phases := make([]string, 0, len(tail))
	for _, ev := range tail {
		if ev.Subject != autoRepairEventSubject {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err == nil {
			if phase, _ := payload["phase"].(string); phase != "" {
				phases = append(phases, phase)
			}
		}
	}
	if !containsString(phases, "delegation_queued") || !containsString(phases, "delegation_woke") {
		t.Fatalf("delegation phases = %+v", phases)
	}
}

func TestApplyAutoRepairResolution_ForcesRoutingChain(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	src := &fakeAutoRepairSource{
		applyChain: overseertool.RepairResult{
			Agent:                         "builder",
			Applied:                       []string{"task_model_chain"},
			RoutingTaskType:               "code",
			RoutingTaskModelChain:         []string{"gpt-5", "gpt-4.1"},
			PreviousRoutingTaskModelChain: []string{"gpt-4.1", "deepseek-chat"},
		},
	}
	outcome, err := coord.applyAutoRepairResolution(context.Background(), nil, src, nil, nil, autoRepairCandidate{Slug: "builder"}, autoRepairWakeResult{
		Target: "lead",
		Resolution: &autoRepairResolution{
			Resolution:     "force_chain",
			Summary:        "lock code onto stable chain",
			TaskType:       "code",
			TaskModelChain: []string{"gpt-5", "gpt-4.1"},
		},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome == nil || outcome.Phase != "resolution_applied" || outcome.RoutingTaskType != "code" || strings.Join(outcome.RoutingTaskModelChain, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("force-chain outcome = %+v", outcome)
	}
	if src.lastApplyRef != "builder" || src.lastApplyTaskType != "code" || strings.Join(src.lastApplyChain, ",") != "gpt-5,gpt-4.1" {
		t.Fatalf("forced chain call = ref=%q task=%q chain=%v", src.lastApplyRef, src.lastApplyTaskType, src.lastApplyChain)
	}
}

func TestApplyAutoRepairResolution_ForceChainIncrementsGeneration(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	publishAutoRepair(k.Bus(), "corr-old", map[string]any{
		"phase":                    "resolution_applied",
		"agent":                    "builder",
		"resolution":               "force_chain",
		"routing_task_type":        "code",
		"routing_task_model_chain": []string{"gpt-4.1", "deepseek-chat"},
		"routing_force_generation": 1,
	})
	coord := newAutoRepairCoordinator(time.Hour)
	src := &fakeAutoRepairSource{
		applyChain: overseertool.RepairResult{
			Agent:                         "builder",
			Applied:                       []string{"task_model_chain"},
			RoutingTaskType:               "code",
			RoutingTaskModelChain:         []string{"gpt-5", "gpt-4.1"},
			PreviousRoutingTaskModelChain: []string{"gpt-4.1", "deepseek-chat"},
		},
	}
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, src, nil, nil, autoRepairCandidate{Slug: "builder"}, autoRepairWakeResult{
		Target: "lead",
		Resolution: &autoRepairResolution{
			Resolution:     "force_chain",
			Summary:        "lock code onto stable chain",
			TaskType:       "code",
			TaskModelChain: []string{"gpt-5", "gpt-4.1"},
		},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome == nil || outcome.RoutingForceGeneration != 2 || outcome.PreviousRoutingForceGeneration != 1 {
		t.Fatalf("force-chain generations = %+v", outcome)
	}
}

func TestApplyAutoRepairResolution_ExhaustedRoutingRejectsHandledAndBlocked(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	cand := autoRepairCandidate{
		Slug:                    "builder",
		Mode:                    "routing_forced_exhausted",
		RoutingRollbackTaskType: "code",
		RoutingRollbackToChain:  []string{"gpt-5", "gpt-4.1"},
	}
	for _, resolution := range []string{"handled", "blocked"} {
		t.Run(resolution, func(t *testing.T) {
			outcome, err := coord.applyAutoRepairResolution(context.Background(), nil, nil, nil, nil, cand, autoRepairWakeResult{
				Target:     "lead",
				Resolution: &autoRepairResolution{Resolution: resolution, Summary: "ignore it"},
			})
			if err == nil || !strings.Contains(err.Error(), "not allowed for exhausted routing policy") {
				t.Fatalf("err = %v, want exhausted policy rejection", err)
			}
			if outcome != nil {
				t.Fatalf("outcome = %+v, want nil", outcome)
			}
		})
	}
}

func TestApplyAutoRepairResolution_ExhaustedRoutingRejectsSameForcedChain(t *testing.T) {
	coord := newAutoRepairCoordinator(time.Hour)
	src := &fakeAutoRepairSource{}
	outcome, err := coord.applyAutoRepairResolution(context.Background(), nil, src, nil, nil, autoRepairCandidate{
		Slug:                    "builder",
		Mode:                    "routing_forced_exhausted",
		RoutingRollbackTaskType: "code",
		RoutingRollbackToChain:  []string{"gpt-5", "gpt-4.1"},
	}, autoRepairWakeResult{
		Target: "lead",
		Resolution: &autoRepairResolution{
			Resolution:     "force_chain",
			Summary:        "force it again",
			TaskType:       "code",
			TaskModelChain: []string{"gpt-5", "gpt-4.1"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "must choose a new chain for exhausted routing policy") {
		t.Fatalf("err = %v, want same-chain rejection", err)
	}
	if outcome != nil {
		t.Fatalf("outcome = %+v, want nil", outcome)
	}
	if src.lastApplyRef != "" {
		t.Fatalf("apply should not run, got ref=%q", src.lastApplyRef)
	}
}

func TestApplyAutoRepairResolution_DelegationCarriesIncidentTreeIDs(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}); err != nil {
		t.Fatalf("add builder: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "infra-lead", Enabled: true}); err != nil {
		t.Fatalf("add infra-lead: %v", err)
	}
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	box := &fakeAutoRepairMailbox{}
	rootIncident := autoRepairIncidentID("builder", "fp-1", coord.now())
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, nil, box, nil, autoRepairCandidate{
		Slug:        "builder",
		Mode:        "degraded",
		Reason:      "repeated provider failure",
		Fingerprint: "fp-1",
		RootAgent:   "builder",
		IncidentID:  rootIncident,
		RootChainID: rootIncident,
	}, autoRepairWakeResult{
		Target:      "lead",
		Correlation: "corr-owner-1",
		Resolution:  &autoRepairResolution{Resolution: "delegated", Summary: "needs infra owner review", DelegateTo: "infra-lead"},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome != nil {
		t.Fatalf("delegation should not return direct outcome: %+v", outcome)
	}
	tail, err := k.Journal().Tail(20)
	if err != nil {
		t.Fatalf("journal tail: %v", err)
	}
	var queued map[string]any
	for _, ev := range tail {
		if ev.Subject != autoRepairEventSubject {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err == nil && payload["phase"] == "delegation_queued" {
			queued = payload
			break
		}
	}
	if queued == nil {
		t.Fatal("delegation_queued not found in journal tail")
	}
	if queued["incident_id"] == "" {
		t.Fatalf("delegation_queued missing incident_id: %+v", queued)
	}
	if queued["root_incident_id"] != rootIncident {
		t.Fatalf("root_incident_id = %v, want %q", queued["root_incident_id"], rootIncident)
	}
	if queued["parent_incident_id"] != rootIncident {
		t.Fatalf("parent_incident_id = %v, want %q", queued["parent_incident_id"], rootIncident)
	}
}

func TestApplyAutoRepairResolution_DelegationFailureIsJournaled(t *testing.T) {
	k := newAutoRepairKernel(t, mock.New(mock.FinalText("ok")))
	if _, err := k.AddProfile(roster.Profile{Slug: "builder", Enabled: true, SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true}}); err != nil {
		t.Fatalf("add builder: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "infra-lead"}); err != nil {
		t.Fatalf("add infra-lead: %v", err)
	}
	if _, err := k.SetProfileEnabled("infra-lead", false); err != nil {
		t.Fatalf("pause infra-lead: %v", err)
	}
	coord := newAutoRepairCoordinator(time.Hour)
	coord.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	box := &fakeAutoRepairMailbox{}
	outcome, err := coord.applyAutoRepairResolution(context.Background(), k, nil, box, nil, autoRepairCandidate{
		Slug:   "builder",
		Mode:   "degraded",
		Reason: "repeated provider failure",
	}, autoRepairWakeResult{
		Target:      "lead",
		Correlation: "corr-owner-1",
		Resolution:  &autoRepairResolution{Resolution: "delegated", Summary: "needs infra owner review", DelegateTo: "infra-lead"},
	})
	if err != nil {
		t.Fatalf("applyAutoRepairResolution: %v", err)
	}
	if outcome != nil {
		t.Fatalf("delegation failure should not return direct outcome: %+v", outcome)
	}
	var found bool
	deadline := time.Now().Add(500 * time.Millisecond)
	for !found && time.Now().Before(deadline) {
		tail, err := k.Journal().Tail(20)
		if err != nil {
			t.Fatalf("journal tail: %v", err)
		}
		for _, ev := range tail {
			if ev.Subject != autoRepairEventSubject {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(ev.Payload, &payload); err == nil {
				if payload["phase"] == "delegation_failed" && payload["target_agent"] == "infra-lead" {
					found = true
					break
				}
			}
		}
		if !found {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !found {
		t.Fatalf("delegation_failed not found in journal tail")
	}
}

func newAutoRepairKernel(t *testing.T, prov agent.Provider) *kernelruntime.Kernel {
	t.Helper()
	k, err := kernelruntime.Open(kernelruntime.Config{BaseDir: t.TempDir(), Provider: prov})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	return k
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

type routingProvider struct {
	chains map[string][]string
}

func newRoutingProvider(chains map[string][]string) *routingProvider {
	cp := make(map[string][]string, len(chains))
	for task, models := range chains {
		cp[task] = append([]string(nil), models...)
	}
	return &routingProvider{chains: cp}
}

func (p *routingProvider) Name() string { return "routing-test" }

func (p *routingProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return nil, errors.New("not implemented")
}

func (p *routingProvider) TaskModelChainsView() map[string][]string {
	out := make(map[string][]string, len(p.chains))
	for task, models := range p.chains {
		out[task] = append([]string(nil), models...)
	}
	return out
}

func (p *routingProvider) SetTaskModelChains(chains map[string][]string) {
	p.chains = make(map[string][]string, len(chains))
	for task, models := range chains {
		p.chains[task] = append([]string(nil), models...)
	}
}

type fakeAutoRepairSource struct {
	err               error
	rollback          overseertool.RepairResult
	applyChain        overseertool.RepairResult
	lastApplyRef      string
	lastApplyTaskType string
	lastApplyChain    []string
}

type countingAutoRepairSource struct {
	calls int
}

func (f *countingAutoRepairSource) RepairAgent(ref, reason string) (overseertool.RepairResult, error) {
	f.calls++
	return overseertool.RepairResult{Agent: ref, Answer: reason}, nil
}

func (f fakeAutoRepairSource) RepairAgent(ref, reason string) (overseertool.RepairResult, error) {
	return overseertool.RepairResult{}, f.err
}

func (f fakeAutoRepairSource) RollbackRouting(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error) {
	return f.rollback, f.err
}

func (f *fakeAutoRepairSource) ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error) {
	f.lastApplyRef = ref
	f.lastApplyTaskType = taskType
	f.lastApplyChain = append([]string(nil), targetChain...)
	return f.applyChain, f.err
}

type fakeMailboxCall struct {
	from string
	to   string
	text string
	when int64
}

type fakeAutoRepairMailbox struct {
	calls []fakeMailboxCall
	msgs  map[string]board.Message
}

func (f *fakeAutoRepairMailbox) HelpRequest(from, to, text string, nowMS int64) (board.Message, error) {
	if f.msgs == nil {
		f.msgs = map[string]board.Message{}
	}
	f.calls = append(f.calls, fakeMailboxCall{from: from, to: to, text: text, when: nowMS})
	msg := board.Message{ID: "m1", Topic: "help", From: from, To: to, Text: text, Help: true, TSMS: nowMS}
	f.msgs[msg.ID] = msg
	return msg, nil
}

func (f *fakeAutoRepairMailbox) Get(id string) (board.Message, bool) {
	if f.msgs == nil {
		return board.Message{}, false
	}
	msg, ok := f.msgs[id]
	return msg, ok
}

func (f *fakeAutoRepairMailbox) Send(m board.Message, nowMS int64) (board.Message, error) {
	if f.msgs == nil {
		f.msgs = map[string]board.Message{}
	}
	if strings.TrimSpace(m.ID) == "" {
		m.ID = "reply-" + strings.TrimSpace(m.ReplyTo)
	}
	m.TSMS = nowMS
	f.msgs[m.ID] = m
	return m, nil
}

func mustEvent(t *testing.T, subject string, payload map[string]any) *event.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &event.Event{Subject: subject, Payload: raw}
}
