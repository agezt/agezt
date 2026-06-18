// SPDX-License-Identifier: MIT

package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// A future cutoff makes every existing agent "old enough to judge", so the scan
// flags exactly the enabled, non-retired one with no activity — and a past cutoff
// (everything within the grace window) flags nothing.
func TestReaperScan_FlagsIdleFiltersRetiredPausedAndNew(t *testing.T) {
	k := openCausesKernel(t)
	for _, slug := range []string{"live", "retired", "paused"} {
		if _, err := k.Roster().Add(roster.Profile{Slug: slug}); err != nil {
			t.Fatalf("add %s: %v", slug, err)
		}
	}
	if _, err := k.Roster().SetRetired("retired", true); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := k.Roster().SetEnabled("paused", false); err != nil {
		t.Fatalf("pause: %v", err)
	}

	future := time.Now().Add(time.Hour).UnixMilli()
	rep := k.ReaperScan(future, future)
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "live" {
		t.Fatalf("dead agents = %+v, want exactly [live]", rep.DeadAgents)
	}
	if rep.Empty() {
		t.Errorf("report with a dead agent should not be Empty()")
	}

	// Past cutoff: every agent is within the grace window → nothing flagged.
	past := time.Now().Add(-time.Hour).UnixMilli()
	if rep2 := k.ReaperScan(past, past); len(rep2.DeadAgents) != 0 || !rep2.Empty() {
		t.Errorf("past cutoff should flag nothing, got %+v", rep2.DeadAgents)
	}
}

func TestReaperScan_FlagsDegradedAgentsFromHealthPolicy(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "worker",
		HealthPolicy: &roster.HealthPolicy{
			FailureThreshold: 2,
			FailureWindow:    3,
			DoctorAgent:      "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
	}); err != nil {
		t.Fatalf("add worker: %v", err)
	}
	for _, corr := range []string{"c1", "c2"} {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "task",
			Kind:          event.KindTaskReceived,
			Actor:         "agent-" + corr,
			CorrelationID: corr,
			Payload:       map[string]any{"agent": "worker", "intent": "x"},
		})
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "task",
			Kind:          event.KindTaskFailed,
			Actor:         "agent-" + corr,
			CorrelationID: corr,
			Payload:       map[string]any{"reason": "error"},
		})
	}

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.DegradedAgents) != 1 {
		t.Fatalf("degraded agents = %+v, want exactly one", rep.DegradedAgents)
	}
	got := rep.DegradedAgents[0]
	if got.Slug != "worker" || got.Failures != 2 || got.Threshold != 2 || got.DoctorAgent != "guardian-health" {
		t.Fatalf("bad degraded row: %+v", got)
	}
	if !got.SelfRepairEnabled || got.EscalateTo != "lead" {
		t.Fatalf("self-repair metadata not carried: %+v", got)
	}
}

func TestReaperScan_FlagsRetryPressureAgents(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "worker",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
	}); err != nil {
		t.Fatalf("add worker: %v", err)
	}
	if _, err := k.Roster().Add(roster.Profile{
		Slug:             "paused",
		Enabled:          false,
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true},
	}); err != nil {
		t.Fatalf("add paused: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "agent.worker.retry",
			Kind:          event.KindAgentRetry,
			Actor:         "agent-retry",
			CorrelationID: "corr-retry",
			Payload: map[string]any{
				"agent":        "worker",
				"reason":       "timeout",
				"next_attempt": 2,
				"max_attempts": 3,
			},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "agent.paused.retry",
		Kind:    event.KindAgentRetry,
		Actor:   "agent-retry",
		Payload: map[string]any{"agent": "paused", "reason": "timeout"},
	})

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RetryPressure) != 1 {
		t.Fatalf("retry pressure agents = %+v, want exactly one", rep.RetryPressure)
	}
	got := rep.RetryPressure[0]
	if got.Slug != "worker" || got.Count != 3 || got.Threshold != 3 || got.LastReason != "timeout" {
		t.Fatalf("bad retry pressure row: %+v", got)
	}
	if got.DoctorAgent != "guardian-health" || !got.SelfRepairEnabled || got.EscalateTo != "lead" {
		t.Fatalf("repair metadata not carried: %+v", got)
	}
}

