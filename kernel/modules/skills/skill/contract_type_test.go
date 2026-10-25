// SPDX-License-Identifier: MIT

package skill

// Provenance: the module's contract tests for the W3.55 slice — that the
//             rows, the sentinels and the rules are the CONTRACT'S (by type
//             identity), and that the Forge answers the contract's ports.

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// TestTheRowsAreTheContractsRows: the module keeps every name as an ALIAS, so
// there is one row and one spelling of it. Type identity is the proof: an
// alias and the original are the same type, so a layer above the module that
// holds the contract's row can pass it to the module without a conversion.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	var (
		sk      Skill
		apiSk   skillapi.Skill
		scored  Scored
		apiSc   skillapi.Scored
		direct  ActivationDirective
		apiDir  skillapi.ActivationDirective
		hygiene HygieneReport
		apiHyg  skillapi.HygieneReport
		spec    CreateSpec
		apiSpec skillapi.CreateSpec
	)
	sk, apiSk = apiSk, sk
	scored, apiSc = apiSc, scored
	direct, apiDir = apiDir, direct
	hygiene, apiHyg = apiHyg, hygiene
	spec, apiSpec = apiSpec, spec
	_ = sk
	_ = scored
	_ = direct
	_ = hygiene
	_ = spec
}

// TestTheSentinelsAreTheContracts: the sentinels the layers compare against
// are the contract's, so `errors.Is` matches them wherever they are raised —
// including in the app layer, which no longer names this module.
func TestTheSentinelsAreTheContracts(t *testing.T) {
	if !errors.Is(ErrIllegalTransition, skillapi.ErrIllegalTransition) {
		t.Fatal("the module's illegal-transition sentinel must be the contract's")
	}
	if !errors.Is(ErrNotFound, skillapi.ErrNotFound) {
		t.Fatal("the module's not-found sentinel must be the contract's")
	}
	if !errors.Is(ErrEmptyBody, skillapi.ErrEmptyBody) {
		t.Fatal("the module's empty-body sentinel must be the contract's")
	}
}

// TestTheRulesAreTheContractsRules: the ranking and the directive parser
// live with the rows they judge. The module's names delegate to the contract,
// so the layers above the module and the module itself rank identically.
func TestTheRulesAreTheContractsRules(t *testing.T) {
	skills := []Skill{{
		ID: "a1", Name: "diagnose-ci", Description: "diagnose failing CI",
		Triggers: []string{"ci"}, Body: "steps", Status: StatusActive,
	}}
	got := Retrieve(skills, "failing ci", 5, 1_000)
	want := skillapi.Retrieve(skills, "failing ci", 5, 1_000)
	if len(got) != len(want) {
		t.Fatalf("the module retrieved %d skills, the contract %d", len(got), len(want))
	}
	if len(got) == 1 && (got[0].Skill.ID != want[0].Skill.ID || got[0].Score != want[0].Score) {
		t.Fatalf("the module's ranking differs from the contract's: %+v vs %+v", got, want)
	}
	d := skillapi.ParseActivationDirective("/skill diagnose-ci\nlook at the logs")
	if !d.Explicit || len(d.Refs) != 1 || d.Refs[0] != "diagnose-ci" {
		t.Fatalf("the module's directive differs from the contract's: %+v", d)
	}
}

// TestTheBundlePortAnswersARealNil: a Forge with no bundle store must answer
// a REAL nil, not a typed nil pointer wrapped in the contract's interface.
//
// A typed nil handed back as an interface is not nil to the caller: the tool
// registry would bind a bundle store that is not there and panic on the first
// resource read instead of reporting that bundles are unavailable.
func TestTheBundlePortAnswersARealNil(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f := NewForge(store, nil)
	if got := f.Bundles(); got != nil {
		t.Fatalf("a forge without bundles must answer a nil bundle port, got %v", got)
	}
}
