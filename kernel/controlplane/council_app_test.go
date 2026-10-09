// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestCouncilMembershipBindsTheKernel: the membership edit persists under the
// server's directory, applies to the primary kernel's panel, and the catalog
// decides which models are unknown.
func TestCouncilMembershipBindsTheKernel(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{"mock": {ID: "mock", Models: map[string]*catalog.Model{"known": {ID: "known"}}}}}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	serverDir := t.TempDir()
	s := NewServer(k, serverDir)
	s.token = "primary"
	call := func(cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "c", Cmd: cmd, Token: "primary", Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, resp.Error)
		}
		raw, _ := json.Marshal(resp.Result)
		return string(raw)
	}
	if got := call(CmdCouncilSet, map[string]any{"members": []any{map[string]any{"seat": "Chair", "model": "known"}, map[string]any{"model": "zz"}}}); got != `{"applied":"live","member_count":2,"saved":true,"unknown_models":["zz"]}` {
		t.Fatal(got)
	}
	members := k.CouncilDefaultMembers()
	if len(members) != 2 || members[0].Seat != "Elder 1" || members[0].Model != "zz" || members[1].Seat != "Chair" || members[1].Model != "known" {
		t.Fatal("applied live to the primary kernel", members)
	}
	store := settings.NewStore(serverDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get("AGEZT_COUNCIL_MEMBERS"); got != "zz,known" {
		t.Fatal("persisted in seat order under the server directory", got)
	}
	if got := call(CmdCouncilMembers, nil); got != `{"count":2,"members":[{"model":"zz","seat":"Elder 1"},{"model":"known","seat":"Chair"}]}` {
		t.Fatal(got)
	}
	for cmd, read := range map[string]bool{CmdCouncilMembers: true, CmdCouncilSet: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
