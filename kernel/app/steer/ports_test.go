// SPDX-License-Identifier: MIT

package steer

import (
	"testing"

	"github.com/agezt/agezt/kernel/intervention"
)

// interventionRequest is the smallest intervention the controls answer.
func interventionRequest() intervention.Request {
	return intervention.Request{CorrelationID: "corr"}
}

// The run controls REFUSE when the handle is not the daemon's own.
//
// A host with no daemon holds no run to steer, pause, resume or step — and
// the controls say so rather than pretending.
func TestTheRunControlsRefuseWithoutADaemon(t *testing.T) {
	runs := RunsFromHandle("not a daemon")

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
