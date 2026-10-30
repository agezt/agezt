// SPDX-License-Identifier: MIT

package council

import "testing"

// The council's ports are EMPTY when there is no daemon.
//
// The council's ports are filled from the daemon's own kernel, by type: there
// is no foreign handle to fill them with any more. A host with no daemon
// holds no council: the seats are empty and the store is nowhere. The council
// answers with what it has rather than calling into nothing.
func TestTheCouncilPortsAreEmptyWithoutADaemon(t *testing.T) {
	ports := PortsFromKernel(nil, t.TempDir())

	if members := ports.Members(); len(members) != 0 {
		t.Errorf("members = %v, want none", members)
	}
	if ports.Store() != nil {
		t.Error("the council's ports hold a store on a host with no daemon")
	}
}
