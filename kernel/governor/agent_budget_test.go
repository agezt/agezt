// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// TestAgentDailyBudget_MetersAndRefuses: requests carrying a named agent's
// identity + daily ceiling (M793) accrue an identity ledger; once today's
// spend reaches the ceiling the next completion is refused with a
// budget.exceeded that wraps ErrBudgetExceeded — while OTHER identities and
// unattributed requests keep flowing.
func TestAgentDailyBudget_MetersAndRefuses(t *testing.T) {
	reg := governor.NewRegistry()
	// claude-sonnet-4-6 is in the fallback price table: 1M in + 1M out ≈
	// 1.8e9 microcents per call — comfortably past a 1e9 ceiling in one call.
	stub := &stubProvider{name: "stub", model: "claude-sonnet-4-6", inToks: 1_000_000, outToks: 1_000_000}
	if err := reg.Register(&governor.ProviderInfo{Name: stub.name, Provider: stub, AuthMode: governor.AuthLocal}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	g, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}

	asAgent := agent.CompletionRequest{Agent: "researcher", AgentDailyCeilingMc: 1_000_000_000}

	// First call passes (ledger empty) and accrues spend to the identity.
	if _, err := g.Complete(context.Background(), asAgent); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if spent := g.SpentByAgentMicrocents("researcher"); spent <= 0 {
		t.Fatalf("identity ledger not credited: %d", spent)
	}

	// Second call: today's spend is past the ceiling → refused.
	_, err = g.Complete(context.Background(), asAgent)
	if !errors.Is(err, governor.ErrAgentBudgetExceeded) || !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Fatalf("want ErrAgentBudgetExceeded (wrapping ErrBudgetExceeded), got %v", err)
	}

	// A DIFFERENT identity and an unattributed request still flow.
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Agent: "ops", AgentDailyCeilingMc: 1_000_000_000}); err != nil {
		t.Fatalf("other identity blocked: %v", err)
	}
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{}); err != nil {
		t.Fatalf("unattributed request blocked: %v", err)
	}

	// No ceiling carried → the same identity is metered but never refused.
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Agent: "researcher"}); err != nil {
		t.Fatalf("ceiling-less request refused: %v", err)
	}
}
