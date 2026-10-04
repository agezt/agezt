// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/system"
)

func (s *Server) systemService() *system.Service {
	var tenants system.TenantCounter
	if s.tenants != nil {
		tenants = s.tenants
	}
	return system.New(s.k, tenants, s.httpBindings, s.channels, s.credChain)
}
