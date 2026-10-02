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
// system.anomaly event then invokes onTrip (the daemon wires onTrip to halt the
// kernel — SPEC-06 §5 anomaly auto-halt).
//
// After a trip the breaker stays QUIET until the kernel resumes (a `resume`
// event): the halt cancels the runs that generated the spike, and the calls
// still draining out must not re-trip it. On resume it re-arms with an empty
// window. It used to latch instead — the goroutine returned after one trip — so
// once an operator resumed the kernel, the daemon ran with no runaway
// protection for the rest of its life.
//
// Returns false (no watcher started) when the config is disabled. The watcher
// goroutine stops on ctx cancellation or bus close. A panic while handling one
// event is recovered and costs only that event; it used to stop the watcher
// for good, silently.
func Start(ctx context.Context, b *bus.Bus, cfg Config, onTrip func(reason string)) bool {
	det := NewDetector(cfg.MaxToolCalls, cfg.Window)
	if b == nil || !det.Enabled() {
		return false
	}
	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return false
	}
	m := &monitor{b: b, cfg: cfg, det: det, onTrip: onTrip}
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
				m.handleSafely(ev)
			}
		}
	}()
	return true
}

// monitor is the breaker's state, owned by the watcher goroutine.
type monitor struct {
	b       *bus.Bus
	cfg     Config
	det     *Detector
	onTrip  func(reason string)
	tripped bool // waiting for the kernel to resume
}

// handleSafely confines a panic to the event that caused it: a monitor bug
// must neither crash the daemon nor silently disarm the breaker.
func (m *monitor) handleSafely(ev *event.Event) {
	defer func() { _ = recover() }()
	m.handle(ev)
}

func (m *monitor) handle(ev *event.Event) {
	switch ev.Kind {
	case event.KindResume:
		if m.tripped {
			m.tripped = false
			m.det = NewDetector(m.cfg.MaxToolCalls, m.cfg.Window) // fresh window
		}
		return
	case event.KindToolInvoked:
	default:
		return
	}
	if m.tripped {
		return
	}
	trip, count := m.det.Observe(time.UnixMilli(ev.TSUnixMS))
	if !trip {
		return
	}
	m.tripped = true
	reason := fmt.Sprintf(
		"tool-call rate anomaly: %d tool calls within %s exceeds ceiling %d (possible runaway loop)",
		count, m.cfg.Window, m.cfg.MaxToolCalls)
	_, _ = m.b.Publish(event.Spec{
		Subject: "system.anomaly",
		Kind:    event.KindAnomalyDetected,
		Actor:   "anomaly",
		Payload: map[string]any{
			"signal":    "tool_call_rate",
			"count":     count,
			"window_ms": m.cfg.Window.Milliseconds(),
			"ceiling":   m.cfg.MaxToolCalls,
			"reason":    reason,
		},
	})
	if m.onTrip != nil {
		m.onTrip(reason)
	}
}
