// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the world's ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the graph follows the ROUTED kernel —
// a tenant's world is the tenant's — and the daemon's own request answers for
// the daemon's own.
func TestTheAdapterBindsTheWorldPortsToTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(withWorld(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(withWorld(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})

	// The request's ports follow the routed kernel.
	if got := requestWorldPorts(tenantCtx).Graph(); got != tenant.World() {
		t.Fatal("the request's world ports hand over a different graph than the routed kernel holds")
	}

	// The daemon's own request answers for the daemon's own.
	if got := requestWorldPorts(daemonCtx).Graph(); got != root.World() {
		t.Fatal("the daemon's own world ports hand over a different graph than the daemon's kernel holds")
	}
	if requestWorldPorts(tenantCtx).Graph() == requestWorldPorts(daemonCtx).Graph() {
		t.Fatal("the tenant's world is the daemon's — a tenant's operations must run on the tenant's own world")
	}
}

// A daemon that was given no world model holds none, and the port hands over
// nothing rather than inventing one.
func TestTheAdapterBindsTheWorldPortsToAHostWithNoWorld(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	if got := requestWorldPorts(ctx).Graph(); got != nil {
		t.Fatal("a daemon with no world model hands over a world graph")
	}
}
