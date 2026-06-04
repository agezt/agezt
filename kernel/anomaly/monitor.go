// SPDX-License-Identifier: MIT

package anomaly

import (
	"context"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Config tunes the tool-call-rate circuit breaker.
type Config struct {
	// MaxToolCalls is the ceiling: more than this many tool.invoked events
	// within Window trips the breaker. <= 0 disables the monitor.
	MaxToolCalls int
	// Window is the trailing window the rate is measured over. <= 0 disables.
	Window time.Duration
}

// Start wires the anomaly circuit breaker onto the bus. It subscribes to events,
// feeds tool.invoked into a sliding-window Detector, and on a trip publishes a
// system.anomaly event then invokes onTrip exactly once (the daemon wires onTrip
// to halt the kernel — SPEC-06 §5 anomaly auto-halt). It latches: after one trip
// the watcher stops (a halt cancels the runs that were generating the spike).
//
// Returns false (no watcher started) when the config is disabled. The watcher
// goroutine stops on ctx cancellation or bus close. A panic in the loop is
// recovered so a monitor bug can never crash the daemon.
func Start(ctx context.Context, b *bus.Bus, cfg Config, onTrip func(reason string)) bool {
	det := NewDetector(cfg.MaxToolCalls, cfg.Window)
	if b == nil || !det.Enabled() {
		return false
	}
	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return false
	}
	go func() {
		defer func() {
			sub.Cancel()
			_ = recover() // a watcher panic must never take down the daemon
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if ev.Kind != event.KindToolInvoked {
					continue
				}
				tripped, count := det.Observe(time.UnixMilli(ev.TSUnixMS))
				if !tripped {
					continue
				}
				reason := fmt.Sprintf(
					"tool-call rate anomaly: %d tool calls within %s exceeds ceiling %d (possible runaway loop)",
					count, cfg.Window, cfg.MaxToolCalls)
				_, _ = b.Publish(event.Spec{
					Subject: "system.anomaly",
					Kind:    event.KindAnomalyDetected,
					Actor:   "anomaly",
					Payload: map[string]any{
						"signal":    "tool_call_rate",
						"count":     count,
						"window_ms": cfg.Window.Milliseconds(),
						"ceiling":   cfg.MaxToolCalls,
						"reason":    reason,
					},
				})
				if onTrip != nil {
					onTrip(reason)
				}
				return // latch: fire once, then let the halt do its work
			}
		}
	}()
	return true
}
