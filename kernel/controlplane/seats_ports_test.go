// SPDX-License-Identifier: MIT
package controlplane

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/work/seat"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the seats' ports — the APPLICATION fills them from the
// kernel handle.
//
// The test pins that binding end to end, and it pins the FILLED half: the
// catalog is the DAEMON's own, and the daemon-level binding hands exactly that
// catalog over. A binding that answered for nobody would still pass against a
// daemon that holds no catalog at all — so the daemon here holds one.
func TestTheAdapterBindsTheSeatPortsToTheDaemon(t *testing.T) {
	base := t.TempDir()
	catalog, err := seat.OpenStore(filepath.Join(base, "seats"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	root, err := runtime.Open(runtime.Config{BaseDir: base, Provider: mock.New(), Seats: catalog})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	if got := s.seatPorts().Store(); got == nil {
		t.Fatal("the daemon's seat ports hand over no store — the catalog never reached the family")
	}
	if s.seatPorts().Store() != root.Seats() {
		t.Fatal("the daemon's seat ports hand over a different store than the daemon's kernel holds")
	}
}

// And the EMPTY half: a daemon that was given no seat catalog holds none, and
// the binding says so instead of inventing one.
func TestTheAdapterBindsNoSeatStoreToADaemonWithoutACatalog(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	if got := s.seatPorts().Store(); got != nil {
		t.Fatal("a daemon with no seat catalog hands over a store")
	}
}
