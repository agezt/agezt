// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
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
