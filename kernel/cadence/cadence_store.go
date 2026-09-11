// SPDX-License-Identifier: MIT

// Cadence store: OpenStore + Add variants (Add/AddDaily/AddOnce/AddContinuous/AddWindow) + SetEnabled + validateWindow.
// Code extracted from cadence_store.go during the Day-58 god-file split. Public API unchanged.
package cadence


import (
	"fmt"
	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
	"strings"
	"time"
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
