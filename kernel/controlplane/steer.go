// SPDX-License-Identifier: MIT

package controlplane

// Live run-steering handlers (M608): pause / resume / step / inject for a
// single in-flight run, the control-plane face of kernel.PauseRun et al. Each
// is tenant-routable (kernelFor(tenantOf(req))) so a tenant can steer its own
// runs without the primary token — the same posture as cancel_run.

import (
	"net"
	"time"

	"github.com/agezt/agezt/kernel/intervention"
)

// runCorr pulls and validates the required correlation arg shared by every
// steering handler, writing the error response itself. ok=false ⇒ return.
func (s *Server) runCorr(conn net.Conn, req Request) (string, bool) {
	corr, err := requiredArgString(req.Args, "correlation")
	if err != nil {
		s.fail(conn, req, err)
		return "", false
	}
	return corr, true
}

func (s *Server) handleRunPause(conn net.Conn, req Request) {
	corr, ok := s.runCorr(conn, req)
	if !ok {
		return
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"ok":          k.PauseRun(corr),
	}})
}

func (s *Server) handleRunResume(conn net.Conn, req Request) {
	corr, ok := s.runCorr(conn, req)
	if !ok {
		return
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"ok":          k.ResumeRun(corr),
	}})
}

func (s *Server) handleRunStep(conn net.Conn, req Request) {
	corr, ok := s.runCorr(conn, req)
	if !ok {
		return
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"ok":          k.StepRun(corr),
	}})
}

func (s *Server) handleRunSteer(conn net.Conn, req Request) {
	corr, ok := s.runCorr(conn, req)
	if !ok {
		return
	}
	directive, err := requiredArgString(req.Args, "directive")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// mode "note" = a soft BTW (read it, stay on task); anything else = a forceful
	// steer that re-prioritises (the default, M962).
	mode, _, err := argString(req.Args, "mode")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	note := mode == "note"
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"mode":        map[bool]string{true: "note", false: "steer"}[note],
		"accepted":    k.SteerRun(corr, directive, note),
	}})
}

func (s *Server) handleRunIntervene(conn net.Conn, req Request) {
	corr, ok := s.runCorr(conn, req)
	if !ok {
		return
	}
	sa, err := argStrings(req.Args, "primitive", "directive", "scope", "idempotency_key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	primitive, directive, scope, key := sa["primitive"], sa["directive"], sa["scope"], sa["idempotency_key"]
	var lease time.Duration
	if ms, _, lerr := argFloat64(req.Args, "lease_ms"); lerr != nil {
		s.fail(conn, req, lerr)
		return
	} else if ms > 0 {
		lease = time.Duration(ms) * time.Millisecond
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	res, err := k.InterveneRun(intervention.Request{
		Primitive:      intervention.Primitive(primitive),
		CorrelationID:  corr,
		Directive:      directive,
		Lease:          lease,
		Scope:          scope,
		IdempotencyKey: key,
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := map[string]any{
		"primitive":       string(res.Primitive),
		"correlation":     res.CorrelationID,
		"accepted":        res.Accepted,
		"applied":         res.Applied,
		"state":           res.State,
		"paused":          res.Paused,
		"pending":         res.Pending,
		"idempotency_key": res.IdempotencyKey,
		"reason":          res.Reason,
	}
	if !res.LeaseExpires.IsZero() {
		out["lease_expires_unix"] = res.LeaseExpires.Unix()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}

// registerSteerCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerSteerCommands() {
	register(
		commandSpec{Cmd: CmdRunPause, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunPause(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunResume, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunResume(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunStep, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunStep(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunSteer, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunSteer(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRunIntervene, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleRunIntervene(dc.Conn, dc.Req) }},
	)
}
