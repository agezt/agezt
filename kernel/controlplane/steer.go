// SPDX-License-Identifier: MIT

package controlplane

// Live run-steering handlers (M608): pause / resume / step / inject for a
// single in-flight run, the control-plane face of kernel.PauseRun et al. Each
// is tenant-routable (kernelFor(tenantOf(req))) so a tenant can steer its own
// runs without the primary token — the same posture as cancel_run.

import "net"

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
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"accepted":    k.SteerRun(corr, directive),
	}})
}
