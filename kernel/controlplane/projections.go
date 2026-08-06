// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
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
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"]) // A2 cursor pagination
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	type row struct {
		ts, seq int64
		view    map[string]any
	}
	rows := make([]row, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		view, ok := decode(e)
		if !ok {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		view["ts_unix_ms"] = e.TSUnixMS
		view["seq"] = e.Seq // A2: stable per-row id for the frontend cursor pager
		rows = append(rows, row{ts: e.TSUnixMS, seq: e.Seq, view: view})
		return nil
	}); err != nil {
		s.fail(conn, req, err)
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if cursorOK { // A2: keep rows strictly older than the cursor, before the limit
		kept := rows[:0]
		for _, r := range rows {
			if journal.KeepBeforeCursor(r.ts, r.seq, cursorMS, cursorSeq) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view)
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(rows); n > 0 {
		nextCursor = journal.NextCursor(rows[n-1].ts, rows[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{resultKey: out, "count": len(out), "next_cursor": nextCursor},
	})
}
