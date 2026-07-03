// SPDX-License-Identifier: MIT

package controlplane

// World-model operation log (M86) — a read-only timeline of the journal's
// worldmodel.entity.upserted / relation.upserted / forgotten events. `agt world
// list` shows the CURRENT graph (the projection); this shows the HISTORY of how
// it formed — what entities and relations the agent observed, reinforced, and
// forgot, and when. The world-model analogue of `agt memory log` (M85): both are
// knowledge stores, both keep an audit timeline.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func (s *Server) handleWorldLog(conn net.Conn, req Request) {
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
	kindFilter, _ := req.Args["kind"].(string)                          // entity|relation
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"]) // A2 cursor pagination
	cutoff := sinceCutoff(req.Args["since_ms"])                             // M65 helper

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type worldOp struct {
		ts, seq int64
		op      string // upsert verb (observe/reinforce/revive/decay) or forget
		what    string // entity | relation
		label   string // entity name, or "from verb to"
	}
	ops := make([]worldOp, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var o worldOp
		o.ts, o.seq = e.TSUnixMS, e.Seq
		switch e.Kind {
		case event.KindWorldEntityUpserted:
			var p struct{ Action, Name, Kind string }
			_ = json.Unmarshal(e.Payload, &p)
			o.op, o.what, o.label = p.Action, "entity", p.Name
			if p.Kind != "" {
				o.label += " [" + p.Kind + "]"
			}
		case event.KindWorldRelationUpserted:
			var p struct{ Action, From, Verb, To string }
			_ = json.Unmarshal(e.Payload, &p)
			o.op, o.what, o.label = p.Action, "relation", p.From+" "+p.Verb+" "+p.To
		case event.KindWorldForgotten:
			var p struct {
				Name, Verb, What string
			}
			_ = json.Unmarshal(e.Payload, &p)
			o.op, o.what = "forget", p.What
			if p.Name != "" {
				o.label = p.Name
			} else {
				o.label = p.Verb
			}
		default:
			return nil
		}
		if o.op == "" {
			o.op = "upsert"
		}
		if kindFilter != "" && o.what != kindFilter {
			return nil
		}
		ops = append(ops, o)
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].ts != ops[j].ts {
			return ops[i].ts > ops[j].ts
		}
		return ops[i].seq > ops[j].seq
	})
	if cursorOK { // A2: keep rows strictly older than the cursor, before the limit
		kept := ops[:0]
		for _, o := range ops {
			if journal.KeepBeforeCursor(o.ts, o.seq, cursorMS, cursorSeq) {
				kept = append(kept, o)
			}
		}
		ops = kept
	}
	if len(ops) > limit {
		ops = ops[:limit]
	}

	out := make([]map[string]any, 0, len(ops))
	for _, o := range ops {
		out = append(out, map[string]any{
			"ts_unix_ms": o.ts,
			"op":         o.op,
			"what":       o.what,
			"label":      o.label,
		})
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(ops); n > 0 {
		nextCursor = journal.NextCursor(ops[n-1].ts, ops[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"ops": out, "count": len(out), "next_cursor": nextCursor},
	})
}
