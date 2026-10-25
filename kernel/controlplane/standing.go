// SPDX-License-Identifier: MIT

package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
)

func (s *Server) standingService() *appstanding.Service {
	return appstanding.New(s.k.Standing(), s.k, appstanding.Host{Agent: s.k.RosterGet, ManagedDirectError: approster.ManagedDirectCallError})
}
