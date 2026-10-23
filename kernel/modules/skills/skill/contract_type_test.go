// SPDX-License-Identifier: MIT

package skill

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the CONTRACT'S
// row, not a copy of it. A copy would drift — the module would then persist
// a shape the contract no longer describes, and the agent-facing tool (which
// speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Status", Status(""), skillapi.Status("")},
		{"Skill", Skill{}, skillapi.Skill{}},
		{"Metrics", Metrics{}, skillapi.Metrics{}},
		{"CreateSpec", CreateSpec{}, skillapi.CreateSpec{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheForgeIsTheContractPort: the module's Forge — the journaled,
// reversible state machine the CLI drives — satisfies the contract's port,
// so the agent-facing tool can be handed it without naming the module.
func TestTheForgeIsTheContractPort(t *testing.T) {
	var _ skillapi.Forge = (*Forge)(nil)
}

// TestTheBundleStoreIsTheContractPort: the on-disk bundle store satisfies
// the contract's bundle port too.
func TestTheBundleStoreIsTheContractPort(t *testing.T) {
	var _ skillapi.BundleStore = (*BundleStore)(nil)
}

// TestTheConstantsAreTheContracts: the lifecycle constants are values the
// daemon persists, so the module's names must BE the contract's values.
func TestTheConstantsAreTheContracts(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   Status
		contract skillapi.Status
	}{
		{"StatusDraft", StatusDraft, skillapi.StatusDraft},
		{"StatusShadow", StatusShadow, skillapi.StatusShadow},
		{"StatusActive", StatusActive, skillapi.StatusActive},
		{"StatusQuarantined", StatusQuarantined, skillapi.StatusQuarantined},
		{"StatusArchived", StatusArchived, skillapi.StatusArchived},
	} {
		if c.module != c.contract {
			t.Errorf("the %s constant = %q, want the contract's %q", c.name, c.module, c.contract)
		}
	}
	if DefaultVersion != skillapi.DefaultVersion {
		t.Errorf("the default version = %q, want the contract's %q", DefaultVersion, skillapi.DefaultVersion)
	}
}

// TestTheRulesAreTheContracts: the module's lifecycle rules and the
// contract's must agree on every verdict — a second copy of the state machine
// would drift silently, and the state machine decides what may be deployed.
func TestTheRulesAreTheContracts(t *testing.T) {
	for _, from := range []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived, "bogus"} {
		for _, to := range []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived, "bogus"} {
			if got, want := CanTransition(from, to), skillapi.CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%q, %q) = %v, want the contract's %v", from, to, got, want)
			}
		}
		if got, want := ValidStatus(from), skillapi.ValidStatus(from); got != want {
			t.Errorf("ValidStatus(%q) = %v, want the contract's %v", from, got, want)
		}
		gotTo, gotOK := PromoteTarget(from)
		wantTo, wantOK := skillapi.PromoteTarget(from)
		if gotTo != wantTo || gotOK != wantOK {
			t.Errorf("PromoteTarget(%q) = %q, %v; want the contract's %q, %v", from, gotTo, gotOK, wantTo, wantOK)
		}
	}
}
