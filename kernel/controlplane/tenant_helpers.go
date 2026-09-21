// SPDX-License-Identifier: MIT
//
// kernel/controlplane tenant-lookup helpers (tenantOf, Server.kernelFor, Server.SetTenants).
// Extracted from tenant.go during Day 211 god-file refactor (#98).
// Public API unchanged.
package controlplane

import (
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
)

func tenantOf(req Request) string {
	t, _, _ := argString(req.Args, "tenant")
	return t
}
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
func (s *Server) SetTenants(r *tenant.Registry) { s.tenants = r }
