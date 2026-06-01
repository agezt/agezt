// SPDX-License-Identifier: MIT

package controlplane

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
)

// tenantOf extracts the optional "tenant" routing arg from a request (empty
// when absent or not a string → the primary kernel).
func tenantOf(req Request) string {
	t, _ := req.Args["tenant"].(string)
	return t
}

// kernelFor resolves the kernel that should handle a request: the primary kernel
// when tenantID is empty (the single-tenant default), otherwise the named
// tenant's kernel, opening it on demand. It errors if a tenant is requested but
// multi-tenancy is disabled or the tenant cannot be opened.
func (s *Server) kernelFor(tenantID string) (*runtime.Kernel, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return s.k, nil
	}
	if s.tenants == nil {
		return nil, fmt.Errorf("multi-tenancy is disabled (no tenant registry configured)")
	}
	t, err := s.tenants.Acquire(tenantID, time.Now())
	if err != nil {
		return nil, err
	}
	k, ok := t.Kernel.(*runtime.Kernel)
	if !ok {
		return nil, fmt.Errorf("tenant %q: kernel is not a *runtime.Kernel", tenantID)
	}
	return k, nil
}

// Multi-tenant management handlers (ROADMAP P6-MULTI) — the control-plane
// surface behind `agt tenant`. They operate on the daemon's tenant.Registry,
// injected via SetTenants. When no registry is configured the handlers return a
// clear "disabled" error rather than dereferencing nil.

// SetTenants injects the daemon's multi-tenant registry. Called once at startup
// when multi-tenancy is enabled.
func (s *Server) SetTenants(r *tenant.Registry) { s.tenants = r }

// tenantTokenAllows is the deny-by-default allowlist of commands a TENANT
// token may invoke (M38). It contains exactly the commands that route to the
// caller's kernel via kernelFor/edictFor — running and cancelling the
// tenant's own work, and managing the tenant's own Edict policy. Everything
// else (tenant-registry management, daemon-global halt/resume/shutdown,
// pulse, and the primary-journal run stats) requires the primary token. New
// tenant-routed commands must be added here explicitly; forgetting to is the
// safe failure (the tenant is denied, not over-granted).
func tenantTokenAllows(cmd string) bool {
	switch cmd {
	case CmdRun, CmdCancelRun,
		CmdRunsList, CmdRunsStats, CmdWhy, CmdWhoami, CmdToolLog, CmdToolStats,
		CmdRateLimitLog, CmdRateLimitStats,
		CmdEdictLog, CmdEdictStats, CmdEdictShow, CmdEdictOverlay, CmdEdictTest, CmdEdictDenyList, CmdEdictDenyAdd,
		CmdEdictDenyRemove, CmdEdictSetLevel, CmdEdictSetMode:
		return true
	default:
		return false
	}
}

func (s *Server) handleTenantCreate(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	existed := s.tenants.Exists(id)
	t, err := s.tenants.Acquire(id, time.Now())
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	token, err := s.tenants.Token(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	released, err := s.tenants.Release(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"released": released}})
}

func (s *Server) handleTenantRemove(conn net.Conn, req Request) {
	if s.tenants == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "multi-tenancy is disabled (no tenant registry configured)"})
		return
	}
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	removed, err := s.tenants.Remove(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}
