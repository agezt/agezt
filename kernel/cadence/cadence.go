// SPDX-License-Identifier: MIT

// Package cadence is the scheduled-intents subsystem (autonomy): it fires
// intents on a recurring timer through the normal governed kernel loop, so the
// system acts on its own ("every morning, summarise new commits and brief me")
// rather than only reacting. It is the timer companion to Pulse's event-driven
// proactivity.
//
// Schedules live in a persistent Store (survives restarts) and are managed by
// the operator over the control plane (`agt schedule add|list|rm|run`).
// Operator-configured AGEZT_SCHEDULE env jobs are synced into the same store at
// startup (source="env"), so both paths share one source of truth. The Engine
// ticks, asks the Store which entries are due, and fires each through a RunFunc;
// a still-running entry is skipped (no overlap). Every firing is journaled
// (schedule.fired) and the run it launches is governed exactly like `agt run`.
package cadence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// MinInterval guards against a busy-loop from a misconfigured tiny interval.
const MinInterval = time.Second

// DefaultResolution is how often the ticker wakes to check for due entries.
const DefaultResolution = 10 * time.Second

// Source values distinguish operator-managed entries from env-seeded ones.
const (
	SourceOperator = "operator"
	SourceEnv      = "env"
)

// Scheduling modes. The zero value ("") is ModeInterval for backward
// compatibility with stores written before daily scheduling existed.
const (
	ModeInterval = "" // fire every IntervalSec seconds
	ModeDaily    = "daily"
	ModeOnce     = "once"   // fire exactly once at NextRunUnix, then self-remove
	ModeWindow   = "window" // fire every IntervalSec, but only within a daily time window
)

// AllDays is the day-mask meaning "every day" (all seven bits set); the zero
// value 0 means the same (an unrestricted daily schedule).
const AllDays = 0x7F

