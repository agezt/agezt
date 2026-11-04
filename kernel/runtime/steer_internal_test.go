// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"strings"
	"testing"
)

// The steering operations moved into the runs module in W3.76b, and the
// daemon's NAMES moved with them in W3.76z.
//
// W3.76b left the daemon with six verbs over the module's own operations —
// PauseRun, ResumeRun, StepRun, SteerRun, RunControlState, InterveneRun —
// which was a second spelling of one family: the module's service already
// answered in its own names (Pause, Resume, Step, Steer, State, Intervene).
//
// W3.76z took the names away. The daemon hands over the module's FACE
// (SteeringService) and keeps the DOORS: the live runs the module steers, the
// bus it publishes on and the cancel it asks for.
//
// These tests pin the daemon's half of the move: the daemon does not spell
// out the operations, and it does not grow its own copy of them.

// TestTheSteeringOperationsLiveInTheRunsModule is the ratchet: the daemon's
// steer file must not spell out the steering operations — it hands over the
// module's face and asks nothing.
func TestTheSteeringOperationsLiveInTheRunsModule(t *testing.T) {
	src, err := os.ReadFile("steer.go")
	if err != nil {
		t.Fatalf("read steer.go: %v", err)
	}
	text := string(src)
	for _, forbidden := range []string{
		"k.steerSvc.Pause(",
		"k.steerSvc.Resume(",
		"k.steerSvc.Step(",
		"k.steerSvc.Steer(",
		"k.steerSvc.State(",
		"k.steerSvc.Intervene(",
		"event.KindRunPaused",
		"event.KindRunResumed",
		"event.KindRunStepped",
		"event.KindRunIntervention",
		"intervention.Normalize(",
		"intervention.PrimitiveHalt",
		"intervention.PrimitiveAbort",
		"rc.Pause(",
		"rc.Resume(",
		"rc.Step(",
		"rc.Inject(",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("steer.go grew the steering operation %q back; the operations are the module's", forbidden)
		}
	}
}

// TestTheDaemonHoldsNoSteeringForwarder pins the second half of the move
// (W3.76z): the daemon holds NO verb per steering operation.
//
// The six forwarders were a THIRD spelling of one family — the module's
// service already answered the module's names — and the daemon hands over the
// module's face instead.
func TestTheDaemonHoldsNoSteeringForwarder(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{
		"PauseRun", "ResumeRun", "StepRun", "SteerRun",
		"RunControlState", "InterveneRun",
	} {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the steering is the runs module's, and one operation has one name (W3.76z)", name)
		}
	}
	if !methods["SteeringService"] {
		t.Error("*Kernel lost the SteeringService verb — the daemon hands over the runs module's face")
	}
}

// TestTheDaemonKeepsTheDoors: the module's face answers for a live run only,
// and a daemon without a steering service steers nothing.
func TestTheDaemonKeepsTheDoors(t *testing.T) {
	k := &Kernel{}
	if k.SteeringService().Pause("corr-1") || k.SteeringService().Resume("corr-1") || k.SteeringService().Step("corr-1") {
		t.Fatal("a daemon without a steering service steers nothing")
	}
	if _, _, ok := k.SteeringService().State("corr-1"); ok {
		t.Fatal("a daemon without a steering service has no steering state")
	}
}
