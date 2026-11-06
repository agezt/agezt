// SPDX-License-Identifier: MIT

package state

// The ratchets of W3.76ae: the live-run state is the RUNS MODULE's.
//
// What is pinned here:
//
//   - the state HOLDS the steering registry — the live control of every run
//     in flight — and the run's registration and cleanup;
//   - the state registers and releases the run in the MODULE's own store.
//
// The daemon held both until W3.76ae: the `k.steers` map behind its own
// mutex, and the three verbs (`SetupRunState`, `CleanupRunState`,
// `DeregisterRunSteer`) that the run engine — which lives inside this
// module — asked the host for through its port.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: giving the steering registry back to the
// daemon has to make these files fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// theSteeringRegistryIsTheModules pins the ownership: the live steering
// controls live HERE.
func theSteeringRegistryIsTheModules(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatalf("read state.go: %v", err)
	}
	text := string(src)
	for _, want := range []string{
		"steers map[string]*steer.Control",
		"func (s *State) RegisterSteer(",
		"func (s *State) UnregisterSteer(",
		"func (s *State) Control(",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the live-run state must carry %s — the live steering controls are the module's (W3.76ae)", want)
		}
	}
}

// TestTheLiveRunStateHoldsTheSteeringRegistry pins the state's half: the
// steering registry is the module's.
func TestTheLiveRunStateHoldsTheSteeringRegistry(t *testing.T) {
	theSteeringRegistryIsTheModules(t)
}

// TestTheLiveRunStateRegistersInTheModulesStore pins the direction of the
// registration: the run is registered in the module's own store, never in a
// map of the state's own.
func TestTheLiveRunStateRegistersInTheModulesStore(t *testing.T) {
	src, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatalf("read state.go: %v", err)
	}
	text := string(src)
	for _, want := range []string{"reg.Register(", "reg.Unregister(", "reg.Add(", "reg.Done()"} {
		if !strings.Contains(text, want) {
			t.Errorf("the live-run state must %s in the module's store", want)
		}
	}
	if regexp.MustCompile(`(?m)^\s*s\.runs\[`).MatchString(text) {
		t.Error("the live-run state keeps a map of its own — the run lives in the module's store")
	}
}

// TestTheDaemonHoldsNoSteeringRegistry pins the OTHER half: the daemon does
// not keep the registry the module took. The check reads the daemon's
// production files — a test file may legitimately mention a name.
func TestTheDaemonHoldsNoSteeringRegistry(t *testing.T) {
	root := filepath.Join("..", "..", "..", "runtime")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if regexp.MustCompile(`k\.steers\b`).Match(src) {
			t.Errorf("%s reaches for the daemon's steering registry — the live controls are the module's (W3.76ae)", e.Name())
		}
		if regexp.MustCompile(`(?m)^func \(k \*Kernel\) (SetupRunState|CleanupRunState|DeregisterRunSteer)\(`).Match(src) {
			t.Errorf("%s declares the daemon's own run-state verb — the run's live state is the module's (W3.76ae)", e.Name())
		}
	}
}
