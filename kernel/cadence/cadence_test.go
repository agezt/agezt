// SPDX-License-Identifier: MIT

package cadence

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func mustStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s
}

// recorder counts the intents a RunFunc was asked to run.
type recorder struct {
	mu      sync.Mutex
	intents []string
	block   chan struct{}
}

func (r *recorder) run(_ context.Context, intent, _ string) error {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.intents = append(r.intents, intent)
	r.mu.Unlock()
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.intents)
}

func waitCount(t *testing.T, r *recorder, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.count() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected %d runs, got %d", n, r.count())
}

func TestStore_AddListGetRemove(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	e, err := s.Add("daily brief", time.Hour, "sonnet", SourceOperator, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" || e.Intent != "daily brief" || e.IntervalSec != 3600 || e.Model != "sonnet" {
		t.Fatalf("entry = %+v", e)
	}
	if e.NextRunUnix != now.Add(time.Hour).Unix() {
		t.Errorf("next run not one interval out: %+v", e)
	}
	if got, ok := s.Get(e.ID); !ok || got.Intent != "daily brief" {
		t.Errorf("Get = %+v %v", got, ok)
	}
	if len(s.List()) != 1 {
		t.Errorf("List len = %d", len(s.List()))
	}
	ok, _ := s.Remove(e.ID)
	if !ok || s.Count() != 0 {
		t.Errorf("Remove failed: ok=%v count=%d", ok, s.Count())
	}
}

