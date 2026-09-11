// SPDX-License-Identifier: MIT

// Pulse engine status: Status + StatusMap + IsPaused + Pause + Resume.
// Code extracted from engine.go during the Day-54 god-file split. Public API unchanged.
package pulse


import (
	"github.com/agezt/agezt/kernel/event"
)


func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := make([]string, 0, len(e.observers))
	removable := make([]string, 0, len(e.removable))
	seenRemovable := map[string]bool{}
	for _, o := range e.observers {
		names = append(names, o.Name())
		// A removable observer's name is reported once even if several share it (all the
		// disk watches register as "system:disk"); RemoveObserver(name) drops them together.
		if e.removable[o] && !seenRemovable[o.Name()] {
			seenRemovable[o.Name()] = true
			removable = append(removable, o.Name())
		}
	}
	return Status{
		Running:       !e.paused,
		Paused:        e.paused,
		Beats:         e.ticks,
		Observers:     names,
		Removable:     removable,
		Dial:          string(e.dial),
		Initiative:    string(e.initiative),
		Quiet:         e.quiet,
		CadenceMS:     e.cadence.Milliseconds(),
		LastTickMS:    e.lastTickMS,
		DigestPending: len(e.digest),
	}
}

// StatusMap is the control-plane-facing snapshot: the same data as Status()
// but as a map[string]any so the control plane can return it directly without
// importing this package (it depends on the interface, the daemon injects the
// engine).
func (e *Engine) StatusMap() map[string]any {
	s := e.Status()
	return map[string]any{
		"running":        s.Running,
		"paused":         s.Paused,
		"beats":          s.Beats,
		"observers":      s.Observers,
		"removable":      s.Removable,
		"dial":           s.Dial,
		"initiative":     s.Initiative,
		"quiet":          map[string]any{"enabled": s.Quiet.Enabled, "start": s.Quiet.Start, "end": s.Quiet.End, "spec": s.Quiet.Spec()},
		"cadence_ms":     s.CadenceMS,
		"last_tick_ms":   s.LastTickMS,
		"digest_pending": s.DigestPending,
	}
}

// IsPaused reports whether beats are currently suppressed.
func (e *Engine) IsPaused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

// Pause suppresses new beats (in-flight processing finishes). Journaled.
func (e *Engine) Pause() {
	e.mu.Lock()
	if e.paused {
		e.mu.Unlock()
		return
	}
	e.paused = true
	e.mu.Unlock()
	e.publish(event.KindPulsePaused, "pulse.control", "", "", map[string]any{"paused": true})
}

// Resume re-enables beats. Journaled.
func (e *Engine) Resume() {
	e.mu.Lock()
	if !e.paused {
		e.mu.Unlock()
		return
	}
	e.paused = false
	e.mu.Unlock()
	e.publish(event.KindPulseResumed, "pulse.control", "", "", map[string]any{"paused": false})
}
