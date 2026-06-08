// SPDX-License-Identifier: MIT

package controlplane

// Budget query handler — turns governor.BudgetSnapshot into the
// JSON shape the `agt budget` CLI renders. Read-only: no mutation
// happens here, the handler just reads the governor's current
// counters under the same mutex Complete uses.

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: budgetResult(gov.Snapshot())})
}

// handleBudgetSet serves CmdBudgetSet (M607): adjusts the governor's global
// daily spend ceiling at runtime, then returns the post-set snapshot in the
// SAME shape as CmdBudget so the Web UI can re-render without a follow-up read.
// Mirrors handleBudget's not-a-governor guard. The ceiling_mc arg crosses the
// socket as a number (CLI/JSON) or string (Web UI query proxy); both are
// accepted via numArg.
func (s *Server) handleBudgetSet(conn net.Conn, req Request) {
	gov, ok := s.k.Provider().(*governor.Governor)
	if !ok {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespError,
			Error: "budget_set: daemon's provider is not a governor; cannot adjust the ceiling",
		})
		return
	}
	if _, present := req.Args["ceiling_mc"]; !present {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ceiling_mc required (microcents; 0 = unlimited)"})
		return
	}
	mc, err := numArg(req.Args["ceiling_mc"])
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ceiling_mc: " + err.Error()})
		return
	}
	gov.SetDailyCeiling(mc)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: budgetResult(gov.Snapshot())})
}

// budgetResult renders a governor snapshot into the JSON-friendly map shared by
// CmdBudget and CmdBudgetSet. Per-task rows are sorted by task type so the
// output is deterministic across calls (no row-order flicker for operators).
func budgetResult(snap governor.BudgetSnapshot) map[string]any {
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
	return map[string]any{
		"utc_date":       snap.UTCDate,
		"spent_mc":       snap.SpentMicrocents,
		"ceiling_mc":     snap.CeilingMicrocents,
		"per_task":       perTask,
		"strict_pricing": snap.StrictPricing,
	}
}

// numArg coerces a control-plane argument into an int64. JSON unmarshals
// numbers into float64, but the Web UI's query-string write proxy forwards
// everything as a string; accept both so the same command works from the CLI
// (typed number) and the browser (string). Rejects fractional and unparseable
// values rather than silently truncating.
func numArg(v any) (int64, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int64(n)) {
			return 0, fmt.Errorf("must be a whole number, got %v", n)
		}
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("not an integer: %q", n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}
