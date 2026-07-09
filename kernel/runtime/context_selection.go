// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/event"
	kmemory "github.com/agezt/agezt/kernel/memory"
	kskill "github.com/agezt/agezt/kernel/skill"
	kworld "github.com/agezt/agezt/kernel/worldmodel"
)

// publishContextSelection publishes a context.selection event when there is
// data worth sharing.
func (k *Kernel) publishContextSelection(corr, actor string, manifest contextselect.Manifest) {
	if len(manifest.Chosen) == 0 && len(manifest.Rejected) == 0 && len(manifest.Summary) == 0 {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "context.selection",
		Kind:          event.KindContextSelection,
		Actor:         actor,
		CorrelationID: corr,
		Payload:       manifest,
	})
}

// publishContextFailureAnalysis scans the run's journal for the last
// context.selection event and re-ranks its rejected candidates to identify
// likely context-omission suspects after a run failure.
func (k *Kernel) publishContextFailureAnalysis(corr, actor string, runErr error) {
	if runErr == nil || k == nil || k.journal == nil {
		return
	}
	var latest *contextselect.Manifest
	_ = k.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindContextSelection {
			return nil
		}
		var m contextselect.Manifest
		if json.Unmarshal(e.Payload, &m) == nil && m.Phase != "failure_analysis" {
			latest = &m
		}
		return nil
	})
	if latest == nil || len(latest.Rejected) == 0 {
		return
	}
	suspects := contextselect.FailureAnalysisSuspects(latest.Rejected)
	k.publishContextSelection(corr, actor, contextselect.Manifest{
		Phase: "failure_analysis",
		Summary: map[string]any{
			"classifier":        "heuristic_external",
			"failure":           runErr.Error(),
			"suspect_omission":  len(suspects) > 0,
			"counterfactuals":   contextselect.CandidateIDs(suspects),
			"credit_assignment": fmt.Sprintf("replay with %d rejected candidate(s) from previous selection", len(suspects)),
		},
		Rejected: suspects,
	})
}

// contextSelectionRunner returns a RunnerOption that wires per-run
// context-selection event publishing. This is the glue between the agent loop
// and the context-selection publishing logic.
func contextSelectionRunner(ctx context.Context, corr, actor string) func(contextselect.Manifest) {
	return func(manifest contextselect.Manifest) {
		if k, ok := ctx.Value(kernelKey{}).(*Kernel); ok {
			k.publishContextSelection(corr, actor, manifest)
		}
	}
}

// KernelKey is a context value key used to pass *Kernel into agent callbacks.
// This is the same pattern as kernelKey{} but exported for use by agent
// middleware that needs to access the kernel from context.
type kernelKey struct{}

// Note: the types Candidate, Manifest, and pure helper functions
// (SplitCandidates, TokenCost, Freshness, Risk, etc.) have been moved to
// kernel/contextselect. Import that package directly when only the data
// types or pure computation are needed.

// The following conversion functions stay in runtime because they depend
// on Kernel-owned stores or agent loop state. They call contextselect
// helpers internally.

// memoryContextCandidates converts memory search hits to context candidates.
func memoryContextCandidates(hits []kmemory.Scored, nowMS int64) []contextselect.Candidate {
	return contextselect.MemoryCandidates(hits, nowMS)
}

// worldContextCandidates converts world-model search hits to context candidates.
func worldContextCandidates(hits []kworld.ScoredEntity, nowMS int64) []contextselect.Candidate {
	return contextselect.WorldCandidates(hits, nowMS)
}

// skillContextCandidates converts skill search hits to context candidates.
func skillContextCandidates(hits []kskill.Scored, nowMS int64) []contextselect.Candidate {
	return contextselect.SkillCandidates(hits, nowMS)
}
