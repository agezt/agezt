// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWorldOperationsSocketTenantJournalIsolation(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	_ = mustTenant(t, registry, "other")
	kernels := map[string]*runtime.Kernel{"primary": primary}
	for _, id := range []string{"acme", "other"} {
		handle, err := registry.Acquire(id, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		kernels[id] = handle.Kernel.(*runtime.Kernel)
	}
	for id, k := range kernels {
		if _, _, err := k.World().Upsert("", worldmodel.UpsertSpec{Name: id + "-private"}); err != nil {
			t.Fatal(err)
		}
	}
	client := tenantClient(t, dir, token)
	for _, read := range []struct {
		client *controlplane.Client
		args   map[string]any
		want   string
	}{{owner, nil, "primary-private"}, {owner, map[string]any{"tenant": "acme"}, "acme-private"}, {client, map[string]any{"tenant": "acme"}, "acme-private"}} {
		result, err := read.client.Call(context.Background(), controlplane.CmdWorldLog, read.args)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(wire), read.want) {
			t.Fatalf("world log missing selected reader %s: %s", read.want, wire)
		}
		for _, identity := range []string{"primary-private", "acme-private", "other-private"} {
			if identity != read.want && strings.Contains(string(wire), identity) {
				t.Fatalf("world log leaked %s: %s", identity, wire)
			}
		}
	}
	if _, err := client.Call(context.Background(), controlplane.CmdWorldLog, map[string]any{"tenant": "other"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatalf("foreign world log=%v", err)
	}
	for _, command := range []string{controlplane.CmdWorldAdd, controlplane.CmdWorldEdit, controlplane.CmdWorldRelate, controlplane.CmdWorldForget, controlplane.CmdWorldGet, controlplane.CmdWorldList, controlplane.CmdWorldResolve, controlplane.CmdWorldNeighbors} {
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "acme", "id": "fixture", "name": "fixture", "query": "fixture", "from": "a", "to": "b"}); err == nil {
			t.Errorf("primary-only %s allowed tenant token", command)
		}
	}
	result, err := owner.Call(context.Background(), controlplane.CmdWorldList, map[string]any{"tenant": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), "primary-private") || strings.Contains(string(wire), "acme-private") {
		t.Fatalf("primary-only list selected tenant store: %s", wire)
	}
}
