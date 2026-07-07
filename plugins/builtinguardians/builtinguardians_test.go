package builtinguardians

import (
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

type fakeHost struct {
	agents    []roster.Profile
	standings []standing.Order
	intervals []cadence.Entry
	dailies   []cadence.Entry
}

func (h *fakeHost) Agents() []roster.Profile { return h.agents }
func (h *fakeHost) AddAgent(p roster.Profile) (roster.Profile, error) {
	p.Enabled = true
	h.agents = append(h.agents, p)
	return p, nil
}
func (h *fakeHost) UpdateAgent(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	for i := range h.agents {
		if h.agents[i].Slug == ref || h.agents[i].ID == ref {
			mutate(&h.agents[i])
			return h.agents[i], true, nil
		}
	}
	return roster.Profile{}, false, nil
}
func (h *fakeHost) StandingOrders() []standing.Order {
	out := make([]standing.Order, len(h.standings))
	copy(out, h.standings)
	return out
}
func (h *fakeHost) UpdateStanding(id string, mutate func(*standing.Order)) (standing.Order, bool, error) {
	for i := range h.standings {
		if h.standings[i].ID == id {
			mutate(&h.standings[i])
			return h.standings[i], true, nil
		}
	}
	return standing.Order{}, false, nil
}
func (h *fakeHost) AddStanding(o standing.Order) (standing.Order, error) {
	if o.ID == "" {
		o.ID = "standing-" + o.Agent
	}
	o.Enabled = true // mirror Store.Add, which always creates enabled
	h.standings = append(h.standings, o)
	return o, nil
}
func (h *fakeHost) SetStandingEnabled(id string, enabled bool) (standing.Order, error) {
	for i := range h.standings {
		if h.standings[i].ID == id {
			h.standings[i].Enabled = enabled
			return h.standings[i], nil
		}
	}
	return standing.Order{}, nil
}
func (h *fakeHost) Schedules() []cadence.Entry {
	out := make([]cadence.Entry, 0, len(h.intervals)+len(h.dailies))
	out = append(out, h.intervals...)
	out = append(out, h.dailies...)
	return out
}
func (h *fakeHost) Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
	for i := range h.intervals {
		if h.intervals[i].ID != id {
			continue
		}
		h.intervals[i].Mode = mode
		h.intervals[i].IntervalSec = int64(interval / time.Second)
		h.intervals[i].AtMinutes = atMinutes
		h.intervals[i].Days = days
		return true, nil
	}
	for i := range h.dailies {
		if h.dailies[i].ID != id {
			continue
		}
		h.dailies[i].Mode = mode
		h.dailies[i].IntervalSec = int64(interval / time.Second)
		h.dailies[i].AtMinutes = atMinutes
		h.dailies[i].Days = days
		return true, nil
	}
	return false, nil
}
func (h *fakeHost) AddInterval(intent string, interval time.Duration, _, agent string) (cadence.Entry, error) {
	e := cadence.Entry{ID: "schedule-" + agent, Intent: intent, Agent: agent, IntervalSec: int64(interval / time.Second)}
	h.intervals = append(h.intervals, e)
	return e, nil
}
func (h *fakeHost) AddDaily(intent string, atMinutes int, _, agent string) (cadence.Entry, error) {
	e := cadence.Entry{ID: "schedule-" + agent, Intent: intent, Agent: agent, Mode: cadence.ModeDaily, AtMinutes: atMinutes}
	h.dailies = append(h.dailies, e)
	return e, nil
}

