// SPDX-License-Identifier: MIT

package budget

import "github.com/agezt/agezt/kernel/governor"

// SnapshotOf maps the governor's snapshot into the budget view's snapshot:
// the day's counters and each configured per-task-type cap with today's
// spend against it.
//
// The conversion belongs here, beside the row type: the host reads the
// governor's counters and hands them over, and the application's own row
// stays the application's business.
func SnapshotOf(snap governor.BudgetSnapshot) Snapshot {
	rows := make([]TaskSnapshot, 0, len(snap.PerTask))
	for _, row := range snap.PerTask {
		rows = append(rows, TaskSnapshot{TaskType: row.TaskType, SpentMicrocents: row.SpentMicrocents, CapMicrocents: row.CapMicrocents})
	}
	return Snapshot{
		UTCDate:           snap.UTCDate,
		SpentMicrocents:   snap.SpentMicrocents,
		CeilingMicrocents: snap.CeilingMicrocents,
		PerTask:           rows,
		StrictPricing:     snap.StrictPricing,
	}
}
