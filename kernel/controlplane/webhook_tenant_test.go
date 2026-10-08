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

func TestWebhookNativeSelectedTenantJournal(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New())
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
		if _, err := k.Bus().Publish(event.Spec{Kind: event.KindWebhookDelivered, Actor: "webhook", Subject: "owned", Payload: map[string]any{"url": id + "-private", "status": 200}}); err != nil {
			t.Fatal(err)
		}
	}
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdWebhookLog, controlplane.CmdWebhookStats} {
		for _, read := range []struct {
			client *controlplane.Client
			args   map[string]any
			want   string
		}{{owner, nil, "primary-private"}, {owner, map[string]any{"tenant": "acme"}, "acme-private"}, {client, map[string]any{"tenant": "acme"}, "acme-private"}} {
			out, err := read.client.Call(context.Background(), cmd, read.args)
			if err != nil {
				t.Fatal(cmd, err)
			}
			wire, _ := json.Marshal(out)
			if !strings.Contains(string(wire), read.want) {
				t.Fatal(cmd, string(wire), read.want)
			}
			for _, id := range []string{"primary-private", "acme-private", "other-private"} {
				if id != read.want && strings.Contains(string(wire), id) {
					t.Fatal("cross-tenant read", cmd, string(wire))
				}
			}
		}
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "other"}); err == nil {
			t.Fatal("foreign tenant read", cmd)
		}
	}
}
