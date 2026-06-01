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

// handleProviderStats aggregates provider routing (M90) — total routed calls,
// fallback count + rate, calls-handled-by-provider (the chosen primary per
// routing.decision), and a fallbacks-by-failed-provider breakdown. Answers "how
// reliable is my primary?". since_ms windows by event time. Tenant-routed.
func (s *Server) handleProviderStats(conn net.Conn, req Request) {
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

	var routed, fallbacks int
	byPrimary := map[string]int{}        // routing.decision primary → count
	fallbackByFailed := map[string]int{} // provider.fallback failed → count
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindRoutingDecision:
			var p struct {
				Primary string `json:"primary"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			routed++
			if p.Primary != "" {
				byPrimary[p.Primary]++
			}
		case event.KindProviderFallback:
			var p struct {
				Failed string `json:"failed"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			fallbacks++
			if p.Failed != "" {
				fallbackByFailed[p.Failed]++
			}
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	fallbackRate := 0.0
	if routed > 0 {
		fallbackRate = float64(fallbacks) / float64(routed)
	}
	byPrimaryOut := make(map[string]any, len(byPrimary))
	for n, c := range byPrimary {
		byPrimaryOut[n] = c
	}
	fbOut := make(map[string]any, len(fallbackByFailed))
	for n, c := range fallbackByFailed {
		fbOut[n] = c
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"routed":               routed,
			"fallbacks":            fallbacks,
			"fallback_rate":        fallbackRate,
			"by_primary":           byPrimaryOut,
			"fallbacks_by_primary": fbOut,
			"window_ms":            sinceMS,
		},
	})
}

// handleProviderRejections folds the journal's capability-gating events (M92) —
// capability.rejected (a request blocked because the model lacks a capability:
// tool_call from the M25 strict gate, vision from the M91 image gate) and
// capability.rerouted (the M40 down-route: remapped to a tool-capable model
// instead of rejecting). Together they answer "what did the capability gates do?".
// Newest-first, limited, with the shared --since window.
func (s *Server) handleProviderRejections(conn net.Conn, req Request) {
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type capEvent struct {
		ts, seq            int64
		kind               string // rejected | rerouted
		capability, model  string
		fromModel, toModel string
	}
	rows := make([]capEvent, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindCapabilityRejected:
			var p struct{ Model, Capability string }
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, capEvent{ts: e.TSUnixMS, seq: e.Seq, kind: "rejected", capability: p.Capability, model: p.Model})
		case event.KindCapabilityRerouted:
			var p struct {
				FromModel  string `json:"from_model"`
				ToModel    string `json:"to_model"`
				Capability string `json:"capability"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, capEvent{ts: e.TSUnixMS, seq: e.Seq, kind: "rerouted", capability: p.Capability, fromModel: p.FromModel, toModel: p.ToModel})
		}
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
	if len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{"ts_unix_ms": r.ts, "kind": r.kind, "capability": r.capability}
		if r.kind == "rerouted" {
			row["from_model"] = r.fromModel
			row["to_model"] = r.toModel
		} else {
			row["model"] = r.model
		}
		out = append(out, row)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"rejections": out, "count": len(out)},
	})
}

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
