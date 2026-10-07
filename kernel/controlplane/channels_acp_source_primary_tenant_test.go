// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/acpcatalog"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type ownedACPSourceTransport struct{ calls *atomic.Int32 }

func (p ownedACPSourceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.calls.Add(1)
	body := `{"version":"1.0.0","agents":[{"id":"owned","name":"Owned","version":"1","description":"Owned missing binary","distribution":{"npx":{"package":"owned"}}}]}`
	if r.URL.Path == "/clients" {
		body = "## Editors\n\n- [Owned Client](https://owned.example/docs)\n"
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func isolateOwnedACPSources(t *testing.T) *atomic.Int32 {
	t.Helper()
	oldRegistry, oldClients, oldCatalog := acpcatalog.DefaultRegistry, acpcatalog.DefaultClients, acpcatalog.Catalog
	t.Cleanup(func() {
		acpcatalog.DefaultRegistry, acpcatalog.DefaultClients, acpcatalog.Catalog = oldRegistry, oldClients, oldCatalog
	})
	t.Setenv("PATH", t.TempDir())
	acpcatalog.Catalog = nil
	var calls atomic.Int32
	client := &http.Client{Transport: ownedACPSourceTransport{calls: &calls}}
	registry := acpcatalog.NewRegistryClient("https://owned.example/registry")
	registry.HTTP = client
	registry.Now = func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }
	clients := acpcatalog.NewClientsClient("https://owned.example/clients")
	clients.HTTP = client
	clients.Now = registry.Now
	acpcatalog.DefaultRegistry, acpcatalog.DefaultClients = registry, clients
	return &calls
}

func TestACPAgentsSourceIsolated(t *testing.T) {
	calls := isolateOwnedACPSources(t)
	// Execute the original source test under owned sources and no installed binaries.
	TestACPAgents_Discovery(t)
	if calls.Load() != 2 {
		t.Fatal("original source discovery did not use both owned sources", calls.Load())
	}
}

func TestChannelACPInventoryNativeTenantDenialNoDiscoveryAuditOrProvider(t *testing.T) {
	discoveryCalls := isolateOwnedACPSources(t)
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	if _, err := client.Call(context.Background(), controlplane.CmdACPAgents, map[string]any{"tenant": "acme", "force": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || discoveryCalls.Load() != 0 || p.CallCount() != 0 {
		t.Fatal("denied ACP inventory changed discovery/journal/provider")
	}
}
