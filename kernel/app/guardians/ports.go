// SPDX-License-Identifier: MIT

package guardians

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The guardian seeder's ports, filled from the kernel handle.
//
// The seeder writes to THREE of the daemon's libraries: the roster (the
// agents), the standing library (the orders) and the schedule library (the
// schedules). Each of them is the **daemon's** file, each of them may be
// absent, and each absence has its **own** refusal — a daemon with no
// standing library is not a daemon with no schedule library.
//
// The port holds FUNCTIONS: a question is asked when it is asked, and a
// daemon that grows a capability does not grow the port.
//
// The refusals are written **once**, here, and the host does not spell them
// again.

var (
	// errNoStandingLibrary is the answer the guardian host gives when the
	// daemon holds no standing library.
	//
	// The standing library is the daemon's file (W3.50): the kernel holds
	// the contract's store port, and a daemon that was given none holds a nil
	// port. The refusal is the answer — not a panic, and not a silent
	// "nothing to do".
	errNoStandingLibrary = errors.New("guardians: the standing library is not available on this daemon")

	// errNoScheduleLibrary is the answer the guardian host gives when the
	// daemon holds no schedule library.
	//
	// The schedule library is the daemon's file (W3.49): the kernel holds
	// the contract's host port, and a daemon that was given none holds a nil
	// port. The refusal is the answer — not a panic, and not a silent
	// "nothing to do".
	errNoScheduleLibrary = errors.New("guardians: the schedule library is not available on this daemon")

	// errNoDaemon is the answer the guardian host gives when the host holds
	// no daemon at all.
	errNoDaemon = errors.New("guardians: no daemon on this host")
)

// Ports are the guardian seeder's capabilities.
//
// Every capability is held UNEXPORTED: the daemon may be absent, and the port
// answers for that case instead of handing the caller a nil object to call
// into.
type Ports struct {
	agents      func() []agentapi.Profile
	addAgent    func(agentapi.Profile) (agentapi.Profile, error)
	editAgent   func(string, func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	orders      func() []standingapi.Order
	addOrder    func(standingapi.Order) (standingapi.Order, error)
	editOrder   func(string, func(*standingapi.Order)) (standingapi.Order, bool, error)
	setOrder    func(string, bool) (standingapi.Order, error)
	entries     func() []cadenceapi.Entry
	reschedule  func(string, string, time.Duration, int, int) (bool, error)
	addInterval func(string, time.Duration, string, string) (cadenceapi.Entry, error)
	addDaily    func(string, int, string, string) (cadenceapi.Entry, error)
}

// PortsFromKernel fills the guardian seeder's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to seed.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		agents:    k.RosterList,
		addAgent:  k.AddProfile,
		editAgent: k.UpdateProfile,
		orders: func() []standingapi.Order {
			if k.Standing() == nil {
				return nil
			}
			return k.Standing().List()
		},
		addOrder: func(o standingapi.Order) (standingapi.Order, error) {
			if k.Standing() == nil {
				return standingapi.Order{}, errNoStandingLibrary
			}
			return k.Standing().Add(o)
		},
		editOrder: func(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
			if k.Standing() == nil {
				return standingapi.Order{}, false, errNoStandingLibrary
			}
			return k.UpdateStanding(id, mutate)
		},
		setOrder: func(id string, enabled bool) (standingapi.Order, error) {
			if k.Standing() == nil {
				return standingapi.Order{}, errNoStandingLibrary
			}
			return k.SetStandingEnabled(id, enabled)
		},
		entries: func() []cadenceapi.Entry {
			if k.Schedules() == nil {
				return nil
			}
			return k.Schedules().List()
		},
		reschedule: func(id, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
			if k.Schedules() == nil {
				return false, errNoScheduleLibrary
			}
			return k.Schedules().Reschedule(id, mode, interval, atMinutes, 0, days, "", time.Time{}, time.Now())
		},
		addInterval: func(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error) {
			if k.Schedules() == nil {
				return cadenceapi.Entry{}, errNoScheduleLibrary
			}
			e, err := k.Schedules().Add(intent, interval, model, "system", time.Now())
			if err != nil || agent == "" {
				return e, err
			}
			if _, err := k.Schedules().SetAgent(e.ID, agent); err != nil {
				return cadenceapi.Entry{}, err
			}
			e.Agent = agent
			return e, nil
		},
		addDaily: func(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error) {
			if k.Schedules() == nil {
				return cadenceapi.Entry{}, errNoScheduleLibrary
			}
			e, err := k.Schedules().AddDaily(intent, atMinutes, 0, "", model, "system", time.Now())
			if err != nil || agent == "" {
				return e, err
			}
			if _, err := k.Schedules().SetAgent(e.ID, agent); err != nil {
				return cadenceapi.Entry{}, err
			}
			e.Agent = agent
			return e, nil
		},
	}
}

// Ready reports whether the ports hold a daemon to seed.
func (p Ports) Ready() bool { return p.agents != nil }

// Agents lists the agents the daemon knows.
func (p Ports) Agents() []agentapi.Profile {
	if p.agents == nil {
		return nil
	}
	return p.agents()
}

// AddAgent creates an agent.
func (p Ports) AddAgent(profile agentapi.Profile) (agentapi.Profile, error) {
	if p.addAgent == nil {
		return agentapi.Profile{}, errNoDaemon
	}
	return p.addAgent(profile)
}

// EditAgent edits the agent named by ref.
func (p Ports) EditAgent(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	if p.editAgent == nil {
		return agentapi.Profile{}, false, errNoDaemon
	}
	return p.editAgent(ref, mutate)
}

// Orders lists the standing orders the daemon knows.
//
// A daemon with no standing library holds a nil port, and the seeder sees no
// standing orders rather than calling into nothing.
func (p Ports) Orders() []standingapi.Order {
	if p.orders == nil {
		return nil
	}
	return p.orders()
}

// AddOrder creates a standing order.
func (p Ports) AddOrder(order standingapi.Order) (standingapi.Order, error) {
	if p.addOrder == nil {
		return standingapi.Order{}, errNoStandingLibrary
	}
	return p.addOrder(order)
}

// EditOrder edits the order named by id.
func (p Ports) EditOrder(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	if p.editOrder == nil {
		return standingapi.Order{}, false, errNoStandingLibrary
	}
	return p.editOrder(id, mutate)
}

// SetOrderEnabled pauses or resumes an order.
func (p Ports) SetOrderEnabled(id string, enabled bool) (standingapi.Order, error) {
	if p.setOrder == nil {
		return standingapi.Order{}, errNoStandingLibrary
	}
	return p.setOrder(id, enabled)
}

// Entries lists the schedules the daemon knows.
//
// A daemon with no schedule library holds a nil port, and the seeder sees no
// schedules rather than calling into nothing.
func (p Ports) Entries() []cadenceapi.Entry {
	if p.entries == nil {
		return nil
	}
	return p.entries()
}

// Reschedule moves a schedule to a new cadence.
func (p Ports) Reschedule(id, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
	if p.reschedule == nil {
		return false, errNoScheduleLibrary
	}
	return p.reschedule(id, mode, interval, atMinutes, days)
}

// AddInterval plants a repeating schedule.
func (p Ports) AddInterval(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error) {
	if p.addInterval == nil {
		return cadenceapi.Entry{}, errNoScheduleLibrary
	}
	return p.addInterval(intent, interval, model, agent)
}

// AddDaily plants a daily schedule.
func (p Ports) AddDaily(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error) {
	if p.addDaily == nil {
		return cadenceapi.Entry{}, errNoScheduleLibrary
	}
	return p.addDaily(intent, atMinutes, model, agent)
}
