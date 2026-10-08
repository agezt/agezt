// SPDX-License-Identifier: MIT
package roster

import (
	core "github.com/agezt/agezt/kernel/roster"
	"reflect"
	"testing"
	"time"
)

func richStatusSnapshot() StatusSnapshot {
	return StatusSnapshot{
		Degraded: map[string]DegradedStatus{"a": {Failures: 2, Threshold: 3, Window: 4, LastFailureMS: 17}}, Misconfigured: map[string]MisconfigurationStatus{"a": {Issues: []string{"owned config"}}},
		ForcedExhausted: map[string]ForcedStatus{"a": {Count: 5, TaskType: "coding", ForcedChain: []string{"forced"}, ForceGeneration: 6}}, ForcedFailed: map[string]ForcedStatus{"a": {Count: 7, TaskType: "coding", ForcedChain: []string{"failed"}, ForceGeneration: 8}},
		Unstable: map[string]UnstableStatus{"a": {Count: 9, TaskType: "research", CurrentChain: []string{"current"}, PreviousChain: []string{"previous"}}}, Forced: map[string]ForcedStatus{"a": {Count: 10, TaskType: "coding", ForcedChain: []string{"probation"}, ForceGeneration: 11}}, Routing: map[string]RoutingStatus{"a": {Count: 12}}, Dead: map[string]DeadStatus{"a": {LastActiveMS: 13}},
		Repairs:       map[string]RepairSummary{"a": {InflightCount: 1, HasLatest: true, Latest: RepairRow{Mode: "routing", Phase: "completed", NextEligibleMS: 14, TSUnixMS: 15, CorrelationID: "repair-corr", SelfRepairAttempt: 2, SelfRepairMaxAttempts: 3, IncidentID: "i", RootIncidentID: "root-i", ParentIncidentID: "parent-i", RootAgent: "root-agent", ChainDepth: 4, Error: "owned error"}}},
		RoutingCounts: map[string]RoutingPressure{"a": {Count: 16, LastReason: "reason", LastFailed: "failed-model", LastNext: "next-model", LastTSMS: 17}}, RetryCounts: map[string]RetryPressure{"a": {Count: 18, LastReason: "retry", LastTSMS: 19, NextAttempt: 2, MaxAttempts: 4}},
		Escalations: map[string]EscalationLoad{"a": {Open: 3, Acked: 2}}, Wakes: map[string]WakeStatus{"a": {ScheduleCount: 4, StandingCount: 5, EventSubjects: []string{"board.a"}, NextScheduledWakeMS: 20, NextScheduledLabel: "next"}},
		Live:           map[string]LiveStatus{"a": {ActiveRuns: 1, ActiveCorrelationID: "live-corr", ActiveIntent: "intent", ActiveStartedMS: 21, ActiveModel: "model", ActiveSpentMc: 9007199254740993, ActivePhase: "tool", ActiveLastEventMS: 22, ActiveLastEventKind: "event", ActiveDetail: "detail", ActiveTool: "owned-tool", ActiveIter: 6, ActiveWakeSource: "schedule", ActiveWakeReason: "wake", ActiveScheduleID: "s", ActiveStandingID: "standing", ActiveStandingName: "name", ActiveTriggerSubject: "subject", ActiveParentCorrelation: "parent"}},
		LastActivities: map[string]LastActivity{"a": {TSUnixMS: 23, Kind: "last-kind", CorrelationID: "last-corr", Summary: "last-summary"}}, Runbooks: map[string]map[string]any{"a": {"plan": "owned"}}, MailboxWakes: map[string]map[string]any{"a": {"mail": "owned"}}, PolicyDenials: map[string]PolicyDenials{"a": {Count: 7, LastTool: "denied-tool", LastReason: "denied", LastCapability: "network", LastHard: true, LastTSMS: 24}},
	}
}
func TestRosterStatusHealthPriorityAndOperationalOverlay(t *testing.T) {
	p := core.Profile{Slug: "a", Enabled: true}
	d := richStatusSnapshot()
	d.Live = nil
	d.RoutingCounts = nil
	steps := []struct {
		state, label string
		remove       func()
	}{{"degraded", "degraded", func() { d.Degraded = nil }}, {"misconfigured", "misconfigured", func() { d.Misconfigured = nil }}, {"force_exhausted", "forced chain exhausted", func() { d.ForcedExhausted = nil }}, {"force_failed", "forced chain failed", func() { d.ForcedFailed = nil }}, {"unstable", "unstable routing", func() { d.Unstable = nil }}, {"stabilizing", "forced-chain probation", func() { d.Forced = nil }}, {"degraded", "fallback pressure", func() { d.Routing = nil }}, {"stale", "stale", func() { d.Dead = nil }}, {"healthy", "healthy", func() {}}}
	for _, step := range steps {
		row := RenderStatus([]core.Profile{p}, d)["a"]
		if row["health_state"] != step.state || row["health_label"] != step.label {
			t.Fatal(row, step.state)
		}
		step.remove()
	}
	d = richStatusSnapshot()
	p.Retired = true
	p.Enabled = false
	row := RenderStatus([]core.Profile{p}, d)["a"]
	if row["health_state"] != "retired" || row["health_label"] != "graveyard" || row["operational_state"] != "running" || row["operational_label"] != "tool" {
		t.Fatal("retired/live layering", row)
	}
	p.Retired = false
	d.Live = nil
	row = RenderStatus([]core.Profile{p}, d)["a"]
	if row["health_state"] != "healthy" || row["operational_state"] != "paused" {
		t.Fatal("paused priority", row)
	}
	p.Enabled = true
	d.Live = map[string]LiveStatus{"a": {ActiveRuns: 1}}
	row = RenderStatus([]core.Profile{p}, d)["a"]
	if row["operational_label"] != "running" {
		t.Fatal("empty active phase fallback", row)
	}
}
func TestRosterStatusCompleteSupplementalFieldsAndPresence(t *testing.T) {
	p := core.Profile{Slug: "a", Enabled: true, SelfRepairPolicy: &core.SelfRepairPolicy{Enabled: true}}
	d := richStatusSnapshot()
	row := RenderStatus([]core.Profile{p}, d)["a"]
	want := map[string]any{"self_repair_enabled": true, "health_failures": 2, "health_threshold": 3, "health_window": 4, "last_failure_ms": int64(17), "config_issues": []string{"owned config"}, "invalid_runtime_overrides": 1, "misconfiguration_count": 1, "repair_inflight": 1, "repair_mode": "routing", "repair_state": "completed", "repair_label": "routing stabilized", "repair_next_eligible_ms": int64(14), "repair_last_ts_ms": int64(15), "repair_last_correlation_id": "repair-corr", "repair_self_attempt": 2, "repair_self_max_attempts": 3, "repair_incident_id": "i", "repair_root_incident_id": "root-i", "repair_parent_incident_id": "parent-i", "repair_root_agent": "root-agent", "repair_chain_depth": 4, "repair_last_error": "owned error", "routing_fallback_count": 16, "routing_last_reason": "reason", "routing_last_failed": "failed-model", "routing_last_next": "next-model", "routing_last_ts_ms": int64(17), "retry_count": 18, "retry_last_reason": "retry", "retry_last_ts_ms": int64(19), "retry_next_attempt": 2, "retry_max_attempts": 4, "escalation_open_count": 3, "escalation_acked_count": 2, "wake_schedule_count": 4, "wake_standing_count": 5, "wake_event_subjects": []string{"board.a"}, "next_wake_ms": int64(20), "next_wake_label": "next", "active_run_count": 1, "active_correlation_id": "live-corr", "active_intent": "intent", "active_started_ms": int64(21), "active_model": "model", "active_spent_mc": int64(9007199254740993), "active_phase": "tool", "active_last_event_ms": int64(22), "active_last_event_kind": "event", "active_detail": "detail", "active_tool": "owned-tool", "active_iter": 6, "active_wake_source": "schedule", "active_wake_reason": "wake", "active_schedule_id": "s", "active_standing_id": "standing", "active_standing_name": "name", "active_trigger_subject": "subject", "active_parent_correlation": "parent", "last_activity_ms": int64(23), "last_activity_kind": "last-kind", "last_activity_correlation_id": "last-corr", "last_activity_summary": "last-summary", "last_autonomy_runbook": map[string]any{"plan": "owned"}, "mailbox_wakes": map[string]any{"mail": "owned"}, "policy_denied_count": 7, "policy_denied_last_tool": "denied-tool", "policy_denied_last_reason": "denied", "policy_denied_last_capability": "network", "policy_denied_last_hard": true, "policy_denied_last_ms": int64(24)}
	for key, value := range want {
		if !reflect.DeepEqual(row[key], value) {
			t.Fatalf("%s: %#v != %#v", key, row[key], value)
		}
	}
	empty := RenderStatus([]core.Profile{{Slug: "a", Enabled: true}}, StatusSnapshot{})["a"]
	if len(empty) != 18 {
		t.Fatal("default shape", empty)
	}
	for _, key := range []string{"repair_last_error", "config_issues", "active_spent_mc", "policy_denied_count", "mailbox_wakes", "last_autonomy_runbook"} {
		if _, found := empty[key]; found {
			t.Fatal("unexpected optional field", key)
		}
	}
	if out := RenderStatus(nil, d); out == nil || len(out) != 0 {
		t.Fatal("empty map", out)
	}
}
func TestRosterStatusSelectedProviderAndClockWindows(t *testing.T) {
	profiles := []core.Profile{{Slug: "a", Enabled: true}}
	now := time.UnixMilli(9007199254740993)
	calls := 0
	clockCalls := 0
	s := NewStatus(func(got []core.Profile, reaperCut, routingCut int64) StatusSnapshot {
		calls++
		if !reflect.DeepEqual(got, profiles) || reaperCut != now.Add(-30*24*time.Hour).UnixMilli() || routingCut != now.Add(time.Millisecond).Add(-24*time.Hour).UnixMilli() {
			t.Fatal("window/provider drift", reaperCut, routingCut)
		}
		return StatusSnapshot{Live: map[string]LiveStatus{"a": {ActiveRuns: 1}}}
	}, func() time.Time { clockCalls++; return now.Add(time.Duration(clockCalls-1) * time.Millisecond) })
	out := s.Views(profiles)
	if calls != 1 || clockCalls != 2 || out["a"]["operational_state"] != "running" {
		t.Fatal(out, calls, clockCalls)
	}
}
func TestRosterRepairPhaseLabels(t *testing.T) {
	for _, tc := range []struct{ mode, phase, want string }{{"routing", " completed ", "routing stabilized"}, {"degraded", "completed", "doctor repaired"}, {"other", "completed", "repaired"}, {"routing", "failed", "routing failed"}, {"degraded", "failed", "doctor failed"}, {"other", "failed", "repair failed"}, {"routing_unstable", "queued", "unstable routing"}, {"degraded", "queued", "doctor queued"}, {"routing", "queued", "routing queued"}, {"other", "queued", "repair queued"}, {"any", " ", "idle"}, {"any", " raw-unknown ", " raw-unknown "}, {"any", "routing_force_exhausted_detected", "forced chain exhausted"}, {"any", "delegation_failed", "delegation failed"}, {"any", "resolution_applied", "manager applied"}} {
		if got := RepairPhaseLabel(tc.mode, tc.phase); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
