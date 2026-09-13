// SPDX-License-Identifier: MIT

// Cadence Entry: SystemTasks + IsSystemTask + Entry type + Validate + Interval + Cadence.
// Forecast/advance moved to cadence_forecast.go; applyZone/nextWindowSlot/dayAllowed/
// nextDaily moved to cadence_helpers.go; dayAbbr/maskWeekdays/maskWeekends/FormatDays
// doc moved to cadence_entry_days.go. Day-211 god-file split. Public API unchanged.
package cadence


import (
	"fmt"
	"strings"
	"time"

	"encoding/json"
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
