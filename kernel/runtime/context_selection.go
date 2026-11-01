// SPDX-License-Identifier: MIT

package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/event"
)

// publishContextFailureAnalysis scans the run's journal for the last
// context.selection event and re-ranks its rejected candidates to identify
// likely context-omission suspects after a run failure.
//
// The ANALYSIS stays in the daemon because it reads the run's JOURNAL — the
// daemon's — but the manifest it derives is PUBLISHED by the runs module's
// prompt service (W3.76e), which owns the context-selection surface: the
// module knows what a manifest is, the daemon knows what the run wrote.
//
// Note: the types Candidate, Manifest, and pure helper functions
// (SplitCandidates, TokenCost, Freshness, Risk, etc.) have been moved to
// kernel/contextselect. Import that package directly when only the data
// types or pure computation are needed.
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
	k.prompt.PublishSelection(corr, actor, contextselect.Manifest{
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