func TestReaperScan_FlagsMisconfiguredAgentsFromRuntimeOverrides(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "miswired",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		ConfigOverrides: map[string]string{
			"AGEZT_MAX_ITER":                 "abc",
			"AGEZT_DISABLE_HEURISTIC_BYPASS": "maybe",
		},
	}); err != nil {
		t.Fatalf("add miswired: %v", err)
	}

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.MisconfiguredAgents) != 1 {
		t.Fatalf("misconfigured agents = %+v, want exactly one", rep.MisconfiguredAgents)
	}
	got := rep.MisconfiguredAgents[0]
	if got.Slug != "miswired" || len(got.Issues) != 2 {
		t.Fatalf("bad misconfigured row: %+v", got)
	}
	if got.DoctorAgent != "guardian-health" || !got.SelfRepairEnabled || got.EscalateTo != "lead" {
		t.Fatalf("repair metadata not carried: %+v", got)
	}
}

func TestReaperScan_FlagsMisconfiguredAgentHierarchy(t *testing.T) {
	k := openCausesKernel(t)
	directFalse := false
	for _, p := range []roster.Profile{
		{Slug: "lead"},
		{Slug: "retired-owner"},
		{Slug: "missing-owner-agent", OwnerAgent: "ghost"},
		{Slug: "paused-worker", ParentAgent: "lead", DirectCallable: &directFalse},
		{Slug: "retired-owned", OwnerAgent: "retired-owner"},
	} {
		if _, err := k.Roster().Add(p); err != nil {
			t.Fatalf("add %s: %v", p.Slug, err)
		}
	}
	if _, err := k.Roster().SetEnabled("lead", false); err != nil {
		t.Fatalf("pause lead: %v", err)
	}
	if _, err := k.Roster().SetRetired("retired-owner", true); err != nil {
		t.Fatalf("retire owner: %v", err)
	}

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	issues := map[string]string{}
	for _, row := range rep.MisconfiguredAgents {
		issues[row.Slug] = strings.Join(row.Issues, "\n")
	}
	for slug, want := range map[string]string{
		"missing-owner-agent": "owner_agent: ghost is missing from the roster",
		"paused-worker":       "parent_agent: lead is paused",
		"retired-owned":       "owner_agent: retired-owner is retired",
	} {
		if !strings.Contains(issues[slug], want) {
			t.Fatalf("%s issue = %q, want %q; all=%+v", slug, issues[slug], want, rep.MisconfiguredAgents)
		}
	}
}

