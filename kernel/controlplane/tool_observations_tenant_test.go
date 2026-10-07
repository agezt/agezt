// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestToolObservationsSocketTenantJournalIsolationAndUnauditedReads(t *testing.T) {
	provider := mock.New()
	primary, server, owner, dir := startPair(t, provider)
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
	heads := map[string]int64{}
	hashes := map[string]string{}
	for id, k := range kernels {
		for _, spec := range []event.Spec{{Kind: event.KindToolInvoked, Payload: map[string]any{"call_id": "shared", "input": map[string]any{"private": id}}}, {Kind: event.KindToolResult, Payload: map[string]any{"tool": id + "-private", "call_id": "shared", "output": id + "-private", "error": false}}} {
			spec.Subject = "tool"
			spec.Actor = "fixture"
			spec.CorrelationID = id
			if _, err := k.Bus().Publish(spec); err != nil {
				t.Fatal(err)
			}
		}
		heads[id], hashes[id] = k.Journal().Head()
	}
	client := tenantClient(t, dir, token)
	for _, command := range []string{controlplane.CmdToolLog, controlplane.CmdToolStats} {
		for _, read := range []struct {
			client *controlplane.Client
			args   map[string]any
			want   string
		}{{owner, nil, "primary"}, {owner, map[string]any{"tenant": "acme"}, "acme"}, {client, map[string]any{"tenant": "acme"}, "acme"}} {
			result, err := read.client.Call(context.Background(), command, read.args)
			if err != nil {
				t.Fatal(command, err)
			}
			wire, _ := json.Marshal(result)
			if !strings.Contains(string(wire), read.want+"-private") {
				t.Fatal(command, read.want, string(wire))
			}
			for _, id := range []string{"primary", "acme", "other"} {
				if id != read.want && strings.Contains(string(wire), id+"-private") {
					t.Fatal("journal leaked", command, id, string(wire))
				}
			}
		}
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "other"}); err == nil {
			t.Fatal(command, "cross-tenant call admitted")
		}
	}
	for id, k := range kernels {
		head, hash := k.Journal().Head()
		if heads[id] != head || hashes[id] != hash {
			t.Fatal("read changed journal", id, heads[id], head)
		}
	}
	if provider.CallCount() != 0 {
		t.Fatal(provider.CallCount())
	}
}
