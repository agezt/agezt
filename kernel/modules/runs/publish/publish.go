// SPDX-License-Identifier: MIT

// Package publish is the RUNS MODULE's publish surface: what a run says
// about itself.
//
// A run is not silent. It publishes the reading it made of the user's
// utterance (`intent.interpreted`), and — when the run fails — the analysis
// of what its context selection left out (a `context.selection` manifest
// with the phase `failure_analysis`).
//
// Both events were the DAEMON's: the daemon held two verbs over them
// (`PublishIntentInterpreted`, `PublishContextFailureAnalysis`) and the run
// engine — which lives INSIDE this module — asked the host for them
// through its port. A round trip inside one module is a promise the host
// never had to keep.
//
// The module owns them now: the engine asks the module.
//
// The DEPENDENCIES are narrow ports, never the host's implementations: the
// bus the events travel on, the journal the failure analysis reads, and the
// prompt module's manifest publisher (the module knows what a context
// selection manifest is; the analysis asks it to publish the one it
// derives).
package publish

import (
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
)

// Bus is the narrow port the run's own events travel on.
type Bus interface {
	Publish(spec event.Spec) (*event.Event, error)
}

// Journal is the narrow read port over the event log the failure analysis
// scans.
type Journal interface {
	Range(fn func(*event.Event) error) error
}

// Selection is the prompt module's manifest publisher: the module knows what
// a context selection manifest is, so the failure analysis asks it to
// publish the one it derives.
type Selection interface {
	PublishSelection(corr, actor string, manifest contextselect.Manifest)
}

// Dependencies are the three things the publisher needs. Every one of them
// is a PORT: the module names the shape it needs, never the host's
// implementation.
type Dependencies struct {
	// Bus carries the run's own events.
	Bus Bus
	// Journal is the event log the failure analysis scans.
	Journal Journal
	// Selection publishes the context-selection manifest the analysis
	// derives.
	Selection Selection
}

// Publisher is the runs module's publish surface.
type Publisher struct {
	bus       Bus
	journal   Journal
	selection Selection
}

// New builds the publisher over its dependencies.
func New(deps Dependencies) *Publisher {
	return &Publisher{bus: deps.Bus, journal: deps.Journal, selection: deps.Selection}
}

// IntentInterpreted publishes the run's reading of the user's utterance.
//
// The payload carries the reading, never the raw utterance: the hash, the
// canonical intent, the assumptions and the ambiguity — what an operator
// needs to see WHY the run did what it did, and not what was typed.
func (p *Publisher) IntentInterpreted(corr, actor string, frame intentmodel.Frame) {
	if p == nil || p.bus == nil {
		return
	}
	_, _ = p.bus.Publish(event.Spec{
		Subject:       "intent.interpreted",
		Kind:          event.KindIntentInterpreted,
		Actor:         actor,
		CorrelationID: corr,
		Payload: map[string]any{
			"user_utterance_hash": frame.UserUtteranceHash,
			"canonical_intent":    frame.CanonicalIntent,
			"assumptions":         frame.Assumptions,
			"explicit_exclusions": frame.ExplicitExclusions,
			"candidate_plans":     frame.CandidatePlans,
			"harmful_reading":     frame.HarmfulReading,
			"ambiguity_score":     frame.AmbiguityScore,
			"underdetermined":     frame.Underdetermined,
		},
	})
}

// ContextFailureAnalysis publishes the analysis of what the run's context
// selection left out, after the run failed.
//
// The analysis scans the run's journal for the last context-selection
// manifest, re-ranks the candidates it rejected, and asks the prompt
// module to publish the manifest it derives. A run that failed without a
// selection — or with nothing rejected — says nothing.
func (p *Publisher) ContextFailureAnalysis(corr, actor string, runErr error) {
	if p == nil || runErr == nil || p.journal == nil || p.selection == nil {
		return
	}
	var latest *contextselect.Manifest
	_ = p.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindContextSelection {
			return nil
		}
		var m contextselect.Manifest
		if json.Unmarshal(e.Payload, &m) == nil && m.Phase != "failure_analysis" {
			latest = &m
		}
		return nil
	})
	if latest == nil || len(latest.Rejected) == 0 {
		return
	}
	suspects := contextselect.FailureAnalysisSuspects(latest.Rejected)
	p.selection.PublishSelection(corr, actor, contextselect.Manifest{
		Phase: "failure_analysis",
		Summary: map[string]any{
			"classifier":        "heuristic_external",
			"failure":           runErr.Error(),
			"suspect_omission":  len(suspects) > 0,
			"counterfactuals":   contextselect.CandidateIDs(suspects),
			"credit_assignment": fmt.Sprintf("replay with %d rejected candidate(s) from previous selection", len(suspects)),
		},
		Rejected: suspects,
	})
}
