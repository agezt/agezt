// SPDX-License-Identifier: MIT

package reaper

// The reaper's own tests. They run the MODULE over the host's real ports —
// the same ports the daemon wires — so a port that lies (a journal that is
// not the kernel's, a roster that is not the fleet) fails here rather than in
// production.
//
// The host is the runtime kernel; the test reaches it only to build the ports
// (kernel.ReaperPorts) and to publish the events the scan reads.

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openScanner spins a real kernel over a temp journal and returns it with the
// module built over the host's ports.
//
// The schedule library is the daemon's file (W3.49), so the test opens it and
// hands the kernel the contract's host port.
func openScanner(t *testing.T) (*runtime.Kernel, *Scanner) {
	t.Helper()
	dir := t.TempDir()
	sched, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	// The standing library is the daemon's file too (W3.50): the test opens
	// it and hands the kernel the contract's store port.
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:   dir,
		Provider:  mock.New(mock.FinalText("ok")),
		Tools:     map[string]toolapi.Tool{},
		Schedules: sched,
		Standing:  standingStore,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k, New(k.ReaperPorts())
}

// A future cutoff makes every existing agent "old enough to judge", so the scan
// flags exactly the enabled, non-retired one with no activity — and a past cutoff
// (everything within the grace window) flags nothing.
func TestReaperScan_FlagsIdleFiltersRetiredPausedAndNew(t *testing.T) {
	k, scan := openScanner(t)
	for _, p := range []roster.Profile{
		{Slug: "live"},
		{Slug: "retired"},
		{Slug: "paused"},
		// A shipped guardian is long-lived by design — it is never reaped.
		{Slug: "system", System: true},
	} {
		if _, err := k.Roster().Add(p); err != nil {
			t.Fatalf("add %s: %v", p.Slug, err)
		}
	}
	if _, err := k.Roster().SetRetired("retired", true); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := k.Roster().SetEnabled("paused", false); err != nil {
		t.Fatalf("pause: %v", err)
	}

	future := time.Now().Add(time.Hour).UnixMilli()
	rep := scan.Scan(future, future)
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "live" {
		t.Fatalf("dead agents = %+v, want exactly [live]", rep.DeadAgents)
	}
	if rep.Empty() {
		t.Errorf("report with a dead agent should not be Empty()")
	}

	// Past cutoff: every agent is within the grace window → nothing flagged.
	past := time.Now().Add(-time.Hour).UnixMilli()
	if rep2 := scan.Scan(past, past); len(rep2.DeadAgents) != 0 || !rep2.Empty() {
		t.Errorf("past cutoff should flag nothing, got %+v", rep2.DeadAgents)
	}
}