func TestReaperScan_FlagsMisconfiguredAutomationBindings(t *testing.T) {
	k := openCausesKernel(t)
	directFalse := false
	for _, p := range []roster.Profile{
		{Slug: "lead"},
		{Slug: "worker", ParentAgent: "lead", DirectCallable: &directFalse},
		{Slug: "paused"},
	} {
		if _, err := k.Roster().Add(p); err != nil {
			t.Fatalf("add %s: %v", p.Slug, err)
		}
	}
	if _, err := k.Roster().SetEnabled("paused", false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	schedWorker, err := k.Schedules().Add("worker job", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("add worker schedule: %v", err)
	}
	if _, err := k.Schedules().SetAgent(schedWorker.ID, "worker"); err != nil {
		t.Fatalf("bind worker schedule: %v", err)
	}
	schedMissing, err := k.Schedules().Add("ghost job", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("add ghost schedule: %v", err)
	}
	if _, err := k.Schedules().SetAgent(schedMissing.ID, "ghost"); err != nil {
		t.Fatalf("bind ghost schedule: %v", err)
	}
	standingPaused, err := k.Standing().Add(standing.Order{
		Name:     "paused order",
		Agent:    "paused",
		Plan:     "do it",
		Triggers: []standing.Trigger{{Type: standing.TriggerCron, Schedule: "0 8 * * *"}},
	})
	if err != nil {
		t.Fatalf("add paused standing: %v", err)
	}
	_ = standingPaused

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	issues := map[string]string{}
	for _, row := range rep.MisconfiguredAgents {
		issues[row.Slug] = strings.Join(row.Issues, "\n")
	}
	for slug, want := range map[string]string{
		"worker": "bound schedule cannot call managed sub-agent",
		"ghost":  "bound schedule targets missing agent",
		"paused": "bound standing order targets paused agent",
	} {
		if !strings.Contains(issues[slug], want) {
			t.Fatalf("%s issue = %q, want %q; all=%+v", slug, issues[slug], want, rep.MisconfiguredAgents)
		}
	}
}

func TestReaperScan_FlagsRoutingPressureAgentsFromModelFallbacks(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "router",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		Model:            "gpt-5",
		TaskType:         "code",
	}); err != nil {
		t.Fatalf("add router: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.fallback",
			Kind:    event.KindProviderFallback,
			Actor:   "governor",
			Payload: map[string]any{
				"failed_model": "gpt-5",
				"next_model":   "gpt-4.1",
				"reason":       "provider timeout",
				"scope":        "model-chain",
				"task_type":    "code",
			},
		})
	}

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RoutingPressure) != 1 {
		t.Fatalf("routing pressure agents = %+v, want exactly one", rep.RoutingPressure)
	}
	got := rep.RoutingPressure[0]
	if got.Slug != "router" || got.Count != 3 || got.LastFailedModel != "gpt-5" || got.LastNextModel != "gpt-4.1" {
		t.Fatalf("bad routing pressure row: %+v", got)
	}
	if got.DoctorAgent != "guardian-health" || !got.SelfRepairEnabled || got.EscalateTo != "lead" {
		t.Fatalf("routing repair metadata not carried: %+v", got)
	}
}

func TestReaperScan_FlagsRoutingUnstableAgentsAfterRollbackAndRenewedPressure(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "router",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		Model:            "gpt-5",
		TaskType:         "code",
	}); err != nil {
		t.Fatalf("add router: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.fallback",
			Kind:    event.KindProviderFallback,
			Actor:   "governor",
			Payload: map[string]any{
				"failed_model": "gpt-5",
				"next_model":   "gpt-4.1",
				"reason":       "provider timeout",
				"scope":        "model-chain",
				"task_type":    "code",
			},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"agent":                             "router",
			"mode":                              "routing",
			"phase":                             "routing_rollback_completed",
			"routing_task_type":                 "code",
			"routing_task_model_chain":          []string{"gpt-5", "gpt-4.1"},
			"previous_routing_task_model_chain": []string{"gpt-4.1", "deepseek-chat"},
		},
	})

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RoutingUnstable) != 1 {
		t.Fatalf("routing unstable agents = %+v, want exactly one", rep.RoutingUnstable)
	}
	got := rep.RoutingUnstable[0]
	if got.Slug != "router" || got.TaskType != "code" || got.Count != 1 {
		t.Fatalf("bad routing unstable row: %+v", got)
	}
	if strings.Join(got.CurrentChain, ",") != "gpt-5,gpt-4.1" || strings.Join(got.PreviousChain, ",") != "gpt-4.1,deepseek-chat" {
		t.Fatalf("unexpected unstable chains: %+v", got)
	}
}

func TestReaperScan_SuppressesRoutingPressureDuringForcedChainProbation(t *testing.T) {
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "router",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		Model:            "gpt-5",
		TaskType:         "code",
	}); err != nil {
		t.Fatalf("add router: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.fallback",
			Kind:    event.KindProviderFallback,
			Actor:   "governor",
			Payload: map[string]any{
				"failed_model": "gpt-5",
				"next_model":   "gpt-4.1",
				"reason":       "provider timeout",
				"scope":        "model-chain",
				"task_type":    "code",
			},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"agent":                    "router",
			"phase":                    "resolution_applied",
			"resolution":               "force_chain",
			"routing_task_type":        "code",
			"routing_task_model_chain": []string{"gpt-5"},
			"routing_force_generation": 1,
		},
	})

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RoutingPressure) != 0 {
		t.Fatalf("routing pressure should be suppressed during forced probation: %+v", rep.RoutingPressure)
	}
	if len(rep.RoutingForced) != 1 {
		t.Fatalf("forced probation agents = %+v, want exactly one", rep.RoutingForced)
	}
	got := rep.RoutingForced[0]
	if got.Slug != "router" || got.TaskType != "code" || strings.Join(got.ForcedChain, ",") != "gpt-5" {
		t.Fatalf("bad forced probation row: %+v", got)
	}
	if got.ForceGeneration != 1 {
		t.Fatalf("forced probation generation = %d, want 1", got.ForceGeneration)
	}
}

