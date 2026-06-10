// SPDX-License-Identifier: MIT

package workflow

// The trigger runner (M799): arms enabled workflows' cron and event
// triggers. Mirrors kernel/standing's runner shape — the daemon injects a
// FireFunc (a closure over Kernel.RunWorkflow) and the runner decides WHEN
// to call it. Event triggers ride a single bus subscription with glob
// matching and a per-workflow cooldown (event storms must not launch run
// floods); workflow.* events are never trigger sources (validation also
// refuses such subjects — defense in both layers). Cron triggers tick on a
// coarse clock: interval_sec anchored at arm time, daily_at fired once per
// local day. Trigger state is in-memory: a daemon restart re-arms cleanly.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// FireFunc executes one triggered workflow. payload becomes
// {{trigger.payload}}; reason is a short human label ("cron interval",
// "event task.failed") for the daemon's logging.
type FireFunc func(ctx context.Context, w Workflow, payload any, reason string)

// RunnerConfig tunes the trigger runner. Zero values mean defaults.
type RunnerConfig struct {
	// EventCooldown is the minimum gap between event-triggered fires of ONE
	// workflow (default 30s).
	EventCooldown time.Duration
	// Tick is the cron resolution (default 15s).
	Tick time.Duration
	// Now is the clock (default time.Now) — injectable for tests.
	Now func() time.Time
}

// StartTriggers arms cron + event triggers for every ENABLED workflow in
// store, firing through fire. It returns after spawning its goroutines;
// ctx cancellation stops both. The store is consulted live on every tick /
// event, so saves, enables, and removes take effect without a restart.
func StartTriggers(ctx context.Context, b *bus.Bus, store *Store, cfg RunnerConfig, fire FireFunc) error {
	if cfg.EventCooldown <= 0 {
		cfg.EventCooldown = 30 * time.Second
	}
	if cfg.Tick <= 0 {
		cfg.Tick = 15 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	r := &triggerRunner{
		store: store, cfg: cfg, fire: fire,
		lastEvent: map[string]time.Time{},
		lastCron:  map[string]time.Time{},
		lastDay:   map[string]string{},
		armedAt:   cfg.Now(),
	}

	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return err
	}
	go func() {
		defer sub.Cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				r.onEvent(ctx, ev)
			}
		}
	}()
	go func() {
		t := time.NewTicker(cfg.Tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.onTick(ctx)
			}
		}
	}()
	return nil
}

type triggerRunner struct {
	store *Store
	cfg   RunnerConfig
	fire  FireFunc

	mu        sync.Mutex
	lastEvent map[string]time.Time // workflow id → last event-fire
	lastCron  map[string]time.Time // workflow id → last interval-fire
	lastDay   map[string]string    // workflow id → last daily_at fire day (YYYY-MM-DD)
	armedAt   time.Time
}

// onEvent matches one journal event against every enabled event trigger.
func (r *triggerRunner) onEvent(ctx context.Context, ev *event.Event) {
	// A workflow run's own events must never become trigger fuel — the
	// feedback loop would be immediate. Validation refuses workflow.*
	// subjects too; this is the second layer.
	if strings.HasPrefix(ev.Subject, "workflow.") {
		return
	}
	now := r.cfg.Now()
	for _, w := range r.store.List() {
		if !w.Enabled {
			continue
		}
		spec := w.TriggerSpec()
		if spec.Kind != "event" || !SubjectMatch(spec.Subject, ev.Subject) {
			continue
		}
		r.mu.Lock()
		last := r.lastEvent[w.ID]
		if now.Sub(last) < r.cfg.EventCooldown {
			r.mu.Unlock()
			continue
		}
		r.lastEvent[w.ID] = now
		r.mu.Unlock()

		payload := map[string]any{
			"kind":    "event",
			"subject": ev.Subject,
			"event":   string(ev.Kind),
		}
		if len(ev.Payload) > 0 {
			var body any
			if err := json.Unmarshal(ev.Payload, &body); err == nil {
				payload["data"] = body
			}
		}
		wf := w
		go r.fire(ctx, wf, payload, "event "+ev.Subject)
	}
}

// onTick walks enabled cron triggers: interval anchored at arm time (first
// fire one full interval after the daemon starts), daily_at once per local
// day when the clock has passed HH:MM.
func (r *triggerRunner) onTick(ctx context.Context) {
	now := r.cfg.Now()
	for _, w := range r.store.List() {
		if !w.Enabled {
			continue
		}
		spec := w.TriggerSpec()
		if spec.Kind != "cron" {
			continue
		}
		switch {
		case spec.IntervalSec > 0:
			interval := time.Duration(spec.IntervalSec) * time.Second
			r.mu.Lock()
			last := r.lastCron[w.ID]
			if last.IsZero() {
				last = r.armedAt
			}
			due := now.Sub(last) >= interval
			if due {
				r.lastCron[w.ID] = now
			}
			r.mu.Unlock()
			if due {
				wf := w
				go r.fire(ctx, wf, map[string]any{"kind": "cron", "fired_at": now.Format(time.RFC3339)}, "cron interval")
			}
		case spec.DailyAt != "":
			hhmm := now.Format("15:04")
			day := now.Format("2006-01-02")
			if hhmm < strings.TrimSpace(spec.DailyAt) {
				continue
			}
			r.mu.Lock()
			already := r.lastDay[w.ID] == day
			if !already {
				r.lastDay[w.ID] = day
			}
			r.mu.Unlock()
			if !already {
				wf := w
				go r.fire(ctx, wf, map[string]any{"kind": "cron", "fired_at": now.Format(time.RFC3339)}, "cron daily "+spec.DailyAt)
			}
		}
	}
}

// SubjectMatch implements bus glob semantics over dotted subjects: "*"
// matches exactly one token, a trailing ">" matches one-or-more remaining
// tokens, literals match themselves.
func SubjectMatch(pattern, subject string) bool {
	p := strings.Split(pattern, ".")
	s := strings.Split(subject, ".")
	for i, tok := range p {
		if tok == ">" {
			return i == len(p)-1 && len(s) > i
		}
		if i >= len(s) {
			return false
		}
		if tok != "*" && tok != s[i] {
			return false
		}
	}
	return len(p) == len(s)
}
