// SPDX-License-Identifier: MIT

package okr

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
)

// TestTheRowsAreTheContractsRows: the module keeps the names its own callers
// use as ALIASES of the contract's rows — one row, one spelling of it.
//
// The check is on the TYPE IDENTITY, not on a field-by-field comparison: two
// shapes that happen to have the same fields are still two shapes, and a
// caller that passes one where the other is expected has to convert.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	if reflect.TypeOf(Objective{}) != reflect.TypeOf(okrapi.Objective{}) {
		t.Errorf("the objective row is not the contract's row")
	}
	if reflect.TypeOf(KeyResult{}) != reflect.TypeOf(okrapi.KeyResult{}) {
		t.Errorf("the key-result row is not the contract's row")
	}
	if reflect.TypeOf(ObjectiveProgress{}) != reflect.TypeOf(okrapi.ObjectiveProgress{}) {
		t.Errorf("the objective-progress row is not the contract's row")
	}
	if reflect.TypeOf(KeyResultProgress{}) != reflect.TypeOf(okrapi.KeyResultProgress{}) {
		t.Errorf("the key-result-progress row is not the contract's row")
	}
	if reflect.TypeOf(CreateSpec{}) != reflect.TypeOf(okrapi.CreateSpec{}) {
		t.Errorf("the create row is not the contract's row")
	}
	if reflect.TypeOf(Filter{}) != reflect.TypeOf(okrapi.Filter{}) {
		t.Errorf("the filter row is not the contract's row")
	}
	if reflect.TypeOf(StatusActive) != reflect.TypeOf(okrapi.StatusActive) {
		t.Errorf("the status constants are not the contract's")
	}
}

// TestTheSentinelsAreTheContracts: the module's sentinels are the contract's,
// so a layer above the module can compare against them without naming the
// module.
func TestTheSentinelsAreTheContracts(t *testing.T) {
	if ErrNotFound != okrapi.ErrNotFound {
		t.Fatal("the module's not-found sentinel must be the contract's")
	}
	if ErrKRNotFound != okrapi.ErrKRNotFound {
		t.Fatal("the module's key-result sentinel must be the contract's")
	}
	if ErrInvalidStatus != okrapi.ErrInvalidStatus {
		t.Fatal("the module's invalid-status sentinel must be the contract's")
	}
}

// TestTheRollupRuleIsTheContracts: the rule that decides whether a goal counts
// as met travels with the row into the contract.
//
// A Go type alias cannot carry methods — so if the rule had stayed in the
// module, the alias would have lost it and the layers above the module would
// have had to reach into the module to compute a rollup.
func TestTheRollupRuleIsTheContracts(t *testing.T) {
	o := Objective{ID: "o-1", KeyResults: []KeyResult{{ID: "kr-1", Target: 1, TaskIDs: []string{"t-1"}}}}
	p := o.Progress(func(string) bool { return true })
	if !p.Achieved {
		t.Fatal("the aliased objective must keep the contract's rollup rule")
	}
}

// TestTheStoreSatisfiesTheContractsPort: the module's store is the
// implementation of the contract's port — the shape the layers above hold.
func TestTheStoreSatisfiesTheContractsPort(t *testing.T) {
	var _ okrapi.Store = (*Store)(nil)
}
