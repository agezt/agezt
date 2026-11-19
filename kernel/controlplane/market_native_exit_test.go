// SPDX-License-Identifier: MIT
package controlplane

import (
	appmarket "github.com/agezt/agezt/kernel/app/market"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"reflect"
	"testing"
)

func TestMarketNativeEightTypedOperationsPolicyAndSignatures(t *testing.T) {
	type signature struct {
		input, output reflect.Type
		read          bool
		stream        opapi.Stream
	}
	expected := map[string]signature{
		opapi.MarketList: {reflect.TypeFor[appmarket.ListRequest](), reflect.TypeFor[appmarket.ListOutput](), true, opapi.StreamNone}, opapi.MarketShow: {reflect.TypeFor[appmarket.ShowRequest](), reflect.TypeFor[appmarket.ShowOutput](), true, opapi.StreamNone}, opapi.MarketSources: {reflect.TypeFor[appmarket.SourcesInput](), reflect.TypeFor[appmarket.SourcesOutput](), true, opapi.StreamNone}, opapi.MarketInstall: {reflect.TypeFor[appmarket.InstallRequest](), reflect.TypeFor[appmarket.InstallOutput](), false, opapi.StreamEvents}, opapi.MarketUninstall: {reflect.TypeFor[appmarket.WriteNameRequest](), reflect.TypeFor[appmarket.UninstallOutput](), false, opapi.StreamEvents}, opapi.MarketAddSource: {reflect.TypeFor[appmarket.AddSourceRequest](), reflect.TypeFor[appmarket.AddSourceOutput](), false, opapi.StreamNone}, opapi.MarketRemoveSource: {reflect.TypeFor[appmarket.WriteNameRequest](), reflect.TypeFor[appmarket.RemoveSourceOutput](), false, opapi.StreamNone}, opapi.MarketSync: {reflect.TypeFor[appmarket.WriteNameRequest](), reflect.TypeFor[appmarket.SyncOutput](), false, opapi.StreamNone}}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		sp := op.Spec()
		sig, ok := expected[sp.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[sp.Name]
		if !ok || !wire.AppOwned || wire.ReadOnly != sig.read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamMode(sig.stream) || sp.ReadOnly != sig.read || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != sig.stream || !sp.AllowUnknownInput || sp.Input != sig.input || sp.Output != sig.output {
			t.Fatal(sp, wire)
		}
		if sig.stream == opapi.StreamNone {
			if sp.Emission != nil || len(sp.EmissionSchema) != 0 {
				t.Fatal(sp)
			}
		} else if sp.Emission != reflect.TypeFor[event.Event]() || len(sp.EmissionSchema) == 0 {
			t.Fatal(sp)
		}
		seen[sp.Name]++
	}
	if len(seen) != 8 || len(marketReadOperations) != 3 || len(marketWriteOperations) != 5 {
		t.Fatal(seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal(seen)
		}
	}
}
