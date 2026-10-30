// SPDX-License-Identifier: MIT

package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
)

func (s *Server) standingService() *appstanding.Service {
	ports := s.scoped().StandingPorts
	return appstanding.New(ports.Reader, ports.Writer, appstanding.Host{Agent: s.rosterPorts().Get, ManagedDirectError: approster.ManagedDirectCallError})
}
