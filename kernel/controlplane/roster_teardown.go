// SPDX-License-Identifier: MIT

package controlplane

// The agent teardown's host bindings: the standing-order and schedule stores
// the application's teardown decisions act on. The DECISIONS — which orders
// and schedules belong to the agent, what a teardown retires, pauses or
// removes — live in the application layer
// (`kernel/app/roster/teardown.go`); what stays here is the host's data
// access: the stores and their writes.

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/cadence"
	core "github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// teardownPorts binds the host's stores to the application's teardown.
func (s *Server) teardownPorts() approster.TeardownPorts {
	return approster.TeardownPorts{
		RetireChild: func(slug, reason string) error {
			_, err := s.k.SetProfileRetired(slug, true, reason)
			return err
		},
		Standing:           func() []standing.Order { return s.k.Standing().List() },
		RemoveStanding:     func(id string) (bool, error) { return s.k.RemoveStanding(id) },
		SetStandingEnabled: func(id string, enabled bool) error { _, err := s.k.SetStandingEnabled(id, enabled); return err },
		Schedules:          func() []cadence.Entry { return s.k.Schedules().List() },
		RemoveSchedule:     func(id string) (bool, error) { return s.k.Schedules().Remove(id) },
		SetScheduleEnabled: func(id string, enabled bool) (bool, error) { return s.k.Schedules().SetEnabled(id, enabled) },
	}
}

// retireAgentSubagents retires the agent's sub-agents and pauses what they
// still wake.
func (s *Server) retireAgentSubagents(parent string, children []core.Profile, on bool) (int, []string, error) {
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
