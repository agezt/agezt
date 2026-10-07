// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

type inventoryAuth struct{ kind opapi.PrincipalKind }

func (a inventoryAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "acme"}, nil
}

type inventoryRoute struct{}

func (inventoryRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type inventoryForbiddenAudit struct{ calls int }

func (a *inventoryForbiddenAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return nil, errors.New("inventory must not audit")
}
func TestInventoryTypedOperationMetadataSchemaAndNoAudit(t *testing.T) {
	if _, err := InventoryOperations(nil); err == nil {
		t.Fatal("missing provider admitted")
	}
	port := &inventoryReader{registered: map[string]toolapi.Tool{"file": &inventoryTool{def: toolapi.ToolDef{Name: "advertised"}}}}
	providers := 0
	ops, err := InventoryOperations(func(context.Context) *Inventory { providers++; return NewInventory(port) })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "tool_list" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input != reflect.TypeFor[InventoryInput]() || spec.Output != reflect.TypeFor[InventoryOutput]() || !spec.AllowUnknownInput {
		t.Fatal(spec)
	}
	audit := &inventoryForbiddenAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Operator}, Router: inventoryRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `{"unknown":true,"tenant":"spoof"}`} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(raw), nil)
		encoded, _ := json.Marshal(out)
		if err != nil {
			t.Fatal(raw, err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, encoded); err != nil {
			t.Fatal(string(encoded), err)
		}
	}
	if providers != 2 || port.reads != 2 || audit.calls != 0 {
		t.Fatal(providers, port.reads, audit.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, spec.Name, json.RawMessage(`{}`), nil); err != context.Canceled {
		t.Fatal(err)
	}
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Tenant}, Router: inventoryRoute{}, Audit: audit})
	if _, err := tenant.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, spec.Name, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant admitted")
	}
	if providers != 2 || port.reads != 2 || audit.calls != 0 {
		t.Fatal(providers, port.reads, audit.calls)
	}
}
func TestInventoryTypedSchemaRequiresSixRowFieldsAndCount(t *testing.T) {
	ops, err := InventoryOperations(func(context.Context) *Inventory { return NewInventory(&inventoryReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	declared := ops[0].Spec().OutputSchema
	raw, _ := json.Marshal(InventoryOutput{Tools: []InventoryItem{{}}, Count: 1})
	if err := schema.ValidateJSON(declared, raw); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	row := body["tools"].([]any)[0].(map[string]any)
	for _, field := range []string{"name", "description", "capability", "effect_class", "rollback_mode", "rollback_notes"} {
		value, ok := row[field]
		if !ok || value != "" {
			t.Fatal(field, row)
		}
		delete(row, field)
		encoded, _ := json.Marshal(body)
		if err := schema.ValidateJSON(declared, encoded); err == nil {
			t.Fatal("missing required field admitted", field)
		}
		row[field] = value
	}
	for _, field := range []string{"tools", "count"} {
		value := body[field]
		delete(body, field)
		encoded, _ := json.Marshal(body)
		if err := schema.ValidateJSON(declared, encoded); err == nil {
			t.Fatal("missing required root field admitted", field)
		}
		body[field] = value
	}
}
