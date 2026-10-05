// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestProviderObservationsSocketTenantJournalIsolation(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("ok")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	_ = mustTenant(t, registry, "other")
	seed := func(k *runtime.Kernel, identity string) {
		t.Helper()
		for _, spec := range []event.Spec{
			{Kind: event.KindRoutingDecision, Payload: map[string]any{"primary": identity}},
			{Kind: event.KindProviderFallback, Payload: map[string]any{"failed": identity, "next": identity}},
			{Kind: event.KindCapabilityRejected, Payload: map[string]any{"model": identity, "capability": "vision"}},
		} {
			spec.Actor, spec.Subject = "fixture", "provider.fixture"
			if _, err := k.Bus().Publish(spec); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(primary, "primary-private")
	for _, tenant := range []string{"acme", "other"} {
		handle, err := registry.Acquire(tenant, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		seed(handle.Kernel.(*runtime.Kernel), tenant+"-private")
	}
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdProviderLog, controlplane.CmdProviderStats, controlplane.CmdProviderRejections} {
		for _, read := range []struct {
			client *controlplane.Client
			args   map[string]any
			want   string
		}{{owner, nil, "primary-private"}, {owner, map[string]any{"tenant": "acme"}, "acme-private"}, {client, map[string]any{"tenant": "acme"}, "acme-private"}} {
			result, err := read.client.Call(context.Background(), cmd, read.args)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(wire), read.want) {
				t.Fatalf("%s missing own journal %q: %s", cmd, read.want, wire)
			}
			for _, identity := range []string{"primary-private", "acme-private", "other-private"} {
				if identity != read.want && strings.Contains(string(wire), identity) {
					t.Fatalf("%s leaked %q: %s", cmd, identity, wire)
				}
			}
		}
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "other"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatalf("%s cross-tenant read = %v", cmd, err)
		}
	}
}
