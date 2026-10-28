// SPDX-License-Identifier: MIT

package budget

import (
	"testing"

	"github.com/agezt/agezt/kernel/governor"
)

// The budget family does not know what the spend keeper is: it knows the two
// questions it asks, and the keeper answers them in the shape the budget
// reports.
//
// The test pins the crossing: the snapshot crosses as the KEEPER's row shape
// and comes back as the BUDGET's, and the ceiling crosses as a question whose
// answer the budget does not report.
func TestTheBudgetsGovernorComesFromTheSpendKeeper(t *testing.T) {
	keeper := &fakeKeeper{snap: governor.BudgetSnapshot{
		UTCDate:           "2026-10-28",
		SpentMicrocents:   1234,
		CeilingMicrocents: 9999,
		PerTask: []governor.TaskBudgetSnapshot{{
			TaskType: "plan", SpentMicrocents: 7, CapMicrocents: 70,
		}},
		StrictPricing: true,
	}}
	gov := GovernorFrom(keeper)
	if gov == nil {
		t.Fatal("the budget has no governor — the keeper is there")
	}

	snap := gov.Snapshot()
	if snap.SpentMicrocents != 1234 {
		t.Fatalf("the budget reports %d spent, the keeper says %d", snap.SpentMicrocents, 1234)
	}
	if snap.CeilingMicrocents != 9999 {
		t.Fatalf("the budget reports a ceiling of %d, the keeper says %d", snap.CeilingMicrocents, 9999)
	}
	if !snap.StrictPricing {
		t.Fatal("the budget lost the keeper's strict pricing")
	}
	if len(snap.PerTask) != 1 || snap.PerTask[0].TaskType != "plan" || snap.PerTask[0].SpentMicrocents != 7 {
		t.Fatalf("the budget reports the per-task rows %v, the keeper says %v", snap.PerTask, keeper.snap.PerTask)
	}

	// The ceiling crosses as a question: the budget sets it and does not
	// report what is left of it.
	gov.SetDailyCeiling(555)
	if keeper.ceiling != 555 {
		t.Fatalf("the budget set the ceiling to %d, the keeper holds %d", 555, keeper.ceiling)
	}
}

// A host with no keeper has no budget.
//
// The keeper is the provider's own; a daemon without one spends nothing, and
// the budget says so by having no governor at all — not by reporting a zero
// that would read as "nothing was spent".
func TestTheBudgetHasNoGovernorWithoutASpendKeeper(t *testing.T) {
	if gov := GovernorFrom(nil); gov != nil {
		t.Fatal("the budget has a governor — there is no keeper")
	}
}

// fakeKeeper is a spend keeper that answers what the test asks it to.
type fakeKeeper struct {
	snap    governor.BudgetSnapshot
	ceiling int64
}

func (k *fakeKeeper) Snapshot() governor.BudgetSnapshot { return k.snap }

func (k *fakeKeeper) SetDailyCeiling(microcents int64) int64 {
	k.ceiling = microcents
	return microcents
}
