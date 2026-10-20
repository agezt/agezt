// SPDX-License-Identifier: MIT

package roster

import (
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

// evSeq builds one journal event with a sequence number, so the "latest wins"
// folds can be told apart.
func evSeq(seq int64, kind event.Kind, subject, corr string, ts int64, payload map[string]any) *event.Event {
	e := ev(kind, subject, corr, ts, payload)
	e.Seq = seq
	return e
}

// wakeSources returns a source set with only the wake stores bound.
func wakeSources(schedules []cadence.Entry, orders []standing.Order) StatusSources {
	return StatusSources{
		Schedules: func() []cadence.Entry { return schedules },
		Standing:  func() []standing.Order { return orders },
	}
}

func TestCollectWakeStatus_ScheduleByAgentField(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := wakeSources([]cadence.Entry{{
		ID: "sched-1", Agent: "runner", Intent: "check the feed", Enabled: true, NextRunUnix: 1_700_000_000,
	}}, nil)
	wakes := CollectWakeStatus(src, profiles)
	row, ok := wakes["runner"]
	if !ok {
		t.Fatalf("no wake status: %+v", wakes)
	}
	if row.ScheduleCount != 1 {
		t.Errorf("schedule count = %d, want 1", row.ScheduleCount)
	}
	if row.NextScheduledWakeMS != 1_700_000_000_000 {
		t.Errorf("next wake = %d, want the next run in ms", row.NextScheduledWakeMS)
	}
	if row.NextScheduledLabel != "check the feed" {
		t.Errorf("label = %q, want the intent", row.NextScheduledLabel)
	}
}

func TestCollectWakeStatus_LegacyAgentFlagInTheIntent(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := wakeSources([]cadence.Entry{{
		ID: "sched-1", Intent: "run the thing --agent=runner", Enabled: true, NextRunUnix: 10,
	}}, nil)
	wakes := CollectWakeStatus(src, profiles)
	if wakes["runner"].ScheduleCount != 1 {
		t.Fatalf("the legacy --agent flag must still wake the agent: %+v", wakes)
	}
	// The spaced form works too.
	src = wakeSources([]cadence.Entry{{
		ID: "sched-2", Intent: "run the thing --agent runner", Enabled: true, NextRunUnix: 10,
	}}, nil)
	wakes = CollectWakeStatus(src, profiles)
	if wakes["runner"].ScheduleCount != 1 {
		t.Fatalf("the legacy --agent flag with a space must still wake the agent: %+v", wakes)
	}
}

func TestCollectWakeStatus_NearestScheduleWins(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := wakeSources([]cadence.Entry{
		{ID: "late", Agent: "runner", Intent: "late", Enabled: true, NextRunUnix: 2_000},
		{ID: "soon", Agent: "runner", Intent: "soon", Enabled: true, NextRunUnix: 1_000},
	}, nil)
	row := CollectWakeStatus(src, profiles)["runner"]
	if row.NextScheduledWakeMS != 1_000_000 {
		t.Errorf("next wake = %d, want the nearest schedule", row.NextScheduledWakeMS)
	}
	if row.NextScheduledLabel != "soon" {
		t.Errorf("label = %q, want the nearest schedule's label", row.NextScheduledLabel)
	}
	if row.ScheduleCount != 2 {
		t.Errorf("schedule count = %d, want 2", row.ScheduleCount)
	}
}

func TestCollectWakeStatus_DisabledScheduleNeverWakes(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := wakeSources([]cadence.Entry{{
		ID: "sched-1", Agent: "runner", Intent: "off", Enabled: false, NextRunUnix: 1_000,
	}}, nil)
	row := CollectWakeStatus(src, profiles)["runner"]
	if row.ScheduleCount != 1 {
		t.Errorf("schedule count = %d, want 1", row.ScheduleCount)
	}
	if row.NextScheduledWakeMS != 0 {
		t.Errorf("next wake = %d, want none for a disabled schedule", row.NextScheduledWakeMS)
	}
}

func TestCollectWakeStatus_StandingOrderCountsAndSubjects(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := wakeSources(nil, []standing.Order{{
		ID: "order-1", Agent: "runner", Triggers: []standing.Trigger{
			{Type: standing.TriggerEvent, Subject: "board.message"},
			{Type: standing.TriggerEvent, Subject: "board.message"},
			{Type: standing.TriggerEvent, Subject: "  "},
			{Type: standing.TriggerCron, Schedule: "0 * * * *"},
		},
	}})
	row := CollectWakeStatus(src, profiles)["runner"]
	if row.StandingCount != 1 {
		t.Errorf("standing count = %d, want 1", row.StandingCount)
	}
	if len(row.EventSubjects) != 1 || row.EventSubjects[0] != "board.message" {
		t.Errorf("event subjects = %v, want the deduplicated board.subject", row.EventSubjects)
	}
}

func TestCollectWakeStatus_AgentWithoutAScheduleOrOrderIsDropped(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}, {Slug: "idle"}}
	wakes := CollectWakeStatus(wakeSources(nil, nil), profiles)
	if _, ok := wakes["idle"]; ok {
		t.Fatalf("an agent nothing wakes has no wake status: %+v", wakes)
	}
	if _, ok := wakes["runner"]; ok {
		t.Fatalf("an agent nothing wakes has no wake status: %+v", wakes)
	}
}

