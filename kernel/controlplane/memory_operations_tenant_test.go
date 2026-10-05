// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestMemoryOperationsSocketTenantStoreAndJournalIsolation(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	_ = mustTenant(t, registry, "other")
	kernels := map[string]*runtime.Kernel{"primary": primary}
	lowIDs := map[string]string{}
	for _, id := range []string{"acme", "other"} {
		handle, err := registry.Acquire(id, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		kernels[id] = handle.Kernel.(*runtime.Kernel)
	}
	for id, k := range kernels {
		for _, content := range []string{"left fixture fact", "right fixture fact"} {
			if _, _, err := k.Memory().Remember("", memory.RememberSpec{Type: memory.TypeFact, Subject: id + "-private", Content: content, Actor: "operator", Force: true}); err != nil {
				t.Fatal(err)
			}
		}
		rec, _, err := k.Memory().Remember("", memory.RememberSpec{Type: memory.TypeFact, Subject: id + "-private-transient", Content: "x", Tags: map[string]string{"source": "agent"}, Force: true})
		if err != nil {
			t.Fatal(err)
		}
		lowIDs[id] = rec.ID
	}
	client := tenantClient(t, dir, token)
	for _, command := range []string{controlplane.CmdMemoryAudit, controlplane.CmdMemoryLog, controlplane.CmdMemoryClean} {
		for _, read := range []struct {
			client *controlplane.Client
			args   map[string]any
			want   string
		}{{owner, nil, "primary-private"}, {owner, map[string]any{"tenant": "acme"}, "acme-private"}, {client, map[string]any{"tenant": "acme"}, "acme-private"}} {
			result, err := read.client.Call(context.Background(), command, read.args)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(wire), read.want) {
				t.Fatalf("%s missing selected store/journal %s: %s", command, read.want, wire)
			}
			for _, identity := range []string{"primary-private", "acme-private", "other-private"} {
				if identity != read.want && strings.Contains(string(wire), identity) {
					t.Fatalf("%s leaked %s: %s", command, identity, wire)
				}
			}
		}
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "other"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatalf("%s foreign tenant=%v", command, err)
		}
	}
	if _, err := client.Call(context.Background(), controlplane.CmdMemoryList, map[string]any{"tenant": "acme"}); err == nil {
		t.Fatal("primary-only list allowed tenant token")
	}
	beforePrimary, _ := primary.Journal().Head()
	beforeOther, _ := kernels["other"].Journal().Head()
	result, err := client.Call(context.Background(), controlplane.CmdMemoryClean, map[string]any{"tenant": "acme", "dry_run": false})
	if err != nil {
		t.Fatal(err)
	}
	if result["removed"] != float64(1) || result["hard_deleted"] != true {
		t.Fatalf("tenant cleanup=%v", result)
	}
	for id, k := range kernels {
		_, found, err := k.Memory().Get(lowIDs[id])
		if err != nil || found != (id != "acme") {
			t.Errorf("cleanup crossed store id=%s found=%v err=%v", id, found, err)
		}
	}
	afterPrimary, _ := primary.Journal().Head()
	afterOther, _ := kernels["other"].Journal().Head()
	if beforePrimary != afterPrimary || beforeOther != afterOther {
		t.Fatalf("tenant cleanup wrote other journal: primary %d/%d other %d/%d", beforePrimary, afterPrimary, beforeOther, afterOther)
	}
}
