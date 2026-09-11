// SPDX-License-Identifier: MIT

// Workflow helpers: types, workflowView, applyWorkflowRunEvent, summariseWorkflowArcs, workflowLastRuns.
// Code extracted from workflow.go during the Day-38 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/workflow"
)



// workflowRunTimeout bounds one synchronous run over the wire — generous
// (delay nodes alone may sleep minutes), but never unbounded.
const workflowRunTimeout = 15 * time.Minute

// workflowView is the stable wire shape. The full graph rides only when
// full is set (list stays light; show/save carry it for the canvas).
func workflowView(w workflow.Workflow, full bool) map[string]any {
	b, _ := json.Marshal(w)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["node_count"] = len(w.Nodes)
	m["edge_count"] = len(w.Edges)
	// Trigger summary (M799) so list rows can say HOW a workflow starts
	// without carrying the whole graph.
	spec := w.TriggerSpec()
	m["trigger_kind"] = spec.Kind
	switch {
	case spec.Kind == "webhook":
		m["trigger_detail"] = "POST /hooks/" + w.Name
	case spec.IntervalSec > 0:
		m["trigger_detail"] = fmt.Sprintf("every %ds", spec.IntervalSec)
	case spec.DailyAt != "":
		m["trigger_detail"] = "daily at " + spec.DailyAt
	case spec.Subject != "":
		m["trigger_detail"] = "on " + spec.Subject
	}
	if !full {
		delete(m, "nodes")
		delete(m, "edges")
	}
	return m
}

// lastRunSummary is one workflow's most recent run, as the list surfaces it.
type lastRunSummary struct {
	status     string // running | completed | failed
	atMS       int64  // finished, or started while still running
	durationMS int64  // 0 while running
}

// wfArc is one in-progress fold of a single workflow run.
type wfArc struct {
	corr     string
	started  int64
	finished int64
	status   string
}

// applyWorkflowRunEvent folds ONE journal event into the per-workflow
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
func applyWorkflowRunEvent(want map[string]string, latest map[string]*wfArc, e *event.Event) {
	name, ok := want[e.Subject]
	if !ok || e.CorrelationID == "" {
		return
	}
	switch e.Kind {
	case event.KindWorkflowStarted:
		latest[name] = &wfArc{corr: e.CorrelationID, started: e.TSUnixMS, status: "running"}
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

// summariseWorkflowArcs turns the accumulator into the per-workflow summaries.
func summariseWorkflowArcs(latest map[string]*wfArc) map[string]lastRunSummary {
	out := make(map[string]lastRunSummary, len(latest))
	for name, a := range latest {
		sum := lastRunSummary{status: a.status, atMS: a.started}
		if a.finished > 0 {
			sum.atMS = a.finished
			if a.started > 0 && a.finished >= a.started {
				sum.durationMS = a.finished - a.started
			}
		}
		out[name] = sum
	}
	return out
}

// workflowLastRuns folds the journal ONCE and returns each workflow's latest
// run. The alternative — asking workflow_runs per row — is a full journal scan
// per workflow, so the console's list would have cost N scans to answer a
// question one scan answers.
func (s *Server) workflowLastRuns(names []string) map[string]lastRunSummary {
	want := make(map[string]string, len(names)) // subject -> workflow name
	for _, n := range names {
		want["workflow."+n] = n
	}
	latest := map[string]*wfArc{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		applyWorkflowRunEvent(want, latest, e)
		return nil
	})
	return summariseWorkflowArcs(latest)
}
