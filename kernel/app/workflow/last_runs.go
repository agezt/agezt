// SPDX-License-Identifier: MIT
package workflow

import "github.com/agezt/agezt/kernel/event"

type LastRun struct {
	Status     string `json:"status"`
	AtMS       int64  `json:"at_ms"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// runArc is one in-progress fold of a single workflow run.
type runArc struct {
	corr     string
	started  int64
	finished int64
	status   string
}

// applyRunEvent folds ONE journal event into the per-workflow
// accumulator. `want` maps a journal subject to the workflow name it belongs
// to; events for anything else are ignored.
//
// Split out from the Range loop so the fold is testable without standing up a
// kernel and a journal — the two rules that make a single pass correct are
// worth pinning:
//
//   - a newer started arc supersedes the one being tracked, and
//   - a terminal event closes ONLY the arc it belongs to, so an older run
//     finishing after a newer one started cannot report a stale status.
func applyRunEvent(want map[string]string, latest map[string]*runArc, e *event.Event) {
	name, ok := want[e.Subject]
	if !ok || e.CorrelationID == "" {
		return
	}
	switch e.Kind {
	case event.KindWorkflowStarted:
		latest[name] = &runArc{corr: e.CorrelationID, started: e.TSUnixMS, status: "running"}
	case event.KindWorkflowCompleted, event.KindWorkflowFailed:
		cur := latest[name]
		if cur == nil || cur.corr != e.CorrelationID {
			return
		}
		cur.finished = e.TSUnixMS
		if e.Kind == event.KindWorkflowFailed {
			cur.status = "failed"
		} else {
			cur.status = "completed"
		}
	}
}

// summariseArcs turns the accumulator into the per-workflow summaries.
func summariseArcs(latest map[string]*runArc) map[string]LastRun {
	out := make(map[string]LastRun, len(latest))
	for name, a := range latest {
		sum := LastRun{Status: a.status, AtMS: a.started}
		if a.finished > 0 {
			sum.AtMS = a.finished
			if a.started > 0 && a.finished >= a.started {
				sum.DurationMS = a.finished - a.started
			}
		}
		out[name] = sum
	}
	return out
}

func (s *Reads) lastRuns(names []string) map[string]LastRun {
	want := make(map[string]string, len(names))
	for _, name := range names {
		want["workflow."+name] = name
	}
	latest := map[string]*runArc{}
	_ = s.journal.Range(func(e *event.Event) error { applyRunEvent(want, latest, e); return nil })
	return summariseArcs(latest)
}
