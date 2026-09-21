// SPDX-License-Identifier: MIT

package contextselect_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/contextselect"
)

func TestTokenCost_Empty(t *testing.T) {
	if got := contextselect.TokenCost(""); got != 1 {
		t.Fatalf("TokenCost('') = %d, want 1", got)
	}
}

func TestTokenCost_Short(t *testing.T) {
	if got := contextselect.TokenCost("hi"); got != 1 {
		t.Fatalf("TokenCost('hi') = %d, want 1", got)
	}
}

func TestTokenCost_Long(t *testing.T) {
	if got := contextselect.TokenCost("hello world, this is a test of token estimation"); got < 8 || got > 15 {
		t.Fatalf("TokenCost = %d, want ~10", got)
	}
}

func TestFreshness_ZeroInputs(t *testing.T) {
	if got := contextselect.Freshness(0, 0); got != 0.5 {
		t.Fatalf("Freshness(0,0) = %f, want 0.5", got)
	}
}

func TestFreshness_Recent(t *testing.T) {
	if got := contextselect.Freshness(9999000, 10000000); got < 0.9 {
		t.Fatalf("Freshness(9999000,10000000) = %f, want > 0.9 for 1s old", got)
	}
}

func TestFreshness_Old(t *testing.T) {
	nowMS := int64(1_700_000_000_000) // ~2023
	thirtyDaysMS := int64(30 * 24 * 60 * 60 * 1000)
	if got := contextselect.Freshness(nowMS-thirtyDaysMS, nowMS); got > 0.1 {
		t.Fatalf("Freshness(30d old) = %f, want < 0.1", got)
	}
}

func TestRisk_NoConfidence(t *testing.T) {
	if got := contextselect.Risk(0, 0.5, "memory"); got <= 0 {
		t.Fatalf("Risk(0,0.5,'memory') = %f, want > 0", got)
	}
}

func TestRisk_SkillDiscount(t *testing.T) {
	skillRisk := contextselect.Risk(0.8, 0.7, "skill")
	memoryRisk := contextselect.Risk(0.8, 0.7, "memory")
	if skillRisk >= memoryRisk {
		t.Fatalf("skill risk %f should be < memory risk %f due to 0.75 discount", skillRisk, memoryRisk)
	}
}

func TestSplitCandidates_AllChosen(t *testing.T) {
	all := []contextselect.Candidate{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.5},
	}
	chosenIDs := map[string]bool{"a": true, "b": true}
	chosen, rejected := contextselect.SplitCandidates(all, chosenIDs, "test")
	if len(chosen) != 2 {
		t.Fatalf("got %d chosen, want 2", len(chosen))
	}
	if len(rejected) != 0 {
		t.Fatalf("got %d rejected, want 0", len(rejected))
	}
}

func TestSplitCandidates_RejectedLimit(t *testing.T) {
	all := make([]contextselect.Candidate, 20)
	for i := range all {
		all[i] = contextselect.Candidate{ID: string(rune('a' + i)), Score: float64(20 - i)}
	}
	chosenIDs := map[string]bool{"a": true}
	_, rejected := contextselect.SplitCandidates(all, chosenIDs, "test")
	if len(rejected) > 10 {
		t.Fatalf("got %d rejected, want <= 10", len(rejected))
	}
}

func TestSummary_Counts(t *testing.T) {
	chosen := []contextselect.Candidate{
		{ID: "a", Tokens: 50},
		{ID: "b", Tokens: 150},
	}
	summary := contextselect.Summary(chosen, nil)
	if summary["chosen"] != 2 {
		t.Fatalf("chosen count = %d, want 2", summary["chosen"])
	}
	if summary["chosen_tokens"] != 200 {
		t.Fatalf("chosen_tokens = %d, want 200", summary["chosen_tokens"])
	}
}

func TestChosenIDSet(t *testing.T) {
	set := contextselect.ChosenIDSet([]string{"a", "", "c"})
	if !set["a"] {
		t.Fatal("expected 'a' in set")
	}
	if set[""] {
		t.Fatal("did not expect empty string in set")
	}
}

func TestFailureAnalysisSuspects_OrdersByScore(t *testing.T) {
	rejected := []contextselect.Candidate{
		{ID: "low", Score: 0.1, Tokens: 100, RiskCost: 0.5},
		{ID: "high", Score: 0.9, Tokens: 100, RiskCost: 0.5},
	}
	suspects := contextselect.FailureAnalysisSuspects(rejected)
	if len(suspects) == 0 || suspects[0].ID != "high" {
		t.Fatalf("expected 'high' first, got %v", suspects)
	}
}

