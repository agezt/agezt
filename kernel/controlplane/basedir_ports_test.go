// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the data root's ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end: the daemon's own root answers for the
// daemon's kernel, and the request's root follows the ROUTED kernel — a
// tenant's kernel keeps the tenant's files under the tenant's OWN root, and
// the tenant registry opens one base directory per tenant.
//
// This is the one binding where "the daemon's base dir" would be **wrong**:
// reading the server's own root for a tenant's files would put the tenant's
// journal and the tenant's artifacts under the daemon's directory.
func TestTheAdapterBindsTheBaseDirPortsToItsOwnKernel(t *testing.T) {
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
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})

	// The daemon's own ports answer for its own kernel.
	if got := s.baseDirPorts().Dir(); got != rootDir {
		t.Fatalf("the daemon's data-root ports report %q, the daemon keeps its files under %q", got, rootDir)
	}

	// The request's ports follow the routed kernel.
	if got := requestBaseDirPorts(ctx).Dir(); got != tenantDir {
		t.Fatalf("the request's data-root ports report %q, the tenant keeps its files under %q", got, tenantDir)
	}
	if requestBaseDirPorts(ctx).Dir() == s.baseDirPorts().Dir() {
		t.Fatal("the request's data root is the daemon's — a tenant's files must not land in the daemon's directory")
	}
}
