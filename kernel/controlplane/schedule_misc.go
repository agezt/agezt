// SPDX-License-Identifier: MIT

package controlplane

import (
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
)

func (s *Server) scheduleAdmission() *appschedule.Admission {
	return appschedule.NewAdmission(appschedule.AdmissionHost{
		Agent:              s.k.Roster().Get,
		WorkflowName:       func(ref string) (string, bool) { w, ok := s.k.Workflows().Get(ref); return w.Name, ok },
		Workflow:           func(ref string) bool { _, ok := s.k.Workflows().Get(ref); return ok },
		Tool:               func(name string) bool { _, ok := s.k.Tools()[name]; return ok },
		ManagedDirectError: managedSubagentDirectCallError,
	})
}
func (s *Server) validateScheduleRunnable(e cadence.Entry) error {
	return s.scheduleAdmission().Runnable(e)
}

func (s *Server) scheduleFrequencyWarning(e cadence.Entry) string {
	return s.scheduleAdmission().Warning(e)
}
