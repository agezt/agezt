// SPDX-License-Identifier: MIT

package binding

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The bridge hands the daemon's own kernel to the family that owns the ports.
//
// The hand-over is TYPED: the bridge takes the daemon's own kernel, and the
// family fills its own ports from it. There is no untyped handle in the
// hand-over any more — an `any` in a handle's place is a door, and the scope's
// door closed with the adapter's.
//
// The test pins the FILLED half of several families at once: a bridge that
// filled one family and left the rest empty would still answer "the bridge
// handed the handle to the family" for the one it filled, and every other
// family would be calling into nothing.
func TestTheBridgeFillsTheFamilysPortsFromTheHandle(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	dir := t.TempDir()
	scope := ScopeFromKernel(k, dir)
	if scope.JournalPorts.Events == nil {
		t.Fatal("the bridge did not hand the daemon to the journal family")
	}
	if !scope.BusPorts.Ready() {
		t.Error("the bridge did not hand the daemon to the bus family")
	}
	if scope.ToolsPorts.Registry == nil {
		t.Error("the bridge did not hand the daemon to the tools family")
	}
	if scope.CouncilPorts.Store() == nil {
		t.Error("the bridge did not hand the daemon to the council family")
	}
	if scope.Directory != dir {
		t.Errorf("directory = %q, want the scope's own %q", scope.Directory, dir)
	}
	if !scope.Filled() {
		t.Error("the bridge did not fill the scope")
	}
}

// The bridge's fill is TYPED: it takes the daemon's own kernel, not an
// untyped handle.
//
// There is no foreign handle to refuse any more: the bridge never sees a
// value it cannot name, so the refusal it used to carry was a door that
// nothing walked through.
func TestTheBridgesFillIsTyped(t *testing.T) {
	fill := reflect.TypeOf(ScopeFromKernel)
	if fill.NumIn() != 2 {
		t.Fatalf("the scope's fill takes %d arguments, want 2", fill.NumIn())
	}
	kernel := reflect.TypeOf((*runtime.Kernel)(nil))
	if fill.In(0) != kernel {
		t.Fatalf("the scope's fill takes %v, want the daemon's own kernel %v", fill.In(0), kernel)
	}
}

// The bridge is a hand-over, not a decision: the family's own refusal reaches
// the caller as it is.
//
// With no daemon there is nothing to hand over, so the bridge asks for the
// refusing scope — and the scope carries no family's port.
func TestTheBridgeHandsTheFamilysRefusalThrough(t *testing.T) {
	scope := ScopeFromKernel(nil, t.TempDir())
	if scope.Filled() {
		t.Fatal("the bridge filled a scope with no daemon")
	}
	if scope.JournalPorts.Events != nil {
		t.Error("the bridge filled a family's port with no daemon")
	}
}
