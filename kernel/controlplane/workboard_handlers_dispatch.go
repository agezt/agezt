// SPDX-License-Identifier: MIT
package controlplane

import (
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/workboard"
)

func (s *Server) workboardDispatcher() *appworkboard.Dispatch {
	service := appworkboard.NewDispatch(s.k.Workboard(), s.k,
		func(ref string) (appworkboard.DispatchAgent, bool) {
			p, ok := s.k.Roster().Get(ref)
			if !ok {
				return appworkboard.DispatchAgent{}, false
			}
			direct := p.AllowsDirectCall()
			directError := ""
			if !direct {
				directError = managedSubagentDirectCallError(p, "dispatched")
			}
			return appworkboard.DispatchAgent{Slug: p.Slug, Retired: p.Retired, Enabled: p.Enabled, DirectAllowed: direct, DirectError: directError, Run: func(corr string, task workboard.Task, intent, reason string) {
				s.runWorkboardDispatch(corr, p, task, intent, reason)
			}}, true
		},
		func(corr string, task workboard.Task, phase, agent, reason, answer, errText string) {
			publishWorkboardDispatch(s.k, corr, task, phase, agent, reason, answer, errText)
		})
	return service
}
