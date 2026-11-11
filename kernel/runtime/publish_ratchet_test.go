// SPDX-License-Identifier: MIT

package runtime

// The ratchet of W3.76ag: the daemon holds no verb over the RUN's own
// publish surface.
//
// A run is not silent. It publishes the reading it made of the user's
// utterance (`intent.interpreted`), and — when it fails — the analysis of
// what its context selection left out (a `context.selection` manifest with
// the phase `failure_analysis`).
//
// That surface was the daemon's: two public verbs (PublishIntentInterpreted,
// PublishContextFailureAnalysis) over two private helpers, and the run
// engine — which lives INSIDE the runs module — asked the host for them
// through its port.
//
// The surface is the module's now (kernel/modules/runs/publish). The daemon
// holds no verb per run-publish operation, its production files declare
// none, and the reading's own shape does not come back under a new name.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the two back has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theRunPublishOperations are the run's own publish operations — the ones the
// daemon must not answer for.
var theRunPublishOperations = []string{
	"PublishIntentInterpreted",
	"PublishContextFailureAnalysis",
}

// TestTheDaemonHoldsNoRunPublishVerb pins the daemon's HALF: it holds no verb
// per run-publish operation.
func TestTheDaemonHoldsNoRunPublishVerb(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range theRunPublishOperations {
		if methods[name] {
			t.Errorf("*Kernel has the %s verb again — what a run says about itself is the runs module's, and one operation has one name (W3.76ag)", name)
		}
	}
}

// TestTheDaemonDoesNotDeclareARunPublishVerb pins the daemon's SHAPE: it
// asks the module for the run's own words, it does not declare its own verb
// for them. The check reads the PRODUCTION files — a test file may
// legitimately mention a name.
func TestTheDaemonDoesNotDeclareARunPublishVerb(t *testing.T) {
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
		for _, name := range theRunPublishOperations {
			declaration := regexp.MustCompile(`(?m)^func \(k \*Kernel\) ` + name + `\(`)
			if declaration.Match(src) {
				t.Errorf("%s declares the daemon's %s verb — what a run says about itself lives in the runs module's publish package (W3.76ag)", e.Name(), name)
			}
		}
	}
}

// TestTheDaemonHoldsNoRunPublishBody pins the BODY: the reading's own shape
// (the event the run publishes) and the failure analysis (the journal walk
// that derives the manifest) are the module's. The daemon's policy events
// stay — they are the daemon's policy path's events, not a run's.
func TestTheDaemonHoldsNoRunPublishBody(t *testing.T) {
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
		body := string(src)
		for _, bad := range []string{
			"func (k *Kernel) publishIntentInterpreted(",
			"func (k *Kernel) publishContextFailureAnalysis(",
			"event.KindIntentInterpreted",
			"event.KindContextSelection",
			"contextselect.FailureAnalysisSuspects",
		} {
			if strings.Contains(body, bad) {
				t.Errorf("%s carries %q — what a run says about itself is the runs module's (W3.76ag)", e.Name(), bad)
			}
		}
	}
}
