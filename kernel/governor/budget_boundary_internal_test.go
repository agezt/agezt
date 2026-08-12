// SPDX-License-Identifier: MIT

package governor

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// The spend-enforcement checks use `>=` (spend AT the ceiling is over budget). The
// existing end-to-end budget tests overshoot — their first call blows well past the
// cap — so they assert blocking only when spend is strictly greater than the ceiling.
// That leaves the exact boundary unpinned: a `>=` → `>` regression (allowing one
// more call once spend has reached the ceiling) would pass them all. Mutation testing
// (M497) confirmed both `spentToday >= ceiling` and `spent >= cap` survived. These
// white-box tests pin the boundary directly.
//
// The Governor is built directly (not via New, which requires a registry) because
// budgetExceeded / taskBudgetExceeded touch only the spend ledger. `today` is pre-set
// to the current UTC day so rolloverIfNeededLocked does not reset the installed spend.

// budgetScopeNamed / budgetExceeded / taskBudgetExceeded are single-scope
// probes over the budgetScopes table. They lived in budgetgate.go until
// 2026-08-12 with a comment explaining they were "kept as a named helper
// because it is the boundary the budget tests drive directly" — which is
// precisely why no binary reached them: production calls gateBudgets, which
// walks every scope. They belong with the tests that are their only caller.
func budgetScopeNamed(name string) budgetScope {
	for _, s := range budgetScopes {
		if s.Name == name {
			return s
		}
	}
	panic("governor: unknown budget scope " + name) // table is a compile-time constant
}

// budgetExceeded reports the daemon-wide ceiling's state.
func (g *Governor) budgetExceeded() (bool, int64, int64) {
	return g.evalBudgetScope(budgetScopeNamed("global"), &agent.CompletionRequest{})
}

// taskBudgetExceeded reports the per-task-type ceiling's state (M1.zz).
// (false, 0, 0) when no cap is configured for that type.
func (g *Governor) taskBudgetExceeded(taskType string) (bool, int64, int64) {
	return g.evalBudgetScope(budgetScopeNamed("task"), &agent.CompletionRequest{TaskType: taskType})
}

func newBudgetGov(cfg Config) *Governor {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	g := &Governor{cfg: cfg, spentByTaskToday: map[string]int64{}}
	g.today = g.cfg.Now().UTC().Format("2006-01-02")
	return g
}

func TestBudgetExceeded_AtExactCeilingBlocks(t *testing.T) {
	g := newBudgetGov(Config{DailyCeilingMicrocents: 1000})

	g.mu.Lock()
	g.spentToday.Store(1000) // exactly the ceiling
	g.mu.Unlock()
	if ex, _, _ := g.budgetExceeded(); !ex {
		t.Error("spend exactly at the daily ceiling must be over budget (>=), got allowed")
	}

	g.mu.Lock()
	g.spentToday.Store(999) // one microcent below
	g.mu.Unlock()
	if ex, _, _ := g.budgetExceeded(); ex {
		t.Error("spend one microcent below the ceiling must NOT be over budget")
	}
}

func TestTaskBudgetExceeded_AtExactCapBlocks(t *testing.T) {
	g := newBudgetGov(Config{TaskBudgets: map[string]int64{"plan": 1000}})

	g.mu.Lock()
	g.spentByTaskToday["plan"] = 1000 // exactly the cap
	g.mu.Unlock()
	if ex, _, _ := g.taskBudgetExceeded("plan"); !ex {
		t.Error("task spend exactly at the cap must be over budget (>=), got allowed")
	}

	g.mu.Lock()
	g.spentByTaskToday["plan"] = 999
	g.mu.Unlock()
	if ex, _, _ := g.taskBudgetExceeded("plan"); ex {
		t.Error("task spend one microcent below the cap must NOT be over budget")
	}
}
