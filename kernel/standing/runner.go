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
}

// StartRunner wires the event-trigger half of Chronos onto the bus (SPEC-16 §4).
// It subscribes to every event and, for each, fires every enabled standing order
// whose event-trigger subject matches — subject to a per-order cooldown so an
// event storm can't launch a flood of runs. Lifecycle events (standing.*) are
// skipped so an order can never trigger itself. Returns false when it can't start
// (nil bus/store/fire). The goroutine stops on ctx cancellation or bus close; a
// panic in the loop is recovered so a runner bug never crashes the daemon.
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
				// Never let an order's own lifecycle (or another order's) trigger a
				// run — that's a self-amplifying loop the cooldown shouldn't have to
				// absorb.
				if strings.HasPrefix(ev.Subject, "standing.") {
					continue
				}
				for _, o := range store.List() {
					if !o.Enabled {
						continue
					}
					if !matchesAnyEventTrigger(o, ev.Subject) {
						continue
					}
					if ev.TSUnixMS-lastFireMS[o.ID] < cooldownMS {
						continue
					}
					lastFireMS[o.ID] = ev.TSUnixMS
					ord := o
					subj := ev.Subject
					go fire(ctx, ord, subj)
				}
			}
		}
	}()
	return true
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
