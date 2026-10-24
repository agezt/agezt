// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolexec"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type appBootTool struct{ calls atomic.Int32 }

func (*appBootTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
}
func (t *appBootTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	t.calls.Add(1)
	return toolapi.Result{Output: "ok"}, nil
}

func TestOpenAppKernelBindsPrimaryAndTenant(t *testing.T) {
	var unusedFactory atomic.Int32
	tool := &appBootTool{}
	cfg := kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{"probe": tool}, NewToolInvoker: func(toolexec.Dependencies) toolapi.Invoker { unusedFactory.Add(1); return nil }}
	primary, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })
	cfg.BaseDir = t.TempDir()
	cfg.TenantID = "tenant"
	tenant, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tenant.Close() })
	tenant.Edict().SetLevel(edict.CapIntrospect, edict.LevelDeny)
	if res, err := primary.RunTool(context.Background(), "same", "call", "probe", json.RawMessage(`{}`)); err != nil || res.Output != "ok" {
		t.Fatalf("primary result=%+v error=%v", res, err)
	}
	if _, err := tenant.RunTool(context.Background(), "same", "call", "probe", json.RawMessage(`{}`)); err == nil {
		t.Fatal("tenant borrowed primary allow")
	}
	if unusedFactory.Load() != 0 || tool.calls.Load() != 1 {
		t.Fatalf("factory=%d backend=%d", unusedFactory.Load(), tool.calls.Load())
	}
	for name, k := range map[string]*kernelruntime.Kernel{"primary": primary, "tenant": tenant} {
		found := 0
		if err := k.Journal().Range(func(e *event.Event) error {
			if e.Kind != event.KindPolicyDecision || e.CorrelationID != "same" {
				return nil
			}
			found++
			var p struct{ Allow bool }
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return err
			}
			if p.Allow != (name == "primary") {
				t.Errorf("%s policy=%s", name, e.Payload)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Errorf("%s decisions=%d", name, found)
		}
	}
}

// TestOpenAppKernelWiresTheSeatCatalog: the daemon IS the composition root —
// it opens the seat catalog (the file is the daemon's business) and hands the
// kernel the contract's port, so the daemon's seat operations are a real
// catalog and not an unavailable one.
//
// The other end of the rule is the host's: a kernel that was given no catalog
// says "not available" instead of calling into nothing.
func TestOpenAppKernelWiresTheSeatCatalog(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	store := k.Seats()
	if store == nil {
		t.Fatal("the daemon did not wire the seat catalog — the kernel holds no port")
	}
	if !store.Valid("reader") {
		t.Error("the daemon's seat catalog does not know the built-in seats")
	}
	if store.Valid("no-such-seat") {
		t.Error("the daemon's seat catalog accepts a seat that does not exist")
	}
}

// TestOpenAppKernelWiresTheDataLake: the daemon IS the composition root —
// it opens the data lake, seeds its built-in collections and hands the kernel
// the contract's port.
//
// The seeding lives HERE (W3.42): it is not part of the port the host is
// given, and it is the daemon's file that is seeded. A daemon that stopped
// seeding would leave every live box without the built-in collections —
// which is why the assertion is on the composition root's lake.
func TestOpenAppKernelWiresTheDataLake(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	lake := k.DataLake()
	if lake == nil {
		t.Fatal("the daemon did not wire the data lake — the kernel holds no port")
	}
	all := lake.ListCollections()
	if len(all) == 0 {
		t.Fatal("the daemon's data lake carries no built-in collections")
	}
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if names[0] != "bookmarks" {
		t.Fatalf("the daemon did not seed the built-in collections, got %v", names)
	}
}

// TestOpenAppKernelWiresTheReaper: the daemon IS the composition root — it
// builds the agents module's reaper over the kernel's own ports and injects
// it, so the daemon's scan is the module's scan and not an empty report.
func TestOpenAppKernelWiresTheReaper(t *testing.T) {
	k, err := openAppKernel(kernelruntime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Tools: map[string]toolapi.Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	if _, err := k.Roster().Add(roster.Profile{Slug: "dormant"}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UnixMilli()
	rep := k.ReaperScan(future, future)
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "dormant" {
		t.Fatalf("the daemon's reaper did not answer: %+v", rep.DeadAgents)
	}
}
