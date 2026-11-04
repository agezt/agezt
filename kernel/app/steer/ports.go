// SPDX-License-Identifier: MIT

package steer

import (
	"errors"

	"github.com/agezt/agezt/kernel/modules/runs/api"
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
	"github.com/agezt/agezt/kernel/runtime"
)

// RunsFromKernel returns the run controls from the daemon's own kernel.
//
// Two faces of the RUNS MODULE answer the port, and the adapter says which
// is which: the CANCEL is the module's LIFECYCLE face (W3.76aa — the daemon
// used to answer it under a name of its own), and the STEERING is the
// module's steering face (W3.76z — the daemon used to answer that under
// names of its own too, and the application bound the daemon HANDLE as this
// port).
//
// A daemon that does not exist yields controls that REFUSE — a run is not
// steered by nobody.
func RunsFromKernel(k *runtime.Kernel) Runs {
	if k == nil {
		return refusingRuns{}
	}
	return runsFromKernel{
		steering:  k.SteeringService(),
		lifecycle: k.LifecycleService(),
	}
}

// runsFromKernel answers the port from the module's two faces.
type runsFromKernel struct {
	steering  api.SteeringService
	lifecycle api.LifecycleService
}

// CancelRun cancels a live run — the RUNS MODULE's operation (W3.76aa).
func (r runsFromKernel) CancelRun(corr string) bool { return r.lifecycle.CancelRun(corr) }

// Pause parks a running agent at its next iteration boundary.
func (r runsFromKernel) Pause(corr string) bool { return r.steering.Pause(corr) }

// Resume lets a paused agent run freely again.
func (r runsFromKernel) Resume(corr string) bool { return r.steering.Resume(corr) }

// Step advances a run by exactly one iteration then re-pauses it.
func (r runsFromKernel) Step(corr string) bool { return r.steering.Step(corr) }

// Steer injects an operator directive into a running agent.
func (r runsFromKernel) Steer(corr, directive string, note bool) bool {
	return r.steering.Steer(corr, directive, note)
}

// State reports a run's live steering state.
func (r runsFromKernel) State(corr string) (bool, int, bool) { return r.steering.State(corr) }

// Intervene applies one protocolized live intervention.
func (r runsFromKernel) Intervene(req intervention.Request) (intervention.Result, error) {
	return r.steering.Intervene(req)
}

// refusingRuns are the run controls a host without a daemon gets.
//
// The refusal is written **once**, here: the port's own, so that the adapter
// and the tool never have to spell "there is no daemon" twice.
type refusingRuns struct{}

func (refusingRuns) CancelRun(string) bool { return false }

func (refusingRuns) Pause(string) bool { return false }

func (refusingRuns) Resume(string) bool { return false }

func (refusingRuns) Step(string) bool { return false }

func (refusingRuns) Steer(string, string, bool) bool { return false }

func (refusingRuns) State(string) (bool, int, bool) { return false, 0, false }

func (refusingRuns) Intervene(intervention.Request) (intervention.Result, error) {
	return intervention.Result{}, errors.New("the run controls are not available")
}
