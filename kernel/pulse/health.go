// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"fmt"
)

// HealthStat is a compact snapshot of the daemon's OWN recent operational
// health, sampled from the journal. It is deliberately small — the observer
// works in ratios, not raw events — so the data source (the journal scan in
// the daemon) stays cheap and the observer stays unit-testable.
type HealthStat struct {
	ToolCalls  int // tool.invoked count in the window
	ToolErrors int // tool.result with error=true in the window
	Runs       int // task.completed + task.failed in the window
	FailedRuns int // task.failed in the window
}

// HealthStatFunc returns the current health snapshot. It decouples the
// HealthObserver from the journal/controlplane exactly as DiskFreeFunc
// decouples the DiskObserver from the OS — the daemon owns the data source,
// tests inject a fake.
type HealthStatFunc func(ctx context.Context) (HealthStat, error)

// healthLevel is the observer's three-state assessment of the snapshot.
type healthLevel int

const (
	healthOK healthLevel = iota
	healthDegraded
	healthCritical
)

func (l healthLevel) String() string {
	switch l {
	case healthCritical:
		return "critical"
	case healthDegraded:
		return "degraded"
	default:
		return "healthy"
	}
}

// DefaultHealthDegradeAt is the tool-error-rate at which health is judged
// "degraded" — matches the threshold the Analyst view already uses when it
// grades a system (≥30% tool errors reads as unhealthy).
const DefaultHealthDegradeAt = 0.30

// DefaultHealthMinSample is the minimum number of samples (tool calls or runs)
// on an axis before that axis is allowed to move the assessment — so a single
// early failure can't flap the daemon into a "critical" alert.
const DefaultHealthMinSample = 5

// HealthObserver watches the daemon's own run/tool reliability and emits a
// Delta when its health *transitions* between healthy / degraded / critical —
// turning the system's self-observation (the reactive Analyst) into proactive
// self-monitoring: the operator is briefed (over whatever channels pulse feeds)
// the moment the daemon's own health changes, without anyone having to ask.
//
// Like DiskObserver it tracks edge transitions, not levels: it stays silent on
// every poll where the level is unchanged, so it never floods. The first poll
// establishes the baseline and emits nothing.
type HealthObserver struct {
	stat      HealthStatFunc
	degradeAt float64 // tool-error-rate floor for "degraded"
	minSample int

	prev     healthLevel
	hasState bool
}

// NewHealthObserver constructs a health observer. degradeAt<=0 falls back to
// DefaultHealthDegradeAt; minSample<=0 to DefaultHealthMinSample.
func NewHealthObserver(stat HealthStatFunc, degradeAt float64, minSample int) *HealthObserver {
	if degradeAt <= 0 {
		degradeAt = DefaultHealthDegradeAt
	}
	if minSample <= 0 {
		minSample = DefaultHealthMinSample
	}
	return &HealthObserver{stat: stat, degradeAt: degradeAt, minSample: minSample}
}

// Name implements Observer.
func (o *HealthObserver) Name() string { return "self:health" }

// assess maps a snapshot to a health level. Each axis (tool errors, run
// failures) only counts once it has at least minSample observations, so a thin
// sample can't manufacture an alert. The level is the worse of the two axes.
func (o *HealthObserver) assess(s HealthStat) healthLevel {
	level := healthOK
	worsen := func(l healthLevel) {
		if l > level {
			level = l
		}
	}

	if s.ToolCalls >= o.minSample {
		rate := float64(s.ToolErrors) / float64(s.ToolCalls)
		switch {
		case rate >= 2*o.degradeAt || rate >= 0.6:
			worsen(healthCritical)
		case rate >= o.degradeAt:
			worsen(healthDegraded)
		}
	}
	if s.Runs >= o.minSample {
		rate := float64(s.FailedRuns) / float64(s.Runs)
		switch {
		case rate >= 0.5:
			worsen(healthCritical)
		case rate >= 0.25:
			worsen(healthDegraded)
		}
	}
	return level
}

// Poll implements Observer. It emits a Delta only when the assessed level
// changes from the previously-seen level (after a baseline first poll).
func (o *HealthObserver) Poll(ctx context.Context) ([]Delta, error) {
	if o.stat == nil {
		return nil, nil
	}
	s, err := o.stat(ctx)
	if err != nil {
		return nil, fmt.Errorf("health: %w", err)
	}
	level := o.assess(s)

	prev := o.prev
	prevKnown := o.hasState
	o.prev = level
	o.hasState = true

	if !prevKnown || prev == level {
		return nil, nil // baseline or no transition
	}

	toolRate, runRate := 0.0, 0.0
	if s.ToolCalls > 0 {
		toolRate = float64(s.ToolErrors) / float64(s.ToolCalls)
	}
	if s.Runs > 0 {
		runRate = float64(s.FailedRuns) / float64(s.Runs)
	}
	metrics := fmt.Sprintf("tool errors %d/%d (%.0f%%), run failures %d/%d (%.0f%%)",
		s.ToolErrors, s.ToolCalls, toolRate*100, s.FailedRuns, s.Runs, runRate*100)

	worsening := level > prev
	var summary string
	var sev Severity
	var kind string
	if worsening {
		kind = "health_degraded"
		summary = fmt.Sprintf("daemon health %s → %s: %s", prev, level, metrics)
		if level == healthCritical {
			sev = SevCritical
		} else {
			sev = SevHigh
		}
	} else {
		kind = "health_recovered"
		summary = fmt.Sprintf("daemon health recovered %s → %s: %s", prev, level, metrics)
		sev = SevMedium
	}

	return []Delta{{
		Source:  o.Name(),
		Kind:    kind,
		Summary: summary,
		Before:  prev.String(),
		After:   level.String(),
		Hints:   map[string]string{"severity": string(sev)},
	}}, nil
}
