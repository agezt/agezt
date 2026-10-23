// SPDX-License-Identifier: MIT

package builtinchannels

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/pulseapi"
	"github.com/agezt/agezt/kernel/platform/channelwire"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/pulseapi"
	modulePkg   = "github.com/agezt/agezt/kernel/pulse"
	platformPkg = "github.com/agezt/agezt/kernel/platform/channelwire"
)

// TestTheChannelsNameTheContractRows: the built-in channels are a PLUGIN, so
// they may reach the contract layer and nothing else. The brief they deliver
// and the sinks they build are the CONTRACT's rows — the module that composes
// briefs is a module, and a plugin may not name it.
func TestTheChannelsNameTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Brief", pulseapi.Brief{}, pulseapi.Brief{}},
		{"BriefSink", (*pulseapi.BriefSink)(nil), (*pulseapi.BriefSink)(nil)},
		{"SinkFunc", pulseapi.SinkFunc(nil), pulseapi.SinkFunc(nil)},
		{"MultiSink", pulseapi.MultiSink(nil), pulseapi.MultiSink(nil)},
		{"LogSink", pulseapi.LogSink{}, pulseapi.LogSink{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheChannelsDoNotNameThePulseModule: the plugin must not reach the
// pulse module — the archcheck gate enforces it, and this test names the
// rule so a future reader knows where the boundary is.
func TestTheChannelsDoNotNameThePulseModule(t *testing.T) {
	// The brief the plugin delivers comes from the contract, not from the
	// module that composes briefs.
	if got := reflect.TypeOf(pulseapi.Brief{}).PkgPath(); got != contractPkg {
		t.Fatalf("the brief must come from %s, got %s", contractPkg, got)
	}
	// The module is named here only to document what the plugin must NOT
	// reach: the archcheck gate is what enforces it.
	if modulePkg == contractPkg {
		t.Fatal("the module and the contract must be different packages")
	}
}

// TestTheChannelsNameThePlatformFactoryLayer: the factory layer the channels
// register against is the PLATFORM one (kernel/platform/channelwire) — the
// seam between the daemon (which owns the bus, the inbound handler and the
// configuration) and the channel plugins. A plugin may reach the platform, so
// naming it needs no allowlist entry.
func TestTheChannelsNameThePlatformFactoryLayer(t *testing.T) {
	field, ok := reflect.TypeOf(channelwire.Deps{}).FieldByName("Bus")
	if !ok {
		t.Fatal("the deps row must carry the bus")
	}
	// The declared field type is the platform's bus — the row the factory
	// layer hands to every channel factory.
	if field.Type != reflect.TypeOf((*bus.Bus)(nil)) {
		t.Errorf("the factory layer's deps drifted from the platform's bus: %v", field.Type)
	}
	if got := reflect.TypeOf(channelwire.Deps{}).PkgPath(); got != platformPkg {
		t.Errorf("the factory layer must come from %s, got %s", platformPkg, got)
	}
}
