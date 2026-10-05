// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestProviderProbeSocketTenantDenialPreventsEndpointRequest(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer endpoint.Close()
	_, server, owner, dir := startPair(t, mock.New())
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	if _, err := owner.Call(context.Background(), controlplane.CmdProviderProbe, map[string]any{"url": endpoint.URL, "tenant": "ignored"}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("primary request count = %d", requests.Load())
	}
	client := tenantClient(t, dir, token)
	if _, err := client.Call(context.Background(), controlplane.CmdProviderProbe, map[string]any{"url": endpoint.URL, "tenant": "acme"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("tenant probe = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("tenant reached endpoint: %d", requests.Load())
	}
}
