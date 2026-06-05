// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
)

// DefaultRunnerCooldown is the minimum gap between event-trigger firings of the
// SAME order, so a burst of matching events launches at most one run per window
// rather than a flood.
const DefaultRunnerCooldown = 60 * time.Second

// FireFunc launches a standing order's plan in response to a matched trigger.
// It must not block the runner loop — the daemon's implementation starts the run
// asynchronously. triggerSubject is the subject of the event that matched.
type FireFunc func(ctx context.Context, o Order, triggerSubject string)

// RunnerConfig tunes the event-trigger runner.
type RunnerConfig struct {
	// Cooldown is the per-order minimum gap between firings. <=0 uses the default.
	Cooldown time.Duration
	// Now is the clock used for the cooldown. nil → time.Now. Injectable for tests.
	Now func() time.Time
}

// StartRunner wires the event-trigger half of Chronos onto the bus (SPEC-16 §4).
// It subscribes to every event and, for each, fires every enabled standing order
// whose event-trigger subject matches — subject to a per-order cooldown so an
// event storm can't launch a flood of runs. The order's OWN lifecycle events
// (standing.*) are skipped so a fire can't immediately re-trigger; a run's other
// downstream events (task.*/tool.*/…) can still re-match a broadly-subscribed
// order, but only after the cooldown elapses (so it's rate-bounded, not a tight
// loop). Returns false when it can't start (nil bus/store/fire). The goroutine
// stops on ctx cancellation or bus close; a panic in the loop is recovered so a
// runner bug never crashes the daemon.
//
// Cron triggers are handled separately (they reuse kernel/cadence); this runner
// is only the event-driven path.
func StartRunner(ctx context.Context, b *bus.Bus, store *Store, cfg RunnerConfig, fire FireFunc) bool {
	if b == nil || store == nil || fire == nil {
		return false
	}
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = DefaultRunnerCooldown
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return false
	}
	lastFireMS := map[string]int64{} // order id → last fire time; runner-goroutine-only
	go func() {
		defer func() {
			sub.Cancel()
			_ = recover()
		}()
		cooldownMS := cooldown.Milliseconds()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if strings.HasPrefix(ev.Subject, "standing.") {
					continue
				}
				// Cooldown keys off the LOCAL monotonic clock, never the event's own
				// TSUnixMS — an externally-sourced (webhook/mesh) event could carry a
				// skewed/far-future timestamp that would otherwise permanently
				// suppress or prematurely release the order.
				nowMS := now().UnixMilli()
				for _, o := range store.List() {
					if !o.Enabled {
						continue
					}
					if !matchesAnyEventTrigger(o, ev.Subject) {
						continue
					}
					if last, ok := lastFireMS[o.ID]; ok && nowMS-last < cooldownMS {
						continue
					}
					lastFireMS[o.ID] = nowMS
					ord := o
					subj := ev.Subject
					go fire(ctx, ord, subj)
				}
			}
		}
	}()
	return true
}

// ScopedIntent grounds a fired order's run in what the order watches (SPEC-16 §4
// scope.entities): when the order names scope entities, it prefixes the intent
// with a one-line scope note so the agent knows the subject it is acting on. No
// scope entities → the intent is returned unchanged.
func ScopedIntent(o Order, intent string) string {
	if len(o.ScopeEntities) == 0 {
		return intent
	}
	return "Scope (what this standing order watches): " + strings.Join(o.ScopeEntities, ", ") + ".\n\n" + intent
}

// BriefText formats the briefing an order sends after a run, and reports whether
// a briefing should be sent at all (SPEC-16 §4). A briefing is sent only when the
// order names a channel AND the run produced a non-empty answer — an empty result
// is nothing to report. The text is prefixed with the order name so the operator
// knows which standing goal spoke.
func BriefText(o Order, answer string) (text string, ok bool) {
	if strings.TrimSpace(o.BriefingChan) == "" || strings.TrimSpace(answer) == "" {
		return "", false
	}
	return "[standing: " + o.Name + "]\n" + answer, true
}

// matchesAnyEventTrigger reports whether subject matches any of the order's event
// triggers (NATS-style wildcards). Cron triggers are ignored here.
func matchesAnyEventTrigger(o Order, subject string) bool {
	for _, t := range o.Triggers {
		if t.Type == TriggerEvent && bus.MatchSubject(t.Subject, subject) {
			return true
		}
	}
	return false
}
