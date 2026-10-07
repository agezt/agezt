// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

func TestMCPCatalogTypedOperationPolicySchemaContextNoAuditAndAdmission(t *testing.T) {
	if _, err := MCPCatalogOperations(nil); err == nil {
		t.Fatal("missing provider")
	}
	store := &mcpRegistrationProbe{rows: []mcp.Server{{Name: "owned", Env: map[string]string{"OWNED": "owned-value"}, Headers: map[string]string{"Owned": "owned-header"}}}}
	host := &mcpAttachmentProbe{snapshots: []map[string]int{{"owned": 0}}}
	factory := 0
	var factoryCtx context.Context
	ops, err := MCPCatalogOperations(func(ctx context.Context) *MCPCatalog { factory++; factoryCtx = ctx; return NewMCPCatalog(store, host) })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "mcp_list" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[MCPListInput]() || spec.Output != reflect.TypeFor[MCPListOutput]() {
		t.Fatal(spec)
	}
	audit := &inventoryForbiddenAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Operator}, Router: inventoryRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), struct{ owned bool }{true}, "owned")
	for _, raw := range []string{`{}`, `{"unknown":true,"tenant":"spoof","server":null}`} {
		out, err := d.Dispatch(ctx, opapi.Caller{}, spec.Name, json.RawMessage(raw), nil)
		encoded, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(spec.OutputSchema, encoded) != nil || factoryCtx != ctx {
			t.Fatal(out, err)
		}
		value := out.(MCPListOutput)
		if value.Servers[0].ToolCount == nil || *value.Servers[0].ToolCount != 0 {
			t.Fatal(value)
		}
	}
	if factory != 2 || store.calls != 2 || host.calls != 4 || audit.calls != 0 {
		t.Fatal(factory, store.calls, host.calls, audit.calls)
	}
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(stopped, opapi.Caller{}, spec.Name, json.RawMessage(`{}`), nil); err != context.Canceled {
		t.Fatal(err)
	}
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Tenant}, Router: inventoryRoute{}, Audit: audit})
	if _, err := tenant.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant admitted")
	}
	if factory != 2 || store.calls != 2 || host.calls != 4 || audit.calls != 0 {
		t.Fatal(factory, store.calls, host.calls, audit.calls)
	}
}
func TestMCPCatalogTypedSchemaRequiresSevenPublicFieldsAndThreeRoots(t *testing.T) {
	ops, err := MCPCatalogOperations(func(context.Context) *MCPCatalog { return NewMCPCatalog(nil, nil) })
	if err != nil {
		t.Fatal(err)
	}
	decl := ops[0].Spec().OutputSchema
	var root map[string]any
	_ = json.Unmarshal(decl, &root)
	props := root["properties"].(map[string]any)
	if len(root["required"].([]any)) != 3 || len(props) != 3 {
		t.Fatal(root)
	}
	rowSchema := props["servers"].(map[string]any)["items"].(map[string]any)
	rowProps := rowSchema["properties"].(map[string]any)
	if len(rowProps) != 16 || len(rowSchema["required"].([]any)) != 7 {
		t.Fatal(rowSchema)
	}
	for _, key := range []string{"env", "headers"} {
		if _, ok := rowProps[key]; ok {
			t.Fatal("private field declared", key)
		}
	}
	value := forgeJSON(t, MCPListOutput{Servers: []MCPServerView{{}}})
	raw, _ := json.Marshal(value)
	if err := schema.ValidateJSON(decl, raw); err != nil {
		t.Fatal(err)
	}
	for key := range value {
		clone := forgeJSON(t, value)
		delete(clone, key)
		raw, _ := json.Marshal(clone)
		if err := schema.ValidateJSON(decl, raw); err == nil {
			t.Fatal("missing root accepted", key)
		}
	}
	for _, key := range []string{"id", "name", "enabled", "created_ms", "updated_ms", "transport", "attached"} {
		clone := forgeJSON(t, value)
		row := clone["servers"].([]any)[0].(map[string]any)
		delete(row, key)
		raw, _ := json.Marshal(clone)
		if err := schema.ValidateJSON(decl, raw); err == nil {
			t.Fatal("missing row field accepted", key)
		}
	}
	for _, count := range []int{0, -2, 3} {
		view := NewMCPCatalog(nil, &mcpAttachmentProbe{snapshots: []map[string]int{{"owned": count}}}).View(mcp.Server{Name: "owned"})
		raw, _ := json.Marshal(MCPListOutput{Servers: []MCPServerView{view}})
		if err := schema.ValidateJSON(decl, raw); err != nil {
			t.Fatal(string(raw), err)
		}
	}
}
