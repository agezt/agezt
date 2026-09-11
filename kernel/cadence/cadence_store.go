// SPDX-License-Identifier: MIT

package cadence

// Store CRUD methods (Add / AddDaily / AddOnce / AddContinuous / SetEnabled
// / AddWindow / SetIntent / SetModel / SetAgent / SetIntentTarget /
// SetWorkflowTarget / SetSystemTaskTarget / SetToolTarget / Reschedule /
// Remove / List / Get / RunNow / Due / CompleteFiring / SetAssure /
// SyncEnv / Count / save). Carved out of cadence.go during the Day 30
// god file split #1.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)

func OpenStore(dir string) (*Store, error) {
	s := &Store{}
	path, err := jsonstore.LoadFrom(dir, "schedules.json", &s.entries)
	if err != nil {
		return nil, fmt.Errorf("cadence: %w", err)
	}
	s.path = path
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

// AddContinuous creates a completion-anchored continuous entry (M646): it fires
// immediately, then re-fires `cooldown` after each run COMPLETES, forever, never
// overlapping — a living, never-tiring agent. cooldown is clamped to MinInterval.
func (s *Store) AddContinuous(intent string, cooldown time.Duration, model, source string, now time.Time) (Entry, error) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return Entry{}, fmt.Errorf("cadence: intent is required")
	}
	if cooldown < MinInterval {
		cooldown = MinInterval
	}
	if source == "" {
		source = SourceOperator
	}
	e := &Entry{
		ID:          "sched-" + ulid.New(),
		Intent:      intent,
		Mode:        ModeContinuous,
		IntervalSec: int64(cooldown / time.Second),
		Model:       strings.TrimSpace(model),
		Source:      source,
		Enabled:     true,
		CreatedUnix: now.Unix(),
		NextRunUnix: now.Unix(), // due immediately — start living right away
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

// SetAgent changes the roster agent binding in place (empty clears it). Returns
// whether the entry exists. The caller validates that a non-empty slug exists.
func (s *Store) SetAgent(id, agent string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Agent = strings.TrimSpace(agent)
			return true, s.save()
		}
	}
	return false, nil
}

// SetIntentTarget restores the historical LLM intent target and clears
// target-specific fields.
func (s *Store) SetIntentTarget(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetIntent
			e.Workflow = ""
			e.SystemTask = ""
			e.Tool = ""
			e.Payload = nil
			return true, s.save()
		}
	}
	return false, nil
}

// SetWorkflowTarget makes an entry fire a stored workflow instead of a governed
// agent/intent run. The caller validates the workflow ref exists. Agent binding
// is intentionally preserved: a workflow schedule may run under an agent's
// identity, budget, and tool policy.
func (s *Store) SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false, fmt.Errorf("cadence: workflow ref is required")
	}
	cp := append(json.RawMessage(nil), payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetWorkflow
			e.Workflow = ref
			e.SystemTask = ""
			e.Tool = ""
			e.Payload = cp
			return true, s.save()
		}
	}
	return false, nil
}

// SetSystemTaskTarget makes an entry fire a daemon maintenance task instead of
// an LLM intent. The task name is validated against the known system-task enum
// (IsSystemTask) as defense-in-depth: the control plane already validates, but
// a direct store caller (env-seeded schedule, a future code path, or a
// hand-edited schedules.json) should not be able to set an arbitrary task name
// that could smuggle unexpected behavior through the fire path.
func (s *Store) SetSystemTaskTarget(id, task string) (bool, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return false, fmt.Errorf("cadence: system task is required")
	}
	if !IsSystemTask(task) {
		return false, fmt.Errorf("cadence: unknown system task %q (valid: %s)", task, strings.Join(SystemTasks(), ", "))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetSystemTask
			e.SystemTask = task
			e.Workflow = ""
			e.Tool = ""
			e.Payload = nil
			e.Agent = ""
			e.Model = ""
			return true, s.save()
		}
	}
	return false, nil
}

// SetToolTarget makes an entry invoke a registered tool directly instead of
// asking an LLM to interpret an intent. The caller validates the tool exists.
// Agent binding is preserved so a tool schedule can run under that agent's
// permissions and spend limits; model overrides are cleared because direct
// tool invocations do not select an LLM.
func (s *Store) SetToolTarget(id, tool string, payload json.RawMessage) (bool, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return false, fmt.Errorf("cadence: tool is required")
	}
	cp := append(json.RawMessage(nil), payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.Target = TargetTool
			e.Tool = tool
			e.Workflow = ""
			e.SystemTask = ""
			e.Payload = cp
			e.Model = ""
			return true, s.save()
		}
	}
	return false, nil
}

// Reschedule replaces an entry's cadence in place (preserving id/source/created/
// enabled), recomputing its next-run time. mode selects which of the cadence
// parameters apply: ModeOnce → onceAt; ModeDaily → atMinutes+days; ModeWindow
// → interval+window; ModeContinuous → completion cooldown; ModeInterval →
// interval. Returns whether the entry exists.
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
		case ModeContinuous:
			if interval < MinInterval {
				return false, fmt.Errorf("cadence: cooldown %s is below the %s minimum", interval, MinInterval)
			}
			e.Mode = ModeContinuous
			e.AtMinutes, e.EndMinutes, e.Days, e.TZ = 0, 0, 0, ""
			e.IntervalSec = int64(interval / time.Second)
			e.NextRunUnix = now.Unix()
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
		if e.Mode == ModeOnce || e.Mode == ModeContinuous {
			// Crash-safe one-shot AND completion-anchored continuous: leave
			// NextRunUnix untouched here. The engine's in-flight guard stops a
			// second fire while the run is live, and CompleteFiring re-anchors a
			// continuous entry forward (or removes a one-shot) once it finishes.
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
func (s *Store) CompleteFiring(id string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID != id {
			continue
		}
		switch e.Mode {
		case ModeOnce:
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true, s.save()
		case ModeContinuous:
			// Completion-anchored loop: schedule the next cycle `cooldown` after
			// this run finished, so cycles never overlap and the agent runs
			// forever with a steady breather between cycles (M646). Count the cycle
			// just lived — this is the loop's heartbeat (M650).
			s.entries[i].Fires++
			s.entries[i].LastRunUnix = now.Unix()
			s.entries[i].NextRunUnix = now.Add(e.safeInterval()).Unix()
			return false, s.save()
		default:
			// Recurring (interval/daily/window): Due already advanced NextRunUnix
			// before the run; here we only record that a firing completed (M650).
			s.entries[i].Fires++
			return false, s.save()
		}
	}
	return false, nil
}

// SetAssure sets an entry's do-it-for-sure attempt budget (M654): n > 0 makes
// each firing run-verify-retry up to n times; n <= 0 clears it back to a single
// pass. Returns whether an entry with that id existed.
func (s *Store) SetAssure(id string, n int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID != id {
			continue
		}
		if n < 0 {
			n = 0
		}
		s.entries[i].Assure = n
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
	return jsonstore.Save(s.path, s.entries)
}

// --- Engine ---

// RunFunc executes one due schedule through the target dispatcher. The engine
// calls it on its own goroutine; a returned error is logged, not fatal. id is
// the firing schedule's entry id (M55) so the caller can attribute the run to
// its schedule (e.g. stamp it on the schedule.fired event). intent is kept for
// the legacy agent-task label and for backward-compatible stores.
