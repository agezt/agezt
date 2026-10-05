// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"

	"github.com/agezt/agezt/kernel/app/providers"
)

func (s *Server) handleProviderStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	sinceMS := int64Arg(req.Args["since_ms"])
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	result, err := providers.NewObservations(k.Journal()).Stats(providers.ObservationInput{CutoffMS: cutoff, WindowMS: sinceMS})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

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
		s.fail(conn, req, err)
		return
	}

	result, err := providers.NewObservations(k.Journal()).Rejections(providers.ObservationInput{Limit: limit, CutoffMS: cutoff})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleProviderLog(conn net.Conn, req Request) {
	fallbacksOnly, _, err := argBool(req.Args, "fallbacks")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
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
		s.fail(conn, req, err)
		return
	}

	result, err := providers.NewObservations(k.Journal()).Log(providers.ObservationInput{Limit: limit, CutoffMS: cutoff, Cursor: req.Args["cursor"], FallbacksOnly: fallbacksOnly})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
