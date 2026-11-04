// SPDX-License-Identifier: MIT

package steer

// The steering application's BINDING — the ratchet of W3.76z.
//
// The steering family is the RUNS MODULE's: the target architecture gives the
// module the "steering/interventions" (pause, resume, step, steer, observe,
// intervene), and the module's service answered in its own names since
// W3.76b.
//
// The application used to reach the DAEMON for its steering — the daemon held
// six forwarders over the module's own operations (PauseRun, ResumeRun,
// StepRun, SteerRun, RunControlState, InterveneRun) and the application bound
// the daemon HANDLE as its port. Those six were a second spelling of one
// family and they are gone.
//
// What is pinned here:
//
//   - the application's port speaks the MODULE's vocabulary — the module's
//     port (api.SteeringService), not six verbs spelled the daemon's way;
//   - the application does not bind the daemon handle as its port.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: spelling the daemon's verbs in the port
// again has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// theDaemonsSteeringVerbs are the daemon's spellings of the steering
// family's operations.
//
// The name must be followed by a PARENTHESIS: that is what a declaration and
// a call look like, and it keeps the file's own history — the comment that
// names the retired verbs — out of the ratchet. A ratchet that trips on the
// story of the move is a ratchet nobody can keep green.
var theDaemonsSteeringVerbs = regexp.MustCompile(`\b(PauseRun|ResumeRun|StepRun|SteerRun|RunControlState|InterveneRun)\s*\(`)

// theDaemonHandleAsThePort is the binding this package must not carry: the
// daemon handle as the application's runs port.
//
// The value must BE the handle — `return k` — not a call on it.
var theDaemonHandleAsThePort = regexp.MustCompile(`(?m)^\s*return k\s*$`)

// TestTheSteeringApplicationSpeaksTheModulesVocabulary pins the port: the
// application's port carries the module's port, not the daemon's names.
func TestTheSteeringApplicationSpeaksTheModulesVocabulary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the steering application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if theDaemonsSteeringVerbs.Match(src) {
			t.Errorf("%s spells the daemon's steering verbs — the steering family is the runs module's, and one operation has one name (W3.76z)", e.Name())
		}
		if theDaemonHandleAsThePort.Match(src) {
			t.Errorf("%s binds the daemon handle as its runs port — the application binds the runs module's face (W3.76z)", e.Name())
		}
	}
}
