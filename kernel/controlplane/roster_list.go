// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) rosterListService() *approster.ListService {
	s.rosterListOnce.Do(func() {
		s.rosterList = approster.NewList(func() []roster.Profile { return s.k.Roster().List() }, s.agentStatusViews, nil)
	})
	return s.rosterList
}
func (s *Server) invalidateAgentListCache() { s.rosterListService().Invalidate() }
