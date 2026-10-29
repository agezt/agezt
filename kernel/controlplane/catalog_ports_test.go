// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the catalog's ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the daemon's own catalog answers for
// the daemon's kernel, and the request's catalog follows the ROUTED kernel — a
// tenant's operations run on the tenant's own kernel, and the tenant's catalog
// is the tenant's.
func TestTheAdapterBindsTheCatalogPortsToItsOwnKernel(t *testing.T) {
	rootDir := t.TempDir()
	root, err := runtime.Open(runtime.Config{BaseDir: rootDir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenantDir := t.TempDir()
	tenant, err := runtime.Open(runtime.Config{BaseDir: tenantDir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})
	ctx = context.WithValue(ctx, systemHostKey{}, s)

	// The daemon's own ports answer for its own kernel.
	rootSnapshot := appcatalog.PortsFromKernel(root).Snapshot()
	if got := s.catalogPorts().Snapshot(); got != rootSnapshot {
		t.Fatal("the daemon's catalog ports do not answer for the daemon's own kernel")
	}

	// The request's ports follow the routed kernel.
	tenantSnapshot := appcatalog.PortsFromKernel(tenant).Snapshot()
	if got := requestCatalogPorts(ctx).Snapshot(); got != tenantSnapshot {
		t.Fatal("the request's catalog ports do not answer for the routed kernel")
	}
	if requestCatalogPorts(ctx).Snapshot() == s.catalogPorts().Snapshot() {
		t.Fatal("the request's catalog is the daemon's — a tenant's operations must run on the tenant's own catalog")
	}
}
