// SPDX-License-Identifier: MIT

package runner

// The ratchet of W3.76ac: the run engine's OWN face is the runs module's.
//
// The daemon used to answer the run engine's operations under names of its own
// (Run, RunWith, RunWithRetry, RunAssured, CompleteAgentLifecycle) — the
// engine already answered in exactly these names, so the daemon's five were a
// second spelling of one family. The daemon now hands over the module's FACE:
// the port lives in the runs module's own api package
// (kernel/modules/runs/api) and the engine is its implementation.
//
// What is pinned:
//
//   - the PORT declares the engine's four operations and nothing else;
//   - the ENGINE carries the port's evidence — the compile-time assertion
//     that says the engine IS the port.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for), so
// the probe is part of the slice: taking the port's evidence away, or adding
// an operation to the port, has to make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunEngineFaceAreTheEnginesOwn are the operations the run engine's own
// body answers: the port carries these and only these.
var theRunEngineFaceAreTheEnginesOwn = []string{
	"RunWith",
	"RunWithRetry",
	"RunAssured",
	"CompleteAgentLifecycle",
}

// theRunEnginePortFile is the runs module's own api package — the port's home.
const theRunEnginePortFile = "../api/runner.go"

// TestTheRunEngineFaceCarriesTheEnginesOwnOperations pins the PORT: it
// carries the engine's own operations, in the engine's own names.
//
// The check reads the DECLARATION (a tab-indented name at the start of a
// line), so the file's own history — the comment that names the retired
// operations — stays out of the ratchet.
func TestTheRunEngineFaceCarriesTheEnginesOwnOperations(t *testing.T) {
	src, err := os.ReadFile(theRunEnginePortFile)
	if err != nil {
		t.Fatalf("read the run engine's port: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "type RunnerService interface {") {
		t.Fatal("the runs module's port file has no RunnerService interface — the run engine's face belongs to the module (W3.76ac)")
	}
	for _, name := range theRunEngineFaceAreTheEnginesOwn {
		declaration := regexp.MustCompile(`(?m)^\t` + name + `\(`)
		if !declaration.MatchString(text) {
			t.Errorf("the run engine's port does not carry %s — the engine answers it, so the port must promise it (W3.76ac)", name)
		}
	}
}

// TestTheRunEngineFaceCarriesNothingElse pins the PORT's other half: it
// carries the engine's own operations and NOTHING ELSE. A port that grows a
// promise the engine does not own is a promise the host would have to keep —
// and the host keeping it is the second spelling this slice removed.
func TestTheRunEngineFaceCarriesNothingElse(t *testing.T) {
	src, err := os.ReadFile(theRunEnginePortFile)
	if err != nil {
		t.Fatalf("read the run engine's port: %v", err)
	}
	text := string(src)
	begin := strings.Index(text, "type RunnerService interface {")
	if begin < 0 {
		t.Fatal("the runs module's port file has no RunnerService interface — the run engine's face belongs to the module (W3.76ac)")
	}
	body := text[begin:]
	if end := strings.Index(body, "\n}"); end >= 0 {
		body = body[:end]
	}
	declared := regexp.MustCompile(`(?m)^\t([A-Z][A-Za-z0-9_]*)\(`).FindAllStringSubmatch(body, -1)
	owned := map[string]bool{}
	for _, name := range theRunEngineFaceAreTheEnginesOwn {
		owned[name] = true
	}
	for _, m := range declared {
		if !owned[m[1]] {
			t.Errorf("the run engine's port carries %s — the face carries the engine's own operations and nothing else (W3.76ac)", m[1])
		}
	}
}

// TestTheRunEngineCarriesThePortsEvidence pins the ENGINE: it says it IS the
// port. The assertion is the proof that the module's face and the module's
// body have not drifted apart.
func TestTheRunEngineCarriesThePortsEvidence(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the run engine package: %v", err)
	}
	evidence := regexp.MustCompile(`var _ api\.RunnerService = \(\*Runner\)\(nil\)`)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if evidence.Match(src) {
			return
		}
	}
	t.Error("the run engine does not say it is the api.RunnerService port — the module's face and the module's body must be pinned together (W3.76ac)")
}
