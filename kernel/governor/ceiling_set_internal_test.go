// SPDX-License-Identifier: MIT

package governor

import "testing"

// SetDailyCeiling is the operator's runtime "ayarla" knob (M607). These
// white-box tests pin its enforcement semantics against budgetExceeded /
// Snapshot, reusing newBudgetGov (governor built directly, no registry) since
// the override only touches the spend ledger and ceiling fields.

func TestSetDailyCeiling_RaiseUnblocks(t *testing.T) {
	g := newBudgetGov(Config{DailyCeilingMicrocents: 1000})
	g.mu.Lock()
	g.spentToday.Store(1500) // already past the configured cap
	g.mu.Unlock()

	if ex, _, _ := g.budgetExceeded(); !ex {
		t.Fatal("precondition: spend above the configured ceiling must be over budget")
	}
	if got := g.SetDailyCeiling(2000); got != 2000 {
		t.Fatalf("SetDailyCeiling returned %d, want 2000", got)
	}
	if ex, _, ceiling := g.budgetExceeded(); ex {
		t.Errorf("after raising the ceiling to 2000 with spend 1500, must be under budget; got exceeded (ceiling=%d)", ceiling)
	}
	if snap := g.Snapshot(); snap.CeilingMicrocents != 2000 {
		t.Errorf("Snapshot ceiling = %d, want the override 2000", snap.CeilingMicrocents)
	}
	if eff := g.DailyCeilingMicrocents(); eff != 2000 {
		t.Errorf("DailyCeilingMicrocents = %d, want the override 2000", eff)
	}
}

func TestSetDailyCeiling_LowerBlocks(t *testing.T) {
	g := newBudgetGov(Config{DailyCeilingMicrocents: 5000})
	g.mu.Lock()
	g.spentToday.Store(1500)
	g.mu.Unlock()

	if ex, _, _ := g.budgetExceeded(); ex {
		t.Fatal("precondition: spend below the configured ceiling must be under budget")
	}
	g.SetDailyCeiling(1000) // below current spend
	if ex, spent, ceiling := g.budgetExceeded(); !ex {
		t.Errorf("after lowering the ceiling to 1000 below spend 1500, must be over budget; got allowed (spent=%d ceiling=%d)", spent, ceiling)
	}
}

func TestSetDailyCeiling_ZeroIsUnlimited(t *testing.T) {
	g := newBudgetGov(Config{DailyCeilingMicrocents: 1000})
	g.mu.Lock()
	g.spentToday.Store(1_000_000) // far past any finite cap
	g.mu.Unlock()

	g.SetDailyCeiling(0) // 0 = unlimited
	if ex, _, _ := g.budgetExceeded(); ex {
		t.Error("ceiling 0 means unlimited: arbitrarily high spend must not be over budget")
	}
	if snap := g.Snapshot(); snap.CeilingMicrocents != 0 {
		t.Errorf("Snapshot ceiling = %d, want 0 (unlimited)", snap.CeilingMicrocents)
	}
}

func TestSetDailyCeiling_NegativeClampsToUnlimited(t *testing.T) {
	g := newBudgetGov(Config{DailyCeilingMicrocents: 1000})
	if got := g.SetDailyCeiling(-42); got != 0 {
		t.Errorf("negative ceiling must clamp to 0 (unlimited); got %d", got)
	}
	g.mu.Lock()
	g.spentToday.Store(99_999)
	g.mu.Unlock()
	if ex, _, _ := g.budgetExceeded(); ex {
		t.Error("after clamping a negative ceiling to 0 (unlimited), spend must not be over budget")
	}
}
