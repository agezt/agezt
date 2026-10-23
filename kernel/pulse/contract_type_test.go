// SPDX-License-Identifier: MIT

package pulse

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pulseapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the brief the
// engine composes would no longer fit the brief a plugin delivers.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Brief", Brief{}, pulseapi.Brief{}},
		{"BriefSink", (*BriefSink)(nil), (*pulseapi.BriefSink)(nil)},
		{"LogSink", LogSink{}, pulseapi.LogSink{}},
		{"SinkFunc", SinkFunc(nil), pulseapi.SinkFunc(nil)},
		{"MultiSink", MultiSink(nil), pulseapi.MultiSink(nil)},
		{"Disposition", Disposition(""), pulseapi.Disposition("")},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheDispositionsAreTheContracts: the module's dispositions are the
// contract's — the values are wire, and a copy would drift.
func TestTheDispositionsAreTheContracts(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   pulseapi.Disposition
		contract pulseapi.Disposition
	}{
		{"DispDrop", DispDrop, pulseapi.DispDrop},
		{"DispDigest", DispDigest, pulseapi.DispDigest},
		{"DispNotify", DispNotify, pulseapi.DispNotify},
		{"DispAlert", DispAlert, pulseapi.DispAlert},
		{"DispAct", DispAct, pulseapi.DispAct},
	} {
		if c.module != c.contract {
			t.Errorf("the %s disposition is %q, want %q", c.name, c.module, c.contract)
		}
	}
}