func TestSeedAll_SeedsEveryGuardian(t *testing.T) {
	h := &fakeHost{}
	out, err := SeedAll(h, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	if len(out) != len(guardians) {
		t.Fatalf("seeded %d, want %d", len(out), len(guardians))
	}
	if len(h.agents) != len(guardians) {
		t.Fatalf("created %d agents, want %d", len(h.agents), len(guardians))
	}
	// Every seeded agent is System-marked and carries a soul + budget cap.
	for _, p := range h.agents {
		if !p.System {
			t.Errorf("guardian %s not marked System", p.Slug)
		}
		if p.Soul == "" {
			t.Errorf("guardian %s has no soul", p.Slug)
		}
		if p.MaxDailyMc == 0 {
			t.Errorf("guardian %s has no daily budget cap", p.Slug)
		}
		if p.TrustCeiling != defaultTrustCeiling {
			t.Errorf("guardian %s trust ceiling = %q, want %q", p.Slug, p.TrustCeiling, defaultTrustCeiling)
		}
		if !containsString(p.ToolDeny, "memory") {
			t.Errorf("guardian %s should deny memory tool by default, got %v", p.Slug, p.ToolDeny)
		}
		if p.NoisePolicy == nil {
			t.Fatalf("guardian %s has no noise policy", p.Slug)
		}
		if !p.NoisePolicy.SilentOnSuccess || !p.NoisePolicy.DisableMemoryWrites {
			t.Errorf("guardian %s noise policy should stay silent and disable memory writes: %+v", p.Slug, p.NoisePolicy)
		}
		if p.NoisePolicy.MinNotifySeverity != "warning" || p.NoisePolicy.MinNotifyIntervalSec < 8*3600 {
			t.Errorf("guardian %s notify noise policy = %+v, want warning and >=8h cooldown", p.Slug, p.NoisePolicy)
		}
		if want := "system/" + p.Slug; p.MemoryScope != want {
			t.Errorf("guardian %s memory scope = %q, want %q", p.Slug, p.MemoryScope, want)
		}
	}
	// Each guardian validates as a real roster profile.
	for _, p := range h.agents {
		if err := roster.Validate(p); err != nil {
			t.Errorf("guardian %s invalid: %v", p.Slug, err)
		}
	}
	// Triggers: event guardians got standing orders, periodic ones got schedules.
	if len(h.standings) == 0 || len(h.intervals) == 0 || len(h.dailies) == 0 {
		t.Errorf("expected standing+interval+daily triggers, got standing=%d interval=%d daily=%d",
			len(h.standings), len(h.intervals), len(h.dailies))
	}
	// Standing orders run AS the guardian and validate.
	for _, o := range h.standings {
		if o.Agent == "" {
			t.Errorf("standing %q has no agent", o.Name)
		}
		if o.CooldownSec < 8*3600 {
			t.Errorf("standing %q cooldown = %ds, want >= 28800s", o.Name, o.CooldownSec)
		}
		if err := standing.Validate(o); err != nil {
			t.Errorf("standing %q invalid: %v", o.Name, err)
		}
	}
	// Schedule entries bind to a guardian structurally; the intent remains a pure task.
	for _, in := range append(h.intervals, h.dailies...) {
		if in.Agent == "" {
			t.Errorf("schedule intent %q missing agent binding", in.Intent)
		}
		if containsAgentFlag(in.Intent) {
			t.Errorf("schedule intent %q still embeds --agent", in.Intent)
		}
	}
}

func TestInitiativeResponderSeededDisabled(t *testing.T) {
	h := &fakeHost{}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, o := range h.standings {
		if o.Agent != "guardian-initiative" {
			continue
		}
		found = true
		if o.Enabled {
			t.Errorf("initiative responder must ship DISABLED (dormant until opt-in)")
		}
		if !sameEventSubjects(o.Triggers, []string{"pulse.initiative.act"}) {
			t.Errorf("initiative responder should bind pulse.initiative.act, got %+v", o.Triggers)
		}
	}
	if !found {
		t.Fatal("guardian-initiative standing order not seeded")
	}
}

func TestPeriodicGuardiansDoNotWakeTooOften(t *testing.T) {
	h := &fakeHost{}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	for _, in := range h.intervals {
		if in.IntervalSec < 12*60*60 {
			t.Fatalf("periodic guardian %s wakes every %ds, want >= 43200s", in.Agent, in.IntervalSec)
		}
	}
}

func TestSeedAll_Idempotent(t *testing.T) {
	h := &fakeHost{}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	agentsAfterFirst := len(h.agents)
	standingsAfterFirst := len(h.standings)

	out, err := SeedAll(h, "") // re-seed: everything already present
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if len(h.agents) != agentsAfterFirst {
		t.Errorf("re-seed added agents: %d → %d", agentsAfterFirst, len(h.agents))
	}
	if len(h.standings) != standingsAfterFirst {
		t.Errorf("re-seed added standing orders: %d → %d", standingsAfterFirst, len(h.standings))
	}
	for _, s := range out {
		if s.Created {
			t.Errorf("guardian %s reported Created on re-seed", s.Slug)
		}
	}
}

func TestSeedAll_ReconcilesExistingGuardianSafetyDefaults(t *testing.T) {
	h := &fakeHost{agents: []roster.Profile{{
		Slug:    "guardian-health",
		System:  true,
		Enabled: false,
		// Seed caps ABOVE the safety max so reconcile must CLAMP them down — the
		// reconcile only tightens (clamps high / fills zero), it never raises an
		// operator's tighter limit. (Before this fix the seed was `usd`, which the
		// "increase guardian budget limits for diagnostics" commit, 475b9483, left
		// stale: once maxCostMc rose to 5*usd a 1*usd seed was already within the
		// ceiling, so reconcile correctly left it and the == assertion broke.)
		MaxCostMc:    100 * usd,
		MaxDailyMc:   100 * usd,
		TrustCeiling: "L4",
		MemoryScope:  "guardian-health",
		ToolDeny:     []string{"notify"},
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity: "info",
		},
	}}}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	var got *roster.Profile
	for i := range h.agents {
		if h.agents[i].Slug == "guardian-health" {
			got = &h.agents[i]
		}
	}
	if got == nil {
		t.Fatal("guardian-health missing")
	}
	if got.Enabled {
		t.Fatal("reconcile should not resume a paused guardian")
	}
	if got.MaxCostMc != maxCostMc || got.MaxDailyMc != maxDailyMc {
		t.Fatalf("cost caps not reconciled: run=%d daily=%d", got.MaxCostMc, got.MaxDailyMc)
	}
	if got.TrustCeiling != defaultTrustCeiling {
		t.Fatalf("trust ceiling not reconciled: %q", got.TrustCeiling)
	}
	if !containsString(got.ToolDeny, "memory") || !containsString(got.ToolDeny, "notify") {
		t.Fatalf("tool deny not merged: %v", got.ToolDeny)
	}
	if got.MemoryScope != "system/guardian-health" {
		t.Fatalf("memory scope not reconciled: %q", got.MemoryScope)
	}
	if got.NoisePolicy == nil || !got.NoisePolicy.SilentOnSuccess || !got.NoisePolicy.DisableMemoryWrites {
		t.Fatalf("noise policy not reconciled: %+v", got.NoisePolicy)
	}
	if got.NoisePolicy.MinNotifySeverity != "warning" {
		t.Fatalf("system guardian notify severity should be raised to warning, got %+v", got.NoisePolicy)
	}
	if got.NoisePolicy.MinNotifyIntervalSec != 8*3600 {
		t.Fatalf("notify cooldown not reconciled: %+v", got.NoisePolicy)
	}
}