func TestStore_AddRejectsBadInput(t *testing.T) {
	s := mustStore(t)
	now := time.Now()
	if _, err := s.Add("  ", time.Hour, "", SourceOperator, now); err == nil {
		t.Error("empty intent should error")
	}
	if _, err := s.Add("x", time.Millisecond, "", SourceOperator, now); err == nil {
		t.Error("sub-minimum interval should error")
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s1, _ := OpenStore(dir)
	now := time.Now()
	e, _ := s1.Add("survive a restart", 2*time.Hour, "", SourceOperator, now)

	s2, err := OpenStore(dir) // reopen
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(e.ID)
	if !ok || got.Intent != "survive a restart" || got.IntervalSec != 7200 {
		t.Errorf("reopened entry = %+v ok=%v", got, ok)
	}
}

func TestStore_Due_AdvancesAndPersists(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	e, _ := s.Add("x", time.Hour, "", SourceOperator, base) // next = base+1h

	if due := s.Due(base.Add(30 * time.Minute)); len(due) != 0 {
		t.Fatalf("not due yet, got %d", len(due))
	}
	due := s.Due(base.Add(time.Hour + time.Second))
	if len(due) != 1 || due[0].ID != e.ID {
		t.Fatalf("should be due: %+v", due)
	}
	// Next run advanced ~one interval; last run recorded.
	got, _ := s.Get(e.ID)
	if got.LastRunUnix == 0 || got.NextRunUnix <= base.Add(time.Hour).Unix() {
		t.Errorf("due did not advance schedule: %+v", got)
	}
}

func TestStore_RunNow_MakesDue(t *testing.T) {
	s := mustStore(t)
	now := time.Now()
	e, _ := s.Add("later", 24*time.Hour, "", SourceOperator, now) // not due for a day
	if len(s.Due(now)) != 0 {
		t.Fatal("should not be due")
	}
	ok, _ := s.RunNow(e.ID)
	if !ok {
		t.Fatal("RunNow returned false")
	}
	if len(s.Due(now)) != 1 {
		t.Error("RunNow should make the entry due immediately")
	}
}

func TestStore_SyncEnv_ReplacesEnvKeepsOperator(t *testing.T) {
	s := mustStore(t)
	now := time.Now()
	op, _ := s.Add("operator job", time.Hour, "", SourceOperator, now)
	_ = s.SyncEnv([]Job{{Interval: time.Hour, Intent: "env one"}}, now)
	if s.Count() != 2 {
		t.Fatalf("expected operator + 1 env = 2, got %d", s.Count())
	}
	// A second sync with a different env set replaces only env entries.
	_ = s.SyncEnv([]Job{{Interval: 2 * time.Hour, Intent: "env two"}}, now)
	got := s.List()
	var envIntents, opStillThere = []string{}, false
	for _, e := range got {
		if e.Source == SourceEnv {
			envIntents = append(envIntents, e.Intent)
		}
		if e.ID == op.ID {
			opStillThere = true
		}
	}
	if !opStillThere {
		t.Error("operator entry should survive SyncEnv")
	}
	if len(envIntents) != 1 || envIntents[0] != "env two" {
		t.Errorf("env entries should be replaced: %v", envIntents)
	}
}

func TestEngine_FireDue_FiresAndSkipsOverlap(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	s.Add("slow", time.Hour, "", SourceOperator, base)

	rec := &recorder{block: make(chan struct{})}
	e := NewEngine(s, rec.run, 0, nil)

	e.fireDue(context.Background(), base.Add(time.Hour+time.Second)) // fires (blocks)
	waitRunningCount(t, e, 1)
	e.fireDue(context.Background(), base.Add(2*time.Hour+time.Second)) // due again but still running → skip
	close(rec.block)
	waitCount(t, rec, 1)
	time.Sleep(20 * time.Millisecond)
	if rec.count() != 1 {
		t.Errorf("overlap should be skipped: got %d", rec.count())
	}
}

func waitRunningCount(t *testing.T, e *Engine, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c := 0
		e.running.Range(func(_, _ any) bool { c++; return true })
		if c >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("run did not enter running state")
}

func TestEngine_Start_FiresLive(t *testing.T) {
	s := mustStore(t)
	// Already-due entry (next run in the past) → fires on the first tick.
	now := time.Now()
	e, _ := s.Add("tick", time.Hour, "", SourceOperator, now)
	_, _ = s.RunNow(e.ID) // make it due now

	rec := &recorder{}
	eng := NewEngine(s, rec.run, 100*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng.Start(ctx)
	waitCount(t, rec, 1)
}

func TestStore_Daily_FiresAtTimeAndAdvancesOneDay(t *testing.T) {
	s := mustStore(t)
	// "now" is 08:00 UTC; schedule daily at 09:00 → next run today 09:00.
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	e, err := s.AddDaily("morning brief", 9*60, 0, "", SourceOperator, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != ModeDaily || e.AtMinutes != 540 {
		t.Fatalf("entry = %+v", e)
	}
	wantNext := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC).Unix()
	if e.NextRunUnix != wantNext {
		t.Errorf("next run = %d want %d (today 09:00)", e.NextRunUnix, wantNext)
	}
	// Not due at 08:30; due at 09:00.
	if len(s.Due(now.Add(30*time.Minute))) != 0 {
		t.Fatal("should not be due before 09:00")
	}
	due := s.Due(time.Date(2026, 5, 31, 9, 0, 1, 0, time.UTC))
	if len(due) != 1 {
		t.Fatalf("should fire at 09:00, got %d", len(due))
	}
	// Next run advanced to tomorrow 09:00.
	got, _ := s.Get(e.ID)
	wantTomorrow := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC).Unix()
	if got.NextRunUnix != wantTomorrow {
		t.Errorf("after firing, next = %d want %d (tomorrow 09:00)", got.NextRunUnix, wantTomorrow)
	}
}

// TestStore_Daily_CatchesUpOnceAfterDowntime locks in the restart behavior: if
// the daemon was down across one (or several) daily slots, the entry fires
// exactly once on the next due check and then advances to the next *future*
// occurrence — never a burst of back-dated runs.
func TestStore_Daily_CatchesUpOnceAfterDowntime(t *testing.T) {
	s := mustStore(t)
	// Created Monday 08:00; daily at 09:00 → next run Monday 09:00.
	created := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	e, _ := s.AddDaily("morning brief", 9*60, 0, "", SourceOperator, created)

	// Daemon "down" until Thursday 10:00 — three 09:00 slots passed.
	back := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	due := s.Due(back)
	if len(due) != 1 {
		t.Fatalf("a long downtime should fire exactly once, got %d", len(due))
	}
	// Advanced to the next future slot (Friday 09:00), not a back-dated one.
	got, _ := s.Get(e.ID)
	wantFri := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC).Unix()
	if got.NextRunUnix != wantFri {
		t.Errorf("after catch-up, next = %d want %d (Fri 09:00)", got.NextRunUnix, wantFri)
	}
	// Immediately checking again does not double-fire.
	if len(s.Due(back)) != 0 {
		t.Error("must not fire again on the same tick after catch-up")
	}
}

func TestStore_AddDaily_Validates(t *testing.T) {
	s := mustStore(t)
	now := time.Now()
	if _, err := s.AddDaily("x", -1, 0, "", SourceOperator, now); err == nil {
		t.Error("negative time-of-day should error")
	}
	if _, err := s.AddDaily("x", 1440, 0, "", SourceOperator, now); err == nil {
		t.Error("24:00 (1440) should error")
	}
	if _, err := s.AddDaily("  ", 540, 0, "", SourceOperator, now); err == nil {
		t.Error("empty intent should error")
	}
}

