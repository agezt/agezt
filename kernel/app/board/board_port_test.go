// SPDX-License-Identifier: MIT

package board

// Provenance: the app's own test for the board's PORT (W3.59). The app used
//
//	to declare its own interface with the same eleven methods as the
//	contract's port — the board's surface had three shapes (the contract's
//	port, the module's concrete store and the app's) and the app's was the
//	only one that did not need to exist.
//
//	Identity is asserted at RUNTIME: a compile-time assertion would be a
//	build error, and a build error is not a test result.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// TestTheAppsStorePortIsTheContractsPort: one shape for the board's surface.
func TestTheAppsStorePortIsTheContractsPort(t *testing.T) {
	if got, want := reflect.TypeFor[Store](), reflect.TypeFor[boardapi.Store](); got != want {
		t.Fatalf("the app's store port is %v, want the contract's %v", got, want)
	}
}
