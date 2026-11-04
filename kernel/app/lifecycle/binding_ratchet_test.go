// SPDX-License-Identifier: MIT

package lifecycle

// The daemon-lifecycle application's BINDING — the ratchet of W3.76aa.
//
// The run lifecycle — the halt latch, the drain, the cancel and the queries
// over the live-run registry — is the RUNS MODULE's: the module owns the
// runs, so it owns the runs' control. The module's manager answers in the
// module's own names, and the daemon used to hold NINE verbs over those very
// operations (IsHalted, Halt, HaltWith, Resume, ResumeWith, CancelRun,
// DrainAndHalt, ActiveRuns, ActiveRunIDs) — a second spelling of one family.
//
// W3.76aa took the daemon's copy away. The daemon hands over the module's
// FACE, and this application binds that face.
//
// What is pinned here:
//
//   - the application's port asks the MODULE's face for the drain
//     (`k.LifecycleService().DrainAndHalt(…)`), and not a verb of the
//     DAEMON's — the daemon holds none, and one that came back would be a
//     second spelling again;
//   - the port reaches the drain THROUGH the face, so the face is what the
//     application holds.
//
// The verb names are the same on both sides — that is the point of one
// operation having one name — so the ratchet looks at the SHAPE: the call
// must not be made on the daemon itself.
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

// theDaemonsLifecycleVerbs is a lifecycle operation called ON THE DAEMON —
// the shape the application must not carry.
//
// The receiver must be the daemon (`k.`): the module's face is reached as
// `k.LifecycleService().DrainAndHalt(…)`, and that call is NOT this shape.
// The name must be followed by a PARENTHESIS, so that the file's own history
// — the comment that names the retired verbs — stays out of the ratchet.
var theDaemonsLifecycleVerbs = regexp.MustCompile(`\bk\.(IsHalted|Halt|HaltWith|DrainAndHalt|CancelRun|Resume|ResumeWith|ActiveRuns|ActiveRunIDs)\s*\(`)

// theModulesFace is the binding the application must carry: the runs
// module's lifecycle face, which the daemon hands over.
var theModulesFace = regexp.MustCompile(`LifecycleService\(\)`)

// TestTheLifecycleApplicationBindsTheModulesFace pins the binding: the
// application's port asks the runs module's face for the drain.
func TestTheLifecycleApplicationBindsTheModulesFace(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon-lifecycle application package: %v", err)
	}
	face := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if theDaemonsLifecycleVerbs.Match(src) {
			t.Errorf("%s asks the daemon for a lifecycle operation — the run lifecycle is the runs module's, and one operation has one name (W3.76aa)", e.Name())
		}
		if theModulesFace.Match(src) {
			face = true
		}
	}
	if !face {
		t.Error("the application does not reach the runs module's lifecycle face — the drain is the module's operation")
	}
}