// Entry is one persisted schedule. An entry is either interval-based
// (Mode==ModeInterval, fires every IntervalSec) or daily (Mode==ModeDaily,
// fires once a day at AtMinutes minutes past local midnight). A daily entry may
// be restricted to certain weekdays via Days (a bitmask over time.Weekday, bit
// Sunday=0 .. Saturday=6); Days==0 (or AllDays) means every day.
type Entry struct {
	ID          string `json:"id"`
	Intent      string `json:"intent"`
	Mode        string `json:"mode,omitempty"`
	IntervalSec int64  `json:"interval_sec,omitempty"`
	AtMinutes   int    `json:"at_minutes,omitempty"`  // daily/window: minutes since local midnight (window: start)
	EndMinutes  int    `json:"end_minutes,omitempty"` // window: window end, minutes since local midnight
	Days        int    `json:"days,omitempty"`        // daily/window: weekday bitmask (0/AllDays = every day)
	TZ          string `json:"tz,omitempty"`          // daily/window: IANA zone for the wall-clock time (empty = daemon local)
	Model       string `json:"model,omitempty"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	CreatedUnix int64  `json:"created_unix"`
	LastRunUnix int64  `json:"last_run_unix,omitempty"`
	NextRunUnix int64  `json:"next_run_unix"`
}

// Interval is the entry's firing period (interval mode only).
func (e Entry) Interval() time.Duration { return time.Duration(e.IntervalSec) * time.Second }

// usesInterval reports whether the entry's IntervalSec is load-bearing — true
// for interval and windowed-interval modes, false for daily/once (which carry
// IntervalSec == 0 legitimately).
func (e Entry) usesInterval() bool { return e.Mode == ModeInterval || e.Mode == ModeWindow }

// safeInterval is Interval clamped to MinInterval (M196). `advance` and the
// window walker use it so a zero/negative IntervalSec — which Add rejects but a
// hand-edited or corrupt schedules.json could carry — can never make the next
// run land on `now` (or the past) and busy-loop the ticker into firing a run
// every tick. A bad value degrades to the slowest safe rate instead.
func (e Entry) safeInterval() time.Duration {
	if iv := e.Interval(); iv >= MinInterval {
		return iv
	}
	return MinInterval
}

// safeIntervalSec is safeInterval in whole seconds, for the window walker.
func (e Entry) safeIntervalSec() int64 { return int64(e.safeInterval() / time.Second) }

// Cadence renders the entry's schedule for display.
func (e Entry) Cadence() string {
	switch e.Mode {
	case ModeDaily:
		hhmm := fmt.Sprintf("%02d:%02d", e.AtMinutes/60, e.AtMinutes%60)
		out := "daily at " + hhmm
		if d := FormatDays(e.Days); d != "" {
			out = d + " at " + hhmm
		}
		if e.TZ != "" {
			out += " " + e.TZ
		}
		return out
	case ModeOnce:
		return "once at " + time.Unix(e.NextRunUnix, 0).Format("2006-01-02 15:04")
	case ModeWindow:
		w := fmt.Sprintf("every %s %02d:%02d-%02d:%02d", e.Interval(),
			e.AtMinutes/60, e.AtMinutes%60, e.EndMinutes/60, e.EndMinutes%60)
		if d := FormatDays(e.Days); d != "" {
			w += " " + d
		}
		if e.TZ != "" {
			w += " " + e.TZ
		}
		return w
	}
	return "every " + e.Interval().String()
}

// Forecast returns the next n fire times (Unix seconds) strictly after `from`,
// simulating the cadence forward — the dry-run behind `agt schedule test` (M120).
// The first entry matches the engine's current NextRunUnix when that is still in
// the future, so the forecast lines up with what the daemon will actually do; the
// rest are simulated by repeatedly advancing. A `once` schedule yields its single
// future fire (or none). Pure: no engine state, deterministic given `from`.
func (e Entry) Forecast(from time.Time, n int) []int64 {
	if n <= 0 {
		return nil
	}
	if e.Mode == ModeOnce {
		if e.NextRunUnix > from.Unix() {
			return []int64{e.NextRunUnix}
		}
		return nil
	}
	out := make([]int64, 0, n)
	var first int64
	if e.NextRunUnix > from.Unix() {
		first = e.NextRunUnix
	} else {
		first = e.advance(from)
	}
	out = append(out, first)
	cur := time.Unix(first, 0).In(from.Location())
	for len(out) < n {
		t := e.advance(cur)
		if t <= cur.Unix() {
			break // no forward progress (defensive; shouldn't happen for valid entries)
		}
		out = append(out, t)
		cur = time.Unix(t, 0).In(from.Location())
	}
	return out
}

// advance computes the next-run time after firing at now. Wall-clock cadences
// (daily/window) are evaluated in the entry's zone so "09:00" means 09:00 there;
// an empty TZ leaves now in whatever zone the caller passed (the daemon local).
func (e Entry) advance(now time.Time) int64 {
	n, _ := applyZone(now, e.TZ) // e.TZ already validated at write time
	switch e.Mode {
	case ModeDaily:
		return nextDaily(n, e.AtMinutes, e.Days).Unix()
	case ModeWindow:
		return nextWindowSlot(n, e.AtMinutes, e.EndMinutes, e.safeIntervalSec(), e.Days).Unix()
	}
	return now.Add(e.safeInterval()).Unix()
}

// applyZone returns now converted into the IANA zone tz, or now unchanged when
// tz is empty (use the caller's zone). It errors on an unloadable zone name.
func applyZone(now time.Time, tz string) (time.Time, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return now, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return now, err
	}
	return now.In(loc), nil
}

// nextWindowSlot returns the next firing instant strictly after now for a
// windowed-interval schedule: slots are start, start+interval, … up to and
// including end, on permitted weekdays. After the window closes for a day it
// jumps to the next permitted day's start. Walks by calendar date (DST-correct).
func nextWindowSlot(now time.Time, start, end int, intervalSec int64, days int) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	iv := time.Duration(intervalSec) * time.Second
	for i := 0; i < 8; i++ {
		day := time.Date(y, m, d+i, 0, 0, 0, 0, loc)
		if !dayAllowed(day.Weekday(), days) {
			continue
		}
		startT := time.Date(y, m, d+i, start/60, start%60, 0, 0, loc)
		endT := time.Date(y, m, d+i, end/60, end%60, 0, 0, loc)
		if now.Before(startT) {
			return startT
		}
		if !now.Before(endT) {
			continue // today's window has closed
		}
		// now is inside [startT, endT): next aligned slot strictly after now.
		k := now.Sub(startT)/iv + 1
		slot := startT.Add(k * iv)
		if !slot.After(endT) {
			return slot
		}
		// no slot left today before end → fall through to the next permitted day
	}
	return now.Add(iv) // unreachable for a valid window
}

// dayAllowed reports whether wd is permitted by the day-mask. A zero mask (or
// AllDays) permits every day.
func dayAllowed(wd time.Weekday, days int) bool {
	if days == 0 || days == AllDays {
		return true
	}
	return days&(1<<uint(wd)) != 0
}

// nextDaily returns the next local-time occurrence of atMinutes-past-midnight,
// strictly after now, that falls on a weekday permitted by days. It walks
// forward by calendar date (not by adding 24h) so it stays correct across DST
// transitions.
func nextDaily(now time.Time, atMinutes, days int) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	nowMin := now.Hour()*60 + now.Minute()
	for i := 0; i < 8; i++ {
		cand := time.Date(y, m, d+i, atMinutes/60, atMinutes%60, 0, 0, loc)
		if !cand.After(now) {
			continue
		}
		// DST fall-back guard (M197): on a fall-back day the wall-clock atMinutes
		// occurs twice (e.g. 01:30 happens at both the DST and standard offset). The
		// second occurrence is After(now) yet shares the just-fired now's wall clock,
		// so without this guard the daily schedule fires AGAIN ~1h later. For today
		// (i==0) require the slot to be strictly later in the day than now; the fold
		// re-entry (same minutes-since-midnight) is rejected and we move to the next
		// permitted day. In normal time this rejects nothing real — a same/earlier
		// today slot already fails cand.After(now).
		if i == 0 && atMinutes <= nowMin {
			continue
		}
		if dayAllowed(cand.Weekday(), days) {
			return cand
		}
	}
	return time.Date(y, m, d+1, atMinutes/60, atMinutes%60, 0, 0, loc) // unreachable for any non-empty mask
}

var dayAbbr = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// Weekday bitmask shortcuts (over time.Weekday: Sunday=0 .. Saturday=6).
const (
	maskWeekdays = 1<<int(time.Monday) | 1<<int(time.Tuesday) | 1<<int(time.Wednesday) | 1<<int(time.Thursday) | 1<<int(time.Friday)
	maskWeekends = 1<<int(time.Sunday) | 1<<int(time.Saturday)
)

// FormatDays renders a weekday bitmask compactly ("" for every day, "Mon-Fri",
// "Sat,Sun", or "Mon,Wed,Fri").
func FormatDays(days int) string {
	if days == 0 || days == AllDays {
		return ""
	}
	switch days {
	case maskWeekdays:
		return "Mon-Fri"
	case maskWeekends:
		return "Sat,Sun"
	}
	var names []string
	for wd := 0; wd < 7; wd++ {
		if days&(1<<uint(wd)) != 0 {
			names = append(names, dayAbbr[wd])
		}
	}
	return strings.Join(names, ",")
}

// dayTokens maps the accepted weekday spellings to their time.Weekday index.
var dayTokens = map[string]int{
	"sun": 0, "sunday": 0,
	"mon": 1, "monday": 1,
	"tue": 2, "tues": 2, "tuesday": 2,
	"wed": 3, "weds": 3, "wednesday": 3,
	"thu": 4, "thur": 4, "thurs": 4, "thursday": 4,
	"fri": 5, "friday": 5,
	"sat": 6, "saturday": 6,
}

// ParseDays parses a day specification into a weekday bitmask. It accepts the
// shortcuts "daily"/"everyday"/"all" (every day), "weekdays", "weekends", a
// comma-separated list ("mon,wed,fri"), and inclusive ranges ("mon-fri",
// wrapping like "fri-mon"). Day names are case-insensitive. An empty/"daily"
// spec yields 0 (every day).
func ParseDays(spec string) (int, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	switch spec {
	case "", "daily", "everyday", "every-day", "all":
		return 0, nil
	case "weekdays", "weekday":
		return maskWeekdays, nil
	case "weekends", "weekend":
		return maskWeekends, nil
	}
	mask := 0
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			loIdx, ok1 := dayTokens[strings.TrimSpace(lo)]
			hiIdx, ok2 := dayTokens[strings.TrimSpace(hi)]
			if !ok1 || !ok2 {
				return 0, fmt.Errorf("cadence: bad day range %q", part)
			}
			// Inclusive, wrapping (e.g. fri-mon = Fri,Sat,Sun,Mon).
			for d := loIdx; ; d = (d + 1) % 7 {
				mask |= 1 << uint(d)
				if d == hiIdx {
					break
				}
			}
			continue
		}
		idx, ok := dayTokens[part]
		if !ok {
			return 0, fmt.Errorf("cadence: unknown day %q", part)
		}
		mask |= 1 << uint(idx)
	}
	if mask == 0 {
		return 0, fmt.Errorf("cadence: no valid days in %q", spec)
	}
	return mask, nil
}

// Job is an interval+intent pair parsed from AGEZT_SCHEDULE (see ParseJobs),
// used to seed env-sourced entries into the store.
type Job struct {
	Interval time.Duration
	Intent   string
	Model    string
}

// --- Store ---

// Store is the persistent set of schedules, written as a single JSON file
// rewritten atomically on change. It is safe for concurrent use.
type Store struct {
	path    string
	mu      sync.Mutex
	entries []*Entry
}

// OpenStore opens (or creates) the schedule store under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cadence: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "schedules.json")}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("cadence: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.entries); err != nil {
			return nil, fmt.Errorf("cadence: parse %s: %w", s.path, err)
		}
		// Repair against a corrupt or hand-edited file (M196): an interval or
		// window entry with a sub-minimum IntervalSec would advance the next run
		// onto `now`/the past and busy-loop the ticker. Clamp to MinInterval so a
		// bad value degrades to the slowest safe rate. `advance` floors defensively
		// too, but repairing here makes the clamp durable and visible in
		// `agt schedule list`.
		for i := range s.entries {
			if s.entries[i].usesInterval() && s.entries[i].Interval() < MinInterval {
				s.entries[i].IntervalSec = int64(MinInterval / time.Second)
			}
		}
	}
	return s, nil
}

// Add creates an enabled entry firing every interval, first run one interval
// from now. source is SourceOperator or SourceEnv.
func (s *Store) Add(intent string, interval time.Duration, model, source string, now time.Time) (Entry, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return Entry{}, fmt.Errorf("cadence: intent is required")
	}
	if interval < MinInterval {
		return Entry{}, fmt.Errorf("cadence: interval %s is below the %s minimum", interval, MinInterval)
	}
	if source == "" {
		source = SourceOperator
	}
	e := &Entry{
		ID:          "sched-" + ulid.New(),
		Intent:      intent,
		IntervalSec: int64(interval / time.Second),
		Model:       strings.TrimSpace(model),
		Source:      source,
		Enabled:     true,
		CreatedUnix: now.Unix(),
		NextRunUnix: now.Add(interval).Unix(),
	}
	s.mu.Lock()
	s.entries = append(s.entries, e)
	err := s.save()
	s.mu.Unlock()
	if err != nil {
		return Entry{}, err
	}
	return *e, nil
}

// AddDaily creates an enabled entry firing once a day at atMinutes minutes past
// local midnight (0..1439), first run at the next such time. days restricts the
// schedule to certain weekdays (a time.Weekday bitmask); 0 or AllDays = every
// day.
func (s *Store) AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (Entry, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return Entry{}, fmt.Errorf("cadence: intent is required")
	}
	if atMinutes < 0 || atMinutes > 1439 {
		return Entry{}, fmt.Errorf("cadence: time-of-day must be 00:00..23:59")
	}
	if days < 0 || days > AllDays {
		return Entry{}, fmt.Errorf("cadence: day-mask must be 0..%d", AllDays)
	}
	zoned, err := applyZone(now, tz)
	if err != nil {
		return Entry{}, fmt.Errorf("cadence: unknown timezone %q: %w", tz, err)
	}
	if source == "" {
		source = SourceOperator
	}
	e := &Entry{
		ID:          "sched-" + ulid.New(),
		Intent:      intent,
		Mode:        ModeDaily,
		AtMinutes:   atMinutes,
		Days:        days,
		TZ:          strings.TrimSpace(tz),
		Model:       strings.TrimSpace(model),
		Source:      source,
		Enabled:     true,
		CreatedUnix: now.Unix(),
		NextRunUnix: nextDaily(zoned, atMinutes, days).Unix(),
	}
	s.mu.Lock()
	s.entries = append(s.entries, e)
	err = s.save()
	s.mu.Unlock()
	if err != nil {
		return Entry{}, err
	}
	return *e, nil
}

// AddOnce creates an enabled one-shot entry that fires exactly once at the
// wall-clock instant at (which must be in the future) and then removes itself.
// It is the reminder/at-job primitive ("at 14:00 today, summarise the deploy").
func (s *Store) AddOnce(intent string, at time.Time, model, source string, now time.Time) (Entry, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return Entry{}, fmt.Errorf("cadence: intent is required")
	}
	if !at.After(now) {
		return Entry{}, fmt.Errorf("cadence: one-shot time must be in the future")
	}
	if source == "" {
		source = SourceOperator
	}
	e := &Entry{
		ID:          "sched-" + ulid.New(),
		Intent:      intent,
		Mode:        ModeOnce,
		Model:       strings.TrimSpace(model),
		Source:      source,
		Enabled:     true,
		CreatedUnix: now.Unix(),
		NextRunUnix: at.Unix(),
	}
	s.mu.Lock()
	s.entries = append(s.entries, e)
	err := s.save()
	s.mu.Unlock()
	if err != nil {
		return Entry{}, err
	}
	return *e, nil
}

// SetEnabled enables or disables an entry (pause/resume without deleting).
// Returns whether the entry exists.
func (s *Store) SetEnabled(id string, enabled bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Enabled = enabled
			return true, s.save()
		}
	}
	return false, nil
}

// AddWindow creates an enabled windowed-interval entry: fire every interval, but
// only within the daily time window [startMin, endMin] (minutes since local
// midnight) on permitted weekdays. days==0/AllDays = every day.
func (s *Store) AddWindow(intent string, interval time.Duration, startMin, endMin, days int, tz, model, source string, now time.Time) (Entry, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return Entry{}, fmt.Errorf("cadence: intent is required")
	}
	if err := validateWindow(interval, startMin, endMin, days); err != nil {
		return Entry{}, err
	}
	zoned, err := applyZone(now, tz)
	if err != nil {
		return Entry{}, fmt.Errorf("cadence: unknown timezone %q: %w", tz, err)
	}
	if source == "" {
		source = SourceOperator
	}
	e := &Entry{
		ID:          "sched-" + ulid.New(),
		Intent:      intent,
		Mode:        ModeWindow,
		IntervalSec: int64(interval / time.Second),
		AtMinutes:   startMin,
		EndMinutes:  endMin,
		Days:        days,
		TZ:          strings.TrimSpace(tz),
		Model:       strings.TrimSpace(model),
		Source:      source,
		Enabled:     true,
		CreatedUnix: now.Unix(),
		NextRunUnix: nextWindowSlot(zoned, startMin, endMin, int64(interval/time.Second), days).Unix(),
	}
	s.mu.Lock()
	s.entries = append(s.entries, e)
	err = s.save()
	s.mu.Unlock()
	if err != nil {
		return Entry{}, err
	}
	return *e, nil
}

// validateWindow checks the shared constraints for windowed schedules.
func validateWindow(interval time.Duration, startMin, endMin, days int) error {
	if interval < MinInterval {
		return fmt.Errorf("cadence: interval %s is below the %s minimum", interval, MinInterval)
	}
	if startMin < 0 || startMin > 1439 || endMin < 0 || endMin > 1439 {
		return fmt.Errorf("cadence: window bounds must be 00:00..23:59")
	}
	if endMin <= startMin {
		return fmt.Errorf("cadence: window end must be after its start")
	}
	if days < 0 || days > AllDays {
		return fmt.Errorf("cadence: day-mask must be 0..%d", AllDays)
	}
	return nil
}

// SetIntent changes an entry's intent in place. Returns whether it exists.
func (s *Store) SetIntent(id, intent string) (bool, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return false, fmt.Errorf("cadence: intent is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Intent = intent
			return true, s.save()
		}
	}
	return false, nil
}

// SetModel changes an entry's model in place (empty clears it). Returns whether
// it exists.
func (s *Store) SetModel(id, model string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Model = strings.TrimSpace(model)
			return true, s.save()
		}
	}
	return false, nil
}

// Reschedule replaces an entry's cadence in place (preserving id/source/created/
// enabled), recomputing its next-run time. mode selects which of the cadence
// parameters apply: ModeOnce → onceAt; ModeDaily → atMinutes+days; ModeInterval
// → interval. Returns whether the entry exists.
func (s *Store) Reschedule(id, mode string, interval time.Duration, atMinutes, endMinutes, days int, tz string, onceAt, now time.Time) (bool, error) {
	zoned, err := applyZone(now, tz)
	if err != nil {
		return false, fmt.Errorf("cadence: unknown timezone %q: %w", tz, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID != id {
			continue
		}
		switch mode {
		case ModeOnce:
			if !onceAt.After(now) {
				return false, fmt.Errorf("cadence: one-shot time must be in the future")
			}
			e.Mode = ModeOnce
			e.IntervalSec, e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, 0, ""
			e.NextRunUnix = onceAt.Unix()
		case ModeDaily:
			if atMinutes < 0 || atMinutes > 1439 {
				return false, fmt.Errorf("cadence: time-of-day must be 00:00..23:59")
			}
			if days < 0 || days > AllDays {
				return false, fmt.Errorf("cadence: day-mask must be 0..%d", AllDays)
			}
			e.Mode = ModeDaily
			e.IntervalSec, e.EndMinutes = 0, 0
			e.AtMinutes, e.Days, e.TZ = atMinutes, days, strings.TrimSpace(tz)
			e.NextRunUnix = nextDaily(zoned, atMinutes, days).Unix()
		case ModeWindow:
			if err := validateWindow(interval, atMinutes, endMinutes, days); err != nil {
				return false, err
			}
			e.Mode = ModeWindow
			e.IntervalSec = int64(interval / time.Second)
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = atMinutes, endMinutes, days, strings.TrimSpace(tz)
			e.NextRunUnix = nextWindowSlot(zoned, atMinutes, endMinutes, e.IntervalSec, days).Unix()
		default: // ModeInterval
			if interval < MinInterval {
				return false, fmt.Errorf("cadence: interval %s is below the %s minimum", interval, MinInterval)
			}
			e.Mode = ModeInterval
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, ""
			e.IntervalSec = int64(interval / time.Second)
			e.NextRunUnix = now.Add(interval).Unix()
		}
		return true, s.save()
	}
	return false, nil
}

// Remove deletes the entry with id; returns whether one was removed.
func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// List returns a copy of all entries, sorted by creation time.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedUnix < out[j].CreatedUnix })
	return out
}

// Get returns the entry with id.
func (s *Store) Get(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			return *e, true
		}
	}
	return Entry{}, false
}

// RunNow marks the entry due immediately (the next tick fires it). Returns
// whether the entry exists.
func (s *Store) RunNow(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.NextRunUnix = 0 // epoch → always due
			e.Enabled = true
			return true, s.save()
		}
	}
	return false, nil
}

// Due returns the entries whose next-run time has arrived. Disabled entries are
// never due.
//
// Recurring entries (interval/daily/window) advance eagerly: their NextRunUnix is
// moved to the next slot and persisted here, before the run launches. A crash
// during the run therefore SKIPS that one slot (at-most-once), which self-corrects
// at the next slot — re-running a stale recurring slot after a restart is more
// disruptive than skipping it.
//
// One-shot entries (ModeOnce) are crash-safe (at-least-once): Due returns them but
// does NOT remove or advance them. The entry stays in the store, enabled and due,
// until its run COMPLETES, at which point the engine calls CompleteFiring to remove
// it (M199). So a crash mid-run leaves the one-shot in place to re-fire on restart
// instead of silently vanishing. The engine's in-flight guard (the running map)
// prevents a duplicate fire across ticks while the single run is still going.
func (s *Store) Due(now time.Time) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Entry
	changed := false
	for _, e := range s.entries {
		if !e.Enabled {
			continue
		}
		if now.Unix() < e.NextRunUnix {
			continue
		}
		if e.Mode == ModeOnce {
			// Crash-safe one-shot: leave it untouched; removal is deferred to
			// CompleteFiring after the run finishes.
			due = append(due, *e)
			continue
		}
		e.LastRunUnix = now.Unix()
		e.NextRunUnix = e.advance(now)
		changed = true
		due = append(due, *e)
	}
	if changed {
		_ = s.save()
	}
	return due
}

// CompleteFiring is called by the engine after a fired entry's run has finished.
// For a one-shot (ModeOnce) it removes the entry from the store and persists — this
// is what makes one-shots crash-safe: the entry survives in the store (enabled and
// due) for the entire duration of its run, so a crash before completion re-fires it
// on restart, and it is removed only once the run has actually run to completion
// (whether it succeeded or errored, so a permanently-failing one-shot cannot
// retry-storm). For recurring entries this is a no-op: Due already advanced them.
// Returns whether an entry was removed.
func (s *Store) CompleteFiring(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID != id {
			continue
		}
		if e.Mode != ModeOnce {
			return false, nil
		}
		s.entries = append(s.entries[:i], s.entries[i+1:]...)
		return true, s.save()
	}
	return false, nil
}

// SyncEnv replaces all SourceEnv entries with the given jobs (idempotent across
// restarts): operator-managed entries are untouched, and removing a job from
// AGEZT_SCHEDULE removes its entry on the next start.
func (s *Store) SyncEnv(jobs []Job, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0:0]
	for _, e := range s.entries {
		if e.Source != SourceEnv {
			kept = append(kept, e)
		}
	}
	for _, j := range jobs {
		iv := j.Interval
		if iv < MinInterval {
			iv = MinInterval
		}
		kept = append(kept, &Entry{
			ID:          "sched-" + ulid.New(),
			Intent:      strings.TrimSpace(j.Intent),
			IntervalSec: int64(iv / time.Second),
			Model:       strings.TrimSpace(j.Model),
			Source:      SourceEnv,
			Enabled:     true,
			CreatedUnix: now.Unix(),
			NextRunUnix: now.Add(iv).Unix(),
		})
	}
	s.entries = kept
	return s.save()
}

// Count returns the number of entries.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// save writes the entries atomically (temp file + rename). Caller holds s.mu.
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// --- Engine ---

// RunFunc executes one scheduled intent through the governed loop. The engine
// calls it on its own goroutine; a returned error is logged, not fatal. id is
// the firing schedule's entry id (M55) so the caller can attribute the run to
// its schedule (e.g. stamp it on the schedule.fired event).
type RunFunc func(ctx context.Context, id, intent, model string) error

// Engine fires the store's due entries on a timer.
type Engine struct {
	store *Store
	run   RunFunc
	res   time.Duration
	log   io.Writer

	running sync.Map // entry ID -> struct{} while a run is in flight
	mu      sync.Mutex
	started bool
}

// NewEngine builds an Engine over a store. resolution <= 0 uses
// DefaultResolution. log receives one line per firing (nil = discard).
func NewEngine(store *Store, run RunFunc, resolution time.Duration, log io.Writer) *Engine {
	if log == nil {
		log = io.Discard
	}
	res := resolution
	if res <= 0 {
		res = DefaultResolution
	}
	return &Engine{store: store, run: run, res: res, log: log}
}

// Start runs the ticker until ctx is done. It returns immediately; the loop runs
// on its own goroutine.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.mu.Unlock()

	go func() {
		t := time.NewTicker(e.res)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.fireDue(ctx, time.Now())
			}
		}
	}()
}

// fireDue launches every due entry that is not already running. Tested directly
// with a controlled clock (no ticker, no flakiness).
func (e *Engine) fireDue(ctx context.Context, now time.Time) {
	for _, entry := range e.store.Due(now) {
		if _, busy := e.running.LoadOrStore(entry.ID, struct{}{}); busy {
			fmt.Fprintf(e.log, "schedule: skip %q (previous run still in progress)\n", short(entry.Intent))
			continue
		}
		ent := entry
		go func() {
			defer e.running.Delete(ent.ID)
			fmt.Fprintf(e.log, "schedule: firing %q (%s)\n", short(ent.Intent), ent.Cadence())
			if err := e.run(ctx, ent.ID, ent.Intent, ent.Model); err != nil {
				fmt.Fprintf(e.log, "schedule: %q failed: %v\n", short(ent.Intent), err)
			}
			// A one-shot is removed only after its run completes, so a crash mid-run
			// leaves it in the store to re-fire on restart (M199). This runs before
			// the deferred running.Delete, so no tick can re-fire it in the gap
			// between removal and clearing the in-flight guard.
			if _, err := e.store.CompleteFiring(ent.ID); err != nil {
				fmt.Fprintf(e.log, "schedule: completing %q failed: %v\n", short(ent.Intent), err)
			}
		}()
	}
}

func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}

// --- env parsing ---

// ParseJobs parses the AGEZT_SCHEDULE spec: a semicolon-separated list of jobs
// (semicolon, not comma, because intents commonly contain commas), each
// "interval=intent". The interval is a Go duration (e.g. 30m, 1h, 24h); the
// intent is the rest of the entry verbatim. A malformed entry is a hard error so
// a misconfigured schedule is caught at startup, not silently dropped.
func ParseJobs(spec string) ([]Job, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var jobs []Job
	for _, raw := range strings.Split(spec, ";") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		durStr, intent, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("cadence: entry %q must be interval=intent", entry)
		}
		d, err := time.ParseDuration(strings.TrimSpace(durStr))
		if err != nil {
			return nil, fmt.Errorf("cadence: bad interval %q: %w", durStr, err)
		}
		if d < MinInterval {
			return nil, fmt.Errorf("cadence: interval %s is below the %s minimum", d, MinInterval)
		}
		intent = strings.TrimSpace(intent)
		if intent == "" {
			return nil, fmt.Errorf("cadence: entry %q has an empty intent", entry)
		}
		jobs = append(jobs, Job{Interval: d, Intent: intent})
	}
	return jobs, nil
}

// Describe renders a one-line banner summary of the store's entries.
func Describe(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s → %q", e.Cadence(), short(e.Intent)))
	}
	return fmt.Sprintf("%d schedule(s): %s", len(entries), strings.Join(parts, ", "))
}
