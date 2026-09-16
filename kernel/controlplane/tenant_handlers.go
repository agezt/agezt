// SPDX-License-Identifier: MIT
//
// kernel/controlplane tenant HTTP handlers (handleTenantCreate, handleTenantToken,
// handleTenantList, handleTenantRelease, handleTenantRemove, handleTenantStats).
// Extracted from tenant.go during Day 211 god-file refactor (#98).
// Public API unchanged.
package controlplane

import (
	"net"
	"time"
)

func (s *Server) handleTenantCreate(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	existed := s.tenants.Exists(id)
	t, err := s.tenants.Acquire(id, time.Now())
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": t.ID, "base_dir": t.BaseDir, "created": !existed, "token": t.Token,
		},
	})
}
func (s *Server) handleTenantToken(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	token, err := s.tenants.Token(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"id": id, "token": token}})
}
func (s *Server) handleTenantList(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	infos, err := s.tenants.List()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]map[string]any, 0, len(infos))
	for _, i := range infos {
		out = append(out, map[string]any{"id": i.ID, "base_dir": i.BaseDir, "open": i.Open})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"tenants": out, "count": len(out)},
	})
}
func (s *Server) handleTenantRelease(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	released, err := s.tenants.Release(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"released": released}})
}
func (s *Server) handleTenantRemove(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removed, err := s.tenants.Remove(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}
func (s *Server) handleTenantStats(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	infos, err := s.tenants.List()
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	rows := make([]map[string]any, 0, len(infos))
	var totalRuns int
	var totalSpent int64
	for _, info := range infos {
		k, err := s.kernelFor(info.ID)
		if err != nil {
			rows = append(rows, map[string]any{"id": info.ID, "error": err.Error()})
			continue
		}
		runs, err := s.collectRuns(k)
		if err != nil {
			rows = append(rows, map[string]any{"id": info.ID, "error": err.Error()})
			if !info.Open {
				_, _ = s.tenants.Release(info.ID)
			}
			continue
		}
		var total, completed, failed, active int
		var spent, lastMS int64
		for _, r := range runs {
			total++
			spent += r.SpentMicrocents
			for _, ts := range []int64{r.StartedUnixMS, r.CompletedUnixMS, r.FailedUnixMS} {
				if ts > lastMS {
					lastMS = ts
				}
			}
			switch {
			case r.Completed:
				completed++
			case r.Failed:
				failed++
			default:
				active++ // running or abandoned
			}
		}
		rows = append(rows, map[string]any{
			"id":                    info.ID,
			"runs":                  total,
			"completed":             completed,
			"failed":                failed,
			"active":                active,
			"spent_microcents":      spent,
			"last_activity_unix_ms": lastMS,
		})
		totalRuns += total
		totalSpent += spent
		// Restore prior residency: a stats query shouldn't leave a tenant that
		// was closed loaded in memory.
		if !info.Open {
			_, _ = s.tenants.Release(info.ID)
		}
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"tenants":                rows,
			"count":                  len(rows),
			"total_runs":             totalRuns,
			"total_spent_microcents": totalSpent,
		},
	})
}