func TestStore_SetEnabled_PausesFromDue(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	e, _ := s.Add("x", time.Hour, "", SourceOperator, base)
	// Pause → not due even when the time has come.
	ok, _ := s.SetEnabled(e.ID, false)
	if !ok {
		t.Fatal("SetEnabled returned false")
	}
	if len(s.Due(base.Add(2*time.Hour))) != 0 {
		t.Error("paused entry must not be due")
	}
	// Resume → due again.
	s.SetEnabled(e.ID, true)
	if len(s.Due(base.Add(2*time.Hour))) != 1 {
		t.Error("resumed entry should be due")
	}
}

func TestEntry_Cadence(t *testing.T) {
	if got := (Entry{IntervalSec: 3600}).Cadence(); got != "every 1h0m0s" {
		t.Errorf("interval cadence = %q", got)
	}
	if got := (Entry{Mode: ModeDaily, AtMinutes: 9*60 + 30}).Cadence(); got != "daily at 09:30" {
		t.Errorf("daily cadence = %q", got)
	}
}

func TestParseDays(t *testing.T) {
	cases := map[string]int{
		"":            0,
		"daily":       0,
		"all":         0,
		"weekdays":    maskWeekdays,
		"weekends":    maskWeekends,
		"mon-fri":     maskWeekdays,
		"sat,sun":     maskWeekends,
		"Mon,Wed,Fri": 1<<1 | 1<<3 | 1<<5,
		"fri-mon":     1<<5 | 1<<6 | 1<<0 | 1<<1, // wrapping range
		"tue":         1 << 2,
	}
	for spec, want := range cases {
		got, err := ParseDays(spec)
		if err != nil {
			t.Errorf("ParseDays(%q) error: %v", spec, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDays(%q) = %d, want %d", spec, got, want)
		}
	}
	for _, bad := range []string{"funday", "mon-funday", "mon,,bad", ","} {
		if _, err := ParseDays(bad); err == nil {
			t.Errorf("ParseDays(%q) should error", bad)
		}
	}
}

func TestFormatDays(t *testing.T) {
	cases := map[int]string{
		0:                  "",
		AllDays:            "",
		maskWeekdays:       "Mon-Fri",
		maskWeekends:       "Sat,Sun",
		1<<1 | 1<<3 | 1<<5: "Mon,Wed,Fri",
	}
	for mask, want := range cases {
		if got := FormatDays(mask); got != want {
			t.Errorf("FormatDays(%d) = %q, want %q", mask, got, want)
		}
	}
}

func TestStore_Daily_SkipsDisallowedWeekdays(t *testing.T) {
	s := mustStore(t)
	// 2026-05-31 is a Sunday. Weekdays-only daily at 09:00 → first run should be
	// Monday 2026-06-01 09:00 (Sunday is skipped).
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	weekdays, _ := ParseDays("weekdays")
	e, err := s.AddDaily("standup nudge", 9*60, weekdays, "", SourceOperator, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Cadence() != "Mon-Fri at 09:00" {
		t.Errorf("cadence = %q", e.Cadence())
	}
	wantMon := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC).Unix()
	if e.NextRunUnix != wantMon {
		t.Errorf("next = %d want %d (Mon 06-01 09:00, Sun skipped)", e.NextRunUnix, wantMon)
	}
	// Fire Monday; next advances to Tuesday (still a weekday), not the weekend.
	due := s.Due(time.Date(2026, 6, 1, 9, 0, 1, 0, time.UTC))
	if len(due) != 1 {
		t.Fatalf("should fire Monday, got %d", len(due))
	}
	got, _ := s.Get(e.ID)
	wantTue := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC).Unix()
	if got.NextRunUnix != wantTue {
		t.Errorf("after Mon, next = %d want %d (Tue)", got.NextRunUnix, wantTue)
	}
}

