// SPDX-License-Identifier: MIT

// Orphan-run reconciliation (orphanRun + runScan types + newRunScan +
// observe + orphans + reconcileOrphanRuns). Extracted from main_overlay.go
// during Day 211 god-file refactor (#56). Public API unchanged.
package main

import (
	"encoding/json"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

type orphanRun struct {
	Corr      string
	Intent    string
	StartedMS int64
}
type runScan struct {
	received  map[string]*orphanRun
	completed map[string]bool
	failed    map[string]bool
	abandoned map[string]bool
}
func newRunScan() *runScan {
	return &runScan{
		received:  map[string]*orphanRun{},
		completed: map[string]bool{},
		failed:    map[string]bool{},
		abandoned: map[string]bool{},
	}
}
func (s *runScan) observe(e *event.Event) {
	switch e.Kind {
	case event.KindTaskReceived:
		o := &orphanRun{Corr: e.CorrelationID, StartedMS: e.TSUnixMS}
		var p struct {
			Intent string `json:"intent"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		o.Intent = p.Intent
		s.received[e.CorrelationID] = o
	case event.KindTaskCompleted:
		s.completed[e.CorrelationID] = true
	case event.KindTaskFailed:
		s.failed[e.CorrelationID] = true
	case event.KindTaskAbandoned:
		s.abandoned[e.CorrelationID] = true
	}
}
func (s *runScan) orphans() []orphanRun {
	var out []orphanRun
	for corr, o := range s.received {
		if !s.completed[corr] && !s.failed[corr] && !s.abandoned[corr] {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedMS != out[j].StartedMS {
			return out[i].StartedMS < out[j].StartedMS
		}
		return out[i].Corr < out[j].Corr
	})
	return out
}
func reconcileOrphanRuns(k *kernelruntime.Kernel) (int, error) {
	scan := newRunScan()
	if err := k.Journal().Range(func(e *event.Event) error {
		scan.observe(e)
		return nil
	}); err != nil {
		return 0, err
	}
	orphans := scan.orphans()
	for _, o := range orphans {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "task",
			Kind:          event.KindTaskAbandoned,
			Actor:         "kernel",
			CorrelationID: o.Corr,
			Payload: map[string]any{
				"intent":          o.Intent,
				"reason":          "daemon restart: run was in-flight and never completed",
				"started_unix_ms": o.StartedMS,
			},
		})
	}
	return len(orphans), nil
}
