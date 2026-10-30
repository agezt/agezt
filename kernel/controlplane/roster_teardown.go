// SPDX-License-Identifier: MIT

package controlplane

// The agent teardown's host bindings: the standing-order and schedule stores
// the application's teardown decisions act on. The DECISIONS — which orders
// and schedules belong to the agent, what a teardown retires, pauses or
// removes — live in the application layer
// (`kernel/app/roster/teardown.go`); what stays here is the host's data
// access: the stores and their writes.

import (
	"github.com/agezt/agezt/kernel/app/binding"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// teardownPorts binds the host's stores to the application's teardown.
func (s *Server) teardownPorts() approster.TeardownPorts {
	standing := binding.Fill(s.k, appstanding.PortsFromKernel)
	schedule := binding.Fill(s.k, appschedule.PortsFromKernel)
	return approster.TeardownPorts{
		RetireChild: func(slug, reason string) error {
			_, err := s.rosterPorts().SetRetired(slug, true, reason)
			return err
		},
		Standing:       standing.Orders,
		RemoveStanding: standing.Remove,
		SetStandingEnabled: func(id string, enabled bool) error {
			return standing.SetEnabled(id, enabled)
		},
		Schedules:      schedule.Entries,
		RemoveSchedule: schedule.Remove,
		SetScheduleEnabled: func(id string, enabled bool) (bool, error) {
			return schedule.SetEnabled(id, enabled)
		},
	}
}

// retireAgentSubagents retires the agent's sub-agents and pauses what they
// still wake.
func (s *Server) retireAgentSubagents(parent string, children []agentapi.Profile, on bool) (int, []string, error) {
	return approster.RetireSubagents(s.teardownPorts(), parent, children, on)
}

// removeAgentStanding removes the standing orders that wake the agent.
func (s *Server) removeAgentStanding(slug string, on bool) (int, error) {
	return approster.RemoveStandingOrders(s.teardownPorts(), slug, on)
}

// pauseAgentStanding pauses the enabled standing orders that wake the agent.
func (s *Server) pauseAgentStanding(slug string) (int, error) {
	return approster.PauseStandingOrders(s.teardownPorts(), slug)
}

// countAgentPausedStanding reports how many of the agent's standing orders
// are paused.
func (s *Server) countAgentPausedStanding(slug string) int {
	return approster.CountPausedStandingOrders(s.teardownPorts(), slug)
}

// removeAgentSchedules removes the schedules that wake the agent.
func (s *Server) removeAgentSchedules(slug string, on bool) (int, error) {
	return approster.RemoveSchedules(s.teardownPorts(), slug, on)
}

// pauseAgentSchedules pauses the enabled schedules that wake the agent.
func (s *Server) pauseAgentSchedules(slug string) (int, error) {
	return approster.PauseSchedules(s.teardownPorts(), slug)
}

// countAgentPausedSchedules reports how many of the agent's schedules are
// paused.
func (s *Server) countAgentPausedSchedules(slug string) int {
	return approster.CountPausedSchedules(s.teardownPorts(), slug)
}
