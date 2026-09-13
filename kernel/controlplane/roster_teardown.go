// SPDX-License-Identifier: MIT

// Agent teardown orchestration hooks: retireAgentSubagents + the standing
// hooks + the schedule hooks. These are the high-level "what should this
// agent stop doing" mutators.
// The state-store cleanup (memory + skills + config + workspace) lives in
// roster_teardown_state.go.
// Extracted from roster_teardown.go during the Day-204 god-file split.
// Public API unchanged.
package controlplane

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) retireAgentSubagents(parent string, children []roster.Profile, on bool) (int, []string, error) {
	if !on {
		return 0, nil, nil
	}
	retired := 0
	slugs := []string{}
	reason := "parent/owner " + parent + " removed"
	for _, child := range children {
		if child.Retired {
			continue
		}
		if _, err := s.k.SetProfileRetired(child.Slug, true, reason); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentStanding(child.Slug); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentSchedules(child.Slug); err != nil {
			return retired, slugs, err
		}
		slugs = append(slugs, child.Slug)
		retired++
	}
	sort.Strings(slugs)
	return retired, slugs, nil
}

func (s *Server) removeAgentStanding(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, o := range s.k.Standing().List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			ok, err := s.k.RemoveStanding(o.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentStanding(slug string) (int, error) {
	paused := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled || !strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			continue
		}
		if _, err := s.k.SetStandingEnabled(o.ID, false); err != nil {
			return paused, err
		}
		paused++
	}
	return paused, nil
}

func (s *Server) countAgentPausedStanding(slug string) int {
	n := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled && strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			n++
		}
	}
	return n
}

func (s *Server) removeAgentSchedules(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, e := range s.k.Schedules().List() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			ok, err := s.k.Schedules().Remove(e.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentSchedules(slug string) (int, error) {
	paused := 0
	for _, e := range s.k.Schedules().List() {
		if !e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		ok, err := s.k.Schedules().SetEnabled(e.ID, false)
		if err != nil {
			return paused, err
		}
		if ok {
			paused++
		}
	}
	return paused, nil
}

func (s *Server) countAgentPausedSchedules(slug string) int {
	n := 0
	for _, e := range s.k.Schedules().List() {
		if e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		n++
	}
	return n
}

