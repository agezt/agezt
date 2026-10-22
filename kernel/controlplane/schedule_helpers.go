// SPDX-License-Identifier: MIT

package controlplane

// Schedule selected app-service host bridges.

import (
	"context"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"time"
)

func (s *Server) scheduleEditing() *appschedule.Editing {
	return appschedule.NewEditing(s.k.Schedules(), appschedule.EditingHost{WorkflowName: func(ref string) (string, bool) { w, ok := s.k.Workflows().Get(ref); return w.Name, ok }, Tool: func(name string) bool { _, ok := s.k.Tools()[name]; return ok }, Now: time.Now})
}

func (s *Server) scheduleLifecycle(ctx context.Context) *appschedule.Lifecycle {
	return appschedule.NewLifecycle(s.k.Schedules(), s.validateScheduleRunnable, func(id string, enabled bool, action string, current cadence.Entry) {
		corr := opapi.CorrelationFromContext(ctx)
		if corr == "" {
			corr = s.k.NewCorrelation()
		}
		publishOperatorAction(s.k, "schedule.enable", corr, map[string]any{"id": id, "enabled": enabled, "action": action, "target": current.Target, "agent": current.Agent, "cadence": current.Cadence()})
	})
}

func (s *Server) scheduleFiringReads(k *runtime.Kernel) *appschedule.FiringService {
	return appschedule.NewFiringService(k.Journal(), func() (map[string]appruns.Run, error) {
		return appruns.Collect(k.Journal().Range)
	})
}
func (s *Server) scheduleReads() *appschedule.Service {
	return appschedule.New(s.k.Schedules(), appschedule.Host{LastFirings: func() (map[string]appschedule.LastFiring, error) {
		return s.scheduleFiringReads(s.k).Latest(context.Background())
	}, Validate: s.validateScheduleRunnable, Warning: s.scheduleFrequencyWarning}, time.Now)
}