func TestCollectWakeStatus_NoProfilesMeansNoWakes(t *testing.T) {
	src := wakeSources([]cadence.Entry{{ID: "s", Agent: "runner", Enabled: true, NextRunUnix: 1}}, nil)
	if wakes := CollectWakeStatus(src, nil); wakes != nil {
		t.Fatalf("an empty roster has no wakes: %+v", wakes)
	}
}

func TestScheduleWakeLabel(t *testing.T) {
	cases := []struct {
		name  string
		entry cadence.Entry
		want  string
	}{
		{"workflow", cadence.Entry{Target: cadence.TargetWorkflow, Workflow: "digest"}, "workflow digest"},
		{"system task", cadence.Entry{Target: cadence.TargetSystemTask, SystemTask: "sweep"}, "system task sweep"},
		{"tool", cadence.Entry{Target: cadence.TargetTool, Tool: "search"}, "tool search"},
		{"intent", cadence.Entry{Intent: "do it"}, "do it"},
		{"id", cadence.Entry{ID: "sched-9"}, "sched-9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduleWakeLabel(tc.entry); got != tc.want {
				t.Errorf("label = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCollectEscalationStatus_DirectMessage(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := StatusSources{Board: func() []board.Message {
		return []board.Message{{ID: "m1", To: "runner", From: "lead", Help: true}}
	}}
	load := CollectEscalationStatus(src, profiles)
	if load["runner"].Open != 1 {
		t.Errorf("open = %d, want 1: %+v", load["runner"].Open, load)
	}
}

func TestCollectEscalationStatus_AckedMessage(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := StatusSources{Board: func() []board.Message {
		return []board.Message{{ID: "m1", To: "runner", AckedBy: []string{"Runner"}}}
	}}
	load := CollectEscalationStatus(src, profiles)
	if load["runner"].Acked != 1 || load["runner"].Open != 0 {
		t.Errorf("load = %+v, want one ack and no open", load["runner"])
	}
}

func TestCollectEscalationStatus_BroadcastSkipsTheSender(t *testing.T) {
	profiles := []core.Profile{{Slug: "lead"}, {Slug: "runner"}}
	src := StatusSources{Board: func() []board.Message {
		return []board.Message{{ID: "m1", To: board.Everyone, From: "lead"}}
	}}
	load := CollectEscalationStatus(src, profiles)
	if load["runner"].Open != 1 {
		t.Errorf("a broadcast reaches everyone but the sender: %+v", load)
	}
	if _, ok := load["lead"]; ok {
		t.Errorf("the sender does not receive its own broadcast: %+v", load)
	}
}

func TestCollectEscalationStatus_UnknownRecipientIsIgnored(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := StatusSources{Board: func() []board.Message {
		return []board.Message{{ID: "m1", To: "ghost", From: "lead"}}
	}}
	if load := CollectEscalationStatus(src, profiles); len(load) != 0 {
		t.Fatalf("a message for an unknown agent is not an escalation: %+v", load)
	}
}

func TestCollectEscalationStatus_NoBoardMeansNoEscalations(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	if load := CollectEscalationStatus(StatusSources{}, profiles); load != nil {
		t.Fatalf("no board means no escalations: %+v", load)
	}
	src := StatusSources{Board: func() []board.Message { return nil }}
	if load := CollectEscalationStatus(src, profiles); load != nil {
		t.Fatalf("no open help means no escalations: %+v", load)
	}
}

func TestCollectStatus_ReaperMapping(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	src := StatusSources{
		Reaper: func(cut int64) runtime.ReaperReport {
			return runtime.ReaperReport{
				DegradedAgents:      []runtime.DegradedAgent{{Slug: "runner", Failures: 3, Threshold: 2, Window: 5, LastFailureMS: 42}},
				MisconfiguredAgents: []runtime.MisconfiguredAgent{{Slug: "runner", Issues: []string{"no model"}}},
				RoutingPressure:     []runtime.RoutingPressureAgent{{Slug: "runner", Count: 7}},
				RoutingForced:       []runtime.RoutingForcedProbationAgent{{Slug: "runner", Count: 1, TaskType: "code", ForcedChain: []string{"a", "b"}, ForceGeneration: 4}},
				RoutingForcedFailed: []runtime.RoutingForcedFailedAgent{{Slug: "runner", Count: 2, TaskType: "code"}},
				RoutingForcedExhausted: []runtime.RoutingForcedExhaustedAgent{
					{Slug: "runner", Count: 3, TaskType: "code"},
				},
				RoutingUnstable: []runtime.RoutingUnstableAgent{
					{Slug: "runner", Count: 4, TaskType: "code", CurrentChain: []string{"a"}, PreviousChain: []string{"b"}},
				},
				DeadAgents: []runtime.ReaperAgent{{Slug: "runner", LastActiveMS: 99}},
			}
		},
	}
	snap := CollectStatus(src, profiles, 0, 0)
	degraded := snap.Degraded["runner"]
	if degraded.Failures != 3 || degraded.Threshold != 2 || degraded.Window != 5 || degraded.LastFailureMS != 42 {
		t.Errorf("degraded = %+v", degraded)
	}
	if snap.Misconfigured["runner"].Issues[0] != "no model" {
		t.Errorf("misconfigured = %+v", snap.Misconfigured["runner"])
	}
	if snap.Routing["runner"].Count != 7 {
		t.Errorf("routing = %+v", snap.Routing["runner"])
	}
	forced := snap.Forced["runner"]
	if forced.Count != 1 || forced.TaskType != "code" || forced.ForceGeneration != 4 || len(forced.ForcedChain) != 2 {
		t.Errorf("forced = %+v", forced)
	}
	if snap.ForcedFailed["runner"].Count != 2 {
		t.Errorf("forced failed = %+v", snap.ForcedFailed["runner"])
	}
	if snap.ForcedExhausted["runner"].Count != 3 {
		t.Errorf("forced exhausted = %+v", snap.ForcedExhausted["runner"])
	}
	unstable := snap.Unstable["runner"]
	if unstable.Count != 4 || unstable.TaskType != "code" || len(unstable.CurrentChain) != 1 || len(unstable.PreviousChain) != 1 {
		t.Errorf("unstable = %+v", unstable)
	}
	if snap.Dead["runner"].LastActiveMS != 99 {
		t.Errorf("dead = %+v", snap.Dead["runner"])
	}
}

func TestCollectStatus_ReaperCutReachesTheHost(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	var got int64
	src := StatusSources{Reaper: func(cut int64) runtime.ReaperReport {
		got = cut
		return runtime.ReaperReport{}
	}}
	CollectStatus(src, profiles, 1234, 0)
	if got != 1234 {
		t.Errorf("reaper cut = %d, want the host's cut", got)
	}
}

func TestCollectStatus_NoReaperMeansNoHealthPicture(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	snap := CollectStatus(StatusSources{}, profiles, 0, 0)
	if len(snap.Degraded) != 0 || len(snap.Dead) != 0 {
		t.Fatalf("no reaper means no health picture: %+v", snap)
	}
}

func TestCollectJournalStatus_RepairSummaryLatestWins(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		evSeq(1, event.KindInfo, "doctor.auto_repair", "run-1", 100, map[string]any{
			"agent": "runner", "phase": "completed", "fingerprint": "fp-1", "reason": "first",
		}),
		evSeq(2, event.KindInfo, "doctor.auto_repair", "run-2", 200, map[string]any{
			"agent": "runner", "phase": "completed", "fingerprint": "fp-1", "reason": "second",
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	sum, ok := snap.Repairs["runner"]
	if !ok {
		t.Fatalf("no repair summary: %+v", snap.Repairs)
	}
	if !sum.HasLatest {
		t.Fatalf("the summary has no latest repair: %+v", sum)
	}
	if sum.Latest.Reason != "second" {
		t.Errorf("latest = %+v, want the highest sequence", sum.Latest)
	}
	if sum.Latest.Seq != 2 {
		t.Errorf("latest seq = %d, want 2", sum.Latest.Seq)
	}
}

func TestCollectJournalStatus_RepairInflightCountsFingerprints(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		evSeq(1, event.KindInfo, "doctor.auto_repair", "run-1", 100, map[string]any{
			"agent": "runner", "phase": "queued", "fingerprint": "fp-1",
		}),
		evSeq(2, event.KindInfo, "doctor.auto_repair", "run-2", 200, map[string]any{
			"agent": "runner", "phase": "routing_rollback_queued", "fingerprint": "fp-2",
		}),
		evSeq(3, event.KindInfo, "doctor.auto_repair", "run-3", 300, map[string]any{
			"agent": "runner", "phase": "completed", "fingerprint": "fp-3",
		}),
	}
	snap := collect(t, profiles, events, nil, 0)
	if got := snap.Repairs["runner"].InflightCount; got != 2 {
		t.Errorf("inflight = %d, want the two queued fingerprints", got)
	}
}

func TestCollectJournalStatus_RepairNextEligibleUsesTheCooldown(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		evSeq(1, event.KindInfo, "doctor.auto_repair", "run-1", 1_000, map[string]any{
			"agent": "runner", "phase": "completed", "fingerprint": "fp-1",
		}),
	}
	src := StatusSources{
		Journal: journalOf(events...),
		RepairCooldown: func() time.Duration {
			return 90 * time.Second
		},
	}
	snap := StatusSnapshot{}
	CollectJournalStatus(src, profiles, 0, &snap)
	if got := snap.Repairs["runner"].Latest.NextEligibleMS; got != 1_000+90_000 {
		t.Errorf("next eligible = %d, want the event time plus the cooldown", got)
	}
}

func TestAutoRepairCooldown(t *testing.T) {
	key := brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"
	t.Setenv(key, "5m")
	if got := AutoRepairCooldown(); got != 5*time.Minute {
		t.Errorf("cooldown = %s, want 5m", got)
	}
	t.Setenv(key, "not-a-duration")
	if got := AutoRepairCooldown(); got != 30*time.Minute {
		t.Errorf("a malformed cooldown = %s, want the 30m default", got)
	}
	t.Setenv(key, "-1m")
	if got := AutoRepairCooldown(); got != 30*time.Minute {
		t.Errorf("a non-positive cooldown = %s, want the 30m default", got)
	}
	t.Setenv(key, "")
	if got := AutoRepairCooldown(); got != 30*time.Minute {
		t.Errorf("an empty cooldown = %s, want the 30m default", got)
	}
}

func TestCollectStatus_AssemblesEveryPart(t *testing.T) {
	profiles := []core.Profile{{Slug: "runner"}}
	events := []*event.Event{
		evSeq(1, event.KindInfo, "doctor.auto_repair", "run-1", 100, map[string]any{
			"agent": "runner", "phase": "completed", "fingerprint": "fp-1",
		}),
	}
	src := StatusSources{
		Journal: journalOf(events...),
		Runs:    func() ([]RunRow, error) { return []RunRow{{CorrelationID: "run-1", Agent: "runner"}}, nil },
		Schedules: func() []cadence.Entry {
			return []cadence.Entry{{ID: "s", Agent: "runner", Enabled: true, NextRunUnix: 5}}
		},
		Standing: func() []standing.Order { return nil },
		Board:    func() []board.Message { return []board.Message{{ID: "m1", To: "runner"}} },
		Reaper:   func(int64) runtime.ReaperReport { return runtime.ReaperReport{} },
	}
	snap := CollectStatus(src, profiles, 0, 0)
	if len(snap.Live) != 1 {
		t.Errorf("live = %+v, want the running agent", snap.Live)
	}
	if len(snap.Repairs) != 1 {
		t.Errorf("repairs = %+v, want the repair summary", snap.Repairs)
	}
	if len(snap.Escalations) != 1 {
		t.Errorf("escalations = %+v, want the open help request", snap.Escalations)
	}
	if len(snap.Wakes) != 1 {
		t.Errorf("wakes = %+v, want the schedule", snap.Wakes)
	}
	if snap.Degraded == nil || snap.Dead == nil {
		t.Errorf("the health maps must be present, not nil: %+v", snap)
	}
}
