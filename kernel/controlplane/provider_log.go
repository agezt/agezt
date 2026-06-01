// SPDX-License-Identifier: MIT

package controlplane

// Provider-activity log (M89) — a read-only timeline of the journal's
// routing.decision and provider.fallback events (the governor journals one
// routing.decision per LLM call with the chosen provider + fallback chain, and a
// provider.fallback whenever the primary errors and the next in chain is tried).
// `agt provider check` probes whether a provider WORKS; this shows what the
// governor actually DID at request time — which provider handled calls and when
// it had to fall back. The provider-layer analogue of `agt tool log`.

import (
	"encoding/json"
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleProviderLog(conn net.Conn, req Request) {
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
	fallbacksOnly, _ := req.Args["fallbacks"].(bool)
	cutoff := sinceCutoff(req.Args["since_ms"]) // M65 helper

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type provEvent struct {
		ts, seq int64
		kind    string // route | fallback
		// route fields
		primary, chain, taskType string
		// fallback fields
		failed, next, reason string
	}
	events := make([]provEvent, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindRoutingDecision:
			if fallbacksOnly {
				return nil
			}
			var p struct {
				Primary  string   `json:"primary"`
				Chain    []string `json:"chain"`
				TaskType string   `json:"task_type"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			events = append(events, provEvent{
				ts: e.TSUnixMS, seq: e.Seq, kind: "route",
				primary: p.Primary, chain: strings.Join(p.Chain, ","), taskType: p.TaskType,
			})
		case event.KindProviderFallback:
			var p struct{ Failed, Next, Reason string }
			_ = json.Unmarshal(e.Payload, &p)
			events = append(events, provEvent{
				ts: e.TSUnixMS, seq: e.Seq, kind: "fallback",
				failed: p.Failed, next: p.Next, reason: p.Reason,
			})
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].ts != events[j].ts {
			return events[i].ts > events[j].ts
		}
		return events[i].seq > events[j].seq
	})
	if len(events) > limit {
		events = events[:limit]
	}

	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		row := map[string]any{"ts_unix_ms": e.ts, "kind": e.kind}
		if e.kind == "route" {
			row["primary"] = e.primary
			row["chain"] = e.chain
			row["task_type"] = e.taskType
		} else {
			row["failed"] = e.failed
			row["next"] = e.next
			row["reason"] = e.reason
		}
		out = append(out, row)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"events": out, "count": len(out)},
	})
}
