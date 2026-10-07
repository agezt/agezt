// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestMarketWriteNativeFiveTenantDenialsPreserveOwnedStoreAndJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	store := market.NewStore(dir)
	k.SetMarket(market.NewManager(market.Config{Library: marketReadNativeLibrary{}, Store: store}))
	if err := store.RecordInstall(market.InstalledPack{Name: "owned", Version: "1.0.0", InstalledMS: 10}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddSource(market.Source{Name: "owned", URL: "http://127.0.0.1:1/owned", AddedMS: 10}); err != nil {
		t.Fatal(err)
	}
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	installed, _ := store.Installed()
	sources, _ := store.Sources()
	for _, cmd := range []string{controlplane.CmdMarketInstall, controlplane.CmdMarketUninstall, controlplane.CmdMarketAddSource, controlplane.CmdMarketRemoveSource, controlplane.CmdMarketSync} {
		args := map[string]any{"name": "owned", "url": "http://127.0.0.1:1/owned", "tenant": "acme"}
		var err error
		if cmd == controlplane.CmdMarketInstall || cmd == controlplane.CmdMarketUninstall {
			_, err = client.Stream(context.Background(), cmd, args, nil)
		} else {
			_, err = client.Call(context.Background(), cmd, args)
		}
		if err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	newInstalled, _ := store.Installed()
	newSources, _ := store.Sources()
	if head != after || hash != afterHash || !reflect.DeepEqual(installed, newInstalled) || !reflect.DeepEqual(sources, newSources) || p.CallCount() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}

func TestMarketWriteNativeUnavailableBeforeRequiredArguments(t *testing.T) {
	p := mock.New()
	k, _, client, _ := startPair(t, p)
	head, hash := k.Journal().Head()
	for _, cmd := range []string{controlplane.CmdMarketInstall, controlplane.CmdMarketUninstall, controlplane.CmdMarketAddSource, controlplane.CmdMarketRemoveSource, controlplane.CmdMarketSync} {
		var err error
		args := map[string]any{"name": false, "url": false, "unknown": true}
		if cmd == controlplane.CmdMarketInstall || cmd == controlplane.CmdMarketUninstall {
			_, err = client.Stream(context.Background(), cmd, args, nil)
		} else {
			_, err = client.Call(context.Background(), cmd, args)
		}
		if err == nil || !strings.Contains(err.Error(), "marketplace not available on this daemon") {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if after-head != 10 || afterHash == hash || p.CallCount() != 0 || len(k.MCPAttached()) != 0 {
		t.Fatal("legacy mutation failure audit pairs changed", head, after, p.CallCount())
	}
}
