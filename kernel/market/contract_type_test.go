// SPDX-License-Identifier: MIT

package market

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/marketapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the CONTRACT'S
// row, not a copy of it. A copy would drift — the module would then publish a
// shape the contract no longer describes, and the built-in catalogue (which
// speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Pack", Pack{}, marketapi.Pack{}},
		{"PackSkill", PackSkill{}, marketapi.PackSkill{}},
		{"Signature", Signature{}, marketapi.Signature{}},
		{"Marketplace", Marketplace{}, marketapi.Marketplace{}},
		{"MarketplaceEntry", MarketplaceEntry{}, marketapi.MarketplaceEntry{}},
		{"Listing", Listing{}, marketapi.Listing{}},
		{"InstalledPack", InstalledPack{}, marketapi.InstalledPack{}},
		{"Event", Event{}, marketapi.Event{}},
		{"Source", Source{}, marketapi.Source{}},
		{"SyncResult", SyncResult{}, marketapi.SyncResult{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheLibraryIsTheContractPort: the module's catalogue port IS the
// contract's — the built-in catalogue is a plugin, so it implements the
// contract rather than naming this module.
func TestTheLibraryIsTheContractPort(t *testing.T) {
	var _ marketapi.Library = (Library)(nil)
}

// TestTheManagerIsTheContractPort: the manager IS the contract's marketplace
// port (W3.46). The kernel holds the port and never names this module, so
// this line is what makes a port change a compile error HERE — in the module
// that implements it — instead of a silent drift between two shapes.
func TestTheManagerIsTheContractPort(t *testing.T) {
	var _ marketapi.Manager = (*Manager)(nil)
}

// TestTheRulesAreTheContracts: the module's delegations and the contract's
// must agree on every verdict — a second copy of the ordering rule or the
// summary would drift silently, and the ordering decides which version an
// install resolves to.
func TestTheRulesAreTheContracts(t *testing.T) {
	versions := []string{"", "1.0.0", "1.9.0", "1.10.0", "2.0.0-rc.1", "2.0.0", "0.0.1", "not-a-version"}
	for _, a := range versions {
		for _, b := range versions {
			if got, want := CompareVersions(a, b), marketapi.CompareVersions(a, b); got != want {
				t.Errorf("CompareVersions(%q, %q) = %d, want the contract's %d", a, b, got, want)
			}
		}
	}

	skills := []PackSkill{
		{SkillMD: "---\nname: a\ndescription: d\n---\nbody\n"},
		{SkillMD: "---\nname: b\n---\nbody\n"},
		{SkillMD: "no frontmatter"},
		{SkillMD: ""},
	}
	for _, s := range skills {
		gotMD, gotErr := SkillSummary(s)
		wantMD, wantErr := marketapi.SkillSummary(s)
		if (gotErr == nil) != (wantErr == nil) {
			t.Errorf("the module and the contract disagree on the summary of %q", s.SkillMD)
			continue
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Errorf("the module's error %q differs from the contract's %q", gotErr, wantErr)
		}
		if gotMD != wantMD {
			t.Errorf("the module summarised %q, the contract %q", gotMD, wantMD)
		}
	}
}

// TestThePatternsAreTheContracts: the name and version patterns are the
// rule, and the module compiles its own regex from the contract's pattern —
// so the error text an operator reads is the same on both sides.
func TestThePatternsAreTheContracts(t *testing.T) {
	if nameRe.String() != marketapi.NamePattern {
		t.Errorf("the name pattern = %q, want the contract's %q", nameRe.String(), marketapi.NamePattern)
	}
	if semverRe.String() != marketapi.SemverPattern {
		t.Errorf("the semver pattern = %q, want the contract's %q", semverRe.String(), marketapi.SemverPattern)
	}
}