func TestSeedAll_ReconcilesExistingGuardianStandingCooldown(t *testing.T) {
	existing := make([]roster.Profile, 0, len(guardians))
	for _, g := range guardians {
		existing = append(existing, roster.Profile{Slug: g.slug, System: true, Enabled: true})
	}
	h := &fakeHost{
		agents: existing,
		standings: []standing.Order{{
			ID:       "standing-guardian-routing",
			Name:     "Guardian · Routing",
			Agent:    "guardian-routing",
			Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "provider.fallback"}, {Type: standing.TriggerEvent, Subject: "rate.limited"}},
			Plan:     "old plan",
		}},
	}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	if len(h.standings) != 1 {
		t.Fatalf("reconcile should not create duplicate standings, got %d", len(h.standings))
	}
	if got := h.standings[0].CooldownSec; got != 8*3600 {
		t.Fatalf("standing cooldown = %d, want 28800", got)
	}
	if h.standings[0].Plan != "old plan" {
		t.Fatalf("reconcile should not rewrite operator-edited plan: %q", h.standings[0].Plan)
	}
}

func TestSeedAll_ReconcilesExistingGuardianScheduleCadence(t *testing.T) {
	existing := make([]roster.Profile, 0, len(guardians))
	for _, g := range guardians {
		existing = append(existing, roster.Profile{Slug: g.slug, System: true, Enabled: true})
	}
	h := &fakeHost{
		agents: existing,
		intervals: []cadence.Entry{{
			ID:          "sched-health",
			Agent:       "guardian-health",
			Intent:      "Run one system-health sweep.",
			Mode:        cadence.ModeInterval,
			IntervalSec: 60,
			Enabled:     true,
			Source:      "system",
		}},
	}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	if len(h.intervals) != 1 {
		t.Fatalf("reconcile should not create duplicate schedules, got %d", len(h.intervals))
	}
	if got := h.intervals[0].IntervalSec; got != 12*60*60 {
		t.Fatalf("guardian-health interval = %d, want 43200", got)
	}
}

