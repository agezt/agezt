// SPDX-License-Identifier: MIT

package introspecttool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

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

// TestKernelSourceCarriesTheKernelCeilings: the live kernel's ceilings reach
// the overview as they are — the adapter hands the kernel's own row over, so
// the agent reads the same numbers the status operation publishes.
func TestKernelSourceCarriesTheKernelCeilings(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:                    t.TempDir(),
		Provider:                   mock.New(),
		Tools:                      map[string]toolapi.Tool{},
		SubAgentTool:               true,
		SubAgentMaxDepth:           2,
		SubAgentMaxFanout:          3,
		SubAgentMaxSpendMicrocents: 500,
		SubAgentMaxTotal:           7,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	got := NewKernelSource(k).Overview().Delegation
	want := types.SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 500, MaxTotal: 7}
	if got != want {
		t.Fatalf("delegation = %+v, want %+v", got, want)
	}
}
