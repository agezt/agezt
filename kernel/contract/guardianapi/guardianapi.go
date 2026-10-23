// SPDX-License-Identifier: MIT

// Package guardianapi is the contract the guardian seeder and the
// implementation that answers it agree on.
//
// The guardians are the shipped internal agents — the health responder, the
// trust-ceiling sweeper and the rest. Seeding them means writing to three
// stores: the roster, the standing orders and the schedules. The seeder is a
// PLUGIN, so the port through which the live daemon answers it has to live
// here — a plugin may reach the contract and the platform layers and nothing
// else (archcheck's plugin rule).
//
// The implementation does NOT live here: answering the port needs the live
// kernel, and the app layer is the only layer that may see it.
//
// The rules of the road:
//
//   - Ports and the rows they carry only. No store, no kernel, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package guardianapi

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// Host is the live daemon's surface for the guardian seeder: the agents it
// may create and reconcile, the standing orders it may bind and the
// schedules it may plant.
//
// The rows it carries are the contract's own: the agent profile and the noise
// policy live in `agentapi`, the standing order in `standingapi` and the
// schedule in `cadenceapi`. The roster module keeps aliases of the first two,
// but a plugin may not name a module — so it names the contract.
type Host interface {
	// Agents lists the agents the daemon knows.
	Agents() []agentapi.Profile
	// AddAgent creates an agent.
	AddAgent(p agentapi.Profile) (agentapi.Profile, error)
	// UpdateAgent edits the mutable fields of the agent named by ref. It
	// answers false and no error for an unknown ref (the standing pattern).
	UpdateAgent(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	// StandingOrders lists the standing orders the daemon knows.
	StandingOrders() []standingapi.Order
	// UpdateStanding edits the mutable fields of the order named by id.
	UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error)
	// AddStanding creates a standing order.
	AddStanding(o standingapi.Order) (standingapi.Order, error)
	// SetStandingEnabled pauses or resumes an order.
	SetStandingEnabled(id string, enabled bool) (standingapi.Order, error)
	// Schedules lists the schedules the daemon knows.
	Schedules() []cadenceapi.Entry
	// Reschedule moves a schedule to a new cadence.
	Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error)
	// AddInterval plants a repeating schedule.
	AddInterval(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error)
	// AddDaily plants a daily schedule.
	AddDaily(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error)
}
