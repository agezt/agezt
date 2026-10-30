// SPDX-License-Identifier: MIT

package council

import "testing"

// The council's ports are EMPTY when the handle is not the daemon's own.
//
// A host with no daemon holds no council: the seats are empty and the store
// is nowhere. The council answers with what it has rather than calling into
// nothing.
func TestTheCouncilPortsAreEmptyWithoutADaemon(t *testing.T) {
	ports := PortsFromHandle("not a daemon", t.TempDir())

	if members := ports.Members(); len(members) != 0 {
		t.Errorf("members = %v, want none", members)
	}
	if ports.Store() != nil {
		t.Error("the council's ports hold a store on a host with no daemon")
	}
}
