// SPDX-License-Identifier: MIT

package builtintools

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
	"github.com/agezt/agezt/kernel/toolreg"
)

// TestThePluginsNameTheContractNotTheModule: the built-in tools plugin hosts
// out-of-process plugins, and the host side of that (the binary pinning, the
// frame caps, the callback bridge, the teardown) lives in a MODULE. A plugin
// may not name a module, so the plugin side holds the CONTRACT's rows and
// asks for the spawn through the contract's PORT.
//
// The archcheck gate enforces that the plugin does not reach the module; this
// test names the rule where a reader will find it.
func TestThePluginsNameTheContractNotTheModule(t *testing.T) {
	// The module's path — named here only to document what this package must
	// NOT reach. The gate is what enforces it.
	const modulePkg = "github.com/agezt/agezt/kernel/modules/extensions/plugin"
	const contractPkg = "github.com/agezt/agezt/kernel/contract/pluginapi"
	if modulePkg == contractPkg {
		t.Fatal("the module and the contract must be different packages")
	}
	if got := reflect.TypeOf(pluginapi.PinSpec{}).PkgPath(); got != contractPkg {
		t.Fatalf("the pin row must come from %s, got %s", contractPkg, got)
	}
}

// TestThePluginToolAsksTheContractForTheSpawn: the tool registry's build
// dependencies carry the contract's spawn port — the plugin is built against
// the PORT, and the daemon hands the implementation over.
func TestThePluginToolAsksTheContractForTheSpawn(t *testing.T) {
	field, ok := reflect.TypeOf(toolreg.BuildDeps{}).FieldByName("Plugins")
	if !ok {
		t.Fatal("the build deps must carry the spawn port")
	}
	if field.Type != reflect.TypeOf((*pluginapi.Spawner)(nil)).Elem() {
		t.Errorf("the spawn port drifted from the contract: %v", field.Type)
	}
}
