// SPDX-License-Identifier: MIT

package publish_test

// The runs module's publish surface: what a run says about itself.
//
// The tests pin the two operations and their boundaries: the reading is
// published without the raw utterance, the failure analysis says nothing
// when there is nothing to analyse, and a publisher without its ports stays
// quiet instead of panicking.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/modules/runs/publish"
)

// stubBus records what the publisher publishes.
type stubBus struct {
	specs []event.Spec
}

// Publish records the spec and hands back an empty event.
func (b *stubBus) Publish(spec event.Spec) (*event.Event, error) {
	b.specs = append(b.specs, spec)
	return &event.Event{}, nil
}

// stubJournal replays a fixed list of events.
type stubJournal struct {
	events []*event.Event
}

// Range walks the replayed events.
func (j *stubJournal) Range(fn func(*event.Event) error) error {
	for _, e := range j.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

// stubSelection records the manifests the publisher asks to be published.
type stubSelection struct {
	manifests []contextselect.Manifest
}

// PublishSelection records the manifest.
func (s *stubSelection) PublishSelection(corr, actor string, manifest contextselect.Manifest) {
	s.manifests = append(s.manifests, manifest)
}

// TestTheRunPublishesItsReadingWithoutTheRawUtterance pins the reading: the
// event carries the hash, the canonical intent and the ambiguity — and never
// the words the operator typed.
func TestTheRunPublishesItsReadingWithoutTheRawUtterance(t *testing.T) {
	bus := &stubBus{}
	p := publish.New(publish.Dependencies{Bus: bus})

	p.IntentInterpreted("corr-1", "agent-corr-1", intentmodel.Frame{
		UserUtteranceHash: "abc123",
		CanonicalIntent:   "clean the room",
		AmbiguityScore:    0.7,
		Underdetermined:   true,
	})

	if len(bus.specs) != 1 {
		t.Fatalf("published %d events, want 1", len(bus.specs))
	}
	spec := bus.specs[0]
	if spec.Kind != event.KindIntentInterpreted || spec.Subject != "intent.interpreted" {
		t.Errorf("published %s/%s, want intent.interpreted", spec.Subject, spec.Kind)
	}
	if spec.CorrelationID != "corr-1" || spec.Actor != "agent-corr-1" {
		t.Errorf("published for %s/%s, want corr-1/agent-corr-1", spec.CorrelationID, spec.Actor)
	}
	payload, ok := spec.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload is %T, want a map", spec.Payload)
	}
	if payload["user_utterance_hash"] != "abc123" || payload["canonical_intent"] != "clean the room" {
		t.Errorf("payload lost the reading: %+v", payload)
	}
	if payload["ambiguity_score"] != 0.7 || payload["underdetermined"] != true {
		t.Errorf("payload lost the ambiguity: %+v", payload)
	}
}

// TestThePublisherStaysQuietWithoutItsPorts pins the boundaries: a publisher
// with no bus says nothing, and it does not panic.
func TestThePublisherStaysQuietWithoutItsPorts(t *testing.T) {
	p := publish.New(publish.Dependencies{})
	p.IntentInterpreted("corr-1", "agent-corr-1", intentmodel.Frame{})

	var empty *publish.Publisher
	empty.IntentInterpreted("corr-1", "agent-corr-1", intentmodel.Frame{})
	empty.ContextFailureAnalysis("corr-1", "agent-corr-1", errors.New("boom"))
}

// TestTheFailedRunPublishesItsContextAnalysis pins the analysis: the last
// context-selection manifest of the run is read, its rejected candidates are
// re-ranked, and the manifest the publisher derives is published with the
// failure-analysis phase.
func TestTheFailedRunPublishesItsContextAnalysis(t *testing.T) {
	rejected := []contextselect.Candidate{{ID: "c1", Score: 0.2}}
	manifest := contextselect.Manifest{Phase: "selection", Rejected: rejected}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	journal := &stubJournal{events: []*event.Event{{
		Kind:          event.KindContextSelection,
		CorrelationID: "corr-1",
		Payload:       payload,
	}}}
	selection := &stubSelection{}
	p := publish.New(publish.Dependencies{Journal: journal, Selection: selection})

	p.ContextFailureAnalysis("corr-1", "agent-corr-1", errors.New("the tool refused"))

	if len(selection.manifests) != 1 {
		t.Fatalf("published %d manifests, want 1", len(selection.manifests))
	}
	got := selection.manifests[0]
	if got.Phase != "failure_analysis" {
		t.Errorf("phase = %q, want failure_analysis", got.Phase)
	}
	if got.Summary["failure"] != "the tool refused" {
		t.Errorf("the summary lost the failure: %+v", got.Summary)
	}
	if len(got.Rejected) == 0 {
		t.Error("the analysis dropped the rejected candidates")
	}
}

// TestTheAnalysisSaysNothingWithoutASelection pins the silence: a run that
// failed without a context selection — or with nothing rejected — has
// nothing to analyse, and it says nothing.
func TestTheAnalysisSaysNothingWithoutASelection(t *testing.T) {
	selection := &stubSelection{}
	p := publish.New(publish.Dependencies{Journal: &stubJournal{}, Selection: selection})

	p.ContextFailureAnalysis("corr-1", "agent-corr-1", errors.New("boom"))
	if len(selection.manifests) != 0 {
		t.Errorf("published %d manifests, want none", len(selection.manifests))
	}
}

// TestTheAnalysisIgnoresAnotherRunsManifest pins the scope: the analysis
// reads THIS run's journal, not every run's.
func TestTheAnalysisIgnoresAnotherRunsManifest(t *testing.T) {
	payload, err := json.Marshal(contextselect.Manifest{Phase: "selection", Rejected: []contextselect.Candidate{{ID: "c1"}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	journal := &stubJournal{events: []*event.Event{{
		Kind:          event.KindContextSelection,
		CorrelationID: "corr-other",
		Payload:       payload,
	}}}
	selection := &stubSelection{}
	p := publish.New(publish.Dependencies{Journal: journal, Selection: selection})

	p.ContextFailureAnalysis("corr-1", "agent-corr-1", errors.New("boom"))
	if len(selection.manifests) != 0 {
		t.Errorf("published %d manifests, want none", len(selection.manifests))
	}
}
