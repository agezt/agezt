// SPDX-License-Identifier: MIT

package controlplane

// Memory-operation log (M85) — a read-only timeline of the journal's
// memory.written / memory.forgotten / memory.superseded events. `agt memory
// list` shows the CURRENT records (the projection); this shows the HISTORY of
// how that state came to be — what the agent learned, forgot, and replaced, and
// when. For a persistent-memory agent that audit is a trust surface: it answers
// "why does it believe this?" and "when did it forget that?".

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

func (s *Server) handleMemoryLog(conn net.Conn, req Request) {
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
	opFilter, _ := req.Args["op"].(string)                                    // written|forgotten|superseded
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"]) // A2 cursor pagination
	cutoff := sinceCutoff(req.Args["since_ms"])                               // M65 helper: optional window

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type memOp struct {
		ts, seq           int64
		op                string // write | revive | forget | supersede
		id, subject, mtyp string
	}
	ops := make([]memOp, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var m memOp
		m.ts, m.seq = e.TSUnixMS, e.Seq
		switch e.Kind {
		case event.KindMemoryWritten:
			var p struct {
				Action, ID, Type, Subject string
			}
			_ = json.Unmarshal(e.Payload, &p)
			m.op = p.Action // "write" or "revive"
			if m.op == "" {
				m.op = "write"
			}
			m.id, m.subject, m.mtyp = p.ID, p.Subject, p.Type
		case event.KindMemoryForgotten:
			var p struct{ ID, Subject string }
			_ = json.Unmarshal(e.Payload, &p)
			m.op, m.id, m.subject = "forget", p.ID, p.Subject
		case event.KindMemorySuperseded:
			var p struct {
				OldID string `json:"old_id"`
				NewID string `json:"new_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			m.op, m.id, m.subject = "supersede", p.OldID, "→ "+p.NewID
		case event.KindMemoryPromoted:
			var p struct {
				ID        string `json:"id"`
				Subject   string `json:"subject"`
				FromScope string `json:"from_scope"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			m.op, m.id, m.subject = "promote", p.ID, p.Subject+" (was private to "+p.FromScope+")"
		default:
			return nil
		}
		// op filter (M85): "written" matches write+revive (both are
		// memory.written); the others match by their own verb.
		if opFilter != "" && !memOpMatches(opFilter, e.Kind, m.op) {
			return nil
		}
		ops = append(ops, m)
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
		for _, m := range ops {
			if journal.KeepBeforeCursor(m.ts, m.seq, cursorMS, cursorSeq) {
				kept = append(kept, m)
			}
		}
		ops = kept
	}
	if len(ops) > limit {
		ops = ops[:limit]
	}

	out := make([]map[string]any, 0, len(ops))
	for _, m := range ops {
		out = append(out, map[string]any{
			"ts_unix_ms": m.ts,
			"seq":        m.seq, // A2: stable per-row id for the frontend cursor pager
			"op":         m.op,
			"id":         m.id,
			"type":       m.mtyp,
			"subject":    m.subject,
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

// memOpMatches maps the user's --op filter to the event kind. "written" keeps
// both write and revive (the memory.written kind); "forgotten"/"superseded"/
// "promoted" match their kinds.
func memOpMatches(filter string, kind event.Kind, op string) bool {
	switch filter {
	case "written", "write":
		return kind == event.KindMemoryWritten
	case "forgotten", "forget":
		return kind == event.KindMemoryForgotten
	case "superseded", "supersede":
		return kind == event.KindMemorySuperseded
	case "promoted", "promote":
		return kind == event.KindMemoryPromoted
	default:
		return op == filter
	}
}
