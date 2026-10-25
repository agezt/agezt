// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
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
				directError = approster.ManagedDirectCallError(p, "dispatched")
			}
			return appworkboard.DispatchAgent{Slug: p.Slug, Retired: p.Retired, Enabled: p.Enabled, DirectAllowed: direct, DirectError: directError, Run: func(corr string, task workboardapi.Task, intent, reason string) {
				s.runWorkboardDispatch(corr, p, task, intent, reason)
			}}, true
		},
		func(corr string, task workboardapi.Task, phase, agent, reason, answer, errText string) {
			publishWorkboardDispatch(s.k, corr, task, phase, agent, reason, answer, errText)
		})
	return service
}
