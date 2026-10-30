package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane tenant-lookup helpers
//             (tenantOf, Server.scopeFor, Server.SetTenants). Extracted from
//             tenant.go during Day 211 god-file refactor (#98). Public API
//             unchanged.

import (
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app/binding"
	"github.com/agezt/agezt/kernel/tenant"
)

func tenantOf(req Request) string {
	t, _, _ := argString(req.Args, "tenant")
	return t
}

// scopeFor is the SCOPE of a request's daemon: the primary's, or the
// tenant's when the request names a tenant.
//
// The tenant's scope is filled in the application layer, from the tenant's own
// kernel: the adapter asks for a scope and gets one, and the tenant's handle
// never reaches the adapter.
func (s *Server) scopeFor(tenantID string) (binding.Scope, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return s.scoped(), nil
	}
	if s.tenants == nil {
		return binding.RefusingScope(), fmt.Errorf("multi-tenancy is disabled (no tenant registry configured)")
	}
	return binding.ScopeForTenant(s.tenants, tenantID, time.Now(), s.baseDir)
}
func (s *Server) SetTenants(r *tenant.Registry) { s.tenants = r }
