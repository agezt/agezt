package builtinguardians

// The guardian's spend ceilings are the operator's, not the daemon's.
//
// A guardian is a system agent with a built-in spend ceiling ($5/run,
// $10/day). The reconcile that runs on every boot may TIGHTEN that ceiling
// (a cap above the built-in maximum is clamped down, an unset one is
// filled) — but it may never raise a ceiling the operator lowered. The
// reconcile used to assign the built-in values outright, so an operator's
// tightened budget silently went back to $5/$10 on the next restart: the
// daemon undid the operator's own policy, and the guardian-budget
// guardian's contract ("pause or lower max_daily_mc; never raise ceilings")
// was broken by the daemon itself.

import (
	"testing"

	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// TestSeedAll_KeepsAnOperatorsLoweredGuardianCap: a guardian whose caps the
// operator lowered keeps them across the boot.
func TestSeedAll_KeepsAnOperatorsLoweredGuardianCap(t *testing.T) {
	h := &fakeHost{agents: []roster.Profile{{
		Slug:         "guardian-health",
		System:       true,
		Enabled:      false,
		MaxCostMc:    1 * usd,
		MaxDailyMc:   2 * usd,
		TrustCeiling: defaultTrustCeiling,
		MemoryScope:  "system/guardian-health",
	}}}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	var got *roster.Profile
	for i := range h.agents {
		if h.agents[i].Slug == "guardian-health" {
			got = &h.agents[i]
		}
	}
	if got == nil {
		t.Fatal("guardian-health missing")
	}
	if got.MaxCostMc != 1*usd {
		t.Fatalf("operator's lowered run cap was reset: %d (want %d)", got.MaxCostMc, 1*usd)
	}
	if got.MaxDailyMc != 2*usd {
		t.Fatalf("operator's lowered daily cap was reset: %d (want %d)", got.MaxDailyMc, 2*usd)
	}
}

// TestSeedAll_FillsAnUnsetGuardianCap: a guardian with no cap at all gets
// the built-in ceiling (the reconcile fills, it does not leave a guardian
// without a limit).
func TestSeedAll_FillsAnUnsetGuardianCap(t *testing.T) {
	h := &fakeHost{agents: []roster.Profile{{
		Slug:         "guardian-health",
		System:       true,
		TrustCeiling: defaultTrustCeiling,
		MemoryScope:  "system/guardian-health",
	}}}
	if _, err := SeedAll(h, ""); err != nil {
		t.Fatal(err)
	}
	var got *roster.Profile
	for i := range h.agents {
		if h.agents[i].Slug == "guardian-health" {
			got = &h.agents[i]
		}
	}
	if got == nil {
		t.Fatal("guardian-health missing")
	}
	if got.MaxCostMc != maxCostMc || got.MaxDailyMc != maxDailyMc {
		t.Fatalf("unset caps not filled: run=%d daily=%d (want %d/%d)", got.MaxCostMc, got.MaxDailyMc, maxCostMc, maxDailyMc)
	}
}
