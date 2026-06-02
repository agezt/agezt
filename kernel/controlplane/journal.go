// SPDX-License-Identifier: MIT

package controlplane

// Journal tail handler — one-shot historical read of the last N
// events. Complements CmdPulseSubscribe (live tail) and CmdWhy
// (correlation-scoped walk): tail is "what just happened across
// every correlation?", useful for smoke tests and postmortems
// that want a scroll buffer without committing to a streaming
// subscription.

import (
	"net"
)

const (
	defaultJournalTailN = 20
	maxJournalTailN     = 10_000
)

// handleJournalHead serves CmdJournalHead — the minimal-payload
// "what's the current head?" query. Sister to handleJournalTail
// (which also returns head, but bundles N events). Useful for
// operators who just need a seq checkpoint to pass to
// `pulse --since <seq>` later.
func (s *Server) handleJournalHead(conn net.Conn, req Request) {
	headSeq, headHash := s.k.Journal().Head()
	// Match handleJournalTail's clamp: an empty journal reports
	// head=0, not -1. Hash is empty in both the empty and pre-genesis
	// cases — operators shouldn't need to special-case either.
	if headSeq < 0 {
		headSeq = 0
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"head": headSeq,
			"hash": headHash,
		},
	})
}

func (s *Server) handleJournalTail(conn net.Conn, req Request) {
	n := defaultJournalTailN
	if raw, ok := req.Args["n"]; ok {
		// JSON decodes integers as float64; coerce defensively.
		switch v := raw.(type) {
		case float64:
			n = int(v)
		case int:
			n = v
		case int64:
			n = int(v)
		}
	}
	if n < 1 {
		n = 1
	}
	if n > maxJournalTailN {
		n = maxJournalTailN
	}

	headSeq, _ := s.k.Journal().Head()
	if headSeq < 0 {
		headSeq = 0
	}

	// Read only the last n events (reverse segment read), not the whole journal —
	// the tail of a multi-gigabyte journal shouldn't scan every segment.
	tail, err := s.k.Journal().Tail(n)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Materialise as []any so the JSON encoder emits an array even
	// when empty (matches the shape every other list-returning
	// handler uses).
	out := make([]any, 0, len(tail))
	for _, e := range tail {
		out = append(out, e)
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events": out,
			"count":  len(tail),
			"head":   headSeq,
		},
	})
}
