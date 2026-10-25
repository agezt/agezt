// SPDX-License-Identifier: MIT

package cadence

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

// TestTheStoreIsTheContractsHostPort: the module's store answers the
// contract's HOST port, so the composition root can hand it to the kernel
// without the kernel ever naming this module (W3.49).
//
// The compile-time assertion is the point: a port that grew a verb would
// break HERE, in the module that implements it — not in the daemon.
func TestTheStoreIsTheContractsHostPort(t *testing.T) {
	var _ cadenceapi.HostStore = (*Store)(nil)
	var _ cadenceapi.Store = (*Store)(nil)
}

// TestTheEngineIsTheContractsPort: the live resident answers the contract's
// engine port, so the daemon can hand it over as a port and the kernel can
// hold it without naming the module.
func TestTheEngineIsTheContractsPort(t *testing.T) {
	var _ cadenceapi.Engine = (*Engine)(nil)
}

// TestTheRowsAreTheContractRows: the schedule row and the vocabularies
// around it are the CONTRACT'S rows — the module keeps its own names as
// aliases, so there is one row and one spelling of it.
//
// A copy would drift: the module would then fill a shape the host no longer
// reads, and a schedule's target would silently stop matching what the host
// journals.
func TestTheRowsAreTheContractRows(t *testing.T) {
	if reflect.TypeOf(Entry{}) != reflect.TypeOf(cadenceapi.Entry{}) {
		t.Fatalf("the module's Entry drifted from the contract: %v vs %v",
			reflect.TypeOf(Entry{}), reflect.TypeOf(cadenceapi.Entry{}))
	}
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"TargetWorkflow", TargetWorkflow, cadenceapi.TargetWorkflow},
		{"TargetSystemTask", TargetSystemTask, cadenceapi.TargetSystemTask},
		{"TargetTool", TargetTool, cadenceapi.TargetTool},
		{"SourceOperator", SourceOperator, cadenceapi.SourceOperator},
		{"ModeContinuous", ModeContinuous, cadenceapi.ModeContinuous},
		{"SystemTaskCatalogSync", SystemTaskCatalogSync, cadenceapi.SystemTaskCatalogSync},
	} {
		if c.module != c.contract {
			t.Errorf("the %s vocabulary drifted from the contract: %v vs %v",
				c.name, c.module, c.contract)
		}
	}
}
