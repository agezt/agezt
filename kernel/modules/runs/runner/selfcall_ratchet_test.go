// SPDX-License-Identifier: MIT

package runner

// The ratchet of W3.76ab: the run engine does not ask the HOST for its own
// operations.
//
// PublishHeuristicBypass, MaybeDistill, MaybeForge, MaybeShadowEval,
// RunWith, RunWithRetry and CompleteAgentLifecycle sat in the run engine's
// host port — and the run engine asked the host for them, and the host
// asked the run engine back. A port entry that is a round trip is a promise
// the host never had to keep: the module has the body, so the module calls
// it.
//
// What is pinned:
//
//   - the PORT carries none of the run engine's own operations;
//   - the run engine's PRODUCTION files do not ask the host for them.
//
// The check is over the production files — a test file may legitimately
// mention a name — and the port check looks at the DECLARATION (a
// tab-indented name at the start of a line), so the file's own history —
// the comment that names the retired entries — stays out of the ratchet.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the module's own
// operations back behind the port has to make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunEnginesOwnOperations are the run engine's own operations: the ones
// the module implements and the host used to answer a second time.
var theRunEnginesOwnOperations = []string{
	"PublishHeuristicBypass",
	"MaybeDistill",
	"MaybeForge",
	"MaybeShadowEval",
	"RunWith",
	"RunWithRetry",
	"CompleteAgentLifecycle",
	"VerifyCompletion",
}

// TestTheRunEnginePortCarriesNoOwnOperation pins the PORT: it promises the
// host's capabilities, not the run engine's own body.
func TestTheRunEnginePortCarriesNoOwnOperation(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	text := string(src)
	for _, name := range theRunEnginesOwnOperations {
		declaration := regexp.MustCompile(`(?m)^\t` + name + `\(`)
		if declaration.MatchString(text) {
			t.Errorf("the run engine's port carries %s — the run engine has the body, so the run engine calls it (W3.76ab)", name)
		}
	}
}

// TestTheRunEngineDoesNotAskTheHostForItsOwnOperations pins the CALLS: the
// run engine does not go out to the host for something the host would only
// hand back.
func TestTheRunEngineDoesNotAskTheHostForItsOwnOperations(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the run engine package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, name := range theRunEnginesOwnOperations {
			ask := regexp.MustCompile(`\br\.k\.` + name + `\(`)
			if ask.Match(src) {
				t.Errorf("%s asks the host for %s — the host would only hand it back (W3.76ab)", e.Name(), name)
			}
		}
	}
}