func TestSeedAll_SeedsMissingGuardiansAndLeavesPresentOnesUntouched(t *testing.T) {
	// Built-in system guardians are restored when missing; operator control is
	// expressed through pause/retire/edit, which reconcile preserves.
	h := &fakeHost{agents: []roster.Profile{{Slug: "guardian-health", System: true, Enabled: true}}}
	out, err := SeedAll(h, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range out {
		if s.Slug == "guardian-health" && s.Created {
			t.Error("present guardian-health should be left untouched")
		}
	}
	// The others were seeded.
	if len(h.agents) != len(guardians) {
		t.Errorf("agents = %d, want %d", len(h.agents), len(guardians))
	}
}

func TestGuardianSoulsDoNotTellSystemAgentsToWriteMemoryLogs(t *testing.T) {
	for _, g := range guardians {
		soul := strings.ToLower(g.soul)
		for _, bad := range []string{
			"record what you saw in memory",
			"recording offenders in memory",
			"note it in memory",
		} {
			if strings.Contains(soul, bad) {
				t.Fatalf("guardian %s still prompts system-agent memory logging: %q", g.slug, bad)
			}
		}
	}
}

func TestSameEventSubjects(t *testing.T) {
	trigs := []standing.Trigger{{Type: standing.TriggerEvent, Subject: "a"}, {Type: standing.TriggerEvent, Subject: "b"}}
	if !sameEventSubjects(trigs, []string{"b", "a"}) {
		t.Fatal("same subjects should match regardless of order")
	}
	if sameEventSubjects(trigs, []string{"a"}) {
		t.Fatal("partial subject set should not match")
	}
	if sameEventSubjects(append(trigs, standing.Trigger{Type: standing.TriggerCron, Schedule: "* * * * *"}), []string{"a", "b"}) {
		t.Fatal("mixed trigger order should not match guardian event-only trigger")
	}
}

func containsAgentFlag(s string) bool {
	for i := 0; i+7 <= len(s); i++ {
		if s[i:i+7] == "--agent" {
			return true
		}
	}
	return false
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestNotifySeverityRank_AllLevels(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"critical", 3},
		{"  CRITICAL  ", 3},
		{"warning", 2},
		{"WARNING", 2},
		{"info", 1},
		{"INFO", 1},
		{"", 0},
		{"unknown", 0},
	}
	for _, tc := range tests {
		got := notifySeverityRank(tc.severity)
		if got != tc.want {
			t.Errorf("notifySeverityRank(%q) = %d, want %d", tc.severity, got, tc.want)
		}
	}
}

