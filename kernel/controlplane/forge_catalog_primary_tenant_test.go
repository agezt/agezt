// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestForgeCatalogNativePrimaryReadsAndTenantDenialDoNotMutate(t *testing.T) {
	provider := mock.New()
	k, server, owner, dir := startPair(t, provider)
	st, err := k.DraftScriptTool("owned-seed", toolforge.ScriptTool{Name: "owned", Description: "owned fixture", Language: "python", Code: "owned text, never executed"})
	if err != nil {
		t.Fatal(err)
	}
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	tenant := tenantClient(t, dir, token)
	head, hash := k.Journal().Head()
	ctx := context.Background()
	for _, cmd := range []string{controlplane.CmdToolforgeList, controlplane.CmdToolforgeShow} {
		res, err := owner.Call(ctx, cmd, map[string]any{"ref": " \t" + st.ID + " \n", "tenant": "acme", "ignored": true})
		if err != nil {
			t.Fatal(cmd, err)
		}
		if cmd == controlplane.CmdToolforgeList {
			if res["count"] != float64(1) || res["active_count"] != float64(0) {
				t.Fatal(res)
			}
			rows := res["tools"].([]any)
			if _, present := rows[0].(map[string]any)["code"]; present {
				t.Fatal(res)
			}
		} else {
			if res["tool"].(map[string]any)["code"] != st.Code {
				t.Fatal(res)
			}
		}
		if _, err := tenant.Call(ctx, cmd, map[string]any{"ref": st.Name}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || provider.CallCount() != 0 {
		t.Fatal(head, after, provider.CallCount())
	}
	got, found := k.ToolForge().Get(st.ID)
	if !found || got != st {
		t.Fatal(found, got, st)
	}
}
