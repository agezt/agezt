// SPDX-License-Identifier: MIT
//
// Runtime runexec public delegate wrappers: FoldRunTools + Why + Causes +
// ParentOf + Verify (one-line passthroughs to the runexec sub-package's
// Runner). Split from runexec_helpers.go during Day 211 god-file
// refactor (#34). Public API unchanged.
package runtime

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
)


// FoldRunTools counts tool.result events for corr and collects the
// tool names invoked (in order), for the distillation transcript.
// Public wrapper for the runexec sub-package (Day 32). The body
// stays on *Kernel because the journal range is the per-process
// append-only log; the runner reads through this handle.
func (k *Kernel) FoldRunTools(corr string) (int, []string) {
	var (
		count int
		names []string
	)
	_ = k.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindToolResult {
			return nil
		}
		count++
		var p struct {
			Tool string `json:"tool"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Tool != "" {
			names = append(names, p.Tool)
		}
		return nil
	})
	return count, names
}

// buildTranscript moved to the runexec sub-package on Day 32
// (runexec/helpers.go). The Runner uses it for MaybeDistill /
// MaybeForge / MaybeShadowEval transcripts.

// Why returns every event with the same correlation_id as the named event,
// in seq order (the M0.5 form of `agt why`). Delegates to the runexec
// sub-package's Runner, which routes to the journal's provenance scan
// (Phase 3.1 C, Day 23).
func (k *Kernel) Why(eventID string) ([]*event.Event, error) { return k.runexec.Why(eventID) }

// Causes returns the causation ancestry of an event, root-first — the
// provenance walk SPEC-01 §7.1 describes. Delegates to the runexec
// sub-package's Runner (Day 23).
func (k *Kernel) Causes(eventID string) ([]*event.Event, error) { return k.runexec.Causes(eventID) }

// ParentOf returns the lead run's correlation for a sub-agent run, or "" if
// childCorr was not spawned via delegation (M42). Delegates to the runexec
// sub-package's Runner (Day 23).
func (k *Kernel) ParentOf(childCorr string) string { return k.runexec.ParentOf(childCorr) }

// Verify replays every event and confirms the BLAKE3 chain is intact.
// Returns nil on success. Delegates to the runexec sub-package's
// Runner (Day 23).
func (k *Kernel) Verify() error { return k.runexec.Verify() }
