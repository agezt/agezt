// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/toolbox"
	"reflect"
	"testing"
	"time"
)

func TestToolboxReadTypedPoliciesContextAndNoAudit(t *testing.T) {
	if _, err := ToolboxReadOperations(nil); err == nil {
		t.Fatal("missing provider")
	}
	p := &toolboxReadProbe{inventory: toolbox.Inventory{OS: "owned", Managers: []string{}, Tools: []toolbox.ToolStatus{{}}, MissingCount: 1}, outdated: map[string]bool{" raw ": false}}
	factories := 0
	var factoryCtx context.Context
	ops, err := ToolboxReadOperations(func(ctx context.Context) *ToolboxReads { factories++; factoryCtx = ctx; return NewToolboxReads(p) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	audit := &inventoryForbiddenAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Operator}, Router: inventoryRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i, op := range ops {
		spec := op.Spec()
		input, output := reflect.TypeFor[ToolboxDetectInput](), reflect.TypeFor[toolbox.Inventory]()
		name := "toolbox_detect"
		if i == 1 {
			input, output = reflect.TypeFor[ToolboxOutdatedInput](), reflect.TypeFor[ToolboxOutdatedOutput]()
			name = "toolbox_outdated"
		}
		if spec.Name != name || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != input || spec.Output != output {
			t.Fatal(spec)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, name, json.RawMessage(`{"unknown":true,"tenant":"spoof","names":false}`), nil)
		raw, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(spec.OutputSchema, raw) != nil || factoryCtx != ctx || factories != i+1 {
			t.Fatal(spec, out, err, factories, factoryCtx)
		}
	}
	if p.detectCtx != ctx || p.outdatedCtx != ctx || p.detectCalls != 1 || p.outdatedCalls != 1 || audit.calls != 0 {
		t.Fatal(p, audit.calls)
	}
	stopped, cancelStopped := context.WithCancel(context.Background())
	cancelStopped()
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Tenant}, Router: inventoryRoute{}, Audit: audit})
	for _, op := range ops {
		if _, err := d.Dispatch(stopped, opapi.Caller{}, op.Spec().Name, json.RawMessage(`{}`), nil); err != context.Canceled {
			t.Fatal(err)
		}
		if _, err := tenant.Dispatch(context.Background(), opapi.Caller{}, op.Spec().Name, json.RawMessage(`{}`), nil); err == nil {
			t.Fatal("tenant admitted")
		}
	}
	if factories != 2 || p.detectCalls != 1 || p.outdatedCalls != 1 || audit.calls != 0 {
		t.Fatal(factories, p, audit.calls)
	}
}
func TestToolboxReadSchemasRequireInventoryRowAndOutdatedRoots(t *testing.T) {
	ops, err := ToolboxReadOperations(func(context.Context) *ToolboxReads { return NewToolboxReads(&toolboxReadProbe{}) })
	if err != nil {
		t.Fatal(err)
	}
	inventory := forgeJSON(t, toolbox.Inventory{Tools: []toolbox.ToolStatus{{}}})
	outdated := forgeJSON(t, ToolboxOutdatedOutput{Outdated: []string{}})
	for i, op := range ops {
		value := inventory
		if i == 1 {
			value = outdated
		}
		raw, _ := json.Marshal(value)
		if err := schema.ValidateJSON(op.Spec().OutputSchema, raw); err != nil {
			t.Fatal(string(raw), err)
		}
		for key := range value {
			clone := forgeJSON(t, value)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if err := schema.ValidateJSON(op.Spec().OutputSchema, raw); err == nil {
				t.Fatal("missing required root accepted", op.Spec().Name, key)
			}
		}
	}
	var declaration map[string]any
	_ = json.Unmarshal(ops[0].Spec().OutputSchema, &declaration)
	props := declaration["properties"].(map[string]any)
	if len(props) != 5 || len(declaration["required"].([]any)) != 5 {
		t.Fatal(declaration)
	}
	rowSchema := props["tools"].(map[string]any)["items"].(map[string]any)
	rowProps := rowSchema["properties"].(map[string]any)
	if len(rowProps) != 9 || len(rowSchema["required"].([]any)) != 5 {
		t.Fatal(rowSchema)
	}
	for _, field := range []string{"name", "category", "description", "installed", "installable"} {
		clone := forgeJSON(t, inventory)
		row := clone["tools"].([]any)[0].(map[string]any)
		delete(row, field)
		raw, _ := json.Marshal(clone)
		if err := schema.ValidateJSON(ops[0].Spec().OutputSchema, raw); err == nil {
			t.Fatal("missing required row accepted", field)
		}
	}
	for _, value := range []toolbox.Inventory{{}, {OS: "owned", Managers: []string{}, Tools: []toolbox.ToolStatus{{Version: " ", Path: "raw", Manager: "manager", Command: "command"}}}} {
		raw, _ := json.Marshal(value)
		if err := schema.ValidateJSON(ops[0].Spec().OutputSchema, raw); err != nil {
			t.Fatal(string(raw), err)
		}
	}
}
