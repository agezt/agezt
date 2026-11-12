// SPDX-License-Identifier: MIT

package journal

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// TestFoldTools_OnlyCountsThisCorrelationsToolResults pins the fold's
// isolation: it counts THIS correlation's tool.result events only — not
// another run's tool results, and not this run's non-tool events. The filter
// `e.CorrelationID != corr || e.Kind != event.KindToolResult` carries that
// rule, and nothing tested it until the mutation testing (M501) showed the
// `||` and the two `!=` could flip undetected — a `||`->`&&` regression would
// fold OTHER runs' tool results (and this run's non-tool events) into the
// transcript, corrupting the distilled memory with another run's activity.
//
// The fold moved here from the daemon (W3.76aj): it is a walk over the event
// log, so the log answers it — the same place the provenance walks live.
func TestFoldTools_OnlyCountsThisCorrelationsToolResults(t *testing.T) {
	j := newTestJournal(t, 1<<20)
	const corr, other = "run-A", "run-B"

	pubTool := func(c, tool string) {
		t.Helper()
		if _, err := j.Append(event.Spec{
			Subject:       "tool.result",
			Kind:          event.KindToolResult,
			Actor:         "test",
			CorrelationID: c,
			Payload:       map[string]any{"tool": tool},
		}); err != nil {
			t.Fatalf("append tool.result: %v", err)
		}
	}

	pubTool(corr, "shell") // this run
	pubTool(corr, "file")  // this run
	pubTool(other, "http") // DIFFERENT run — must be excluded by correlation
	// A non-tool-result event under THIS run — must be excluded by kind.
	if _, err := j.Append(event.Spec{
		Subject: "task.received", Kind: event.KindTaskReceived, Actor: "test",
		CorrelationID: corr, Payload: map[string]any{"k": "x"},
	}); err != nil {
		t.Fatalf("append task.received: %v", err)
	}

	count, names := j.FoldTools(corr)
	if count != 2 {
		t.Errorf("count = %d, want 2 (only this run's tool results, not run-B's and not the non-tool event)", count)
	}
	if len(names) != 2 || names[0] != "shell" || names[1] != "file" {
		t.Errorf("names = %v, want [shell file] in order", names)
	}
}

// TestFoldTools_CountsACallWithoutAName pins the second half of the fold's
// contract: the COUNT and the NAMES answer different questions. A tool call
// whose payload carries no tool name still happened — so it counts — but it
// contributes no name to the transcript.
func TestFoldTools_CountsACallWithoutAName(t *testing.T) {
	j := newTestJournal(t, 1<<20)
	const corr = "run-C"

	// A tool.result with a payload that is not a tool name at all.
	if _, err := j.Append(event.Spec{
		Subject: "tool.result", Kind: event.KindToolResult, Actor: "test",
		CorrelationID: corr, Payload: map[string]any{"other": "x"},
	}); err != nil {
		t.Fatalf("append nameless tool.result: %v", err)
	}
	// A tool.result with no payload at all.
	if _, err := j.Append(event.Spec{
		Subject: "tool.result", Kind: event.KindToolResult, Actor: "test",
		CorrelationID: corr,
	}); err != nil {
		t.Fatalf("append payloadless tool.result: %v", err)
	}

	count, names := j.FoldTools(corr)
	if count != 2 {
		t.Errorf("count = %d, want 2 (both tool calls happened)", count)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want none (neither payload carried a tool name)", names)
	}
}

// TestFoldTools_EmptyCorrelationFoldsNothing pins the empty-correlation case:
// a correlation nobody used folds to nothing, and the fold does not panic on
// an empty log.
func TestFoldTools_EmptyCorrelationFoldsNothing(t *testing.T) {
	j := newTestJournal(t, 1<<20)

	count, names := j.FoldTools("run-nobody")
	if count != 0 || len(names) != 0 {
		t.Errorf("FoldTools = (%d, %v), want (0, [])", count, names)
	}
}
