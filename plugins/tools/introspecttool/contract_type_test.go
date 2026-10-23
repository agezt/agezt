// SPDX-License-Identifier: MIT

package introspecttool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/types"
)

// TestTheRowsAreTheContractRows: every row the plugin publishes IS the
// contract's row, not a copy of it. A copy would drift — the plugin would
// then publish a shape the contract no longer describes, and the app-layer
// adapter (which builds the contract's row) would no longer fit the plugin.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		plugin   any
		contract any
	}{
		{"Delegation", Delegation{}, introspectapi.Delegation{}},
		{"ReaperOverview", ReaperOverview{}, introspectapi.ReaperOverview{}},
		{"ReaperAgent", ReaperAgent{}, introspectapi.ReaperAgent{}},
		{"ReaperDegradedAgent", ReaperDegradedAgent{}, introspectapi.ReaperDegradedAgent{}},
		{"ReaperMisconfiguredAgent", ReaperMisconfiguredAgent{}, introspectapi.ReaperMisconfiguredAgent{}},
		{"ReaperReport", ReaperReport{}, introspectapi.ReaperReport{}},
		{"Overview", Overview{}, introspectapi.Overview{}},
	} {
		if reflect.TypeOf(c.plugin) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.plugin), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheSourceIsTheContractPort: the tool reads through the contract's
// port, so the app layer's adapter satisfies it without the plugin naming
// the kernel.
func TestTheSourceIsTheContractPort(t *testing.T) {
	if reflect.TypeOf((*Source)(nil)).Elem() != reflect.TypeOf((*introspectapi.Source)(nil)).Elem() {
		t.Fatal("the source drifted from the contract port")
	}
}

// TestDelegationIsTheContractType: the overview's delegation row IS the
// contract type, not a copy of it. The kernel's ceilings therefore reach the
// agent without a field-by-field projection.
func TestDelegationIsTheContractType(t *testing.T) {
	if reflect.TypeOf(Delegation{}) != reflect.TypeOf(types.SubAgentLimits{}) {
		t.Fatalf("the delegation row drifted from the contract type: %v vs %v", reflect.TypeOf(Delegation{}), reflect.TypeOf(types.SubAgentLimits{}))
	}
}

// TestOverviewCarriesTheDelegationCeilings: the overview publishes the
// ceilings under the row's wire names, and the row is the contract type, so
// the agent reads the same numbers the status operation publishes.
func TestOverviewCarriesTheDelegationCeilings(t *testing.T) {
	ov := Overview{Delegation: Delegation{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 1500, MaxTotal: 4}}
	raw, err := json.Marshal(ov.Delegation)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"enabled":true,"max_depth":2,"max_fanout":3,"max_spend_microcents":1500,"max_total":4}` {
		t.Fatal(string(raw))
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["enabled"] != true || wire["max_depth"] != float64(2) || wire["max_fanout"] != float64(3) || wire["max_spend_microcents"] != float64(1500) || wire["max_total"] != float64(4) {
		t.Fatalf("the delegation wire lost a ceiling: %+v", wire)
	}
}