func TestTrustRank_AllLevels(t *testing.T) {
	tests := []struct {
		level string
		want  int
	}{
		{"L0", 0},
		{"l0", 0},
		{"L1", 1},
		{"  l1  ", 1},
		{"L2", 2},
		{"L3", 3},
		{"L4", 4},
		{"", 4},
		{"L5", 4},
	}
	for _, tc := range tests {
		got := trustRank(tc.level)
		if got != tc.want {
			t.Errorf("trustRank(%q) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestSameEventSubjects_EdgeCases(t *testing.T) {
	// Empty subjects list.
	if sameEventSubjects(nil, nil) {
		t.Error("sameEventSubjects(nil, nil) should be false")
	}
	if sameEventSubjects(nil, []string{}) {
		t.Error("sameEventSubjects(nil, []) should be false")
	}
	// Non-event trigger shouldn't match.
	trigs := []standing.Trigger{{Type: standing.TriggerCron, Schedule: "* * * * *"}}
	if sameEventSubjects(trigs, []string{"a"}) {
		t.Error("cron trigger should not match event subjects")
	}
	// Empty subject trigger shouldn't match.
	trigs2 := []standing.Trigger{{Type: standing.TriggerEvent, Subject: ""}}
	if sameEventSubjects(trigs2, []string{"a"}) {
		t.Error("empty subject trigger should not match")
	}
	// Exact match.
	trigs3 := []standing.Trigger{{Type: standing.TriggerEvent, Subject: "a"}}
	if !sameEventSubjects(trigs3, []string{"a"}) {
		t.Error("exact match should return true")
	}
	// Duplicate subject in triggers (count > 1).
	trigs4 := []standing.Trigger{
		{Type: standing.TriggerEvent, Subject: "a"},
		{Type: standing.TriggerEvent, Subject: "a"},
	}
	if sameEventSubjects(trigs4, []string{"a"}) {
		t.Error("duplicate subject should not match single-entry subjects")
	}
}

func TestReconcileExistingGuardian_NonSystemSkipped(t *testing.T) {
	h := &fakeHost{agents: []roster.Profile{{
		Slug:   "user-agent",
		System: false,
	}}}
	// Should not error (non-system agents are skipped silently).
	p := h.agents[0]
	if err := reconcileExistingGuardian(h, p); err != nil {
		t.Fatalf("reconcileExistingGuardian on non-system agent: %v", err)
	}
}

func TestReconcileExistingGuardianSchedule_NoMatchReturnsNil(t *testing.T) {
	h := &fakeHost{}
	// A guardian with interval but no schedule in Host → should return nil without error.
	g := guardian{slug: "guardian-health", intervalSec: 3600, plan: "test"}
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule with no schedules: %v", err)
	}
}

func TestReconcileExistingGuardianSchedule_NoneTriggerType(t *testing.T) {
	h := &fakeHost{}
	// A guardian with no interval and no daily trigger → early return.
	g := guardian{slug: "guardian-nothing"}
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule with no trigger: %v", err)
	}
}

func TestReconcileExistingGuardianSchedule_DailyMatch(t *testing.T) {
	h := &fakeHost{dailies: []cadence.Entry{{
		ID:        "sched-" + "guardian-code",
		Agent:     "guardian-code",
		Intent:    "Review agent-written tools and workspace code for efficiency and correctness.",
		Mode:      cadence.ModeDaily,
		AtMinutes: 3 * 60,
		Enabled:   true,
		Source:    "system",
	}}}
	// Find the daily guardian.
	var g guardian
	for _, gg := range guardians {
		if gg.slug == "guardian-code" {
			g = gg
			break
		}
	}
	if g.slug == "" {
		t.Fatal("guardian-code not found in guardians list")
	}
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule daily match: %v", err)
	}
	// Should not have changed the schedule (already matching).
	if len(h.dailies) != 1 {
		t.Fatalf("should not have added new schedule entries, got %d", len(h.dailies))
	}
	if h.dailies[0].AtMinutes != 3*60 {
		t.Errorf("atMinutes changed to %d, want %d", h.dailies[0].AtMinutes, 3*60)
	}
}

func TestReconcileExistingGuardianSchedule_DailyMismatch(t *testing.T) {
	h := &fakeHost{dailies: []cadence.Entry{{
		ID:        "sched-" + "guardian-code",
		Agent:     "guardian-code",
		Intent:    "Review agent-written tools and workspace code for efficiency and correctness.",
		Mode:      cadence.ModeInterval,
		AtMinutes: 0,
		Enabled:   true,
		Source:    "system",
	}}}
	var g guardian
	for _, gg := range guardians {
		if gg.slug == "guardian-code" {
			g = gg
			break
		}
	}
	if g.slug == "" {
		t.Fatal("guardian-code not found")
	}
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule daily mismatch: %v", err)
	}
	// Should have rescheduled to daily.
	if len(h.dailies) != 1 {
		t.Fatalf("got %d dailies, want 1", len(h.dailies))
	}
	if h.dailies[0].Mode != cadence.ModeDaily {
		t.Errorf("mode = %q, want daily", h.dailies[0].Mode)
	}
	if h.dailies[0].AtMinutes != 3*60 {
		t.Errorf("atMinutes = %d, want %d", h.dailies[0].AtMinutes, 3*60)
	}
}