func TestStore_Once_FiresOnceAndSelfRemoves(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	at := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	e, err := s.AddOnce("summarise the deploy", at, "", SourceOperator, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != ModeOnce || e.NextRunUnix != at.Unix() {
		t.Fatalf("entry = %+v", e)
	}
	if e.Cadence() != "once at "+at.Local().Format("2006-01-02 15:04") {
		t.Errorf("cadence = %q", e.Cadence())
	}
	// Not due before 08:30.
	if len(s.Due(now)) != 0 {
		t.Fatal("should not be due before its time")
	}
	// Fires at 08:30 and removes itself from the store.
	due := s.Due(at.Add(time.Second))
	if len(due) != 1 || due[0].ID != e.ID {
		t.Fatalf("should fire once: %+v", due)
	}
	if s.Count() != 0 {
		t.Errorf("one-shot should self-remove, count = %d", s.Count())
	}
	// Never fires again.
	if len(s.Due(at.Add(2*time.Hour))) != 0 {
		t.Error("removed one-shot must not fire again")
	}
}

func TestStore_AddOnce_RejectsPastTime(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	if _, err := s.AddOnce("x", now.Add(-time.Minute), "", SourceOperator, now); err == nil {
		t.Error("a past one-shot time should error")
	}
	if _, err := s.AddOnce("  ", now.Add(time.Hour), "", SourceOperator, now); err == nil {
		t.Error("empty intent should error")
	}
}

func TestStore_EditAndReschedule(t *testing.T) {
	s := mustStore(t)
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC) // a Sunday
	e, _ := s.Add("old intent", time.Hour, "sonnet", SourceOperator, now)

	// SetIntent / SetModel preserve everything else.
	if ok, err := s.SetIntent(e.ID, "new intent"); !ok || err != nil {
		t.Fatalf("SetIntent: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.SetModel(e.ID, "opus"); !ok {
		t.Fatal("SetModel returned false")
	}
	if _, err := s.SetIntent(e.ID, "  "); err == nil {
		t.Error("empty intent should error")
	}
	got, _ := s.Get(e.ID)
	if got.Intent != "new intent" || got.Model != "opus" || got.Source != SourceOperator || got.CreatedUnix != now.Unix() {
		t.Errorf("after field edits: %+v", got)
	}

	// Reschedule interval → daily weekdays: mode/at/days change, id preserved,
	// next run recomputed (Sunday skipped → Monday 09:30).
	wd, _ := ParseDays("weekdays")
	if ok, err := s.Reschedule(e.ID, ModeDaily, 0, 9*60+30, wd, time.Time{}, now); !ok || err != nil {
		t.Fatalf("Reschedule daily: ok=%v err=%v", ok, err)
	}
	got, _ = s.Get(e.ID)
	if got.ID != e.ID || got.Mode != ModeDaily || got.AtMinutes != 570 || got.Days != wd || got.IntervalSec != 0 {
		t.Errorf("after reschedule to daily: %+v", got)
	}
	if got.Cadence() != "Mon-Fri at 09:30" {
		t.Errorf("cadence = %q", got.Cadence())
	}
	wantMon := time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC).Unix()
	if got.NextRunUnix != wantMon {
		t.Errorf("next = %d want %d (Mon, Sun skipped)", got.NextRunUnix, wantMon)
	}

	// Reschedule to one-shot; a past instant is rejected.
	if _, err := s.Reschedule(e.ID, ModeOnce, 0, 0, 0, now.Add(-time.Minute), now); err == nil {
		t.Error("past one-shot reschedule should error")
	}
	future := now.Add(2 * time.Hour)
	if ok, _ := s.Reschedule(e.ID, ModeOnce, 0, 0, 0, future, now); !ok {
		t.Fatal("reschedule once returned false")
	}
	got, _ = s.Get(e.ID)
	if got.Mode != ModeOnce || got.NextRunUnix != future.Unix() || got.AtMinutes != 0 || got.Days != 0 {
		t.Errorf("after reschedule to once: %+v", got)
	}

	// Editing a missing id reports not-found, not an error.
	if ok, err := s.SetIntent("nope", "x"); ok || err != nil {
		t.Errorf("missing SetIntent: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.Reschedule("nope", ModeInterval, time.Hour, 0, 0, time.Time{}, now); ok {
		t.Error("missing Reschedule should report false")
	}
}

func TestStore_AddDaily_ValidatesDayMask(t *testing.T) {
	s := mustStore(t)
	if _, err := s.AddDaily("x", 540, AllDays+1, "", SourceOperator, time.Now()); err == nil {
		t.Error("out-of-range day-mask should error")
	}
}

func TestParseJobs(t *testing.T) {
	jobs, err := ParseJobs("1h=summarise new commits; 24h=daily security audit, with commas")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	if jobs[0].Interval != time.Hour || jobs[0].Intent != "summarise new commits" {
		t.Errorf("job0 = %+v", jobs[0])
	}
	if jobs[1].Intent != "daily security audit, with commas" {
		t.Errorf("job1 intent = %q", jobs[1].Intent)
	}
	for _, bad := range []string{"noequals", "notaduration=do x", "500ms=too fast", "1h=  "} {
		if _, err := ParseJobs(bad); err == nil {
			t.Errorf("ParseJobs(%q) should error", bad)
		}
	}
	if j, err := ParseJobs("  "); err != nil || j != nil {
		t.Errorf("empty spec = %v, %v", j, err)
	}
}

func TestDescribe(t *testing.T) {
	out := Describe([]Entry{{Intent: "brief me", IntervalSec: 3600}})
	if !strings.Contains(out, "every 1h0m0s") || !strings.Contains(out, "brief me") {
		t.Errorf("describe = %q", out)
	}
}
