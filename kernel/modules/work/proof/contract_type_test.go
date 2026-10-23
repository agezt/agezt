// SPDX-License-Identifier: MIT

package proof

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/proofapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. The workboard's task row carries a proof,
// so a drifted proof row would not fit the task row that carries it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Criterion", Criterion{}, proofapi.Criterion{}},
		{"Evidence", Evidence{}, proofapi.Evidence{}},
		{"Proof", Proof{}, proofapi.Proof{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}
