// SPDX-License-Identifier: MIT

package controlplane

import (
	"time"

	appjournal "github.com/agezt/agezt/kernel/app/journal"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	apptenants "github.com/agezt/agezt/kernel/app/tenants"
)

// tenantService binds the tenant operations to the daemon's registry, or to
// none when multi-tenancy is disabled, and summarises each tenant's runs from
// its own kernel.
func (s *Server) tenantService() *apptenants.Service {
	var registry apptenants.Registry
	if s.tenants != nil {
		registry = s.tenants
	}
	activity := func(id string) (func() (map[string]appruns.Run, error), error) {
		k, err := s.kernelFor(id)
		if err != nil {
			return nil, err
		}
		return func() (map[string]appruns.Run, error) {
			return appruns.Collect(appjournal.PortsFromKernel(k).Events.Range)
		}, nil
	}
	return apptenants.New(registry, activity, time.Now)
}
