package controlplane

// Provenance: SPDX-License-Identifier: MIT Control-plane command handlers:
//             handleVersion + handleHalt + handleResume +
//             handleWhy + handleWhoami + handleVerify + handleApprovals. Extracted
//             from server_handlers.go during the Day-206 god-file split. Public API
//             unchanged.

import (
	"net"
)

// ----- command handlers -----

func (s *Server) handleHalt(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.HaltWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": true,
		"reason": reason,
	}})
}

func (s *Server) handleResume(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.ResumeWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": false,
		"reason": reason,
	}})
}

func (s *Server) handleWhy(conn net.Conn, req Request) {
	idAny := req.Args["event_id"]
	id, _ := idAny.(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.event_id required"})
		return
	}
	// Tenant-scoped (M53): an empty tenant traces the primary journal; a named
	// tenant traces its own isolated journal, so a tenant walks only its own
	// events — completing tenant isolation on the observability surface (M39
	// did runs list/stats; this does why).
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	events, err := k.Why(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, e)
	}
	// Parent backlink (M42): if this chain belongs to a sub-agent run,
	// surface its lead's correlation so an operator can walk child→parent
	// (only parent→child was visible before). correlation is the chain's
	// shared id; parent_correlation is "" for top-level runs.
	corr := ""
	if len(events) > 0 {
		corr = events[0].CorrelationID
	}
	parent := ""
	if corr != "" {
		parent = k.ParentOf(corr)
	}
	// Causation provenance (SPEC-01 §7.1): the chain of events linked by
	// causation_id from the root cause down to this one, ordered oldest-first.
	// Unlike the correlation grouping above, this crosses correlation
	// boundaries — e.g. a Pulse initiative back to its originating tick, which
	// carries a different correlation and is therefore absent from `events`.
	// Best-effort: a failure here must not sink the whole why response.
	causation := make([]any, 0, 4)
	if chain, cErr := k.Causes(id); cErr == nil && len(chain) > 1 {
		for _, e := range chain {
			causation = append(causation, e)
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":             out,
			"correlation":        corr,
			"parent_correlation": parent,
			"causation_chain":    causation,
		},
	})
}

// handleWhoami reports the authenticated principal (M62). By the time a request
// reaches a handler, handleConn has already verified the token: the primary
// token equals s.Token(); any other token that got here authenticated as the
// tenant named in (and pinned to) req.Args["tenant"]. So identity is a pure
// read of req.Token vs the primary token — no new auth state.
func (s *Server) handleWhoami(conn net.Conn, req Request) {
	if s.tokenIsPrimary(req.Token) {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"identity": "primary",
				"primary":  true,
				"tenant":   "",
			},
		})
		return
	}
	tenant, _, _ := argString(req.Args, "tenant")
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"identity": "tenant",
			"primary":  false,
			"tenant":   tenant,
		},
	})
}

func (s *Server) handleVerify(conn net.Conn, req Request) {
	if err := s.k.Verify(); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true}})
}
