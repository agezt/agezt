// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestConfigShowNativeTenantDenialPreservesConfigurationAndJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPairWithConfig(t, runtime.Config{Provider: p, Model: "owned-model", System: "owned-private-prompt", Plugins: []runtime.PluginInfo{{Prefix: "owned", Path: "never-executed"}}})
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	model, system := k.Model(), k.System()
	if _, err := client.Call(context.Background(), controlplane.CmdConfig, map[string]any{"tenant": "acme", "unknown": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || k.Model() != model || k.System() != system || len(k.Plugins()) != 1 || p.CallCount() != 0 {
		t.Fatal("denied config read changed state")
	}
}
func TestConfigShowNativeLiveFieldsPresenceOnlyWithoutAudit(t *testing.T) {
	t.Setenv("AGEZT_DISCORD_TOKEN", "")
	p := mock.New()
	k, _, client, _ := startPairWithConfig(t, runtime.Config{Provider: p, Model: "owned-model", System: "owned-private-prompt", Plugins: []runtime.PluginInfo{{Prefix: "owned", Path: "never-executed"}}})
	head, hash := k.Journal().Head()
	for i := 0; i < 2; i++ {
		out, err := client.Call(context.Background(), controlplane.CmdConfig, map[string]any{"unknown": true})
		if err != nil || out["model"] != k.Model() || out["system_prompt_set"] != (k.System() != "") || out["plugin_count"] != float64(1) || out["tool_count"] != float64(len(k.Tools())) {
			t.Fatal(out, err)
		}
		env, ok := out["env"].(map[string]any)
		if !ok || env["AGEZT_DISCORD_TOKEN"] != true {
			t.Fatal(out)
		}
		if _, ok := out["system_prompt"]; ok {
			t.Fatal("prompt content exposed")
		}
		if i == 0 {
			k.SetModel(" raw fresh ")
			k.SetSystem("")
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("config read audited or called provider")
	}
}
