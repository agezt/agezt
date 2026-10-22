// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
)

// journalOf returns a journal port over a fixed event list.
func journalOf(events ...*event.Event) func(func(*event.Event) error) error {
	return func(visit func(*event.Event) error) error {
		for _, e := range events {
			if err := visit(e); err != nil {
				return err
			}
		}
		return nil
	}
}

// ev builds one event for the collection.
func ev(kind event.Kind, subject, corr string, ts int64, payload map[string]any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{
		Kind: kind, Subject: subject, Actor: "test", CorrelationID: corr,
		TSUnixMS: ts, Payload: raw,
	}
}

// collect runs the collection over one profile and returns the snapshot.
func collect(t *testing.T, profiles []core.Profile, events []*event.Event, rows []RunRow, routingCut int64) StatusSnapshot {
	t.Helper()
	snap := StatusSnapshot{}
	CollectJournalStatus(StatusSources{
		Journal: journalOf(events...),
		Runs:    func() ([]RunRow, error) { return rows, nil },
	}, profiles, routingCut, &snap)
	return snap
}

func TestRunRowStatus(t *testing.T) {
	if got := (RunRow{}).Status(); got != "running" {
		t.Errorf("an unmarked row = %q, want running", got)
	}
	if got := (RunRow{Completed: true}).Status(); got != "completed" {
		t.Errorf("completed = %q", got)
	}
	if got := (RunRow{Failed: true}).Status(); got != "failed" {
		t.Errorf("failed = %q", got)
	}
	if got := (RunRow{Abandoned: true}).Status(); got != "abandoned" {
		t.Errorf("abandoned = %q", got)
	}
	// Precedence: completed > failed > abandoned.
	if got := (RunRow{Completed: true, Failed: true, Abandoned: true}).Status(); got != "completed" {
		t.Errorf("precedence = %q, want completed", got)
	}
}

func TestCollectJournalStatus_LiveRun(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	rows := []RunRow{{
		CorrelationID: "run-1", Intent: "do the thing", Model: "m1", Agent: "runner",
		SpentMicrocents: 12, StartedUnixMS: 100,
	}}
	snap := collect(t, profiles, nil, rows, 0)
	live, ok := snap.Live["runner"]
	if !ok {
		t.Fatalf("no live status: %+v", snap.Live)
	}
	if live.ActiveRuns != 1 {
		t.Errorf("active runs = %d, want 1", live.ActiveRuns)
	}
	if live.ActiveCorrelationID != "run-1" || live.ActiveIntent != "do the thing" {
		t.Errorf("live row = %+v", live)
	}
	if live.ActiveModel != "m1" || live.ActiveSpentMc != 12 || live.ActiveStartedMS != 100 {
		t.Errorf("live row = %+v", live)
	}
	if live.ActivePhase != "starting" {
		t.Errorf("phase = %q, want starting", live.ActivePhase)
	}
}

func TestCollectJournalStatus_LiveRunIgnoresNonRunningRows(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	for _, row := range []RunRow{
		{CorrelationID: "run-1", Agent: "runner", Completed: true},
		{CorrelationID: "run-2", Agent: "runner", Failed: true},
		{CorrelationID: "run-3", Agent: "runner", Abandoned: true},
	} {
		snap := collect(t, profiles, nil, []RunRow{row}, 0)
		if _, ok := snap.Live["runner"]; ok {
			t.Errorf("a %s row must not be a live run: %+v", row.Status(), snap.Live)
		}
	}
}

func TestCollectJournalStatus_LiveRunIgnoresUnknownAgents(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	rows := []RunRow{{CorrelationID: "run-1", Agent: "ghost"}}
	snap := collect(t, profiles, nil, rows, 0)
	if len(snap.Live) != 0 {
		t.Fatalf("an unknown agent must not be a live run: %+v", snap.Live)
	}
	// A run with no agent at all (an ad-hoc chat run) is not an agent's run.
	rows = []RunRow{{CorrelationID: "run-1"}}
	snap = collect(t, profiles, nil, rows, 0)
	if len(snap.Live) != 0 {
		t.Fatalf("an ad-hoc run must not be a live agent run: %+v", snap.Live)
	}
}

func TestCollectJournalStatus_LastActivity(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindTaskCompleted, "task", "run-1", 100, map[string]any{"agent": "runner", "answer": "done"}),
		ev(event.KindTaskReceived, "task", "run-2", 200, map[string]any{"agent": "runner", "intent": "next"}),
		ev(event.KindTaskReceived, "task", "run-3", 200, map[string]any{"agent": "runner", "intent": "newer"}),
	}
	snap := collect(t, profiles, events, nil, 0)
	last, ok := snap.LastActivities["runner"]
	if !ok {
		t.Fatalf("no last activity: %+v", snap.LastActivities)
	}
	if last.TSUnixMS != 200 {
		t.Fatalf("last activity = %+v, want the latest timestamp", last)
	}
	// Two events can share a millisecond; the later one in the walk wins.
	if last.CorrelationID != "run-3" {
		t.Fatalf("correlation = %q, want run-3 (the last event wins)", last.CorrelationID)
	}
	if last.Kind != string(event.KindTaskReceived) {
		t.Errorf("kind = %q, want task.received", last.Kind)
	}
}

