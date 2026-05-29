// SPDX-License-Identifier: MIT

package controlplane

// Budget query handler — turns governor.BudgetSnapshot into the
// JSON shape the `agt budget` CLI renders. Read-only: no mutation
// happens here, the handler just reads the governor's current
// counters under the same mutex Complete uses.

import (
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/governor"
)

// handleBudget serves CmdBudget. Returns the governor's snapshot
// as a JSON-friendly map. When the daemon's provider isn't a
// *governor.Governor (test rigs that wire a raw provider directly),
// returns a clear error rather than crashing — operators see
// "this daemon doesn't have a governor" instead of a panic trace.
func (s *Server) handleBudget(conn net.Conn, req Request) {
	gov, ok := s.k.Provider().(*governor.Governor)
	if !ok {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespError,
			Error: "budget: daemon's provider is not a governor (likely a test or future provider variant); no budget state to report",
		})
		return
	}
	snap := gov.Snapshot()

	// Sort per-task rows by task type so output is deterministic
	// across runs — operators reading `agt budget` shouldn't see
	// the row order flicker between calls.
	sort.Slice(snap.PerTask, func(i, j int) bool {
		return snap.PerTask[i].TaskType < snap.PerTask[j].TaskType
	})

	perTask := make([]map[string]any, 0, len(snap.PerTask))
	for _, row := range snap.PerTask {
		perTask = append(perTask, map[string]any{
			"task_type":  row.TaskType,
			"spent_mc":   row.SpentMicrocents,
			"ceiling_mc": row.CapMicrocents,
		})
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"utc_date":   snap.UTCDate,
			"spent_mc":   snap.SpentMicrocents,
			"ceiling_mc": snap.CeilingMicrocents,
			"per_task":   perTask,
		},
	})
}
