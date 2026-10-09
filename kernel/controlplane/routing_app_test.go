// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRoutingBindsTheKernel drives the four typed operations through the native
// adapter: edits persist to the server's config store and reach the governor,
// the catalog decides which models are unknown, and usage reads the roster.
func TestRoutingBindsTheKernel(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{Name: "mock", Provider: mock.New(mock.FinalText("ok")), AuthMode: governor.AuthLocal}); err != nil {
		t.Fatal(err)
	}
	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{"mock": {ID: "mock", Models: map[string]*catalog.Model{"known": {ID: "known"}}}}}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: gov, Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, err := k.AddProfile(roster.Profile{Slug: "amy", Model: "m", Fallbacks: []string{"@fast"}}); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	s := NewServer(k, base)
	s.token = "primary"
	result := func(cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "r", Cmd: cmd, Token: "primary", Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, resp.Error)
		}
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if got := result(CmdChainsSet, map[string]any{"chains": map[string]any{"fast": []any{"known", "mock/known", "zz"}}, "default": "fast"}); got != `{"applied":"live","chain_count":1,"default":"fast","saved":true,"unknown_models":["zz"]}` {
		t.Fatal(got)
	}
	if got := result(CmdRoutingSet, map[string]any{"chains": map[string]any{"code": []any{"@fast"}}}); got != `{"applied":"live","saved":true,"task_count":1,"unknown_models":["@fast"]}` {
		t.Fatal(got)
	}
	chains, def := gov.FallbackChainsView()
	if def != "fast" || !reflect.DeepEqual(chains, map[string][]string{"fast": {"known", "mock/known", "zz"}}) || !reflect.DeepEqual(gov.TaskModelChainsView(), map[string][]string{"code": {"@fast"}}) {
		t.Fatal("applied live", chains, def, gov.TaskModelChainsView())
	}
	store := settings.NewStore(base)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"AGEZT_FALLBACK_CHAINS": "fast=known,mock/known,zz", "AGEZT_DEFAULT_CHAIN": "fast", "AGEZT_TASK_MODEL_CHAINS": "code=@fast"} {
		if got, _ := store.Get(name); got != want {
			t.Fatalf("%s persisted %q under %s", name, got, filepath.Join(base, settings.FileName))
		}
	}
	if got := result(CmdChainsGet, nil); got != `{"chains":{"fast":["known","mock/known","zz"]},"default":"fast","usage":{"fast":{"agents":["amy"],"default":true,"tasks":["code"]}}}` {
		t.Fatal(got)
	}
	if got := result(CmdRoutingGet, nil); got != `{"activity":{},"chains":{"code":["@fast"]},"task_types":["chat","plan","code","verify","summarize","salience","distill","forge","shadow-eval","delegate"]}` {
		t.Fatal(got)
	}
	for cmd, read := range map[string]bool{CmdRoutingGet: true, CmdRoutingSet: false, CmdChainsGet: true, CmdChainsSet: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}
