// SPDX-License-Identifier: MIT

package budget

import (
	"testing"

	"github.com/agezt/agezt/kernel/governor"
)

func TestSnapshotOfMapsTheGovernorCounters(t *testing.T) {
	got := SnapshotOf(governor.BudgetSnapshot{
		UTCDate:           "2026-10-22",
		SpentMicrocents:   1234,
		CeilingMicrocents: 99_999,
		StrictPricing:     true,
		PerTask: []governor.TaskBudgetSnapshot{
			{TaskType: "chat", SpentMicrocents: 10, CapMicrocents: 500},
			{TaskType: "tool", SpentMicrocents: 20, CapMicrocents: 0},
		},
	})
	if got.UTCDate != "2026-10-22" || got.SpentMicrocents != 1234 || got.CeilingMicrocents != 99_999 {
		t.Errorf("snapshot = %+v", got)
	}
	if !got.StrictPricing {
		t.Errorf("strict pricing = false, want true")
	}
	if len(got.PerTask) != 2 {
		t.Fatalf("per-task = %v, want two rows", got.PerTask)
	}
	if got.PerTask[0].TaskType != "chat" || got.PerTask[0].SpentMicrocents != 10 || got.PerTask[0].CapMicrocents != 500 {
		t.Errorf("per-task row = %+v", got.PerTask[0])
	}
	if got.PerTask[1].TaskType != "tool" || got.PerTask[1].CapMicrocents != 0 {
		t.Errorf("per-task row = %+v", got.PerTask[1])
	}
}

// A governor without per-task budgets still maps to an empty list — not a
// nil one — so the budget view never has to special-case it.
func TestSnapshotOfMapsAnEmptyPerTaskList(t *testing.T) {
	got := SnapshotOf(governor.BudgetSnapshot{UTCDate: "2026-10-22"})
	if got.PerTask == nil {
		t.Fatalf("per-task = nil, want an empty list")
	}
	if len(got.PerTask) != 0 {
		t.Errorf("per-task = %v, want none", got.PerTask)
	}
	if got.StrictPricing {
		t.Errorf("strict pricing = true, want false")
	}
}
