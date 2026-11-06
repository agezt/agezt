// SPDX-License-Identifier: MIT

package runtime

// The ratchet of W3.76ae: the daemon holds no verb over the RUN's live state.
//
// A run in flight has a live state: it is registered in the runs module's
// live-run registry, it holds a cancel func, a drain ticket and a steering
// control that an operator can pause, step or steer from another goroutine.
//
// That state was the daemon's: the `k.steers` map (correlation id → the live
// `*steer.Control`) behind the daemon's own `steersMu`, plus the three verbs
// that created and released it (`SetupRunState`, `CleanupRunState`,
// `DeregisterRunSteer`). The run engine — which lives INSIDE the runs module
// — asked the host for all three through its port, and the module's steering
// service asked the host for the very controls the module owns.
//
// The state is the module's now (kernel/modules/runs/state). The daemon holds
// the STORE handle (the module's `registry.Registry`) and hands the module its
// own configuration; it holds no verb per run-state operation any more.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the three back has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunStateOperations are the run's live-state operations — the ones the
// daemon must not answer for.
var theRunStateOperations = []string{
	"SetupRunState",
	"CleanupRunState",
	"DeregisterRunSteer",
}

// TestTheDaemonHoldsNoRunStateVerb pins the daemon's HALF: it holds no verb
// per run-state operation.
func TestTheDaemonHoldsNoRunStateVerb(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range theRunStateOperations {
		if methods[name] {
			t.Errorf("*Kernel has the %s verb again — the run's live state is the runs module's, and one operation has one name (W3.76ae)", name)
		}
	}
}

// TestTheDaemonDoesNotDeclareARunStateVerb pins the daemon's SHAPE: it asks
// the module for the run's state, it does not declare its own verb for it.
// The check reads the PRODUCTION files — a test file may legitimately mention
// a name.
func TestTheDaemonDoesNotDeclareARunStateVerb(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, name := range theRunStateOperations {
			declaration := regexp.MustCompile(`(?m)^func \(k \*Kernel\) ` + name + `\(`)
			if declaration.Match(src) {
				t.Errorf("%s declares the daemon's %s verb — the run's live state lives in the runs module's state package (W3.76ae)", e.Name(), name)
			}
		}
	}
}

// TestTheDaemonHoldsNoSteeringRegistry pins the STATE the daemon gave up: the
// live steering controls are the module's, and the daemon holds no map of
// them and no mutex of its own.
func TestTheDaemonHoldsNoSteeringRegistry(t *testing.T) {
	src, err := os.ReadFile("runtime_kernel.go")
	if err != nil {
		t.Fatalf("read runtime_kernel.go: %v", err)
	}
	text := string(src)
	for _, field := range []string{"steersMu", "steers "} {
		if strings.Contains(text, field) {
			t.Errorf("the daemon holds %q again — the live steering controls are the runs module's (W3.76ae)", strings.TrimSpace(field))
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if regexp.MustCompile(`k\.steers\b`).Match(src) {
			t.Errorf("%s reaches for the daemon's steering registry — the live controls are the runs module's (W3.76ae)", e.Name())
		}
	}
}
