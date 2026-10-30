// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the data family's ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end, and it pins the FILLED half: the lake
// is the DAEMON's own, and the daemon-level binding hands exactly that lake
// over. A binding that answered for nobody would still pass against a daemon
// that holds no lake at all — so the daemon here holds one.
func TestTheAdapterBindsTheDataPortsToTheDaemon(t *testing.T) {
	lake, err := datalake.Open(t.TempDir(), func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Lake: lake})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	if got := s.dataPorts().Lake(); got == nil {
		t.Fatal("the daemon's data ports hand over no lake — the lake never reached the family")
	}
	if s.dataPorts().Lake() != root.DataLake() {
		t.Fatal("the daemon's data ports hand over a different lake than the daemon's kernel holds")
	}
}

// And the EMPTY half: a daemon that was given no lake holds none, and the
// binding says so instead of inventing one.
func TestTheAdapterBindsNoLakeToADaemonWithoutOne(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	if got := s.dataPorts().Lake(); got != nil {
		t.Fatal("a daemon with no lake hands over a lake")
	}
}
