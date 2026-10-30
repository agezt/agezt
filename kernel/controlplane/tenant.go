// SPDX-License-Identifier: MIT

package controlplane

import (
	"time"

	appruns "github.com/agezt/agezt/kernel/app/runs"
	apptenants "github.com/agezt/agezt/kernel/app/tenants"
)

// tenantService binds the tenant operations to the daemon's registry, or to
// none when multi-tenancy is disabled, and summarises each tenant's runs from
// its own scope.
func (s *Server) tenantService() *apptenants.Service {
	var registry apptenants.Registry
	if s.tenants != nil {
		registry = s.tenants
	}
	activity := func(id string) (func() (map[string]appruns.Run, error), error) {
		scope, err := s.scopeFor(id)
		if err != nil {
			return nil, err
		}
		return func() (map[string]appruns.Run, error) {
			return appruns.Collect(scope.JournalPorts.Events.Range)
		}, nil
	}
	return apptenants.New(registry, activity, time.Now)
}
