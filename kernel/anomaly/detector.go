// SPDX-License-Identifier: MIT

// Package anomaly is the autonomous-operation circuit breaker (SPEC-06 §5): it
// watches for runaway signals and, on a spike, auto-engages a halt so a looping
// or runaway agent cannot burn budget or take repeated action unsupervised.
//
// v1 watches one signal — the GLOBAL tool-call rate across all runs, channels,
// and Pulse — which is the clearest runaway indicator and complements the
// per-run loop guard (M116, identical-call suppression within a single run) and
// the governor's per-minute throttle (M106, refuses but doesn't halt). The
// spend-rate / error-rate / repetition signals SPEC-06 §5 also names are
// follow-ups that plug into the same Detector shape.
package anomaly

import "time"

// Detector is a sliding-window rate trip: it fires when more than Max events
// are observed within Window. Not safe for concurrent use — the Monitor
// serializes Observe on a single goroutine.
type Detector struct {
	max    int
	window time.Duration
	stamps []time.Time
}

// NewDetector builds a detector that trips when a (Max+1)th event lands within
// Window of the earliest still-in-window event. A non-positive Max or Window
// yields a permanently-disabled detector (Enabled reports false).
func NewDetector(max int, window time.Duration) *Detector {
	return &Detector{max: max, window: window}
}

// Enabled reports whether the detector can ever trip.
func (d *Detector) Enabled() bool { return d.max > 0 && d.window > 0 }

// Observe records an event at t and reports whether the number of events within
// the trailing Window now exceeds Max (a trip), plus that current count.
// Stamps older than the window are pruned. A disabled detector never trips.
//
// t is expected to be non-decreasing across calls (event publish order); an
// out-of-order older t is still counted but won't prune in-window stamps.
func (d *Detector) Observe(t time.Time) (tripped bool, count int) {
	if !d.Enabled() {
		return false, 0
	}
	cutoff := t.Add(-d.window)
	// Stamps are in arrival (≈chronological) order; drop the leading run that
	// is strictly older than the window.
	drop := 0
	for drop < len(d.stamps) && d.stamps[drop].Before(cutoff) {
		drop++
	}
	if drop > 0 {
		d.stamps = d.stamps[drop:]
	}
	d.stamps = append(d.stamps, t)
	count = len(d.stamps)
	return count > d.max, count
}
