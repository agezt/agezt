// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// The console's workflow list needs to say whether each workflow last succeeded.
// Asking workflow_runs per row would be a full journal scan PER WORKFLOW; this
// fold answers the same question for every workflow in one pass. These tests
// pin the two rules that make a single pass correct: the newest started arc
// wins, and a terminal event closes only the arc it belongs to.

func wantFor(names ...string) map[string]string {
	m := make(map[string]string, len(names))
	for _, n := range names {
		m["workflow."+n] = n
	}
	return m
}

func started(name, corr string, ts int64) *event.Event {
	return &event.Event{Subject: "workflow." + name, Kind: event.KindWorkflowStarted, CorrelationID: corr, TSUnixMS: ts}
}

func finished(name, corr string, ts int64, failed bool) *event.Event {
	kind := event.KindWorkflowCompleted
	if failed {
		kind = event.KindWorkflowFailed
	}
	return &event.Event{Subject: "workflow." + name, Kind: kind, CorrelationID: corr, TSUnixMS: ts}
}

func fold(want map[string]string, evs ...*event.Event) map[string]lastRunSummary {
	latest := map[string]*wfArc{}
	for _, e := range evs {
		applyWorkflowRunEvent(want, latest, e)
	}
	return summariseWorkflowArcs(latest)
}

func TestWorkflowLastRunPicksTheNewestArc(t *testing.T) {
	t.Parallel()
	got := fold(wantFor("nightly"),
		started("nightly", "corr-old", 1_000),
		finished("nightly", "corr-old", 1_500, false),
		started("nightly", "corr-new", 2_000),
		finished("nightly", "corr-new", 2_400, true),
	)
	run, ok := got["nightly"]
	if !ok {
		t.Fatalf("no summary for nightly: %+v", got)
	}
	if run.status != "failed" {
		t.Fatalf("status = %q, want the newest arc's status (failed)", run.status)
	}
	if run.atMS != 2_400 {
		t.Fatalf("atMS = %d, want the newest arc's finish (2400)", run.atMS)
	}
	if run.durationMS != 400 {
		t.Fatalf("durationMS = %d, want 400", run.durationMS)
	}
}

func TestWorkflowLastRunIgnoresLateTerminalForOlderArc(t *testing.T) {
	t.Parallel()
	// An older run finishing AFTER a newer one started must not overwrite the
	// newer run's state. A naive "last terminal event wins" fold reports a
	// stale status here.
	got := fold(wantFor("drifty"),
		started("drifty", "corr-old", 1_000),
		started("drifty", "corr-new", 2_000),
		finished("drifty", "corr-old", 2_100, false),
	)
	run := got["drifty"]
	if run.status != "running" {
		t.Fatalf("status = %q, want running (the newest arc has not finished)", run.status)
	}
	if run.atMS != 2_000 {
		t.Fatalf("atMS = %d, want the newest arc's start (2000)", run.atMS)
	}
	if run.durationMS != 0 {
		t.Fatalf("durationMS = %d, want 0 while running", run.durationMS)
	}
}

func TestWorkflowLastRunOnlyFoldsRequestedWorkflows(t *testing.T) {
	t.Parallel()
	got := fold(wantFor("wanted"),
		started("wanted", "c1", 10),
		finished("wanted", "c1", 20, false),
		started("other", "c2", 30),
		finished("other", "c2", 40, false),
	)
	if _, ok := got["other"]; ok {
		t.Fatalf("folded a workflow that was not asked for: %+v", got)
	}
	if len(got) != 1 {
		t.Fatalf("got %d summaries, want 1: %+v", len(got), got)
	}
}

func TestWorkflowLastRunSkipsCorrelationlessAndUnknownEvents(t *testing.T) {
	t.Parallel()
	noCorr := started("wanted", "", 10)
	otherSubject := &event.Event{Subject: "agent.wanted", Kind: event.KindWorkflowStarted, CorrelationID: "c9", TSUnixMS: 10}
	got := fold(wantFor("wanted"), noCorr, otherSubject)
	if len(got) != 0 {
		t.Fatalf("got %+v, want nothing folded", got)
	}
}

func TestWorkflowLastRunEmpty(t *testing.T) {
	t.Parallel()
	if got := fold(wantFor("never-ran")); len(got) != 0 {
		t.Fatalf("got %+v, want no summaries for a workflow that never ran", got)
	}
}

func TestWorkflowLastRunTerminalWithoutStartIsIgnored(t *testing.T) {
	t.Parallel()
	// A completion whose start fell outside the journal window has no arc to
	// close; reporting it would invent a run with no start time.
	got := fold(wantFor("truncated"), finished("truncated", "c1", 500, false))
	if len(got) != 0 {
		t.Fatalf("got %+v, want nothing for a terminal event with no start", got)
	}
}
