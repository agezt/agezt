// SPDX-License-Identifier: MIT

package peer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/tenantctx"
)

func mustJSON(v map[string]string) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// TestParseTenantPeers parses a JSON tenant→peerspec map and validates per tenant.
func TestParseTenantPeers(t *testing.T) {
	tp, err := ParseTenantPeers(`{"alpha":"a=http://a:1|tokA","beta":"b1=http://b1:1,b2=https://b2:2"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(tp) != 2 {
		t.Fatalf("want 2 tenants, got %d", len(tp))
	}
	if tp["alpha"]["a"].URL != "http://a:1" || tp["alpha"]["a"].Token != "tokA" {
		t.Errorf("alpha = %+v", tp["alpha"])
	}
	if len(tp["beta"]) != 2 {
		t.Errorf("beta should have 2 peers: %+v", tp["beta"])
	}
	// Empty spec → nil.
	if got, err := ParseTenantPeers("  "); err != nil || got != nil {
		t.Errorf("empty spec = %v, %v", got, err)
	}
	// Invalid JSON → error.
	if _, err := ParseTenantPeers("not-json"); err == nil {
		t.Error("invalid JSON should error")
	}
	// A bad per-tenant peer spec → error naming the tenant.
	if _, err := ParseTenantPeers(`{"x":"n=not-a-url"}`); err == nil || !strings.Contains(err.Error(), "x") {
		t.Errorf("a bad tenant peer spec should error naming the tenant, got %v", err)
	}
}

// toolWithTenants builds a Tool with a global set + per-tenant overrides and a poster
// that records which endpoint each delegation hit.
func toolWithTenants(t *testing.T) (*Tool, *string) {
	t.Helper()
	var endpoint string
	tp := map[string]map[string]Peer{
		"alpha": {"node": {Name: "node", URL: "http://alpha-peer:1"}},
		"beta":  {"node": {Name: "node", URL: "http://beta-peer:1"}},
	}
	tool := &Tool{
		Peers:       map[string]Peer{"node": {Name: "node", URL: "http://global-peer:1"}},
		TenantPeers: tp,
		post:        fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, &endpoint, nil, nil),
	}
	return tool, &endpoint
}

// TestRemoteRun_TenantUsesOwnPeers: a run tagged for tenant alpha dispatches to alpha's
// peer, and a run for beta to beta's — never the other's (the core isolation property).
func TestRemoteRun_TenantUsesOwnPeers(t *testing.T) {
	tool, endpoint := toolWithTenants(t)

	in := mustJSON(map[string]string{"task": "x"})
	if _, err := tool.Invoke(tenantctx.WithTenant(context.Background(), "alpha"), in); err != nil {
		t.Fatal(err)
	}
	if *endpoint != "http://alpha-peer:1/api/v1/runs" {
		t.Errorf("alpha routed to %q, want alpha-peer", *endpoint)
	}

	if _, err := tool.Invoke(tenantctx.WithTenant(context.Background(), "beta"), in); err != nil {
		t.Fatal(err)
	}
	if *endpoint != "http://beta-peer:1/api/v1/runs" {
		t.Errorf("beta routed to %q, want beta-peer", *endpoint)
	}
}

// TestRemoteRun_UnknownTenantFallsBackToGlobal: a tenant WITHOUT an override (or the
// primary, with no tenant tag) uses the GLOBAL peers — never another tenant's. This is
// the leak-safety guarantee: a missing/misattributed tenant degrades to global, not to
// some other tenant's nodes.
func TestRemoteRun_UnknownTenantFallsBackToGlobal(t *testing.T) {
	tool, endpoint := toolWithTenants(t)
	in := mustJSON(map[string]string{"task": "x"})

	// A tenant with no configured override.
	if _, err := tool.Invoke(tenantctx.WithTenant(context.Background(), "gamma"), in); err != nil {
		t.Fatal(err)
	}
	if *endpoint != "http://global-peer:1/api/v1/runs" {
		t.Errorf("unknown tenant should use global, got %q", *endpoint)
	}

	// The primary (no tenant tag at all).
	if _, err := tool.Invoke(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if *endpoint != "http://global-peer:1/api/v1/runs" {
		t.Errorf("primary should use global, got %q", *endpoint)
	}
}

// TestRemoteRun_TenantErrorScopedToTenantPeers: an unknown-peer error names only the
// tenant's own peers, not the global or another tenant's.
func TestRemoteRun_TenantErrorScopedToTenantPeers(t *testing.T) {
	tp := map[string]map[string]Peer{
		"alpha": {"alphaNode": {Name: "alphaNode", URL: "http://a:1"}},
	}
	tool := &Tool{
		Peers:       map[string]Peer{"globalNode": {Name: "globalNode", URL: "http://g:1"}},
		TenantPeers: tp,
		post:        fakePost(200, `{}`, nil, nil, nil),
	}
	// Run as tenant alpha: "globalNode" is NOT in alpha's set → unknown peer, and the
	// error lists alpha's peers (alphaNode), not the global one.
	res, err := tool.Invoke(tenantctx.WithTenant(context.Background(), "alpha"), mustJSON(map[string]string{"peer": "globalNode", "task": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "alphaNode") {
		t.Errorf("tenant error should list the tenant's own peers: %q", res.Output)
	}
}