func TestReconcileExistingGuardianSchedule_IntervalAlreadySufficient(t *testing.T) {
	h := &fakeHost{intervals: []cadence.Entry{{
		ID:          "sched-" + "guardian-health",
		Agent:       "guardian-health",
		Intent:      "Run one system-health sweep.",
		Mode:        cadence.ModeInterval,
		IntervalSec: 86400, // already > 43200
		Enabled:     true,
		Source:      "system",
	}}}
	// guardian-health has intervalSec = 12*3600 = 43200
	var g guardian
	for _, gg := range guardians {
		if gg.slug == "guardian-health" {
			g = gg
			break
		}
	}
	if g.slug == "" {
		t.Fatal("guardian-health not found")
	}
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule: %v", err)
	}
	// Should not have rescheduled — existing interval is already sufficient.
	if len(h.intervals) != 1 {
		t.Fatalf("got %d intervals, want 1", len(h.intervals))
	}
	if h.intervals[0].IntervalSec != 86400 {
		t.Errorf("interval changed to %d, want 86400", h.intervals[0].IntervalSec)
	}
}

func TestReconcileExistingGuardianSchedule_NonMatchingSchedulesSkipped(t *testing.T) {
	h := &fakeHost{intervals: []cadence.Entry{{
		ID:          "sched-other",
		Agent:       "other-agent",
		Intent:      "something else",
		Mode:        cadence.ModeInterval,
		IntervalSec: 60,
		Enabled:     true,
		Source:      "env",
	}}}
	// guardian-health has intervalSec=43200, plan="Run one system-health sweep."
	var g guardian
	for _, gg := range guardians {
		if gg.slug == "guardian-health" {
			g = gg
			break
		}
	}
	// No matching schedule found → should return nil without rescheduling.
	if err := reconcileExistingGuardianSchedule(h, g); err != nil {
		t.Fatalf("reconcileExistingGuardianSchedule: %v", err)
	}
	// Interval should remain unchanged (no matching schedule to modify).
	if len(h.intervals) != 1 {
		t.Fatalf("got %d intervals, want 1", len(h.intervals))
	}
	if h.intervals[0].IntervalSec != 60 {
		t.Errorf("interval was unexpectedly modified to %d", h.intervals[0].IntervalSec)
	}
}

func TestSeedTrigger_NoneCase(t *testing.T) {
	h := &fakeHost{}
	// A guardian with no events, no daily, no interval → returns "none".
	g := guardian{slug: "no-trigger", name: "No Trigger", soul: "noop"}
	trig, err := seedTrigger(h, g)
	if err != nil {
		t.Fatalf("seedTrigger: %v", err)
	}
	if trig != "none" {
		t.Errorf("trigger = %q, want none", trig)
	}
}

func TestDefaultGuardianNoisePolicy(t *testing.T) {
	np := defaultGuardianNoisePolicy()
	if np == nil {
		t.Fatal("defaultGuardianNoisePolicy returned nil")
	}
	if !np.SilentOnSuccess {
		t.Error("SilentOnSuccess should be true")
	}
	if !np.DisableMemoryWrites {
		t.Error("DisableMemoryWrites should be true")
	}
	if np.MinNotifySeverity != defaultMinNotifySeverity {
		t.Errorf("MinNotifySeverity = %q, want %q", np.MinNotifySeverity, defaultMinNotifySeverity)
	}
	if np.MinNotifyIntervalSec != minNotifyIntervalSec {
		t.Errorf("MinNotifyIntervalSec = %d, want %d", np.MinNotifyIntervalSec, minNotifyIntervalSec)
	}
}

func TestAppendUnique(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, "a")
	if len(got) != 2 {
		t.Errorf("appendUnique existing = %v, want [a b]", got)
	}
	got2 := appendUnique([]string{"a", "b"}, "c")
	if len(got2) != 3 || got2[2] != "c" {
		t.Errorf("appendUnique new = %v, want [a b c]", got2)
	}
	got3 := appendUnique(nil, "x")
	if len(got3) != 1 || got3[0] != "x" {
		t.Errorf("appendUnique nil = %v, want [x]", got3)
	}
}
