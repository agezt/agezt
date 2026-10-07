// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

type readAuth struct{ kind opapi.PrincipalKind }

func (a readAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "acme"}, nil
}

type readRouter struct{}

func (readRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type readForbiddenAudit struct{ calls int }

func (a *readForbiddenAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return nil, errors.New("reads must not audit")
}
func readDispatcher(t *testing.T, p *readProbe, a *readForbiddenAudit, kind opapi.PrincipalKind, factory *int, seen *context.Context) (*app.Dispatcher, []app.Operation) {
	t.Helper()
	ops, err := ReadOperations(func(ctx context.Context) *Reads { *factory++; *seen = ctx; return NewReads(p) })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: readAuth{kind}, Router: readRouter{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops
}
func TestReadOperationsTypedMetadataNoAuditContextAndAdmission(t *testing.T) {
	if _, err := ReadOperations(nil); err == nil {
		t.Fatal("nil factory")
	}
	p := &readProbe{listings: []core.Listing{{MarketplaceEntry: core.MarketplaceEntry{Name: "owned"}}}, pack: core.Pack{Name: "owned"}, sources: []core.Source{{Name: "owned"}}}
	a := &readForbiddenAudit{}
	f := 0
	var seen context.Context
	d, ops := readDispatcher(t, p, a, opapi.Operator, &f, &seen)
	inputs := []reflect.Type{reflect.TypeFor[ListRequest](), reflect.TypeFor[ShowRequest](), reflect.TypeFor[SourcesInput]()}
	outputs := []reflect.Type{reflect.TypeFor[ListOutput](), reflect.TypeFor[ShowOutput](), reflect.TypeFor[SourcesOutput]()}
	names := []string{"market_list", "market_show", "market_sources"}
	if len(ops) != 3 {
		t.Fatal(ops)
	}
	ctx := context.WithValue(context.Background(), struct{ owned bool }{true}, "owned")
	for i, op := range ops {
		sp := op.Spec()
		if sp.Name != names[i] || !sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != inputs[i] || sp.Output != outputs[i] || sp.Emission != nil || len(sp.EmissionSchema) != 0 {
			t.Fatal(sp)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, json.RawMessage(`{"query":" raw-query ","name":" owned ","marketplace":" raw-market ","unknown":true,"tenant":"spoof"}`), nil)
		raw, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(sp.OutputSchema, raw) != nil || f != i+1 || seen != ctx || a.calls != 0 {
			t.Fatal(out, err, f, a)
		}
		if i == 0 && p.query != "raw-query" || i == 1 && (p.name != "owned" || p.marketplace != "raw-market") {
			t.Fatal(p)
		}
	}
	if !reflect.DeepEqual(p.calls, []string{"list", "show", "sources"}) {
		t.Fatal(p)
	}
	for _, name := range names {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := d.Dispatch(ctx, opapi.Caller{}, name, json.RawMessage(`{"name":"owned"}`), nil); err != context.Canceled {
			t.Fatal(name, err)
		}
		tenant, _ := readDispatcher(t, p, a, opapi.Tenant, &f, &seen)
		if _, err := tenant.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"name":"owned"}`), nil); err == nil {
			t.Fatal("tenant admitted", name)
		}
	}
	if f != 3 || len(p.calls) != 3 || a.calls != 0 {
		t.Fatal(f, p, a)
	}
}
func TestReadOperationsPermissiveTextCodecsAndAvailabilityPrecedence(t *testing.T) {
	for _, value := range []string{`null`, `false`, `1`, `[]`, `{}`, `""`, `" \t "`, `" owned "`} {
		p := &readProbe{pack: core.Pack{Name: "owned"}}
		a := &readForbiddenAudit{}
		f := 0
		var seen context.Context
		d, _ := readDispatcher(t, p, a, opapi.Operator, &f, &seen)
		raw := json.RawMessage(`{"name":` + value + `,"marketplace":` + value + `,"query":` + value + `,"unknown":true}`)
		for _, name := range []string{"market_list", "market_show", "market_sources"} {
			out, err := d.Dispatch(context.Background(), opapi.Caller{}, name, raw, nil)
			if name == "market_show" && value != `" owned "` {
				if err == nil || err.Error() != "args.name required" {
					t.Fatal(value, out, err)
				}
			} else if err != nil {
				t.Fatal(value, name, err)
			}
		}
		want := ""
		if value == `" owned "` {
			want = "owned"
		}
		if p.query != want || p.marketplace != want || f != 3 || a.calls != 0 {
			t.Fatal(value, p, f, a)
		}
		ops, err := ReadOperations(func(context.Context) *Reads { return NewReads(nil) })
		if err != nil {
			t.Fatal(err)
		}
		unavailable, _ := app.NewDispatcher(ops, app.Dependencies{Auth: readAuth{opapi.Operator}, Router: readRouter{}, Audit: a})
		for _, name := range []string{"market_list", "market_show", "market_sources"} {
			_, err := unavailable.Dispatch(context.Background(), opapi.Caller{}, name, raw, nil)
			if err == nil || err.Error() != "marketplace not available on this daemon" {
				t.Fatal(name, err)
			}
		}
	}
	p := &readProbe{}
	a := &readForbiddenAudit{}
	f := 0
	var seen context.Context
	d, _ := readDispatcher(t, p, a, opapi.Operator, &f, &seen)
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "market_show", json.RawMessage(`{}`), nil); err == nil || err.Error() != "args.name required" || len(p.calls) != 0 {
		t.Fatal(err, p)
	}
}
func readJSON(t *testing.T, value any) map[string]any {
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
func TestReadOperationsRequiredRootsAndTypedNestedSchemas(t *testing.T) {
	ops, err := ReadOperations(func(context.Context) *Reads { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		sp := op.Spec()
		var decl map[string]any
		json.Unmarshal(sp.OutputSchema, &decl)
		props := decl["properties"].(map[string]any)
		required := decl["required"].([]any)
		var out any
		var keys []string
		switch sp.Name {
		case "market_list":
			out = ListOutput{Packs: []core.Listing{{}}}
			keys = []string{"packs", "count"}
		case "market_sources":
			out = SourcesOutput{Sources: []core.Source{{}}}
			keys = []string{"sources", "count"}
		case "market_show":
			out = ShowOutput{Skills: []SkillView{{SkillMD: "owned"}}}
			keys = []string{"pack", "skill_count", "mcp_count", "tool_count", "skills", "mcp_servers", "tools", "installed", "installed_at", "vet"}
		}
		if len(props) != len(keys) || len(required) != len(keys) {
			t.Fatal(sp.Name, decl)
		}
		for _, key := range keys {
			found := false
			for _, actual := range required {
				if actual == key {
					found = true
				}
			}
			if !found {
				t.Fatal(sp.Name, key)
			}
		}
		value := readJSON(t, out)
		raw, _ := json.Marshal(value)
		if schema.ValidateJSON(sp.OutputSchema, raw) != nil {
			t.Fatal(sp.Name, string(raw))
		}
		for _, key := range keys {
			clone := readJSON(t, value)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
				t.Fatal("missing root accepted", sp.Name, key)
			}
		}
		var rowDecl map[string]any
		var row map[string]any
		var mandatory []string
		switch sp.Name {
		case "market_list":
			rowDecl = props["packs"].(map[string]any)["items"].(map[string]any)
			row = value["packs"].([]any)[0].(map[string]any)
			mandatory = []string{"name", "version", "skill_count", "mcp_count", "tool_count", "marketplace", "builtin", "installed"}
		case "market_sources":
			rowDecl = props["sources"].(map[string]any)["items"].(map[string]any)
			row = value["sources"].([]any)[0].(map[string]any)
			mandatory = []string{"name", "url"}
		case "market_show":
			rowDecl = props["skills"].(map[string]any)["items"].(map[string]any)
			row = value["skills"].([]any)[0].(map[string]any)
			mandatory = []string{"skill_md"}
			packDecl := props["pack"].(map[string]any)
			if len(packDecl["properties"].(map[string]any)) != 11 || len(packDecl["required"].([]any)) != 2 {
				t.Fatal(packDecl)
			}
		}
		wantProps := 3
		if sp.Name == "market_list" {
			wantProps = 17
		} else if sp.Name == "market_sources" {
			wantProps = 4
		}
		if len(rowDecl["properties"].(map[string]any)) != wantProps || len(rowDecl["required"].([]any)) != len(mandatory) {
			t.Fatal(sp.Name, rowDecl)
		}
		for _, key := range mandatory {
			clone := readJSON(t, row)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			rowRaw, _ := json.Marshal(rowDecl)
			if schema.ValidateJSON(rowRaw, raw) == nil {
				t.Fatal("missing nested field accepted", sp.Name, key)
			}
		}
	}
}
func TestReadOperationsSkillSummaryPresenceAndByteResources(t *testing.T) {
	ops, err := ReadOperations(func(context.Context) *Reads { return nil })
	if err != nil {
		t.Fatal(err)
	}
	name, empty := "owned", ""
	for _, tc := range []ShowOutput{{Skills: []SkillView{{SkillMD: "invalid"}}}, {Skills: []SkillView{{Name: &name, Description: &empty, SkillMD: "valid"}}}, {Pack: core.Pack{Skills: []core.PackSkill{{SkillMD: "owned", Resources: map[string][]byte{"bytes": []byte("owned"), "nil": nil, "empty": {}}}}}, Skills: []SkillView{}, MCPServers: []string{}, Tools: []string{}}} {
		raw, _ := json.Marshal(tc)
		if err := schema.ValidateJSON(ops[1].Spec().OutputSchema, raw); err != nil {
			t.Fatal(string(raw), err)
		}
	}
}
