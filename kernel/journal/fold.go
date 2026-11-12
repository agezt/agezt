// SPDX-License-Identifier: MIT

// Folds over the journal: the run's tool-result fold (FoldTools). Like the
// provenance walks in provenance.go (Why / Causes / ParentOf / Verify), a fold
// is an operation over the event log itself — the log is the host's own file,
// so the log answers it. The daemon used to carry its own copy of this fold
// (FoldRunTools) and the run engine asked the host for it: a second spelling
// of one family, and a round trip the host never had to keep (W3.76aj).

package journal

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
)

// FoldTools counts the tool.result events of ONE correlation and collects the
// tool names invoked, in order. It builds the memory-distillation and
// skill-forge transcripts: the caller asks how many tools the run used and
// which ones.
//
// The filter `e.CorrelationID != corr || e.Kind != event.KindToolResult`
// isolates THIS run's tool results. Nothing tested that isolation, so mutation
// testing (M501) showed the `||` and the two `!=` could flip undetected — a
// `||`->`&&` regression would fold OTHER runs' tool results (and this run's
// non-tool events) into the transcript, corrupting the distilled memory with
// another run's activity.
//
// The count and the names answer different questions: a malformed or
// tool-less payload still COUNTS (the run really did make that tool call) but
// contributes no name.
func (j *Journal) FoldTools(corr string) (int, []string) {
	var (
		count int
		names []string
	)
	_ = j.Range(func(e *event.Event) error {
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
