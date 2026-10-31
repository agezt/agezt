// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// The steering OPERATIONS moved to the runs module in W3.76b: the target
// architecture gives the runs module the "steering/interventions", so the
// service — pause, resume, step, steer, observe, intervene — lives in
// kernel/modules/runs/steer, and the per-run steering surface (the Control)
// lives there too.
//
// The daemon keeps the DOORS: the verbs the application layer's port calls,
// and the live runs those verbs steer. The module asks for the live run
// through a port, so it never names the daemon.

// PauseRun parks a running agent at its next iteration boundary (M608).
func (k *Kernel) PauseRun(corr string) bool { return k.steerSvc.Pause(corr) }

// ResumeRun lets a paused agent run freely again (M608).
func (k *Kernel) ResumeRun(corr string) bool { return k.steerSvc.Resume(corr) }

// StepRun advances a run by exactly one iteration then re-pauses it (M608).
func (k *Kernel) StepRun(corr string) bool { return k.steerSvc.Step(corr) }

// SteerRun injects an operator directive into a running agent (M608).
func (k *Kernel) SteerRun(corr, directive string, note bool) bool {
	return k.steerSvc.Steer(corr, directive, note)
}

// RunControlState reports a run's live steering state for the operator UI.
func (k *Kernel) RunControlState(corr string) (paused bool, pending int, ok bool) {
	return k.steerSvc.State(corr)
}

// InterveneRun applies one protocolized live intervention.
func (k *Kernel) InterveneRun(req intervention.Request) (intervention.Result, error) {
	return k.steerSvc.Intervene(req)
}

// controlFor returns the steering surface for a live run, or nil if there is no
// such active run (finished, never existed, wrong id).
func (k *Kernel) controlFor(corr string) *steer.Control {
	k.steersMu.Lock()
	defer k.steersMu.Unlock()
	return k.steers[corr]
}
