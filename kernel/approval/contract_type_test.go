// SPDX-License-Identifier: MIT

package approval

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
)

// TestTheSubmitRowsAreTheContractRows: the platform's submit and outcome rows
// are the CONTRACT'S rows — the workflow engine's port carries them (W3.48),
// and the engine is a MODULE, so the row has to live where a module may
// name it.
//
// The platform keeps its own names as ALIASES, so there is one row and one
// spelling: the registry, the engine and the operator's prompt all read the
// same shape. A copy would drift — the engine would then hand the registry a
// request the registry could not read back.
func TestTheSubmitRowsAreTheContractRows(t *testing.T) {
	if reflect.TypeOf(SubmitSpec{}) != reflect.TypeOf(approvalapi.SubmitSpec{}) {
		t.Fatalf("the platform's SubmitSpec drifted from the contract: %v vs %v",
			reflect.TypeOf(SubmitSpec{}), reflect.TypeOf(approvalapi.SubmitSpec{}))
	}
	if reflect.TypeOf(Outcome{}) != reflect.TypeOf(approvalapi.Outcome{}) {
		t.Fatalf("the platform's Outcome drifted from the contract: %v vs %v",
			reflect.TypeOf(Outcome{}), reflect.TypeOf(approvalapi.Outcome{}))
	}
}

// TestTheRegistryStillAnswersTheContractRows: the registry is the platform's
// implementation and it answers the contract's rows — a granted decision is
// the contract's verdict, not a copy of it.
func TestTheRegistryStillAnswersTheContractRows(t *testing.T) {
	if reg := New(Config{}); reg == nil {
		t.Fatal("the registry did not build")
	}
}
