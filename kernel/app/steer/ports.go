// SPDX-License-Identifier: MIT

package steer

import (
	"errors"

	"github.com/agezt/agezt/kernel/intervention"
	"github.com/agezt/agezt/kernel/runtime"
)

// RunsFromKernel returns the daemon's run controls from the daemon's own
// kernel.
//
// The run controls ARE the daemon's own kernel: the steer family fills its
// own controls from it. A daemon that does not exist yields controls that
// REFUSE — a run is not steered by nobody.
func RunsFromKernel(k *runtime.Kernel) Runs {
	if k == nil {
		return refusingRuns{}
	}
	return k
}

// refusingRuns are the run controls a host without a daemon gets.
type refusingRuns struct{}

func (refusingRuns) CancelRun(string) bool { return false }

func (refusingRuns) PauseRun(string) bool { return false }

func (refusingRuns) ResumeRun(string) bool { return false }

func (refusingRuns) StepRun(string) bool { return false }

func (refusingRuns) SteerRun(string, string, bool) bool { return false }

func (refusingRuns) InterveneRun(intervention.Request) (intervention.Result, error) {
	return intervention.Result{}, errors.New("the run controls are not available")
}