func TestCandidateIDs(t *testing.T) {
	cands := []contextselect.Candidate{
		{ID: "x"},
		{ID: "y"},
	}
	ids := contextselect.CandidateIDs(cands)
	if len(ids) != 2 || ids[0] != "x" || ids[1] != "y" {
		t.Fatalf("got %v, want [x y]", ids)
	}
}

func TestMemoryCandidates_Empty(t *testing.T) {
	cands := contextselect.MemoryCandidates(nil, 1000)
	if len(cands) != 0 {
		t.Fatalf("expected 0 candidates for nil input, got %d", len(cands))
	}
}

func TestWorldCandidates_Empty(t *testing.T) {
	cands := contextselect.WorldCandidates(nil, 1000)
	if len(cands) != 0 {
		t.Fatalf("expected 0 candidates for nil input, got %d", len(cands))
	}
}

func TestSkillCandidates_Empty(t *testing.T) {
	cands := contextselect.SkillCandidates(nil, 1000)
	if len(cands) != 0 {
		t.Fatalf("expected 0 candidates for nil input, got %d", len(cands))
	}
}

// TestRejectReason_PerCandidateFirst pins F1: a rejected candidate's reason
// must reflect its own quality issue (score=0 -> relevance, freshness<0.25
// -> freshness) before being attributed to budget pressure. Before the fix,
// any non-empty chosen set suppressed the per-candidate checks, so freshness
// and zero-score rejections were both reported as "budget" — hiding the
// real cause from operators reading the manifest's top_rejected_reason.
//
// The test exercises the contract through the public API (SplitCandidates
// + Summary) and inspects Candidate.Reason (a public field populated by the
// unexported rejectReason).
func TestRejectReason_PerCandidateFirst(t *testing.T) {
	cands := []contextselect.Candidate{
		{ID: "alive", Score: 0.95, Tokens: 100, Freshness: 0.9},
		{ID: "stale", Score: 0.6, Tokens: 80, Freshness: 0.1},
		{ID: "dead", Score: 0.0, Tokens: 50, Freshness: 0.9},
	}

	// Case A: one item chosen -> stale must keep "freshness", dead must keep "relevance".
	// Before the fix both were labelled "budget".
	_, rejectedA := contextselect.SplitCandidates(cands, contextselect.ChosenIDSet([]string{"alive"}), "test")
	gotByID := map[string]string{}
	for _, c := range rejectedA {
		gotByID[c.ID] = c.Reason
	}
	if gotByID["stale"] != "freshness" {
		t.Errorf("with-chosen stale reason = %q, want %q", gotByID["stale"], "freshness")
	}
	if gotByID["dead"] != "relevance" {
		t.Errorf("with-chosen dead reason = %q, want %q", gotByID["dead"], "relevance")
	}

	// Case B: Summary's top_rejected_reason must reflect the per-candidate
	// issue, not blanket "budget".
	sum := contextselect.Summary(nil, rejectedA)
	if sum["top_rejected_reason"] != "freshness" {
		t.Errorf("Summary.top_rejected_reason = %v, want %q", sum["top_rejected_reason"], "freshness")
	}

	// Case C: a healthy candidate rejected when a healthy sibling IS chosen
	// is "budget" (its own score/freshness don't disqualify it).
	mid := []contextselect.Candidate{
		{ID: "picked", Score: 0.7, Tokens: 50, Freshness: 0.7},
		{ID: "missed", Score: 0.7, Tokens: 50, Freshness: 0.7},
	}
	_, rejectedC := contextselect.SplitCandidates(mid, contextselect.ChosenIDSet([]string{"picked"}), "test")
	if len(rejectedC) != 1 || rejectedC[0].Reason != "budget" {
		t.Errorf("healthy non-chosen candidate reason = %+v, want budget", rejectedC)
	}

	// Case D: nothing chosen at all -> per-candidate reasons are pure
	// (the budget case is skipped). alive is rejected with default
	// "relevance", stale stays "freshness", dead stays "relevance".
	_, rejectedD := contextselect.SplitCandidates(cands, contextselect.ChosenIDSet([]string{}), "test")
	if len(rejectedD) != 3 {
		t.Fatalf("expected 3 rejected, got %d", len(rejectedD))
	}
	byID := map[string]string{}
	for _, c := range rejectedD {
		byID[c.ID] = c.Reason
	}
	if byID["alive"] != "relevance" || byID["stale"] != "freshness" || byID["dead"] != "relevance" {
		t.Errorf("nothing-chosen reasons = %+v, want alive/relevance stale/freshness dead/relevance", byID)
	}
}
