// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

type marketReadNativeLibrary struct{}

func (marketReadNativeLibrary) Marketplaces() []market.Marketplace {
	return []market.Marketplace{{Name: "owned", Builtin: true, Packs: []market.MarketplaceEntry{{Name: "owned", Version: "1.0.0", SkillCount: 1}}}}
}
func (marketReadNativeLibrary) ResolvePack(_, name, _ string) (market.Pack, error) {
	return market.Pack{Name: name, Version: "1.0.0", Skills: []market.PackSkill{{SkillMD: "---\nname: owned\ndescription: owned description\n---\nRead the owned fixture.\n"}}}, nil
}
func TestMarketReadNativeUnavailablePrecedesNameAndReadShapesHaveNoEffects(t *testing.T) {
	p := mock.New()
	k, _, client, dir := startPair(t, p)
	for _, cmd := range []string{controlplane.CmdMarketList, controlplane.CmdMarketShow, controlplane.CmdMarketSources} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"name": false, "unknown": true}); err == nil || !strings.Contains(err.Error(), "marketplace not available on this daemon") {
			t.Fatal(cmd, err)
		}
	}
	store := market.NewStore(dir)
	k.SetMarket(market.NewManager(market.Config{Library: marketReadNativeLibrary{}, Store: store}))
	head, hash := k.Journal().Head()
	list, err := client.Call(context.Background(), controlplane.CmdMarketList, map[string]any{"query": " OWNED ", "unknown": true})
	if err != nil || list["count"] != float64(1) || list["packs"].([]any)[0].(map[string]any)["name"] != "owned" {
		t.Fatal(list, err)
	}
	show, err := client.Call(context.Background(), controlplane.CmdMarketShow, map[string]any{"name": " owned "})
	if err != nil || len(show) != 10 || show["installed"] != false || show["installed_at"] != float64(0) || show["tools"] != nil || show["skill_count"] != float64(1) || show["vet"].(map[string]any)["verdict"] != "clean" {
		t.Fatal(show, err)
	}
	sources, err := client.Call(context.Background(), controlplane.CmdMarketSources, map[string]any{"unknown": true})
	if err != nil || sources["count"] != float64(0) || len(sources["sources"].([]any)) != 0 {
		t.Fatal(sources, err)
	}
	after, afterHash := k.Journal().Head()
	installed, _ := store.Installed()
	srcs, _ := store.Sources()
	if head != after || hash != afterHash || len(installed) != 0 || len(srcs) != 0 || p.CallCount() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}
func TestMarketReadNativeThreeTenantDenialsPreserveManagerAndJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	store := market.NewStore(dir)
	k.SetMarket(market.NewManager(market.Config{Library: marketReadNativeLibrary{}, Store: store}))
	if err := store.RecordInstall(market.InstalledPack{Name: "owned", Version: "1.0.0", InstalledMS: 10}); err != nil {
		t.Fatal(err)
	}
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	before, _ := store.Installed()
	for _, cmd := range []string{controlplane.CmdMarketList, controlplane.CmdMarketShow, controlplane.CmdMarketSources} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"name": "owned", "tenant": "acme"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	rows, _ := store.Installed()
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(rows)
	if head != after || hash != afterHash || !reflect.DeepEqual(a, b) || p.CallCount() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}
