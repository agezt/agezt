// SPDX-License-Identifier: MIT

package runs

// The runs application's BINDING — the ratchet of W3.76ac.
//
// The RUN ENGINE — the plain run, the run under a retry policy, the run under
// the assurance loop and the agent lifecycle's close — is the RUNS MODULE's:
// the module owns the runs, so it owns the run engine. The engine answers in
// the engine's own names, and the daemon used to hold FIVE verbs over those
// very operations (Run, RunWith, RunWithRetry, RunAssured,
// CompleteAgentLifecycle) — a second spelling of one family.
//
// W3.76ac took the daemon's copy away. The daemon hands over the module's
// FACE, and this application binds that face.
//
// What is pinned here:
//
//   - the application's port asks the MODULE's face for the run
//     (`k.RunnerService().RunWith(…)`), and not a verb of the DAEMON's — the
//     daemon holds none, and one that came back would be a second spelling
//     again;
//   - the port reaches the run THROUGH the face, so the face is what the
//     application holds.
//
// The verb names are the same on both sides — that is the point of one
// operation having one name — so the ratchet looks at the SHAPE: the call must
// not be made on the daemon itself.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for), so
// the probe is part of the slice: spelling the daemon's verbs in the port
// again has to make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theDaemonsRunVerbs is a run engine operation called ON THE DAEMON — the
// shape the application must not carry.
//
// The receiver must be the daemon (`k.`): the module's face is reached as
// `k.RunnerService().RunWith(…)`, and that call is NOT this shape. The name
// must be followed by a PARENTHESIS, so that the file's own history — the
// comment that names the retired verbs — stays out of the ratchet.
var theDaemonsRunVerbs = regexp.MustCompile(`\bk\.(Run|RunWith|RunWithRetry|RunAssured|CompleteAgentLifecycle)\s*\(`)

// theModulesFace is the binding the application must carry: the runs module's
// run engine face, which the daemon hands over.
var theModulesFace = regexp.MustCompile(`RunnerService\(\)`)

// TestTheRunsApplicationBindsTheModulesFace pins the binding: the
// application's port asks the runs module's face for the run.
func TestTheRunsApplicationBindsTheModulesFace(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the runs application package: %v", err)
	}
	face := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if theDaemonsRunVerbs.Match(src) {
			t.Errorf("%s asks the daemon for a run engine operation — the run engine is the runs module's, and one operation has one name (W3.76ac)", e.Name())
		}
		if theModulesFace.Match(src) {
			face = true
		}
	}
	if !face {
		t.Error("the application does not reach the runs module's run engine face — the run is the module's operation")
	}
}
