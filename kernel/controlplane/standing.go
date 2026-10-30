// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
)

func (s *Server) standingService() *appstanding.Service {
	ports := binding.Fill(s.k, appstanding.PortsFromKernel)
	return appstanding.New(ports.Reader, ports.Writer, appstanding.Host{Agent: s.rosterPorts().Get, ManagedDirectError: approster.ManagedDirectCallError})
}
