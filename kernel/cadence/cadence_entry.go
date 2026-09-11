// SPDX-License-Identifier: MIT

package cadence

// Entry methods + system-tasks + schedule helpers (SystemTasks /
// SystemTaskInfos / IsSystemTask / Entry Validate/Interval/Cadence/Forecast/
// advance / applyZone / nextWindowSlot / dayAllowed / nextDaily /
// FormatDays / ParseDays). Carved out of cadence.go during the Day 30
// god file split #1.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func SystemTasks() []string {
	out := make([]string, 0, len(systemTaskInfos))
	for _, task := range systemTaskInfos {
		out = append(out, task.Name)
	}
	return out
}

func SystemTaskInfos() []SystemTaskInfo {
	return append([]SystemTaskInfo(nil), systemTaskInfos...)
}

func IsSystemTask(task string) bool {
	task = strings.TrimSpace(task)
	for _, known := range systemTaskInfos {
		if task == known.Name {
			return true
		}
	}
	return false
}

// Scheduling modes. The zero value ("") is ModeInterval for backward
// compatibility with stores written before daily scheduling existed.
const (
	ModeInterval = "" // fire every IntervalSec seconds
	ModeDaily    = "daily"
	ModeOnce     = "once"   // fire exactly once at NextRunUnix, then self-remove
	ModeWindow   = "window" // fire every IntervalSec, but only within a daily time window
	// ModeContinuous is a completion-anchored loop (M646): the entry fires, and
	// once its run COMPLETES it re-anchors NextRunUnix to (completion + cooldown)
	// — so it runs again `cooldown` after each cycle ends, never overlapping (the
	// engine's in-flight guard), forever. A living, never-tiring agent. IntervalSec
	// carries the cooldown.
	ModeContinuous = "continuous"
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
	ID          string          `json:"id"`
	Intent      string          `json:"intent"`
	Mode        string          `json:"mode,omitempty"`
	IntervalSec int64           `json:"interval_sec,omitempty"`
	AtMinutes   int             `json:"at_minutes,omitempty"`  // daily/window: minutes since local midnight (window: start)
	EndMinutes  int             `json:"end_minutes,omitempty"` // window: window end, minutes since local midnight
	Days        int             `json:"days,omitempty"`        // daily/window: weekday bitmask (0/AllDays = every day)
	TZ          string          `json:"tz,omitempty"`          // daily/window: IANA zone for the wall-clock time (empty = daemon local)
	Model       string          `json:"model,omitempty"`
	Agent       string          `json:"agent,omitempty"` // optional roster slug to run this firing AS
	Target      string          `json:"target,omitempty"`
	Workflow    string          `json:"workflow,omitempty"` // workflow ref/name when TargetWorkflow
	SystemTask  string          `json:"system_task,omitempty"`
	Tool        string          `json:"tool,omitempty"`    // registered tool name when TargetTool
	Payload     json.RawMessage `json:"payload,omitempty"` // workflow trigger payload
	Source      string          `json:"source"`
	Enabled     bool            `json:"enabled"`
	CreatedUnix int64           `json:"created_unix"`
	LastRunUnix int64           `json:"last_run_unix,omitempty"`
	NextRunUnix int64           `json:"next_run_unix"`
	// Fires counts completed firings — the heartbeat of the entry. For a
	// continuous loop it is the number of cycles the loop has lived through; for
	// a recurring entry, how many times it has run. Incremented once per run at
	// CompleteFiring (after the run finishes), so it never double-counts an
	// in-flight cycle.
	Fires int64 `json:"fires,omitempty"`
	// Assure, when > 0, makes each firing "do-it-for-sure": the firing runs, a
	// verifier checks the task was actually accomplished, and it retries the gap
	// up to this many attempts (M654). 0 = a single pass (the default). The fire
	// path (cmd/agezt) reads this to choose RunAssured vs RunWith.
	Assure int `json:"assure,omitempty"`
}

// Validate checks that the entry's target-type fields are internally consistent:
// exactly one target type is active, conflicting fields are cleared, and typed
// targets carry the required identifier. Returns nil when valid, an error
// describing the first inconsistency otherwise. Pure; no store access.
//
// This is defense-in-depth: the control plane validates at schedule-add time,
// but a hand-edited schedules.json or a future code path that bypasses the
// control plane should not produce an entry that silently misfires. The fire
// path (cmd/agezt) also guards against unknown targets at runtime.
func (e Entry) Validate() error {
	targetCount := 0
	if e.Workflow != "" {
		targetCount++
	}
	if e.SystemTask != "" {
		targetCount++
	}
	if e.Tool != "" {
		targetCount++
	}
	// TargetIntent (e.Target == "") with a non-empty typed field is suspicious:
	// either a partial edit or a corrupt entry. Allow it only if no typed field
	// is set (the historical intent-only entry).
	if e.Target == "" && targetCount > 0 {
		return fmt.Errorf("cadence: entry %s has target=intent but typed fields are set (workflow=%q system_task=%q tool=%q)", e.ID, e.Workflow, e.SystemTask, e.Tool)
	}
	switch e.Target {
	case TargetWorkflow:
		if e.Workflow == "" {
			return fmt.Errorf("cadence: entry %s has target=workflow but workflow ref is empty", e.ID)
		}
		if e.SystemTask != "" || e.Tool != "" {
			return fmt.Errorf("cadence: entry %s has target=workflow but system_task or tool is also set", e.ID)
		}
	case TargetSystemTask:
		if e.SystemTask == "" {
			return fmt.Errorf("cadence: entry %s has target=system_task but system_task is empty", e.ID)
		}
		if !IsSystemTask(e.SystemTask) {
			return fmt.Errorf("cadence: entry %s has target=system_task but %q is not a known system task", e.ID, e.SystemTask)
		}
		if e.Workflow != "" || e.Tool != "" || e.Agent != "" || e.Model != "" {
			return fmt.Errorf("cadence: entry %s has target=system_task but workflow/tool/agent/model is also set", e.ID)
		}
		if len(e.Payload) > 0 {
			return fmt.Errorf("cadence: entry %s has target=system_task but payload is set (system tasks do not accept payloads)", e.ID)
		}
	case TargetTool:
		if e.Tool == "" {
			return fmt.Errorf("cadence: entry %s has target=tool but tool is empty", e.ID)
		}
		if e.Workflow != "" || e.SystemTask != "" {
			return fmt.Errorf("cadence: entry %s has target=tool but workflow or system_task is also set", e.ID)
		}
	case TargetIntent:
		// Intent target: typed fields must all be clear.
		if targetCount > 0 {
			return fmt.Errorf("cadence: entry %s has target=intent but typed fields are set", e.ID)
		}
	default:
		return fmt.Errorf("cadence: entry %s has unknown target %q", e.ID, e.Target)
	}
	return nil
}

// Interval is the entry's firing period (interval mode only).
func (e Entry) Interval() time.Duration { return time.Duration(e.IntervalSec) * time.Second }

// usesInterval reports whether the entry's IntervalSec is load-bearing — true
// for interval and windowed-interval modes, false for daily/once (which carry
// IntervalSec == 0 legitimately).
func (e Entry) usesInterval() bool {
	return e.Mode == ModeInterval || e.Mode == ModeWindow || e.Mode == ModeContinuous
}

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
	case ModeContinuous:
		return "continuous · " + e.safeInterval().String() + " cooldown"
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
