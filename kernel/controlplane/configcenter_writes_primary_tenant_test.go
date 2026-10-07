// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestConfigCenterWritesNativeTenantDenialPreservesStateJournalAndProvider(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	seed := core.NewConfigEntry("owned", "owned value")
	if err := k.ConfigCenter().Set(seed); err != nil {
		t.Fatal(err)
	}
	want := *seed
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	for _, command := range []string{controlplane.CmdConfigCenterSet, controlplane.CmdConfigCenterDelete, controlplane.CmdConfigCenterSetRating, controlplane.CmdConfigCenterSetAccess} {
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "acme", "key": "owned", "value": "replacement", "rating": "public", "allowed_agents": []any{"other"}}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(command, err)
		}
	}
	after, afterHash := k.Journal().Head()
	got, err := k.ConfigCenter().GetEntry("owned")
	if err != nil || !reflect.DeepEqual(*got, want) || head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("denied writer changed state/journal/provider", err)
	}
}

func TestConfigCenterWritesNativeAuditPairsRedactionAndFailedLookup(t *testing.T) {
	p := mock.New()
	k, _, client, _ := startPair(t, p)
	sub := watchOpAudit(t, k)
	for _, tc := range []struct {
		command string
		args    map[string]any
		failed  bool
	}{
		{controlplane.CmdConfigCenterSet, map[string]any{"key": "owned", "value": "owned-sensitive-middle", "rating": "secret"}, false},
		{controlplane.CmdConfigCenterSetRating, map[string]any{"key": "owned", "rating": "internal"}, false},
		{controlplane.CmdConfigCenterSetAccess, map[string]any{"key": "owned", "allowed_agents": []any{"allowed"}}, false},
		{controlplane.CmdConfigCenterDelete, map[string]any{"key": "owned"}, false},
		{controlplane.CmdConfigCenterDelete, map[string]any{"key": "missing"}, true},
	} {
		seq, _ := k.Journal().Head()
		_, err := client.Call(context.Background(), tc.command, tc.args)
		if (err != nil) != tc.failed {
			t.Fatal(tc.command, err)
		}
		awaitOpAudit(t, sub)
		events := opEvents(t, k, seq)
		terminal := event.KindOpCompleted
		if tc.failed {
			terminal = event.KindOpFailed
		}
		if len(events) != 2 || events[0].Kind != event.KindOpInvoked || events[1].Kind != terminal || events[0].CorrelationID == "" || events[0].CorrelationID != events[1].CorrelationID {
			t.Fatal("incorrect audit pair", tc.command, events)
		}
		for _, e := range events {
			if e.Subject != "op."+tc.command || strings.Contains(string(e.Payload), "sensitive-middle") {
				t.Fatal("audit subject/privacy", e)
			}
		}
		inv := payloadOf(t, events[0])
		args, _ := inv["args"].(map[string]any)
		if inv["op"] != tc.command || inv["caller"] != "operator" || args["key"] != "[redacted]" {
			t.Fatal(inv)
		}
		if _, present := tc.args["value"]; present && args["value"] != "[redacted]" {
			t.Fatal(inv)
		}
	}
	if p.CallCount() != 0 {
		t.Fatal("configcenter writes called provider")
	}
}
