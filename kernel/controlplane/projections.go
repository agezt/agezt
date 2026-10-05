// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// projectJournal is the ONE engine behind every journal-derived "*_log"
// handler (Phase 1.3): tool/warden/policy/provider/webhook/ratelimit/world/
// memory/approvals/netguard logs, plan history, and schedule fires all fold
// the journal into a paginated, newest-first list with the exact same
// mechanics. Before this existed each handler carried a character-identical
// copy of the limit clamp, cursor decode, since_ms cutoff, tenant resolution,
// (ts,seq) sort, cursor filter, truncation, and next_cursor emission — ~15
// copies that had to be edited in lockstep.
//
// decode inspects one event and returns the row's view map (WITHOUT
// ts_unix_ms/seq — stamped here so the frontend cursor pager always has its
// stable per-row id) or false to skip the event. Per-handler concerns live
// inside decode: kind filtering, extra req.Args filters (tool name, errors
// only, latency floor, …), and any cross-event pairing state the closure
// keeps (e.g. tool_log matching tool.invoked inputs to tool.result rows —
// Range is in journal order, so a stash map works).
//
// decode runs for EVERY event and the since_ms cutoff drops decoded ROWS —
// not events — so cross-event stash state (tool_log's invoked inputs) still
// accrues from events outside the window whose row-producing partner falls
// inside it. Rows come back as {resultKey: [...], count, next_cursor} — the
// envelope every log endpoint and the frontend's cursorPager already speak.
func (s *Server) projectJournal(conn net.Conn, req Request, resultKey string, decode func(*event.Event) (map[string]any, bool)) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	output, err := journalview.Project(k.Journal(), journalview.Input{Limit: limit, CutoffMS: cutoff, Cursor: req.Args["cursor"]}, decode)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{resultKey: output.Rows, "count": output.Count, "next_cursor": output.NextCursor}})
}
