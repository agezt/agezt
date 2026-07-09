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
