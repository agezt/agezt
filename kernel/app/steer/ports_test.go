// SPDX-License-Identifier: MIT

package steer

import (
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/intervention"
)

// interventionRequest is the smallest intervention the controls answer.
func interventionRequest() intervention.Request {
	return intervention.Request{CorrelationID: "corr"}
}

// The run controls REFUSE when there is no daemon.
//
// The run controls are filled from the daemon's own kernel, by type: there is
// no foreign handle to fill them with any more. A host with no daemon holds
// no run to steer, pause, resume or step — and the controls say so rather
// than pretending.
func TestTheRunControlsRefuseWithoutADaemon(t *testing.T) {
	runs := RunsFromKernel(nil)

	if runs.CancelRun("corr") {
		t.Error("a host with no daemon cancelled a run")
	}
	if runs.PauseRun("corr") {
		t.Error("a host with no daemon paused a run")
	}
	if runs.ResumeRun("corr") {
		t.Error("a host with no daemon resumed a run")
	}
	if runs.StepRun("corr") {
		t.Error("a host with no daemon stepped a run")
	}
	if runs.SteerRun("corr", "turn left", false) {
		t.Error("a host with no daemon steered a run")
	}
	if _, err := runs.InterveneRun(interventionRequest()); err == nil {
		t.Error("a host with no daemon intervened in a run")
	}
}
