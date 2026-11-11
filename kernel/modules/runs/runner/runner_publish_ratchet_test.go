// SPDX-License-Identifier: MIT

package runner

// The run engine's PUBLISH door (W3.76ag).
//
// What a run says about itself — the reading it made of the user's
// utterance, and the analysis of what its context selection left out — is
// the RUNS MODULE's own (kernel/modules/runs/publish). The daemon held two
// verbs over them (PublishIntentInterpreted, PublishContextFailureAnalysis)
// and the run engine — which lives INSIDE the module — asked the HOST for
// them through its port: a round trip inside ONE module.
//
// The engine holds the module's publisher now and asks the module.
//
// These tests pin that: the port carries no publish entry, the engine holds
// the module's publisher, and a host that wired none leaves the engine quiet
// instead of running silently.

import (
	"os"
	"regexp"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/publish"
)

// thePublishEntriesAreNotTheHosts are the run-publish operations the run
// engine's host port must NOT carry: the run's own publish surface is the
// module's, and a port entry that is a round trip inside one module is a
// promise the host never had to keep.
var thePublishEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(PublishIntentInterpreted|PublishContextFailureAnalysis)\(`)

// TestTheRunEnginePortCarriesNoPublishEntry pins the PORT: it promises the
// host's capabilities, not the module's own publish surface.
func TestTheRunEnginePortCarriesNoPublishEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if thePublishEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a publish entry — the run's own publish surface is the module's (W3.76ag)")
	}
}

// TestTheRunEngineAsksTheModulesPublishSurface pins the wiring: the engine
// holds the module's publisher, not a host door.
func TestTheRunEngineAsksTheModulesPublishSurface(t *testing.T) {
	p := testPublisher()
	r := New(&stubKernel{}, nil, testState(), testPrompt(), p)
	if r.publish != p {
		t.Fatal("the run engine does not hold the module's publish surface")
	}
}

// TestTheRunEngineRefusesWithoutAPublishSurface pins the refusal: a host
// that wired no publisher leaves the engine quiet — it says the surface is
// missing instead of running silently.
func TestTheRunEngineRefusesWithoutAPublishSurface(t *testing.T) {
	r := New(&stubKernel{}, nil, testState(), testPrompt(), nil)
	if _, err := r.RunWith(t.Context(), "corr-1", "do the thing"); err == nil {
		t.Fatal("the engine ran without a publish surface")
	}
}

// stubPublishBus is the narrow bus port the publisher needs.
type stubPublishBus struct{ specs []event.Spec }

// Publish records the spec.
func (b *stubPublishBus) Publish(spec event.Spec) (*event.Event, error) {
	b.specs = append(b.specs, spec)
	return &event.Event{}, nil
}

// testPublisher returns the module's publish surface over a stub bus — the
// smallest publisher a test can run a run's own words through.
func testPublisher() *publish.Publisher {
	return publish.New(publish.Dependencies{Bus: &stubPublishBus{}})
}
