// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// daemonStubMarket is the daemon's marketplace port, stubbed: the market tool
// is the daemon's, and the stub answers enough for the tool to be real.
type daemonStubMarket struct{}

func (daemonStubMarket) List(string) ([]marketapi.Listing, error) {
	return []marketapi.Listing{{MarketplaceEntry: marketapi.MarketplaceEntry{Name: "web-research", Description: "fetch the web"}}}, nil
}
func (daemonStubMarket) Show(string, string) (marketapi.Pack, marketapi.InstalledPack, bool, error) {
	return marketapi.Pack{Name: "web-research", Version: "1.0.0"}, marketapi.InstalledPack{}, false, nil
}
func (daemonStubMarket) Sources() ([]marketapi.Source, error) { return nil, nil }
func (daemonStubMarket) InstallContext(context.Context, string, string, string, string, func(marketapi.Event) error) (marketapi.InstalledPack, error) {
	return marketapi.InstalledPack{Name: "web-research", Version: "1.0.0", SkillIDs: []string{"s1"}}, nil
}
func (daemonStubMarket) UninstallContext(context.Context, string, string, func(marketapi.Event) error) error {
	return nil
}
func (daemonStubMarket) AddSource(string, string, string) (marketapi.Source, error) {
	return marketapi.Source{}, nil
}
func (daemonStubMarket) RemoveSource(string) (bool, error) { return false, nil }
func (daemonStubMarket) Sync(context.Context, string) ([]marketapi.SyncResult, error) {
	return nil, nil
}

// TestOpenAppKernelWiresTheMarketTool: the market tool is the DAEMON's
// (W3.46) — the kernel is a module and may not name the market module, so the
// composition root builds the tool over the contract's marketplace port and
// hands it over as an ordinary tool.
//
// The daemon end: the tool is live in the kernel's tool set, it resolves the
// KERNEL's port (so a manager wired after Open is still found), and the
// caller's tool map comes back untouched.
func TestOpenAppKernelWiresTheMarketTool(t *testing.T) {
	callerTools := map[string]toolapi.Tool{}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    callerTools,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	// The caller's map must come back UNTOUCHED: the primary kernel and every
	// tenant share one map, and each kernel must see its own tools bound to its
	// own marketplace. A map written in place would hand the last kernel's tool
	// to all of them.
	if _, ok := callerTools["market"]; ok {
		t.Fatal("the daemon wrote into the caller's tool map")
	}

	tools := k.Tools()
	tool, ok := tools["market"]
	if !ok {
		t.Fatal("the daemon did not wire the market tool")
	}

	// Before the manager is wired the tool reports the marketplace as
	// unavailable — it does not panic, and it does not reach for a manager the
	// kernel does not have.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"search"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired marketplace must report itself unavailable, got: %+v", res)
	}

	// After the daemon wires the manager, the SAME tool finds it — the tool
	// resolves the kernel's port lazily, because the manager can only be built
	// once the kernel exists.
	k.SetMarket(daemonStubMarket{})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"search"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the wired marketplace tool errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "web-research") {
		t.Fatalf("the tool did not reach the daemon's marketplace:\n%s", res.Output)
	}
}

// TestTheTenantDoesNotBorrowThePrimarysMarketplace: every kernel gets its OWN
// market tool, bound to its OWN kernel's marketplace port.
//
// The tool map is shared by the primary and every tenant, so a tool written
// in place (or a single tool instance handed to both) would give the tenant
// the primary's manager — and with it the primary's Forge and MCP registry to
// install packs into. A tenant that finds no marketplace must be told so.
func TestTheTenantDoesNotBorrowThePrimarysMarketplace(t *testing.T) {
	cfg := kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}}
	primary, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })
	primary.SetMarket(daemonStubMarket{})

	tenantCfg := cfg
	tenantCfg.BaseDir = t.TempDir()
	tenantCfg.TenantID = "tenant"
	tenant, err := openAppKernel(tenantCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tenant.Close() })

	primaryTool, ok := primary.Tools()["market"]
	if !ok {
		t.Fatal("the primary has no market tool")
	}
	tenantTool, ok := tenant.Tools()["market"]
	if !ok {
		t.Fatal("the tenant has no market tool")
	}
	if primaryTool == tenantTool {
		t.Fatal("the tenant borrowed the primary's market tool")
	}

	res, err := tenantTool.Invoke(context.Background(), json.RawMessage(`{"op":"search"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("the tenant reached the primary's marketplace:\n%s", res.Output)
	}

	res, err = primaryTool.Invoke(context.Background(), json.RawMessage(`{"op":"search"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError || !strings.Contains(res.Output, "web-research") {
		t.Fatalf("the primary lost its own marketplace:\n%s", res.Output)
	}
}
