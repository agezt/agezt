// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestStandingPrimaryOperationsSocketTenantDenialPreservesStateAndCallback(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	entry, err := k.AddStanding(standing.Order{Name: "primary-private", Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "fixture"}}})
	if err != nil {
		t.Fatal(err)
	}
	before := k.Standing().List()
	fires := 0
	server.SetStandingFire(func(string) bool { fires++; return true })
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdStandingList, controlplane.CmdStandingAdd, controlplane.CmdStandingEdit, controlplane.CmdStandingSetEnabled, controlplane.CmdStandingRemove, controlplane.CmdStandingWhy, controlplane.CmdStandingFire} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "id": entry.ID, "name": "edited", "enabled": false}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	if after := k.Standing().List(); !reflect.DeepEqual(before, after) || fires != 0 || provider.CallCount() != 0 {
		t.Fatal(before, after, fires, provider.CallCount())
	}
}
