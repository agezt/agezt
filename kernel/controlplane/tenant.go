// SPDX-License-Identifier: MIT

package controlplane

import (
	"time"

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
	activity := func(id string) (func() ([]apptenants.Run, error), error) {
		k, err := s.kernelFor(id)
		if err != nil {
			return nil, err
		}
		return func() ([]apptenants.Run, error) {
			entries, err := s.collectRuns(k)
			if err != nil {
				return nil, err
			}
			runs := make([]apptenants.Run, 0, len(entries))
			for _, r := range entries {
				runs = append(runs, apptenants.Run{SpentMicrocents: r.SpentMicrocents, StartedUnixMS: r.StartedUnixMS, CompletedUnixMS: r.CompletedUnixMS, FailedUnixMS: r.FailedUnixMS, Completed: r.Completed, Failed: r.Failed})
			}
			return runs, nil
		}, nil
	}
	return apptenants.New(registry, activity, time.Now)
}