func TestReaperScan_FlagsDegradedAgentsFromHealthPolicy(t *testing.T) {
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
	directFalse := false
	for _, p := range []roster.Profile{
		{Slug: "lead"},
		// A managed sub-agent with a broken override AND a broken binding:
		// both findings belong to the same agent's row.
		{
			Slug:            "worker",
			ParentAgent:     "lead",
			DirectCallable:  &directFalse,
			ConfigOverrides: map[string]string{"AGEZT_MAX_ITER": "abc"},
		},
		{Slug: "paused"},
		{Slug: "retired"},
	} {
		if _, err := k.Roster().Add(p); err != nil {
			t.Fatalf("add %s: %v", p.Slug, err)
		}
	}
	if _, err := k.Roster().SetEnabled("paused", false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := k.Roster().SetRetired("retired", true); err != nil {
		t.Fatalf("retire: %v", err)
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
	schedRetired, err := k.Schedules().Add("retired job", time.Hour, "", "test", time.Now())
	if err != nil {
		t.Fatalf("add retired schedule: %v", err)
	}
	if _, err := k.Schedules().SetAgent(schedRetired.ID, "retired"); err != nil {
		t.Fatalf("bind retired schedule: %v", err)
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
	standingRetired, err := k.Standing().Add(standing.Order{
		Name:     "retired order",
		Agent:    "retired",
		Plan:     "do it",
		Triggers: []standing.Trigger{{Type: standing.TriggerCron, Schedule: "0 9 * * *"}},
	})
	if err != nil {
		t.Fatalf("add retired standing: %v", err)
	}
	_ = standingRetired

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
	issues := map[string]string{}
	for _, row := range rep.MisconfiguredAgents {
		issues[row.Slug] = strings.Join(row.Issues, "\n")
	}
	for slug, want := range map[string]string{
		"worker":  "bound schedule cannot call managed sub-agent",
		"ghost":   "bound schedule targets missing agent",
		"paused":  "bound standing order targets paused agent",
		"retired": "bound standing order targets retired agent",
	} {
		if !strings.Contains(issues[slug], want) {
			t.Fatalf("%s issue = %q, want %q; all=%+v", slug, issues[slug], want, rep.MisconfiguredAgents)
		}
	}
	// Both findings belong to the same agent: the broken override and the
	// broken binding are one row, not two.
	if !strings.Contains(issues["worker"], "AGEZT_MAX_ITER") {
		t.Fatalf("the worker's override issue was lost: %q", issues["worker"])
	}
	if !strings.Contains(issues["retired"], "bound schedule targets retired agent") {
		t.Fatalf("the retired agent's schedule issue was lost: %q", issues["retired"])
	}
}

func TestReaperScan_FlagsRoutingPressureAgentsFromModelFallbacks(t *testing.T) {
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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
	k, scan := openScanner(t)
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

	rep := scan.Scan(time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-time.Hour).UnixMilli())
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

// The module reads the fleet through the host's ports: a host that supplies no
// ports at all gets an empty report, not a crash.
func TestReaperScan_WithoutPortsFindsNothing(t *testing.T) {
	rep := New(api.ReaperPorts{}).Scan(time.Now().UnixMilli(), time.Now().UnixMilli())
	if !rep.Empty() {
		t.Fatalf("a host with no ports must yield an empty report: %+v", rep)
	}
}

// TestReaperScan_JudgesTheFleetTheHostGave: the scan judges the fleet the host
// hands over and nothing else. An agent that ran a task after the cutoff is
// alive; one that did not is a candidate, and the report carries when it was
// last seen.
func TestReaperScan_JudgesTheFleetTheHostGave(t *testing.T) {
	ports := api.ReaperPorts{
		Profiles: func() []roster.Profile {
			return []roster.Profile{
				{Slug: "stale", Name: "Stale", Enabled: true, CreatedMS: 1000},
				{Slug: "awake", Name: "Awake", Enabled: true, CreatedMS: 1000},
				{Slug: "dormant", Name: "Dormant", Enabled: true, CreatedMS: 1000},
			}
		},
		Journal: func(fn func(*event.Event) error) error {
			for _, e := range []*event.Event{
				{Kind: event.KindTaskReceived, CorrelationID: "c-stale", TSUnixMS: 2000, Payload: []byte(`{"agent":"stale"}`)},
				{Kind: event.KindTaskReceived, CorrelationID: "c-awake", TSUnixMS: 5000, Payload: []byte(`{"agent":"awake"}`)},
			} {
				if err := fn(e); err != nil {
					return err
				}
			}
			return nil
		},
		StaleArtifacts: func(cutoffMS int64) (int, int64) { return 3, 4096 },
	}
	rep := New(ports).Scan(4000, 4000)
	if len(rep.DeadAgents) != 2 {
		t.Fatalf("dead agents = %+v, want exactly the dormant and the stale one", rep.DeadAgents)
	}
	if rep.DeadAgents[0].Slug != "dormant" || rep.DeadAgents[0].LastActiveMS != 0 {
		t.Fatalf("bad dormant row: %+v", rep.DeadAgents[0])
	}
	if rep.DeadAgents[1].Slug != "stale" || rep.DeadAgents[1].LastActiveMS != 2000 {
		t.Fatalf("the stale agent's last activity was lost: %+v", rep.DeadAgents[1])
	}
	if rep.StaleArtifacts != 3 || rep.StaleBytes != 4096 {
		t.Fatalf("the stale artifact totals were lost: %+v", rep)
	}
}
