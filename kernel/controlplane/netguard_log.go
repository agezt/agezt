// SPDX-License-Identifier: MIT

package controlplane

// Egress-block audit (M109). The netguard egress guard refuses the http/browser
// tools' connections to internal/metadata addresses and now journals each refusal
// as a netguard.blocked event. This folds those events into an audit timeline so
// an operator can see what was stopped — a tool reaching for 169.254.169.254 is a
// strong SSRF / prompt-injection / exfiltration signal. Sister to `agt netguard
// test` (M105), which previews the policy; this records what it actually blocked.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func (s *Server) handleNetguardLog(conn net.Conn, req Request) {
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
		ts, seq          int64
		ip, reason, tool string
	}
	rows := make([]row, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindNetguardBlocked {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			IP     string `json:"ip"`
			Reason string `json:"reason"`
			Tool   string `json:"tool"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		rows = append(rows, row{ts: e.TSUnixMS, seq: e.Seq, ip: p.IP, reason: p.Reason, tool: p.Tool})
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
			"ts_unix_ms": r.ts,
			"seq":        r.seq, // A2: stable per-row id for the frontend cursor pager
			"ip":         r.ip,
			"reason":     r.reason,
			"tool":       r.tool,
		})
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(rows); n > 0 {
		nextCursor = journal.NextCursor(rows[n-1].ts, rows[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"blocks": out, "count": len(out), "next_cursor": nextCursor},
	})
}
