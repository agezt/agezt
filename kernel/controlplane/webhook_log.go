// SPDX-License-Identifier: MIT

package controlplane

// Webhook delivery observability (M112). The outbound webhook dispatcher journals
// webhook.delivered (a 2xx) and webhook.failed (exhausted retries) for every
// event it POSTs to an operator-configured sink. Those were only reachable via
// `agt journal grep webhook`; this folds them into a first-class surface so an
// operator can see whether their notifications are getting through — a webhook
// silently failing is the classic "I never got paged" outage. Mirrors the
// edict/warden/provider log+stats pattern.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func (s *Server) handleWebhookLog(conn net.Conn, req Request) {
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
	failedOnly, _ := req.Args["failed"].(bool)
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"]) // A2 cursor pagination
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type row struct {
		ts, seq           int64
		ok                bool
		status, attempts  int
		url, kind, errMsg string
	}
	rows := make([]row, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		isDelivered := e.Kind == event.KindWebhookDelivered
		isFailed := e.Kind == event.KindWebhookFailed
		if !isDelivered && !isFailed {
			return nil
		}
		if failedOnly && !isFailed {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			URL       string `json:"url"`
			EventKind string `json:"event_kind"`
			Status    int    `json:"status"`
			Attempts  int    `json:"attempts"`
			Error     string `json:"error"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		rows = append(rows, row{
			ts: e.TSUnixMS, seq: e.Seq, ok: isDelivered,
			status: p.Status, attempts: p.Attempts, url: p.URL, kind: p.EventKind, errMsg: p.Error,
		})
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
		m := map[string]any{
			"ts_unix_ms": r.ts,
			"seq":        r.seq, // A2: stable per-row id for the frontend cursor pager
			"ok":         r.ok,
			"url":        r.url,
			"event_kind": r.kind,
			"attempts":   r.attempts,
		}
		if r.ok {
			m["status"] = r.status
		} else {
			m["error"] = r.errMsg
		}
		out = append(out, m)
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(rows); n > 0 {
		nextCursor = journal.NextCursor(rows[n-1].ts, rows[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"deliveries": out, "count": len(out), "next_cursor": nextCursor},
	})
}

func (s *Server) handleWebhookStats(conn net.Conn, req Request) {
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

	var delivered, failed int
	byURL := map[string][2]int{} // url → {delivered, failed}
	if err := k.Journal().Range(func(e *event.Event) error {
		isDelivered := e.Kind == event.KindWebhookDelivered
		isFailed := e.Kind == event.KindWebhookFailed
		if !isDelivered && !isFailed {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		c := byURL[p.URL]
		if isDelivered {
			delivered++
			c[0]++
		} else {
			failed++
			c[1]++
		}
		byURL[p.URL] = c
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	total := delivered + failed
	failureRate := 0.0
	if total > 0 {
		failureRate = float64(failed) / float64(total)
	}
	byURLOut := make(map[string]any, len(byURL))
	for u, c := range byURL {
		byURLOut[u] = map[string]any{"delivered": c[0], "failed": c[1]}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":        total,
			"delivered":    delivered,
			"failed":       failed,
			"failure_rate": failureRate,
			"by_url":       byURLOut,
			"window_ms":    sinceMS,
		},
	})
}
