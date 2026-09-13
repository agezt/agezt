// SPDX-License-Identifier: MIT
//
// Control-plane command handlers: handleVersion + handleHalt +
// handleCancelRun + handleResume + handleWhy + handleWhoami +
// handleVerify + handleApprovals.
// Extracted from server_handlers.go during the Day-206 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"

	"github.com/agezt/agezt/internal/brand"
)

// ----- command handlers -----

func (s *Server) handleVersion(conn net.Conn, req Request) {
	rev, committed, modified := brand.BuildInfo()
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			brand.Binary:       brand.Version,
			"protocol_version": brand.ProtocolVersion,
			// Build provenance (M971) — lets operators confirm which build a
			// daemon is actually running, since the semver only moves per release.
			"revision":       rev,
			"built":          committed,
			"build_modified": modified,
		},
	})
}

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

// handleCancelRun cancels a single in-flight run by correlation id (M32),
// leaving the kernel un-halted and other runs untouched — the targeted
// alternative to the global halt. Routes to the tenant kernel when a
// tenant is named (empty → primary), mirroring handleRun.
func (s *Server) handleCancelRun(conn net.Conn, req Request) {
	corr, err := requiredArgString(req.Args, "correlation")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	tenantID, _, err := argString(req.Args, "tenant")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cancelled := k.CancelRun(corr)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"cancelled":   cancelled,
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
	tenant, _ := req.Args["tenant"].(string)
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

func (s *Server) handleApprovals(conn net.Conn, req Request) {
	pending := s.k.Approvals().Pending()
	out := make([]map[string]any, 0, len(pending))
	for _, p := range pending {
		out = append(out, map[string]any{
			"id":                     p.ID,
			"capability":             p.Capability,
			"tool_name":              p.ToolName,
			"input":                  p.Input,
			"reason":                 p.Reason,
			"actor":                  p.Actor,
			"correlation_id":         p.CorrelationID,
			"created_unix":           p.CreatedAt.Unix(),
			"timeout_unix":           p.Timeout.Unix(),
			"effect_class":           p.EffectClass,
			"predicted_effects":      p.PredictedEffects,
			"affected_resources":     p.AffectedResources,
			"rollback_notes":         p.RollbackNotes,
			"confidence":             p.Confidence,
			"canonical_intent":       p.CanonicalIntent,
			"harmful_interpretation": p.HarmfulInterpretation,
			"ambiguity_score":        p.AmbiguityScore,
			"regret_axes":            p.RegretAxes,
			"confirmation_prompt":    p.ConfirmationPrompt,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"pending": out, "count": len(out)},
	})
}

