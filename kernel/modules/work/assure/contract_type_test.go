// SPDX-License-Identifier: MIT

package assure

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/assureapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the proof that
// carries a verdict would no longer fit the verdict the judge produces.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Verdict", Verdict{}, assureapi.Verdict{}},
		{"Attempt", Attempt{}, assureapi.Attempt{}},
		{"Result", Result{}, assureapi.Result{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheAttemptBudgetIsTheContracts: the loop clamps to these bounds, so
// the module and the contract must name the same ones.
func TestTheAttemptBudgetIsTheContracts(t *testing.T) {
	if DefaultMaxAttempts != assureapi.DefaultMaxAttempts {
		t.Errorf("the default attempt budget is %d, want %d",
			DefaultMaxAttempts, assureapi.DefaultMaxAttempts)
	}
	if MaxMaxAttempts != assureapi.MaxMaxAttempts {
		t.Errorf("the attempt ceiling is %d, want %d", MaxMaxAttempts, assureapi.MaxMaxAttempts)
	}
}

// TestTheParseVerdictDelegatesToTheContract: the module's parser is a
// delegation, so the two must agree on EVERY verdict — the parsed one and the
// refused one.
func TestTheParseVerdictDelegatesToTheContract(t *testing.T) {
	for _, in := range []string{
		`{"complete":true}`,
		`{"complete":false,"gap":"the tests are missing"}`,
		"```json\n{\"complete\":true}\n```",
		`Here is my answer: {"complete":true} — hope that helps.`,
		"{}",
		"",
		"   ",
		"not json at all",
		"{",
		`{"complete":"yes"}`,
	} {
		gotVerdict, gotOK := ParseVerdict(in)
		wantVerdict, wantOK := assureapi.ParseVerdict(in)
		if gotOK != wantOK {
			t.Errorf("ParseVerdict(%q) ok = %v, want %v", in, gotOK, wantOK)
		}
		if gotVerdict != wantVerdict {
			t.Errorf("ParseVerdict(%q) = %+v, want %+v", in, gotVerdict, wantVerdict)
		}
	}
}
