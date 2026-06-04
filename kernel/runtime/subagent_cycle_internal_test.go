// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestSubAgentSpendMicrocents_CycleGuardTerminates exercises the `seen` guard in
// subAgentSpendMicrocents. The runtime never emits a cyclic delegation graph, but
// a corrupt/forged journal could — and the spend-cap BFS walks parent→child links
// straight from journal payloads. Without the guard, a cycle (A→B→A) would loop
// forever and wedge every subsequent delegation's spend check. This is a white-box
// test because the method is unexported and the cycle can't arise through the
// public delegate path.
func TestSubAgentSpendMicrocents_CycleGuardTerminates(t *testing.T) {
	k, err := Open(Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	// Fabricate a cyclic spawn graph directly in the journal: A claims B as its
	// child, and B claims A as its child. Each correlation has a spend.
	publish := func(kind event.Kind, corr string, payload map[string]any) {
		if _, perr := k.Bus().Publish(event.Spec{
			Subject: "test.cycle", Kind: kind, Actor: "test", CorrelationID: corr, Payload: payload,
		}); perr != nil {
			t.Fatalf("publish: %v", perr)
		}
	}
	publish(event.KindSubAgentSpawned, "A", map[string]any{"child_correlation": "B", "parent": "A"})
	publish(event.KindSubAgentSpawned, "B", map[string]any{"child_correlation": "A", "parent": "B"})
	publish(event.KindBudgetConsumed, "A", map[string]any{"cost_microcents": float64(1000)})
	publish(event.KindBudgetConsumed, "B", map[string]any{"cost_microcents": float64(2000)})

	// Must terminate. A missing `seen` guard would spin A→B→A→B… forever.
	done := make(chan int64, 1)
	go func() { done <- k.subAgentSpendMicrocents("A") }()

	select {
	case total := <-done:
		// Descendants of A, excluding A itself: just B. So total == B's spend.
		if total != 2000 {
			t.Errorf("transitive spend = %d, want 2000 (B's spend; A excluded, cycle visited once)", total)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subAgentSpendMicrocents did not terminate within 2s — cycle guard regressed")
	}
}
