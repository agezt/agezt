// SPDX-License-Identifier: MIT

package roster

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	core "github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// TeardownPorts are the host's capabilities the agent teardown needs. The
// application owns the DECISIONS (which standing orders and schedules belong
// to the agent, what a teardown retires, pauses or removes and in which
// order); the host owns the DATA (the stores and their writes).
type TeardownPorts struct {
	// RetireChild retires a sub-agent with the given reason.
	RetireChild func(slug, reason string) error
	// Standing lists the standing orders.
	Standing func() []standing.Order
	// RemoveStanding removes a standing order; it reports whether it existed.
	RemoveStanding func(id string) (bool, error)
	// SetStandingEnabled enables or disables a standing order.
	SetStandingEnabled func(id string, enabled bool) error
	// Schedules lists the cadence entries.
	Schedules func() []cadence.Entry
	// RemoveSchedule removes a cadence entry; it reports whether it existed.
	RemoveSchedule func(id string) (bool, error)
	// SetScheduleEnabled enables or disables a cadence entry; it reports
	// whether the entry existed.
	SetScheduleEnabled func(id string, enabled bool) (bool, error)
}

// RetireSubagents retires the agent's sub-agents and pauses what they still
// wake, so a removed agent leaves nothing running behind it. The retired
// slugs come back sorted.
func RetireSubagents(ports TeardownPorts, parent string, children []core.Profile, on bool) (int, []string, error) {
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
		if err := ports.RetireChild(child.Slug, reason); err != nil {
			return retired, slugs, err
		}
		if _, err := PauseStandingOrders(ports, child.Slug); err != nil {
			return retired, slugs, err
		}
		if _, err := PauseSchedules(ports, child.Slug); err != nil {
			return retired, slugs, err
		}
		slugs = append(slugs, child.Slug)
		retired++
	}
	sort.Strings(slugs)
	return retired, slugs, nil
}

// RemoveStandingOrders removes the standing orders that wake the agent.
func RemoveStandingOrders(ports TeardownPorts, slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, o := range ports.Standing() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			ok, err := ports.RemoveStanding(o.ID)
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

// PauseStandingOrders pauses the enabled standing orders that wake the agent
// and reports how many it paused.
func PauseStandingOrders(ports TeardownPorts, slug string) (int, error) {
	paused := 0
	for _, o := range ports.Standing() {
		if !o.Enabled || !strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			continue
		}
		if err := ports.SetStandingEnabled(o.ID, false); err != nil {
			return paused, err
		}
		paused++
	}
	return paused, nil
}

// CountPausedStandingOrders reports how many of the agent's standing orders
// are paused.
func CountPausedStandingOrders(ports TeardownPorts, slug string) int {
	n := 0
	for _, o := range ports.Standing() {
		if !o.Enabled && strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			n++
		}
	}
	return n
}

// RemoveSchedules removes the schedules that wake the agent.
func RemoveSchedules(ports TeardownPorts, slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, e := range ports.Schedules() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			ok, err := ports.RemoveSchedule(e.ID)
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

// PauseSchedules pauses the enabled schedules that wake the agent and
// reports how many it paused.
func PauseSchedules(ports TeardownPorts, slug string) (int, error) {
	paused := 0
	for _, e := range ports.Schedules() {
		if !e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		ok, err := ports.SetScheduleEnabled(e.ID, false)
		if err != nil {
			return paused, err
		}
		if ok {
			paused++
		}
	}
	return paused, nil
}

// CountPausedSchedules reports how many of the agent's schedules are paused.
func CountPausedSchedules(ports TeardownPorts, slug string) int {
	n := 0
	for _, e := range ports.Schedules() {
		if e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		n++
	}
	return n
}