func TestReaperScan_FlagsForcedChainFailureAfterProbationExpires(t *testing.T) {
	t.Setenv("AGEZT_ROUTING_FORCE_PROBATION", "1ms")
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "router",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		Model:            "gpt-5",
		TaskType:         "code",
	}); err != nil {
		t.Fatalf("add router: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.fallback",
			Kind:    event.KindProviderFallback,
			Actor:   "governor",
			Payload: map[string]any{
				"failed_model": "gpt-5",
				"next_model":   "gpt-4.1",
				"reason":       "provider timeout",
				"scope":        "model-chain",
				"task_type":    "code",
			},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"agent":                    "router",
			"phase":                    "resolution_applied",
			"resolution":               "force_chain",
			"routing_task_type":        "code",
			"routing_task_model_chain": []string{"gpt-5"},
			"routing_force_generation": 1,
		},
	})
	time.Sleep(3 * time.Millisecond)

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RoutingForced) != 0 {
		t.Fatalf("forced probation should have expired: %+v", rep.RoutingForced)
	}
	if len(rep.RoutingForcedFailed) != 1 {
		t.Fatalf("forced failed agents = %+v, want exactly one", rep.RoutingForcedFailed)
	}
	got := rep.RoutingForcedFailed[0]
	if got.Slug != "router" || got.TaskType != "code" || strings.Join(got.ForcedChain, ",") != "gpt-5" {
		t.Fatalf("bad forced failed row: %+v", got)
	}
	if got.ForceGeneration != 1 {
		t.Fatalf("forced failed generation = %d, want 1", got.ForceGeneration)
	}
}

func TestReaperScan_FlagsForcedChainExhaustionAfterRepeatedForcedGenerations(t *testing.T) {
	t.Setenv("AGEZT_ROUTING_FORCE_PROBATION", "1ms")
	k := openCausesKernel(t)
	if _, err := k.Roster().Add(roster.Profile{
		Slug: "router",
		HealthPolicy: &roster.HealthPolicy{
			DoctorAgent: "guardian-health",
		},
		SelfRepairPolicy: &roster.SelfRepairPolicy{Enabled: true, EscalateTo: "lead"},
		Model:            "gpt-5",
		TaskType:         "code",
	}); err != nil {
		t.Fatalf("add router: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "governor.fallback",
			Kind:    event.KindProviderFallback,
			Actor:   "governor",
			Payload: map[string]any{
				"failed_model": "gpt-5",
				"next_model":   "gpt-4.1",
				"reason":       "provider timeout",
				"scope":        "model-chain",
				"task_type":    "code",
			},
		})
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "doctor.auto_repair",
		Kind:    event.KindInfo,
		Actor:   "kernel",
		Payload: map[string]any{
			"agent":                    "router",
			"phase":                    "resolution_applied",
			"resolution":               "force_chain",
			"routing_task_type":        "code",
			"routing_task_model_chain": []string{"gpt-5"},
			"routing_force_generation": 2,
		},
	})
	time.Sleep(3 * time.Millisecond)

	rep := k.ReaperScan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	if len(rep.RoutingForcedExhausted) != 1 {
		t.Fatalf("forced exhausted agents = %+v, want exactly one", rep.RoutingForcedExhausted)
	}
	if len(rep.RoutingForcedFailed) != 0 {
		t.Fatalf("forced failed agents should be empty when exhausted: %+v", rep.RoutingForcedFailed)
	}
	got := rep.RoutingForcedExhausted[0]
	if got.Slug != "router" || got.ForceGeneration != 2 {
		t.Fatalf("bad forced exhausted row: %+v", got)
	}
}
