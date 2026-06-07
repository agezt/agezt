// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// foldRunTools builds the memory-distillation transcript for a run: it counts the
// run's tool.result events and collects the tool names. The filter
// `e.CorrelationID != corr || e.Kind != event.KindToolResult` isolates THIS run's
// tool results. Nothing tested that isolation, so mutation testing (M501) showed the
// `||` and the two `!=` could flip undetected — a `||`→`&&` regression would fold
// OTHER runs' tool results (and this run's non-tool events) into the transcript,
// corrupting the distilled memory with another run's activity.
func TestFoldRunTools_OnlyCountsThisCorrelationsToolResults(t *testing.T) {
	k := openCausesKernel(t)
	const corr, other = "run-A", "run-B"

	pubTool := func(c, tool string) {
		t.Helper()
		if _, err := k.Bus().Publish(event.Spec{
			Subject:       "tool.result",
			Kind:          event.KindToolResult,
			Actor:         "test",
			CorrelationID: c,
			Payload:       map[string]any{"tool": tool},
		}); err != nil {
			t.Fatalf("publish tool.result: %v", err)
		}
	}

	pubTool(corr, "shell") // this run
	pubTool(corr, "file")  // this run
	pubTool(other, "http") // DIFFERENT run — must be excluded by correlation
	// A non-tool-result event under THIS run — must be excluded by kind.
	if _, err := k.Bus().Publish(event.Spec{
		Subject: "task.received", Kind: event.KindTaskReceived, Actor: "test",
		CorrelationID: corr, Payload: map[string]any{"k": "x"},
	}); err != nil {
		t.Fatalf("publish task.received: %v", err)
	}

	count, names := k.foldRunTools(corr)
	if count != 2 {
		t.Errorf("count = %d, want 2 (only this run's tool results, not run-B's and not the non-tool event)", count)
	}
	if len(names) != 2 || names[0] != "shell" || names[1] != "file" {
		t.Errorf("names = %v, want [shell file] in order", names)
	}
}
