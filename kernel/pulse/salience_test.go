// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeRelevance matches any text containing one of its known subjects.
type fakeRelevance struct{ known []string }

func (f fakeRelevance) IsActiveSubject(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, k := range f.known {
		if strings.Contains(lower, strings.ToLower(k)) {
			return k, true
		}
	}
	return "", false
}

func newSalience(rel Relevance) *Salience {
	return &Salience{
		dial:       DialBalanced,
		noveltyTTL: 30 * time.Minute,
		relevance:  rel,
		now:        func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func mediumDelta(summary string) Delta {
	return Delta{Source: "probe:ci", Kind: "probe_failed", Summary: summary} // severity defaults to medium
}

// TestDispositionForValue_BandBoundaries pins the inclusive LLM-score → disposition band
// edges. The salience/route tests exercise dispositions directly or via the relevance
// boost, never dispositionForValue at its exact thresholds, so mutation testing (M523)
// left `v >= 0.85`, `v >= 0.45`, and `v >= 0.20` each able to weaken to `>` — a score
// landing exactly on a band edge would drop a notch (an alert silently demoted to a
// notify, a notify to a digest, a digest dropped entirely).
func TestDispositionForValue_BandBoundaries(t *testing.T) {
	cases := []struct {
		v    float64
		want Disposition
	}{
		{1.0, DispAlert},
		{0.85, DispAlert}, // exact alert edge
		{0.84, DispNotify},
		{0.45, DispNotify}, // exact notify edge
		{0.44, DispDigest},
		{0.20, DispDigest}, // exact digest edge
		{0.19, DispDrop},
		{0.0, DispDrop},
	}
	for _, c := range cases {
		if got := dispositionForValue(c.v); got != c.want {
			t.Errorf("dispositionForValue(%.2f) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestSalienceRelevanceBoostLiftsBand(t *testing.T) {
	base := newSalience(nil)
	boosted := newSalience(fakeRelevance{known: []string{"Lictor"}})

	d := mediumDelta("Lictor CI started failing")

	got := base.Score(context.Background(), d)
	if got.Disposition != DispNotify {
		t.Fatalf("baseline medium delta should be notify, got %s (%.2f)", got.Disposition, got.Value)
	}
	b := boosted.Score(context.Background(), d)
	if b.Value <= got.Value {
		t.Errorf("relevance should raise the value: base=%.2f boosted=%.2f", got.Value, b.Value)
	}
	if !strings.Contains(b.Reason, "relevant to Lictor") {
		t.Errorf("reason should name the matched entity, got %q", b.Reason)
	}
}

func TestSalienceNoBoostWhenIrrelevant(t *testing.T) {
	boosted := newSalience(fakeRelevance{known: []string{"Lictor"}})
	d := mediumDelta("some unrelated service hiccuped")
	base := newSalience(nil).Score(context.Background(), d)
	got := boosted.Score(context.Background(), d)
	if got.Value != base.Value {
		t.Errorf("irrelevant delta must not be boosted: base=%.2f got=%.2f", base.Value, got.Value)
	}
	if strings.Contains(got.Reason, "relevant to") {
		t.Errorf("irrelevant delta should not claim relevance, got %q", got.Reason)
	}
}

func TestSalienceBoostMatchesIssueKey(t *testing.T) {
	boosted := newSalience(fakeRelevance{known: []string{"lictor"}})
	// Summary has no entity; the issue_key hint does.
	d := Delta{
		Source: "probe", Kind: "failed", Summary: "a check failed",
		Hints: map[string]string{"issue_key": "ci/lictor"},
	}
	got := boosted.Score(context.Background(), d)
	if !strings.Contains(got.Reason, "relevant to") {
		t.Errorf("issue_key match should boost, got reason %q", got.Reason)
	}
}

func TestSalienceBoostBoundedAndNilSafe(t *testing.T) {
	// nil relevance → no panic, no boost (v1 behaviour).
	d := mediumDelta("anything")
	if got := newSalience(nil).Score(context.Background(), d); got.Value > 1 || got.Value < 0 {
		t.Errorf("value out of range: %.2f", got.Value)
	}
	// A critical delta already at 0.95 + boost must clamp to <= 1.
	crit := Delta{Source: "s", Kind: "k", Summary: "Lictor down", Hints: map[string]string{"severity": string(SevCritical)}}
	got := newSalience(fakeRelevance{known: []string{"Lictor"}}).Score(context.Background(), crit)
	if got.Value > 1.0 {
		t.Errorf("boosted critical must clamp to <= 1.0, got %.2f", got.Value)
	}
}
