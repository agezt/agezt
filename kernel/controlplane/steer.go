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
	corr, _ := req.Args["correlation"].(string)
	if corr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.correlation required"})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	directive, _ := req.Args["directive"].(string)
	if directive == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.directive required"})
		return
	}
	// mode "note" = a soft BTW (read it, stay on task); anything else = a forceful
	// steer that re-prioritises (the default, M962).
	mode, _ := req.Args["mode"].(string)
	note := mode == "note"
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	primitive, _ := req.Args["primitive"].(string)
	directive, _ := req.Args["directive"].(string)
	scope, _ := req.Args["scope"].(string)
	key, _ := req.Args["idempotency_key"].(string)
	var lease time.Duration
	if ms, ok := req.Args["lease_ms"].(float64); ok && ms > 0 {
		lease = time.Duration(ms) * time.Millisecond
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
