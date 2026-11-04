// SPDX-License-Identifier: MIT

package runtime

// The ratchet of W3.76ac: the daemon holds no door over the RUN ENGINE's own
// operations.
//
// The run engine lives in the runs module (kernel/modules/runs/runner) and the
// daemon holds it — the host builds the run engine while opening itself. What
// the daemon must NOT do is ANSWER for the run engine a second time: the
// daemon used to carry five verbs over the module's own body (Run, RunWith,
// RunWithRetry, RunAssured and CompleteAgentLifecycle), and the run engine
// already answered in exactly these names.
//
// The daemon hands over the module's FACE instead: `RunnerService` is the
// runs module's own port (kernel/modules/runs/api), and every caller above the
// daemon asks the module through it.
//
// What the daemon keeps is the HOST's own: the correlation id a run carries
// (NewCorrelation — the host's identity door, W3.76aa) and the run's setup
// and cleanup.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for), so
// the probe is part of the slice: putting one of the run doors back has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunDoorsAreTheModules are the run engine's own operations — the verbs the
// daemon must not answer for a second time.
var theRunDoorsAreTheModules = []string{
	"Run",
	"RunWith",
	"RunWithRetry",
	"RunAssured",
	"CompleteAgentLifecycle",
}

// TestTheDaemonHoldsNoRunDoor pins the daemon's HALF: it holds no verb per
// run engine operation.
func TestTheDaemonHoldsNoRunDoor(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range theRunDoorsAreTheModules {
		if methods[name] {
			t.Errorf("*Kernel has the %s verb again — the run engine is the runs module's, and one operation has one name (W3.76ac)", name)
		}
	}
}

// TestTheDaemonKeepsTheRunEngineFace pins the daemon's OTHER half: it hands
// the module over. A daemon that dropped the face would leave every caller
// above it with nothing to ask.
func TestTheDaemonKeepsTheRunEngineFace(t *testing.T) {
	methods := daemonMethodSet(t)
	if !methods["RunnerService"] {
		t.Error("*Kernel has no RunnerService face — the run engine's face belongs to the runs module, and the daemon hands it over (W3.76ac)")
	}
}

// TestTheDaemonDoesNotDeclareARunDoor pins the daemon's SHAPE: the daemon
// asks the module for the run engine, it does not declare its own verb for
// it. The check reads the PRODUCTION files — a test file may legitimately
// mention a name.
func TestTheDaemonDoesNotDeclareARunDoor(t *testing.T) {
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
		for _, name := range theRunDoorsAreTheModules {
			declaration := regexp.MustCompile(`(?m)^func \(k \*Kernel\) ` + name + `\(`)
			if declaration.Match(src) {
				t.Errorf("%s declares the daemon's %s door — the run engine's operations live in the runs module's runner package (W3.76ac)", e.Name(), name)
			}
		}
	}
}

// TestTheDaemonSaysWhereTheRunEngineLives pins the daemon's own word: the
// file that hands the module over says where the module's body lives, so the
// next reader does not have to guess.
func TestTheDaemonSaysWhereTheRunEngineLives(t *testing.T) {
	src, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatalf("read runner.go: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(src)), "the run engine's own operations live in the runner package") {
		t.Error("the daemon's runner file must say where the run engine's own operations live")
	}
}
