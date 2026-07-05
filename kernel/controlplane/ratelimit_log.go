// SPDX-License-Identifier: MIT

package controlplane

// Rate-limit observability (M106). The governor enforces a per-minute call cap
// (AGEZT_RATE_PER_MIN for the primary, AGEZT_TENANT_RATE_PER_MIN per tenant) and
// journals a rate.limited event whenever it refuses a call. Those events were
// only reachable via `agt journal grep --kind rate.limited`; this folds them
// into a first-class surface so an operator can see, per tenant, whether callers
// are being throttled — silent throttling is an SRE blind spot. Mirrors the
// edict/warden/approvals log+stats pattern.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func (s *Server) handleRateLimitLog(conn net.Conn, req Request) {
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type row struct {
		ts, seq      int64
		used, limitN int
	}
	rows := make([]row, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindRateLimited {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			Used        int `json:"used"`
			LimitPerMin int `json:"limit_per_min"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		rows = append(rows, row{ts: e.TSUnixMS, seq: e.Seq, used: p.Used, limitN: p.LimitPerMin})
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		out = append(out, map[string]any{
			"ts_unix_ms":    r.ts,
			"seq":           r.seq, // A2: stable per-row id for the frontend cursor pager's dedup
			"used":          r.used,
			"limit_per_min": r.limitN,
		})
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(rows); n > 0 {
		nextCursor = journal.NextCursor(rows[n-1].ts, rows[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"throttles": out, "count": len(out), "next_cursor": nextCursor},
	})
}

func (s *Server) handleRateLimitStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	var sinceMS int64
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	total := 0
	limitN := 0
	worstUsed := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindRateLimited {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			Used        int `json:"used"`
			LimitPerMin int `json:"limit_per_min"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		total++
		if p.LimitPerMin > 0 {
			limitN = p.LimitPerMin
		}
		if p.Used > worstUsed {
			worstUsed = p.Used
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"throttled":     total,
			"limit_per_min": limitN,
			"worst_used":    worstUsed,
			"window_ms":     sinceMS,
		},
	})
}
