// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestTasteOperationsSocketRetainPrimaryOnlyStore(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	handle, err := registry.Acquire("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tenant := handle.Kernel.(*runtime.Kernel)
	for label, k := range map[string]*runtime.Kernel{"primary-private": primary, "tenant-private": tenant} {
		if _, err := k.Taste().Create(taste.CreateSpec{Title: label, Body: "body"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdTasteList, controlplane.CmdTasteCreate, controlplane.CmdTasteDelete} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "title": "blocked", "body": "body", "id": "fixture"}); err == nil {
			t.Fatalf("tenant admitted %s", cmd)
		}
	}
	result, err := owner.Call(context.Background(), controlplane.CmdTasteList, map[string]any{"tenant": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), "primary-private") || strings.Contains(string(wire), "tenant-private") || len(primary.Taste().List(taste.Filter{})) != 1 || len(tenant.Taste().List(taste.Filter{})) != 1 {
		t.Fatalf("taste selection/effects: %s", wire)
	}
}
