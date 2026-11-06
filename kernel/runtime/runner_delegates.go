package runtime

// Provenance: SPDX-License-Identifier: MIT Runtime runner public delegate wrappers:
//             FoldRunTools. Split from runexec_helpers.go during Day 211
//             god-file refactor (#34). Public API unchanged.

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
)

// FoldRunTools counts tool.result events for corr and collects the
// tool names invoked (in order), for the distillation transcript.
// Public wrapper for the runner package (Day 32). The body
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

// buildTranscript moved to the runner package on Day 32
// (runner/helpers.go). The Runner uses it for MaybeDistill /
// MaybeForge / MaybeShadowEval transcripts.

// The JOURNAL's provenance walks are the journal's own (W3.76ad): `Why`,
// `Causes`, `ParentOf` and `Verify` answer over the event log, and the event
// log is the host's own file (kernel/journal). The daemon used to carry four
// verbs over them — and the run engine re-exported the same four, so one
// family had THREE spellings: the journal's methods, the engine's re-exports
// and the daemon's copies.
//
// The daemon holds no verb per journal operation any more: a caller that
// needs the provenance walk asks the journal, through the daemon's own
// journal door (`k.Journal().Why(…)`).
