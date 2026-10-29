// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/knowledge/taste"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// withTaste opens the curated exemplar library a daemon would open and hands
// the kernel the contract's store port.
func withTaste(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
	if err != nil {
		t.Fatalf("open the taste store: %v", err)
	}
	cfg.TasteStore = store
	return cfg
}

// The host does not bind the taste's ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the library follows the ROUTED kernel
// — a tenant's exemplars are the tenant's — and the daemon's own request
// answers for the daemon's own.
func TestTheAdapterBindsTheTastePortsToTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(withTaste(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(withTaste(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})

	// The request's ports follow the routed kernel.
	if got := requestTastePorts(tenantCtx).Store(); got == nil {
		t.Fatal("the request's taste ports hand over no store — the tenant holds one")
	} else if got != tenant.Taste() {
		t.Fatal("the request's taste ports hand over a different store than the routed kernel holds")
	}

	// The daemon's own request answers for the daemon's own.
	if got := requestTastePorts(daemonCtx).Store(); got != root.Taste() {
		t.Fatal("the daemon's own taste ports hand over a different store than the daemon's kernel holds")
	}
	if requestTastePorts(tenantCtx).Store() == requestTastePorts(daemonCtx).Store() {
		t.Fatal("the tenant's library is the daemon's — a tenant's operations must run on the tenant's own library")
	}
}

// A daemon that was given no taste library holds none, and the port hands over
// nothing rather than inventing one.
func TestTheAdapterBindsTheTastePortsToAHostWithNoLibrary(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	if got := requestTastePorts(ctx).Store(); got != nil {
		t.Fatal("a daemon with no taste library hands over a taste store")
	}
}
