// SPDX-License-Identifier: MIT

package governor_test

// M191: provider-reported token counts are untrusted (a compat/ollama
// endpoint can be operator-configured to an arbitrary URL). Negative or
// absurd counts must never produce a negative or wrapped cost, which
// would credit the spend ledger and disable the daily ceiling.

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

func TestCostMicrocents_ClampsAndSaturates(t *testing.T) {
	// Negative token counts cost 0, never a negative (credit) amount.
	if c := costMicrocentsForTest("claude-opus-4-7", -1_000_000, -1_000_000); c != 0 {
		t.Errorf("negative tokens cost = %d, want 0", c)
	}
	// A single absurd term that would overflow int64 raw (2e9 * 7.5e9)
	// saturates to a large positive cost, never wraps negative.
	if c := costMicrocentsForTest("claude-opus-4-7", 0, 2_000_000_000); c <= 0 {
		t.Errorf("overflow cost = %d, want a large positive (saturated) value", c)
	}
	// Both terms huge (sum overflows too) still saturates positive.
	if c := costMicrocentsForTest("claude-opus-4-7", 5_000_000_000, 5_000_000_000); c <= 0 {
		t.Errorf("double-overflow cost = %d, want positive saturated", c)
	}
	// Normal range is unchanged.
	if c := costMicrocentsForTest("claude-sonnet-4-6", 1_000_000, 0); c != 300_000_000 {
		t.Errorf("normal cost = %d, want 300000000 ($0.30)", c)
	}
}

// A hostile/buggy usage response reporting NEGATIVE token counts must not
// credit the ledger (which would manufacture budget headroom).
func TestComplete_NegativeUsageDoesNotCreditLedger(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-opus-4-7", -1_000_000, -1_000_000)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{Registry: r, Bus: b, DailyCeilingMicrocents: 1_000_000_000})

	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-opus-4-7"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := g.SpentMicrocents(); got < 0 {
		t.Errorf("spent went negative (%d) on negative-token usage — ledger-credit bug", got)
	}
}

// A usage response with absurd token counts must saturate to a huge cost
// (fail-closed) so the daily ceiling then trips, rather than wrapping to a
// negative cost that disables the gate.
func TestComplete_OverflowUsageTripsBudget(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-opus-4-7", 0, 2_000_000_000)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{Registry: r, Bus: b, DailyCeilingMicrocents: 1_000_000_000}) // $0.01

	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-opus-4-7"}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if got := g.SpentMicrocents(); got <= 0 {
		t.Fatalf("spent = %d; overflow must saturate to a large positive cost, not wrap negative", got)
	}
	_, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-opus-4-7"})
	if !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Errorf("second call after a saturating-cost call: got %v, want ErrBudgetExceeded", err)
	}
}
