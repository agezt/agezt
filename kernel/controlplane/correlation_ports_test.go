// SPDX-License-Identifier: MIT
package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the correlation family's ports — the APPLICATION
// fills them from the kernel handle.
//
// The test pins the daemon's binding: its ports reach a live kernel, and every
// mint is its own. A binding that answered for nobody would mint nothing, and
// an operation with no correlation cannot be traced.
//
// The test also says what it CANNOT hold, because a slice that hides that is a
// slice that lies: the correlation is a fresh ULID every time — the kernel
// mints `"run-" + ulid.New()` — so the daemon's mint and a tenant's mint are
// **indistinguishable by their value**. A binding that answered with the wrong
// kernel would hand out a perfectly good ULID, and no value can tell the two
// apart.
//
// There is no REQUESTED binding either, and the reason is the shape of the
// request path: the router mints the request's correlation BEFORE the
// request's kernel is in the context, and the protocol audit runs on a context
// that never carries one. Both ask the family directly, with the kernel they
// hold.
func TestTheAdapterBindsTheCorrelationPortsToItsOwnKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	// The daemon's own ports mint.
	daemonCorr := s.correlationPorts().Mint()
	if daemonCorr == "" {
		t.Fatal("the daemon's correlation ports mint nothing — the daemon mints a correlation for every operation")
	}

	// And each mint is its own.
	if again := s.correlationPorts().Mint(); again == daemonCorr {
		t.Fatal("the daemon's correlation ports minted the same correlation twice")
	}
}
