// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestPluginInventoryNativeTenantDenialPreservesManifestAndJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPairWithConfig(t, runtime.Config{Provider: p, Plugins: []runtime.PluginInfo{{Prefix: "owned", Path: "never-executed", Args: []string{" raw "}, AllowedTools: []string{"owned"}, ToolCount: 1, HashPinned: true}}})
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	before := append([]runtime.PluginInfo(nil), k.Plugins()...)
	if _, err := client.Call(context.Background(), controlplane.CmdPluginList, map[string]any{"tenant": "acme", "unknown": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || !reflect.DeepEqual(before, k.Plugins()) || p.CallCount() != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}
