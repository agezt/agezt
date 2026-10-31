// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"strings"
	"testing"
)

// The steering operations moved into the runs module in W3.76b. These tests
// pin the daemon's half of the move: the daemon keeps the DOORS (the verbs the
// application layer's port calls, and the live runs they steer) and does not
// grow its own copy of the operations.

// TestTheSteeringOperationsLiveInTheRunsModule is the ratchet: the daemon's
// steer hook must ask the module, not re-implement it.
func TestTheSteeringOperationsLiveInTheRunsModule(t *testing.T) {
	src, err := os.ReadFile("steer.go")
	if err != nil {
		t.Fatalf("read steer.go: %v", err)
	}
	text := string(src)
	for _, required := range []string{
		"k.steerSvc.Pause(",
		"k.steerSvc.Resume(",
		"k.steerSvc.Step(",
		"k.steerSvc.Steer(",
		"k.steerSvc.State(",
		"k.steerSvc.Intervene(",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("steer.go must ask the module's operation %q", required)
		}
	}
	for _, forbidden := range []string{
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

// TestTheDaemonKeepsTheDoors: the verbs the application layer's port calls
// are still the daemon's, and they answer for a live run only.
func TestTheDaemonKeepsTheDoors(t *testing.T) {
	k := &Kernel{}
	if k.PauseRun("corr-1") || k.ResumeRun("corr-1") || k.StepRun("corr-1") {
		t.Fatal("a daemon without a steering service steers nothing")
	}
	if _, _, ok := k.RunControlState("corr-1"); ok {
		t.Fatal("a daemon without a steering service has no steering state")
	}
}
