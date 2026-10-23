// SPDX-License-Identifier: MIT

package channelwire

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pulseapi"
)

// TestTheFactoryLayerNamesTheContractRow: the factory layer's sink is the
// CONTRACT's row — the briefing row lives in the contract layer (W3.34), so
// the channel wiring names the contract and never the pulse module that
// composes briefs. That matters beyond the rule: the pulse module drags
// kernel/agent into its transitive deps, and the whole point of keeping this
// package above kernel/channel was to keep that out of every transport
// plugin's dependency closure.
func TestTheFactoryLayerNamesTheContractRow(t *testing.T) {
	// The DECLARED field type is what matters: the value stored in it is a
	// concrete sink, but the field answers the contract's port.
	field, ok := reflect.TypeOf(Built{}).FieldByName("Sink")
	if !ok {
		t.Fatal("the built row must carry a sink")
	}
	if field.Type != reflect.TypeOf((*pulseapi.BriefSink)(nil)).Elem() {
		t.Errorf("the factory layer's sink drifted from the contract: %v", field.Type)
	}
	var deps Deps
	if got := reflect.TypeOf(deps); got == nil {
		t.Fatal("the deps row must have a type")
	}
}

// TestTheFactoryLayerDoesNotNameThePulseModule: the archcheck gate enforces
// that the channel wiring does not reach the pulse module; this test names the
// rule where a reader of the package will find it.
func TestTheFactoryLayerDoesNotNameThePulseModule(t *testing.T) {
	// The module's own path — named here only to document what this package
	// must NOT reach. The gate is what enforces it.
	const modulePkg = "github.com/agezt/agezt/kernel/pulse"
	const contractPkg = "github.com/agezt/agezt/kernel/contract/pulseapi"
	if modulePkg == contractPkg {
		t.Fatal("the module and the contract must be different packages")
	}
	if got := reflect.TypeOf(pulseapi.Brief{}).PkgPath(); got != contractPkg {
		t.Fatalf("the brief must come from %s, got %s", contractPkg, got)
	}
}
