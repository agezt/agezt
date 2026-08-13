// SPDX-License-Identifier: MIT

package governor_test

// BIZ-001 regression cover. A model the governor could not price cost $0, so
// recordUsage added nothing to spentToday / spentByTaskToday / spentByAgentToday
// and all three ceilings compared `spent >= ceiling` against a ledger that never
// moved — the global daily cap, the per-task cap and the per-agent cap defeated
// simultaneously. The model id is agent-reachable (the schedule tool's free-text
// model override, AGEZT_TASK_MODEL_CHAINS through the config tool), so this was
// a budget bypass, not the accepted soft-cap overshoot.
//
// The property under test is that an unpriced model CONSUMES ledger headroom
// and eventually trips the ceiling, with strict pricing OFF (the default).

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
)

// newUnpricedGov builds a lax-pricing governor whose provider reports usage for
// the given model, so the billing path sees exactly that model id.
func newUnpricedGov(t *testing.T, cfg governor.Config, model string, in, out int) (*governor.Governor, *journal.Journal) {
	t.Helper()
	b, j := newBus(t)
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{
		Name:     "p",
		Provider: &fakeProvider{name: "p", resp: okResp(model, in, out)},
		AuthMode: governor.AuthAPIKey,
	})
	cfg.Registry = r
	cfg.Bus = b
	g, err := governor.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g, j
}

func TestUnpricedModelConsumesBudgetHeadroom(t *testing.T) {
	const unpriced = "some-unpriced-model-9000"
	// A ceiling one call's fallback cost cannot survive. The cap is soft by
	// design (budgetgate.go), so the FIRST call goes through and books the spend;
	// the second must be refused. With a $0 charge the second call — and every
	// call after it, forever — would be admitted.
	g, _ := newUnpricedGov(t, governor.Config{DailyCeilingMicrocents: 1_000_000}, unpriced, 1_000_000, 1_000_000)

	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: unpriced}); err != nil {
		t.Fatalf("first call refused: %v", err)
	}
	if spent := g.SpentMicrocents(); spent <= 0 {
		t.Fatalf("spent=%d after billing an unpriced model — the ledger never moved", spent)
	}
	_, err := g.Complete(context.Background(), agent.CompletionRequest{Model: unpriced})
	if !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Fatalf("second call err = %v, want ErrBudgetExceeded — the daily ceiling is bypassable", err)
	}
}

func TestUnpricedModelExhaustsPerAgentCeiling(t *testing.T) {
	// The per-agent ledger (M793) is fed from the same cost, so it failed the
	// same way. Pin it separately: an agent with its own cap must not be able to
	// spend past it by naming a model the catalog doesn't know.
	const unpriced = "some-unpriced-model-9000"
	g, _ := newUnpricedGov(t, governor.Config{}, unpriced, 1_000_000, 1_000_000)

	req := agent.CompletionRequest{Model: unpriced, Agent: "researcher", AgentDailyCeilingMc: 1_000_000}
	if _, err := g.Complete(context.Background(), req); err != nil {
		t.Fatalf("first call refused: %v", err)
	}
	if _, err := g.Complete(context.Background(), req); !errors.Is(err, governor.ErrAgentBudgetExceeded) {
		t.Fatalf("second call err = %v, want ErrAgentBudgetExceeded", err)
	}
}

func TestUnpricedModelJournalsBudgetUnpricedWhenLax(t *testing.T) {
	// The ledger is now moving on an ESTIMATE. That has to be visible on every
	// such call — previously budget.unpriced was emitted only under strict
	// pricing, i.e. only when the call was refused outright.
	const unpriced = "some-unpriced-model-9000"
	g, j := newUnpricedGov(t, governor.Config{}, unpriced, 1_000, 1_000)

	for i := 0; i < 2; i++ {
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: unpriced}); err != nil {
			t.Fatalf("call %d refused: %v", i, err)
		}
	}

	var seen int
	var model string
	var charged int64
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != event.KindBudgetUnpriced {
			return nil
		}
		seen++
		var p struct {
			Model             string `json:"model"`
			ChargedMicrocents int64  `json:"charged_microcents"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		model, charged = p.Model, p.ChargedMicrocents
		return nil
	})
	if seen != 2 {
		t.Errorf("budget.unpriced events = %d, want one per call (2)", seen)
	}
	if model != unpriced {
		t.Errorf("event model = %q, want %q", model, unpriced)
	}
	if charged <= 0 {
		t.Errorf("event charged_microcents = %d, want the fallback charge", charged)
	}
}

func TestKnownModelsBillUnchanged(t *testing.T) {
	// The fix must not touch models the governor CAN price. A known-free local
	// model still costs nothing (and so never trips a ceiling, and never
	// journals budget.unpriced), and a priced model still bills its own rate —
	// not the fallback.
	t.Run("known-free model stays free", func(t *testing.T) {
		g, j := newUnpricedGov(t, governor.Config{DailyCeilingMicrocents: 1_000_000}, "llama3.2", 1_000_000, 1_000_000)
		for i := 0; i < 3; i++ {
			if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "llama3.2"}); err != nil {
				t.Fatalf("call %d on a free local model refused: %v", i, err)
			}
		}
		if spent := g.SpentMicrocents(); spent != 0 {
			t.Errorf("spent=%d on a known-free model, want 0", spent)
		}
		_ = j.Range(func(e *event.Event) error {
			if e.Kind == event.KindBudgetUnpriced {
				t.Error("known-free model journaled budget.unpriced")
			}
			return nil
		})
	})

	t.Run("priced model bills its own rate", func(t *testing.T) {
		// claude-sonnet-4-6 and the unpriced fallback share an input rate, so
		// use the OUTPUT side, where they differ (1500M vs the haiku entry's
		// 400M), to prove the real price is still the one applied.
		g, j := newUnpricedGov(t, governor.Config{}, "claude-haiku-4-5", 0, 1_000_000)
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-haiku-4-5"}); err != nil {
			t.Fatalf("priced model refused: %v", err)
		}
		const want = int64(400_000_000) // 1 MTok output at the haiku rate
		if got := g.SpentMicrocents(); got != want {
			t.Errorf("spent=%d want %d (the model's own price, not the fallback)", got, want)
		}
		_ = j.Range(func(e *event.Event) error {
			if e.Kind == event.KindBudgetUnpriced {
				t.Error("priced model journaled budget.unpriced")
			}
			return nil
		})
	})
}
