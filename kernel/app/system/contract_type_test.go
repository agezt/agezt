// SPDX-License-Identifier: MIT

package system_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDelegationStatusIsTheContractType: the status row for the delegation
// ceilings IS the contract type, not a copy of it. The runtime's ceilings
// therefore reach the operator without a field-by-field projection.
func TestDelegationStatusIsTheContractType(t *testing.T) {
	if reflect.TypeOf(system.DelegationStatus{}) != reflect.TypeOf(types.SubAgentLimits{}) {
		t.Fatalf("the delegation row drifted from the contract type: %v vs %v", reflect.TypeOf(system.DelegationStatus{}), reflect.TypeOf(types.SubAgentLimits{}))
	}
}

// TestStatusCarriesTheKernelCeilingsUnchanged: the status operation publishes
// the kernel's effective ceilings as they are, under the row's wire names.
func TestStatusCarriesTheKernelCeilingsUnchanged(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
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
	status, err := system.New(k, nil, nil, nil, "").Status(context.Background(), system.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	want := types.SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 500, MaxTotal: 7}
	if status.Delegation != want {
		t.Fatalf("delegation = %+v, want %+v", status.Delegation, want)
	}
	wire, ok := wireMap(t, status)["delegation"].(map[string]any)
	if !ok {
		t.Fatalf("the delegation wire is not an object: %+v", status.Delegation)
	}
	if wire["enabled"] != true || wire["max_depth"] != float64(2) || wire["max_fanout"] != float64(3) || wire["max_spend_microcents"] != float64(500) || wire["max_total"] != float64(7) {
		t.Fatalf("the delegation wire lost a ceiling: %+v", wire)
	}
}
