// SPDX-License-Identifier: MIT

package steer

import (
	"errors"

	"github.com/agezt/agezt/kernel/intervention"
	"github.com/agezt/agezt/kernel/runtime"
)

// RunsFromHandle returns the daemon's run controls from the handle.
//
// The handle is not named by the caller: the steer family fills its own run
// controls from it. A handle that is not the daemon's own yields controls
// that REFUSE — a run is not steered by nobody.
func RunsFromHandle(h any) Runs {
	k, ok := h.(*runtime.Kernel)
	if !ok {
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