func TestCollectJournalStatus_RunbookFromSchedule(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindScheduleFired, "schedule.fired", "run-1", 100, map[string]any{
			"agent":            "runner",
			"autonomy_runbook": map[string]any{"trigger_contract": "operator_schedule_channel"},
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	runbook, ok := snap.Runbooks["runner"]
	if !ok {
		t.Fatalf("no runbook: %+v", snap.Runbooks)
	}
	if runbook["phase"] != "schedule_fired" {
		t.Errorf("phase = %v, want schedule_fired", runbook["phase"])
	}
	if runbook["source"] != "schedule" {
		t.Errorf("source = %v, want schedule", runbook["source"])
	}
	if runbook["correlation_id"] != "run-1" {
		t.Errorf("correlation = %v", runbook["correlation_id"])
	}
	if runbook["ts_unix_ms"].(int64) != 100 {
		t.Errorf("ts = %v", runbook["ts_unix_ms"])
	}
}

func TestCollectJournalStatus_RunbookFromDelegation(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindSubAgentSpawned, "subagent.spawned", "run-parent", 100, map[string]any{
			"agent":             "runner",
			"child_correlation": "run-child",
			"delegated_by":      "lead",
			"autonomy_runbook":  map[string]any{"route_contract": "self_owned"},
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	runbook := snap.Runbooks["runner"]
	if runbook["phase"] != "delegated_wake" {
		t.Errorf("phase = %v, want delegated_wake", runbook["phase"])
	}
	if runbook["source"] != "delegated" || runbook["delegated_by"] != "lead" {
		t.Errorf("runbook = %v", runbook)
	}
	// A delegation names the child's correlation, not the parent's.
	if runbook["correlation_id"] != "run-child" {
		t.Errorf("correlation = %v, want run-child", runbook["correlation_id"])
	}
}

func TestCollectJournalStatus_PolicyDenial(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		// The denial names the run, not the agent — the agent comes from the
		// run's task.received.
		ev(event.KindTaskReceived, "task", "run-1", 100, map[string]any{"agent": "runner"}),
		ev(event.KindPolicyDecision, "policy.decision", "run-1", 110, map[string]any{
			"allow": false, "tool": "shell", "reason": "capability denied", "capability": "exec", "hard_denied": true,
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	denial, ok := snap.PolicyDenials["runner"]
	if !ok {
		t.Fatalf("no policy denial: %+v", snap.PolicyDenials)
	}
	if denial.Count != 1 || denial.LastTool != "shell" || denial.LastReason != "capability denied" {
		t.Errorf("denial = %+v", denial)
	}
	if denial.LastCapability != "exec" || !denial.LastHard || denial.LastTSMS != 110 {
		t.Errorf("denial = %+v", denial)
	}
}

func TestCollectJournalStatus_AllowedPolicyDecisionIsNotADenial(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindTaskReceived, "task", "run-1", 100, map[string]any{"agent": "runner"}),
		ev(event.KindPolicyDecision, "policy.decision", "run-1", 110, map[string]any{"allow": true, "tool": "shell"}),
	}
	snap := collect(t, profiles, events, nil, 0)
	if len(snap.PolicyDenials) != 0 {
		t.Fatalf("an allowed decision is not a denial: %+v", snap.PolicyDenials)
	}
}

func TestCollectJournalStatus_RoutingPressure(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner", Model: "primary", Fallbacks: []string{"backup"}}}
	events := []*event.Event{
		ev(event.KindProviderFallback, "provider.fallback", "run-1", 100, map[string]any{
			"scope": "model-chain", "failed_model": "primary", "next_model": "backup", "reason": "provider timeout",
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	pressure, ok := snap.RoutingCounts["runner"]
	if !ok {
		t.Fatalf("no routing pressure: %+v", snap.RoutingCounts)
	}
	if pressure.Count != 1 || pressure.LastFailed != "primary" || pressure.LastNext != "backup" {
		t.Errorf("pressure = %+v", pressure)
	}
	if pressure.LastReason != "provider timeout" || pressure.LastTSMS != 100 {
		t.Errorf("pressure = %+v", pressure)
	}
}

func TestCollectJournalStatus_RoutingPressureRespectsTheCut(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner", Model: "primary"}}
	events := []*event.Event{
		ev(event.KindProviderFallback, "provider.fallback", "run-1", 100, map[string]any{
			"scope": "model-chain", "failed_model": "primary", "next_model": "backup",
		}),
	}
	// The cut is exclusive: an event older than the cut is not counted.
	snap := collect(t, profiles, events, nil, 200)
	if len(snap.RoutingCounts) != 0 {
		t.Fatalf("an event before the cut must not count: %+v", snap.RoutingCounts)
	}
}

func TestCollectJournalStatus_RoutingPressureIgnoresOtherScopes(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner", Model: "primary"}}
	events := []*event.Event{
		ev(event.KindProviderFallback, "provider.fallback", "run-1", 100, map[string]any{
			"scope": "other", "failed_model": "primary", "next_model": "backup",
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	if len(snap.RoutingCounts) != 0 {
		t.Fatalf("only model-chain fallbacks are routing pressure: %+v", snap.RoutingCounts)
	}
}

func TestCollectJournalStatus_RetryPressure(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindAgentRetry, "agent.retry", "run-1", 100, map[string]any{
			"agent": "runner", "reason": "provider timeout", "next_attempt": 2, "max_attempts": 3,
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	retry, ok := snap.RetryCounts["runner"]
	if !ok {
		t.Fatalf("no retry pressure: %+v", snap.RetryCounts)
	}
	if retry.Count != 1 || retry.LastReason != "provider timeout" {
		t.Errorf("retry = %+v", retry)
	}
	if retry.NextAttempt != 2 || retry.MaxAttempts != 3 || retry.LastTSMS != 100 {
		t.Errorf("retry = %+v", retry)
	}
}

func TestCollectJournalStatus_MailboxWake(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindStandingFired, "standing.fired", "run-1", 100, map[string]any{
			"agent":           "runner",
			"trigger_subject": "board.message",
			"trigger_payload": map[string]any{"id": "msg-1", "from": "lead", "to": "runner"},
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	wakes, ok := snap.MailboxWakes["runner"]
	if !ok {
		t.Fatalf("no mailbox wakes: %+v", snap.MailboxWakes)
	}
	msg, ok := wakes["msg-1"].(map[string]any)
	if !ok {
		t.Fatalf("no message row: %+v", wakes)
	}
	if msg["correlation_id"] != "run-1" {
		t.Errorf("correlation = %v", msg["correlation_id"])
	}
}

func TestCollectJournalStatus_NonMailboxStandingIsNotAMailboxWake(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		ev(event.KindStandingFired, "standing.fired", "run-1", 100, map[string]any{
			"agent":           "runner",
			"trigger_subject": "schedule.tick",
			"trigger_payload": map[string]any{"id": "msg-1"},
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	if len(snap.MailboxWakes) != 0 {
		t.Fatalf("a non-mailbox wake is not a mailbox wake: %+v", snap.MailboxWakes)
	}
}

func TestCollectJournalStatus_RoutingPressureMatchesTheTaskType(t *testing.T) {
	// The profile names a task type, so a fallback in THAT task type is its
	// pressure even when the models are not in its chain.
	profiles := []core.Profile{{Slug: "runner", TaskType: "code"}}
	events := []*event.Event{
		ev(event.KindProviderFallback, "provider.fallback", "run-1", 100, map[string]any{
			"scope": "model-chain", "task_type": "code", "failed_model": "other", "next_model": "another",
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	pressure, ok := snap.RoutingCounts["runner"]
	if !ok {
		t.Fatalf("no routing pressure: %+v", snap.RoutingCounts)
	}
	if pressure.Count != 1 {
		t.Errorf("pressure = %+v", pressure)
	}
}

func TestCollectJournalStatus_LiveWakeContext(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	rows := []RunRow{{CorrelationID: "run-1", Agent: "runner", StartedUnixMS: 100}}
	events := []*event.Event{
		ev(event.KindTaskReceived, "task", "run-1", 110, map[string]any{
			"agent": "runner", "wake_source": "schedule", "wake_reason": "daily", "schedule_id": "sched-1",
		}),
	}
	snap := collect(t, profiles, events, rows, 0)
	live := snap.Live["runner"]
	if live.ActiveWakeSource != "schedule" || live.ActiveWakeReason != "daily" {
		t.Errorf("wake context = %+v", live)
	}
	if live.ActiveScheduleID != "sched-1" {
		t.Errorf("wake context = %+v", live)
	}
}

func TestCollectJournalStatus_NoProfilesMeansNoStatus(t *testing.T) {
	rows := []RunRow{{CorrelationID: "run-1", Agent: "runner"}}
	snap := collect(t, nil, nil, rows, 0)
	if len(snap.Live) != 0 || len(snap.LastActivities) != 0 {
		t.Fatalf("an empty roster has no status: %+v", snap)
	}
}

func TestAgentModelChain(t *testing.T) {
	chain := AgentModelChain("primary", []string{"backup", "", "primary", "  ", "second"})
	want := []string{"primary", "backup", "second"}
	if len(chain) != len(want) {
		t.Fatalf("chain = %v, want %v", chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("chain = %v, want %v", chain, want)
		}
	}
}
