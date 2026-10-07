// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestMCPCatalogNativeTenantDenialPreservesStoreAndJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	srv, err := k.AddMCPServer("owned-seed", mcp.Server{Name: "owned", Command: "owned-never-spawned", Env: map[string]string{"OWNED": "owned-private-value"}})
	if err != nil {
		t.Fatal(err)
	}
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	if _, err := client.Call(context.Background(), controlplane.CmdMCPList, map[string]any{"tenant": "acme"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	got, found := k.MCPStore().Get(srv.ID)
	if head != after || hash != afterHash || !found || got.Env["OWNED"] != "owned-private-value" || len(k.MCPAttached()) != 0 || p.CallCount() != 0 {
		t.Fatal(head, after, found, p.CallCount())
	}
}
