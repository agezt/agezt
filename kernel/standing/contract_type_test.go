// SPDX-License-Identifier: MIT

package standing

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
)

// TestTheStoreIsTheContractsPort: the module's store answers the contract's
// store port, so the daemon can hand it to the kernel without the kernel ever
// naming this module (W3.50).
//
// The compile-time assertion is the point: a port that grew a verb would
// break HERE, in the module that implements it — not in the daemon.
func TestTheStoreIsTheContractsPort(t *testing.T) {
	var _ standingapi.Store = (*Store)(nil)
}

// TestTheRowsAreTheContractRows: the standing-order row and the vocabularies
// around it are the CONTRACT'S rows — the module keeps its own names as
// aliases, so there is one row and one spelling of it.
//
// A copy would drift: the module would then fill a shape the host no longer
// reads, and an order's trigger would silently stop matching what the host
// journals.
func TestTheRowsAreTheContractRows(t *testing.T) {
	if reflect.TypeOf(Order{}) != reflect.TypeOf(standingapi.Order{}) {
		t.Fatalf("the module's Order drifted from the contract: %v vs %v",
			reflect.TypeOf(Order{}), reflect.TypeOf(standingapi.Order{}))
	}
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"TriggerCron", TriggerCron, standingapi.TriggerCron},
		{"TriggerEvent", TriggerEvent, standingapi.TriggerEvent},
		{"InitiativeInformOnly", InitiativeInformOnly, standingapi.InitiativeInformOnly},
		{"InitiativeAsk", InitiativeAsk, standingapi.InitiativeAsk},
		{"InitiativeActOrAsk", InitiativeActOrAsk, standingapi.InitiativeActOrAsk},
	} {
		if c.module != c.contract {
			t.Errorf("the %s vocabulary drifted from the contract: %v vs %v",
				c.name, c.module, c.contract)
		}
	}
}

// TestTheNotFoundErrorIsTheContractsError: the error the layers above
// compare against is the contract's. A copy would make every
// errors.Is(err, standing.ErrNotFound) check in the kernel silently fail.
func TestTheNotFoundErrorIsTheContractsError(t *testing.T) {
	if !errors.Is(ErrNotFound, standingapi.ErrNotFound) {
		t.Fatal("the module's ErrNotFound is not the contract's error")
	}
}
