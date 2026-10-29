// SPDX-License-Identifier: MIT

// Package guardians is the app-layer adapter that lets the guardian seeder —
// a plugin — reach the live daemon without naming the runtime.
//
// The seeder's port lives in the contract layer (kernel/contract/guardianapi).
// Answering it needs the live kernel, and the app layer is the layer that may
// see it: the plugin may not.
package guardians

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/guardianapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// kernelHost answers the seeder's port from the seeder's own ports. Each write
// goes through the daemon's own stores, so seeded guardians, standing orders
// and schedules are persisted and journaled exactly like operator-created
// ones. Schedules are tagged source "system" so SyncEnv (which only replaces
// "env" entries) never prunes them and the UI can tell them apart.
//
// The host holds the PORTS, not the handle they were filled from: a file that
// names the runtime is the port's business, and the adapter that answers the
// contract is not.
type kernelHost struct{ ports Ports }

// NewKernelHost builds the seeder's host from its ports.
//
// A host with no daemon holds no daemon to seed, and the seeder is handed a
// NIL port rather than a port that calls into nothing: the refusal is written
// **once**, in the ports, and the composition root does not spell it again.
func NewKernelHost(ports Ports) guardianapi.Host {
	if !ports.Ready() {
		return nil
	}
	return &kernelHost{ports: ports}
}

// Agents lists the agents the daemon knows.
func (h *kernelHost) Agents() []agentapi.Profile { return h.ports.Agents() }

// AddAgent creates an agent.
func (h *kernelHost) AddAgent(p agentapi.Profile) (agentapi.Profile, error) {
	return h.ports.AddAgent(p)
}

// UpdateAgent edits the agent named by ref.
func (h *kernelHost) UpdateAgent(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	return h.ports.EditAgent(ref, mutate)
}

// StandingOrders lists the standing orders the daemon knows.
//
// The standing library is the daemon's file (W3.50): a host without it holds
// a nil port, and the guardian sees no standing orders rather than calling
// into nothing.
func (h *kernelHost) StandingOrders() []standingapi.Order {
	return h.ports.Orders()
}

// UpdateStanding edits the order named by id.
func (h *kernelHost) UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	return h.ports.EditOrder(id, mutate)
}

// AddStanding creates a standing order.
func (h *kernelHost) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	return h.ports.AddOrder(o)
}

// SetStandingEnabled pauses or resumes an order.
func (h *kernelHost) SetStandingEnabled(id string, enabled bool) (standingapi.Order, error) {
	return h.ports.SetOrderEnabled(id, enabled)
}

// Schedules lists the schedules the daemon knows.
//
// The schedule library is the daemon's file (W3.49): a host without it holds
// a nil port, and the guardian sees no schedules rather than calling into
// nothing.
func (h *kernelHost) Schedules() []cadenceapi.Entry {
	return h.ports.Entries()
}

// Reschedule moves a schedule to a new cadence.
func (h *kernelHost) Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
	return h.ports.Reschedule(id, mode, interval, atMinutes, days)
}

// AddInterval plants a repeating schedule.
func (h *kernelHost) AddInterval(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error) {
	return h.ports.AddInterval(intent, interval, model, agent)
}

// AddDaily plants a daily schedule.
func (h *kernelHost) AddDaily(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error) {
	return h.ports.AddDaily(intent, atMinutes, model, agent)
}
