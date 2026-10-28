// SPDX-License-Identifier: MIT

package budget

import (
	"github.com/agezt/agezt/kernel/governor"
)

// The budget family's port over the daemon's spend keeper.
//
// The budget family does not know what the keeper is: it knows the two
// questions it asks — what has been spent, and what the ceiling is — and the
// keeper answers them in the shape the budget reports.

// Spender is the spend keeper the budget family asks.
//
// The keeper is the provider's own budget surface; the snapshot crosses as the
// keeper's row shape and the budget converts it, because the conversion is
// the budget's business and not the keeper's.
type Spender interface {
	Snapshot() governor.BudgetSnapshot
	SetDailyCeiling(microcents int64) int64
}

// GovernorFrom adapts the provider's spend keeper to the budget family's own
// Governor.
//
// A host with no keeper gets no governor: the budget answers that there is no
// spend to report, spelled once, here.
func GovernorFrom(s Spender) Governor {
	if s == nil {
		return nil
	}
	return spenderGovernor{s: s}
}

// spenderGovernor is the budget family's view of a spend keeper.
type spenderGovernor struct{ s Spender }

// Snapshot is the budget's own snapshot, converted from the keeper's.
func (g spenderGovernor) Snapshot() Snapshot { return SnapshotOf(g.s.Snapshot()) }

// SetDailyCeiling sets the daily ceiling and drops the keeper's answer: the
// budget does not report what is left of the ceiling.
func (g spenderGovernor) SetDailyCeiling(microcents int64) { g.s.SetDailyCeiling(microcents) }
