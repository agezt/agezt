// SPDX-License-Identifier: MIT

package taste

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/tasteapi"
)

// TestTheRowsAreTheContractsRows: the module keeps the names its own callers
// use as ALIASES of the contract's rows — one row, one spelling of it.
//
// The check is on the TYPE IDENTITY, not on a field-by-field comparison: two
// shapes that happen to have the same fields are still two shapes, and a
// caller that passes one where the other is expected has to convert.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	if reflect.TypeOf(Exemplar{}) != reflect.TypeOf(tasteapi.Exemplar{}) {
		t.Errorf("the exemplar row is not the contract's row")
	}
	if reflect.TypeOf(CreateSpec{}) != reflect.TypeOf(tasteapi.CreateSpec{}) {
		t.Errorf("the create row is not the contract's row")
	}
	if reflect.TypeOf(Filter{}) != reflect.TypeOf(tasteapi.Filter{}) {
		t.Errorf("the filter row is not the contract's row")
	}
}

// TestTheNotFoundSentinelIsTheContracts: the module's sentinel is the
// contract's, so a layer above the module can compare against it without
// naming the module.
func TestTheNotFoundSentinelIsTheContracts(t *testing.T) {
	if ErrNotFound != tasteapi.ErrNotFound {
		t.Fatal("the module's not-found sentinel must be the contract's")
	}
}

// TestTheStoreSatisfiesTheContractsPort: the module's store is the
// implementation of the contract's port — the shape the layers above hold.
func TestTheStoreSatisfiesTheContractsPort(t *testing.T) {
	var _ tasteapi.Store = (*Store)(nil)
}
