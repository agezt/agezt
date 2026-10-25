// SPDX-License-Identifier: MIT

// Package guardians is the app-layer adapter that lets the guardian seeder —
// a plugin — reach the live daemon without naming the runtime.
//
// The seeder's port lives in the contract layer (kernel/contract/guardianapi).
// Answering it needs the live kernel, and the app layer is the layer that may
// see it: the plugin may not.
package guardians

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/guardianapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// kernelHost answers the seeder's port from the live kernel. Each write goes
// through the kernel's own stores, so seeded guardians, standing orders and
// schedules are persisted and journaled exactly like operator-created ones.
// Schedules are tagged source "system" so SyncEnv (which only replaces "env"
// entries) never prunes them and the UI can tell them apart.
type kernelHost struct{ k *kernelruntime.Kernel }

// NewKernelHost builds the kernel-backed host of the seeder's port.
//
// The nil check is on the CONCRETE type, on purpose: a nil *Kernel handed
// back as an interface would be a non-nil interface wrapping nothing.
func NewKernelHost(k *kernelruntime.Kernel) guardianapi.Host {
	if k == nil {
		return nil
	}
	return &kernelHost{k: k}
}

// Agents lists the agents the daemon knows.
func (h *kernelHost) Agents() []agentapi.Profile { return h.k.RosterList() }

// AddAgent creates an agent.
func (h *kernelHost) AddAgent(p agentapi.Profile) (agentapi.Profile, error) {
	return h.k.AddProfile(p)
}

// UpdateAgent edits the agent named by ref.
func (h *kernelHost) UpdateAgent(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	return h.k.UpdateProfile(ref, mutate)
}

// errNoStandingLibrary is the answer the guardian host gives when the daemon
// holds no standing library.
//
// The standing library is the daemon's file (W3.50): the kernel holds the
// contract's store port, and a host that was given none holds a nil port.
// The refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoStandingLibrary = errors.New("guardians: the standing library is not available on this daemon")

// StandingOrders lists the standing orders the daemon knows.
//
// The standing library is the daemon's file (W3.50): a host without it holds
// a nil port, and the guardian sees no standing orders rather than calling
// into nothing.
func (h *kernelHost) StandingOrders() []standingapi.Order {
	if h.k.Standing() == nil {
		return nil
	}
	return h.k.Standing().List()
}

// UpdateStanding edits the order named by id.
func (h *kernelHost) UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	return h.k.UpdateStanding(id, mutate)
}

// AddStanding creates a standing order.
func (h *kernelHost) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	if h.k.Standing() == nil {
		return standingapi.Order{}, errNoStandingLibrary
	}
	return h.k.Standing().Add(o)
}

// SetStandingEnabled pauses or resumes an order.
func (h *kernelHost) SetStandingEnabled(id string, enabled bool) (standingapi.Order, error) {
	return h.k.SetStandingEnabled(id, enabled)
}

// errNoScheduleLibrary is the answer the guardian host gives when the daemon
// holds no schedule library.
//
// The schedule library is the daemon's file (W3.49): the kernel holds the
// contract's host port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoScheduleLibrary = errors.New("guardians: the schedule library is not available on this daemon")

// Schedules lists the schedules the daemon knows.
//
// The schedule library is the daemon's file (W3.49): a host without it holds
// a nil port, and the guardian sees no schedules rather than calling into
// nothing.
func (h *kernelHost) Schedules() []cadenceapi.Entry {
	if h.k.Schedules() == nil {
		return nil
	}
	return h.k.Schedules().List()
}

// Reschedule moves a schedule to a new cadence.
func (h *kernelHost) Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
	if h.k.Schedules() == nil {
		return false, errNoScheduleLibrary
	}
	return h.k.Schedules().Reschedule(id, mode, interval, atMinutes, 0, days, "", time.Time{}, time.Now())
}

// AddInterval plants a repeating schedule.
func (h *kernelHost) AddInterval(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error) {
	if h.k.Schedules() == nil {
		return cadenceapi.Entry{}, errNoScheduleLibrary
	}
	e, err := h.k.Schedules().Add(intent, interval, model, "system", time.Now())
	if err != nil || agent == "" {
		return e, err
	}
	if _, err := h.k.Schedules().SetAgent(e.ID, agent); err != nil {
		return cadenceapi.Entry{}, err
	}
	e.Agent = agent
	return e, nil
}

// AddDaily plants a daily schedule.
func (h *kernelHost) AddDaily(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error) {
	// days=0 → every day; tz="" → daemon local.
	if h.k.Schedules() == nil {
		return cadenceapi.Entry{}, errNoScheduleLibrary
	}
	e, err := h.k.Schedules().AddDaily(intent, atMinutes, 0, "", model, "system", time.Now())
	if err != nil || agent == "" {
		return e, err
	}
	if _, err := h.k.Schedules().SetAgent(e.ID, agent); err != nil {
		return cadenceapi.Entry{}, err
	}
	e.Agent = agent
	return e, nil
}
