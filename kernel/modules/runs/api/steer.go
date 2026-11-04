// SPDX-License-Identifier: MIT

// Package api is the runs module's public vocabulary: the ports the host and
// the application layer bind, and the rows they exchange.
//
// The module keeps its rules and its implementation; what crosses the
// boundary is a PORT. A host that holds the port can steer a run and can
// never reach past it into the module's machinery — the same shape the agents
// module's roster port and the extensions module's registry port have.
//
// The steering family is the runs module's own: the target architecture gives
// the module the "steering/interventions" (pause, resume, step, steer,
// observe, intervene), and the verbs are the module's — one operation has one
// name, and the name is the one the module answers in.
package api

import (
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
)

// SteeringService is the runs module's steering surface: the live control of
// a run that is already in flight.
//
// The verbs are the MODULE's own names (W3.76z). The daemon used to answer
// them under names of its own — PauseRun, ResumeRun, StepRun, SteerRun,
// RunControlState, InterveneRun — which was a second spelling of one family:
// the module's service already answered in these names, and every caller above
// had to learn the daemon's.
//
// The port answers with what happened, never with a "the run is not there"
// error: a run that is not live is not steered, and the caller learns that
// from the answer.
type SteeringService interface {
	// Pause parks a running agent at its next iteration boundary (M608).
	Pause(corr string) bool
	// Resume lets a paused agent run freely again (M608).
	Resume(corr string) bool
	// Step advances a run by exactly one iteration then re-pauses it (M608).
	Step(corr string) bool
	// Steer injects an operator directive into a running agent (M608).
	Steer(corr, directive string, note bool) bool
	// State reports a run's live steering state for the operator UI.
	State(corr string) (paused bool, pending int, ok bool)
	// Intervene applies one protocolized live intervention.
	Intervene(req intervention.Request) (intervention.Result, error)
}
