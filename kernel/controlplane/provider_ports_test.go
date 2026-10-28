// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the provider family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own provider answers
// for the daemon's kernel, and the request's provider follows the ROUTED
// kernel — a tenant's routing is the tenant's routing. A provider that
// answered for the wrong kernel would let a tenant edit the daemon's chains,
// and the daemon would route the tenant's calls by them.
//
// The provider is the daemon's live governor, so the two kernels differ by
// their CONTENT (the chains they route by), not by their existence.
func TestTheAdapterBindsTheProviderPortsToItsOwnKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: providerGovernor(t, "daemon-model")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: providerGovernor(t, "tenant-model")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})

	// The daemon's own ports answer for its own kernel.
	if s.providerPorts().Live() == nil {
		t.Fatal("the daemon's provider ports hold no provider — the kernel holds a provider")
	}
	daemonChains, ok := s.providerPorts().TaskChains()
	if !ok {
		t.Fatal("the daemon's provider ports hold no chains")
	}
	if got := daemonChains["plan"]; len(got) != 1 || got[0] != "daemon-model" {
		t.Fatalf("the daemon's provider ports read chains %v, the daemon routes by %v", daemonChains, map[string][]string{"plan": {"daemon-model"}})
	}

	// The request's ports follow the routed kernel.
	if requestProviderPorts(ctx).Live() == nil {
		t.Fatal("the request's provider ports hold no provider — the routed kernel holds a provider")
	}
	tenantChains, ok := requestProviderPorts(ctx).TaskChains()
	if !ok {
		t.Fatal("the request's provider ports hold no chains")
	}
	if got := tenantChains["plan"]; len(got) != 1 || got[0] != "tenant-model" {
		t.Fatalf("the request's provider ports read chains %v, the tenant routes by %v", tenantChains, map[string][]string{"plan": {"tenant-model"}})
	}
}

// providerGovernor opens a live provider that routes by one model.
func providerGovernor(t *testing.T, model string) *governor.Governor {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: mock.New(), AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("register the mock provider: %v", err)
	}
	g, err := governor.New(governor.Config{
		Registry:        reg,
		TaskModelChains: governor.TaskModelChains{"plan": {model}},
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	return g
}
