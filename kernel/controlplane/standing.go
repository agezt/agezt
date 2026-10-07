// SPDX-License-Identifier: MIT

package controlplane

import (
	appstanding "github.com/agezt/agezt/kernel/app/standing"
)

func (s *Server) standingService() *appstanding.Service {
	return appstanding.New(s.k.Standing(), s.k, appstanding.Host{Agent: s.k.Roster().Get, ManagedDirectError: managedSubagentDirectCallError})
}
