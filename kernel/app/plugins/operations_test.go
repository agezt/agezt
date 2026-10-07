// SPDX-License-Identifier: MIT
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
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

type forbiddenInventoryAudit struct{ calls int }

func (a *forbiddenInventoryAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return nil, errors.New("inventory must not audit")
}
func TestInventoryTypedOperationMetadataContextNoAuditAndAdmission(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil factory")
	}
	p := &inventoryProbe{rows: []Registration{{Prefix: "owned", Path: "never-executed", AllowedTools: []string{}}}}
	f := 0
	var seen context.Context
	ops, err := Operations(func(ctx context.Context) *Service { f++; seen = ctx; return New(p) })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	sp := ops[0].Spec()
	if sp.Name != "plugin_list" || !sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != reflect.TypeFor[ListInput]() || sp.Output != reflect.TypeFor[ListOutput]() || sp.Emission != nil || len(sp.EmissionSchema) != 0 {
		t.Fatal(sp)
	}
	a := &forbiddenInventoryAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Operator}, Router: inventoryRoute{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	type callerKey struct{}
	ctx := context.WithValue(context.Background(), callerKey{}, "owned")
	for _, raw := range []string{`{}`, `{"unknown":true,"tenant":"spoof","query":false,"prefix":null}`} {
		out, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, json.RawMessage(raw), nil)
		encoded, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(sp.OutputSchema, encoded) != nil || seen != ctx {
			t.Fatal(out, err)
		}
	}
	if f != 2 || p.calls != 2 || a.calls != 0 {
		t.Fatal(f, p, a)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, json.RawMessage(`{}`), nil); err != context.Canceled {
		t.Fatal(err)
	}
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{opapi.Tenant}, Router: inventoryRoute{}, Audit: a})
	if _, err := tenant.Dispatch(context.Background(), opapi.Caller{}, sp.Name, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant admitted")
	}
	if f != 2 || p.calls != 2 || a.calls != 0 {
		t.Fatal(f, p, a)
	}
}
func inventoryJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestInventoryTypedSchemaRequiresTwoRootsAndSixRowFields(t *testing.T) {
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	sp := ops[0].Spec()
	var decl map[string]any
	json.Unmarshal(sp.OutputSchema, &decl)
	props := decl["properties"].(map[string]any)
	if len(props) != 2 || len(decl["required"].([]any)) != 2 {
		t.Fatal(decl)
	}
	rowDecl := props["plugins"].(map[string]any)["items"].(map[string]any)
	if len(rowDecl["properties"].(map[string]any)) != 6 || len(rowDecl["required"].([]any)) != 6 {
		t.Fatal(rowDecl)
	}
	value := inventoryJSON(t, ListOutput{Plugins: []Row{{}}})
	raw, _ := json.Marshal(value)
	if schema.ValidateJSON(sp.OutputSchema, raw) != nil {
		t.Fatal(string(raw))
	}
	for _, key := range []string{"plugins", "count"} {
		copy := inventoryJSON(t, value)
		delete(copy, key)
		raw, _ := json.Marshal(copy)
		if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
			t.Fatal("missing root", key)
		}
	}
	row := value["plugins"].([]any)[0].(map[string]any)
	rowSchema, _ := json.Marshal(rowDecl)
	for _, key := range []string{"prefix", "path", "args", "tool_count", "hash_pinned", "allowed_tools"} {
		copy := inventoryJSON(t, row)
		delete(copy, key)
		raw, _ := json.Marshal(copy)
		if schema.ValidateJSON(rowSchema, raw) == nil {
			t.Fatal("missing row field", key)
		}
	}
	for _, allowed := range [][]string{nil, {}, {"owned"}} {
		raw, _ := json.Marshal(ListOutput{Plugins: []Row{{Args: []string{}, AllowedTools: allowed}}})
		if schema.ValidateJSON(sp.OutputSchema, raw) != nil {
			t.Fatal(string(raw))
		}
	}
}
