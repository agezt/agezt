// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// salienceReason returns the reason field of the (single) salience.scored event.
func salienceReason(t *testing.T, j interface {
	Range(func(*event.Event) error) error
}) string {
	t.Helper()
	reason := ""
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindSalienceScored {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			reason = p.Reason
		}
		return nil
	})
	return reason
}

// TestTickPlumbsWorldModelRelevance is the lock-in for the (un-deferred) claim
// that the world-model relevance signal is wired through the FULL pulse tick —
// not just the Salience unit in isolation. With Config.Relevance set and a delta
// about a known active entity, the journaled salience.scored reason must name the
// match; with no Relevance the same delta scores plainly. If the engine→salience
// wiring (engine.go: relevance: cfg.Relevance) is dropped, this trips.
func TestTickPlumbsWorldModelRelevance(t *testing.T) {
	delta := Delta{
		Source:  "probe:ci",
		Kind:    "probe_failed",
		Summary: "Lictor CI started failing",
		Hints:   map[string]string{"severity": "medium"},
	}

	// With a relevance signal that knows "Lictor", the tick's salience.scored
	// reason names the match — proving Config.Relevance reached Salience.Score.
	withRel, jRel := newEngine(t, Config{
		Observers: []Observer{&fakeObserver{name: "o", deltas: []Delta{delta}}},
		Dial:      DialBalanced,
		Relevance: fakeRelevance{known: []string{"Lictor"}},
		Sink:      &capturingSink{},
	})
	withRel.tickOnce(context.Background())
	if r := salienceReason(t, jRel); !strings.Contains(r, "relevant to Lictor") {
		t.Errorf("with Relevance set, salience reason = %q, want it to name the matched entity", r)
	}

	// Control: no relevance signal → the same delta scores plainly, no match.
	noRel, jNo := newEngine(t, Config{
		Observers: []Observer{&fakeObserver{name: "o", deltas: []Delta{delta}}},
		Dial:      DialBalanced,
		Sink:      &capturingSink{},
	})
	noRel.tickOnce(context.Background())
	if r := salienceReason(t, jNo); strings.Contains(r, "relevant to") {
		t.Errorf("with no Relevance, salience reason = %q, must not claim relevance", r)
	}
}
