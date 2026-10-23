// SPDX-License-Identifier: MIT

package acpagent

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/acpapi"
)

// TestTheToolHoldsTheContractPort: the bridge tool's catalogue is the
// CONTRACT's port — the type the daemon hands over. The tool is a plugin, so
// it may not name the catalogue module, and the module's own type is not
// something a plugin could hold.
func TestTheToolHoldsTheContractPort(t *testing.T) {
	field, ok := reflect.TypeOf(Tool{}).FieldByName("Catalog")
	if !ok {
		t.Fatal("the tool must carry a catalogue")
	}
	if field.Type != reflect.TypeOf((*acpapi.Catalog)(nil)).Elem() {
		t.Errorf("the tool's catalogue drifted from the contract: %v", field.Type)
	}
}

// TestTheToolDoesNotNameTheCatalogueModule: the archcheck gate enforces that
// the plugin does not reach the catalogue module; this test names the rule so
// a future reader knows where the boundary is.
func TestTheToolDoesNotNameTheCatalogueModule(t *testing.T) {
	// The module's path — named here only to document what this package must
	// NOT reach. The gate is what enforces it.
	const modulePkg = "github.com/agezt/agezt/kernel/modules/extensions/acpcatalog"
	const contractPkg = "github.com/agezt/agezt/kernel/contract/acpapi"
	if modulePkg == contractPkg {
		t.Fatal("the module and the contract must be different packages")
	}
	if got := reflect.TypeOf((*acpapi.Catalog)(nil)).Elem().PkgPath(); got != contractPkg {
		t.Fatalf("the port must come from %s, got %s", contractPkg, got)
	}
}
