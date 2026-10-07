// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"strings"
	"testing"
)

func mcpLifecycleRaw(name string) json.RawMessage {
	if name == "mcp_add" {
		return json.RawMessage(`{"server":{"name":"owned","command":"never-executed","args":[" raw "],"env":{"OWNED":"owned-private-value"}},"unknown":true,"correlation_id":"poison"}`)
	}
	return json.RawMessage(`{"ref":" raw-ref ","enabled":"TRUE","unknown":true,"correlation_id":"poison"}`)
}
func mcpLifecycleDispatcher(t *testing.T, p *mcpLifecyclePort, a *forgeAuditProbe, factory *int, kind opapi.PrincipalKind) (*app.Dispatcher, []app.Operation) {
	t.Helper()
	ops, err := MCPLifecycleOperations(func(ctx context.Context) *MCPLifecycle {
		*factory++
		a.factoryContexts = append(a.factoryContexts, ctx)
		return NewMCPLifecycle(p, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{kind}, Router: inventoryRoute{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops
}
func TestMCPLifecycleTypedMetadataAndMandatoryAudit(t *testing.T) {
	if _, err := MCPLifecycleOperations(nil); err == nil {
		t.Fatal("missing factory")
	}
	p := &mcpLifecyclePort{output: mcp.Server{Name: "owned", CreatedMS: 9007199254740993}, names: []string{}, removed: true}
	a := &forgeAuditProbe{}
	factory := 0
	d, ops := mcpLifecycleDispatcher(t, p, a, &factory, opapi.Operator)
	names := []string{"mcp_add", "mcp_attach", "mcp_detach", "mcp_set_enabled", "mcp_remove"}
	inputs := []reflect.Type{reflect.TypeFor[MCPAddRequest](), reflect.TypeFor[MCPRefRequest](), reflect.TypeFor[MCPRefRequest](), reflect.TypeFor[MCPSetEnabledRequest](), reflect.TypeFor[MCPRefRequest]()}
	outputs := []reflect.Type{reflect.TypeFor[MCPServerOutput](), reflect.TypeFor[MCPAttachOutput](), reflect.TypeFor[MCPDetachOutput](), reflect.TypeFor[MCPServerOutput](), reflect.TypeFor[MCPRemoveOutput]()}
	if len(ops) != 5 {
		t.Fatal(ops)
	}
	ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
	for i, op := range ops {
		sp := op.Spec()
		if sp.Name != names[i] || sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != inputs[i] || sp.Output != outputs[i] || sp.Emission != nil {
			t.Fatal(sp)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, mcpLifecycleRaw(sp.Name), nil)
		raw, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(sp.OutputSchema, raw) != nil || a.begins != i+1 || a.ends != i+1 || a.cause != nil || factory != i+1 || a.records[i].Operation != sp.Name || p.corr != "owned-operation" || a.factoryContexts[i] != ctx || (i > 0 && p.ref != " raw-ref ") {
			t.Fatal(i, out, err, a, p)
		}
		if strings.Contains(string(raw), "owned-private-value") {
			t.Fatal("private output value")
		}
	}
	if p.input.Env["OWNED"] != "owned-private-value" || !reflect.DeepEqual(p.input.Args, []string{" raw "}) || !p.enabled || p.ctx != ctx {
		t.Fatal(p)
	}
}
func TestMCPLifecycleTypedAdmissionBlocksEveryFactory(t *testing.T) {
	for _, mode := range []string{"audit", "tenant", "canceled", "after-begin"} {
		for _, name := range []string{"mcp_add", "mcp_attach", "mcp_detach", "mcp_set_enabled", "mcp_remove"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				p := &mcpLifecyclePort{}
				a := &forgeAuditProbe{}
				factory := 0
				kind := opapi.Operator
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				owned := errors.New("owned audit failure")
				switch mode {
				case "audit":
					a.beginErr = owned
				case "tenant":
					kind = opapi.Tenant
				case "canceled":
					cancel()
				case "after-begin":
					a.cancel = cancel
				}
				d, _ := mcpLifecycleDispatcher(t, p, a, &factory, kind)
				_, err := d.Dispatch(ctx, opapi.Caller{}, name, mcpLifecycleRaw(name), nil)
				if err == nil || factory != 0 || len(p.calls) != 0 || p.views != 0 {
					t.Fatal(err, factory, p)
				}
				switch mode {
				case "audit":
					if !errors.Is(err, owned) || a.begins != 1 || a.ends != 0 {
						t.Fatal(err, a)
					}
				case "tenant", "canceled":
					if a.begins != 0 || a.ends != 0 {
						t.Fatal(a)
					}
				case "after-begin":
					if !errors.Is(err, context.Canceled) || a.begins != 1 || a.ends != 1 || a.cause != context.Canceled {
						t.Fatal(err, a)
					}
				}
			})
		}
	}
}
func TestMCPLifecycleTypedCodecsRetainNativeErrorsAndEnableRules(t *testing.T) {
	cases := []struct{ name, raw, want string }{{"mcp_add", `{}`, "args.server required"}, {"mcp_add", `{"server":false}`, "args.server: json: cannot unmarshal bool into Go value of type mcp.Server"}, {"mcp_add", `{"server":[]}`, "args.server: json: cannot unmarshal array into Go value of type mcp.Server"}, {"mcp_add", `{"server":{"args":1}}`, "args.server: json: cannot unmarshal number into Go struct field Server.args of type []string"}}
	for _, name := range []string{"mcp_attach", "mcp_detach", "mcp_set_enabled", "mcp_remove"} {
		for _, raw := range []string{`{}`, `{"ref":""}`, `{"ref":" \t "}`} {
			cases = append(cases, struct{ name, raw, want string }{name, raw, "args.ref required"})
		}
		for _, raw := range []string{`{"ref":null}`, `{"ref":false}`, `{"ref":1}`, `{"ref":[]}`, `{"ref":{}}`} {
			cases = append(cases, struct{ name, raw, want string }{name, raw, "args.ref must be a string"})
		}
	}
	for _, tc := range cases {
		p := &mcpLifecyclePort{}
		a := &forgeAuditProbe{}
		f := 0
		d, _ := mcpLifecycleDispatcher(t, p, a, &f, opapi.Operator)
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil || err.Error() != tc.want || f != 0 || len(p.calls) != 0 || a.begins != 1 || a.ends != 1 || !errors.Is(err, a.cause) {
			t.Fatal(tc, err, a, p)
		}
	}
	for _, tc := range []struct {
		raw  string
		want bool
	}{{`true`, true}, {`false`, false}, {`"TRUE"`, true}, {`"tRuE"`, true}, {`"1"`, true}, {`" true "`, false}, {`"on"`, false}, {`1`, false}, {`null`, false}, {`[]`, false}, {`{}`, false}, {`""`, false}, {``, false}} {
		p := &mcpLifecyclePort{}
		a := &forgeAuditProbe{}
		f := 0
		d, _ := mcpLifecycleDispatcher(t, p, a, &f, opapi.Operator)
		raw := `{"ref":" raw-ref "`
		if tc.raw != "" {
			raw += `,"enabled":` + tc.raw
		}
		raw += `}`
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, "mcp_set_enabled", json.RawMessage(raw), nil)
		if err != nil || p.enabled != tc.want || p.ref != " raw-ref " || f != 1 || a.begins != 1 || a.ends != 1 {
			t.Fatal(tc, out, err, p)
		}
	}
	p := &mcpLifecyclePort{}
	a := &forgeAuditProbe{}
	f := 0
	d, _ := mcpLifecycleDispatcher(t, p, a, &f, opapi.Operator)
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "mcp_add", json.RawMessage(`{"server":null}`), nil); err != nil || f != 1 || !reflect.DeepEqual(p.input, mcp.Server{}) {
		t.Fatal(err, p)
	}
}
func TestMCPLifecycleTypedOutputRootsAndNullableTools(t *testing.T) {
	ops, err := MCPLifecycleOperations(func(context.Context) *MCPLifecycle { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		sp := op.Spec()
		var out any
		switch sp.Name {
		case "mcp_attach":
			out = MCPAttachOutput{}
		case "mcp_detach":
			out = MCPDetachOutput{}
		case "mcp_remove":
			out = MCPRemoveOutput{}
		default:
			out = MCPServerOutput{}
		}
		var declaration map[string]any
		if err := json.Unmarshal(sp.OutputSchema, &declaration); err != nil {
			t.Fatal(err)
		}
		expected := []string{"server"}
		switch sp.Name {
		case "mcp_attach":
			expected = []string{"server", "tools"}
		case "mcp_detach":
			expected = []string{"detached"}
		case "mcp_remove":
			expected = []string{"removed"}
		}
		required, ok := declaration["required"].([]any)
		if !ok || len(required) != len(expected) {
			t.Fatal(sp.Name, "required roots", declaration)
		}
		for _, key := range expected {
			found := false
			for _, actual := range required {
				if actual == key {
					found = true
				}
			}
			if !found {
				t.Fatal(sp.Name, "required root absent", key)
			}
		}
		value := forgeJSON(t, out)
		raw, _ := json.Marshal(value)
		if err := schema.ValidateJSON(sp.OutputSchema, raw); err != nil {
			t.Fatal(sp.Name, err)
		}
		for key := range value {
			clone := forgeJSON(t, value)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
				t.Fatal(sp.Name, "missing required", key)
			}
		}
		if sp.Name == "mcp_attach" {
			for _, names := range [][]string{nil, {}, {"raw"}} {
				raw, _ := json.Marshal(MCPAttachOutput{Tools: names})
				if schema.ValidateJSON(sp.OutputSchema, raw) != nil {
					t.Fatal(string(raw))
				}
			}
		}
	}
}
func TestMCPLifecycleDirectCanceledServiceBlocksAllFiveWriters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, name := range []string{"add", "attach", "detach", "enabled", "remove"} {
		p := &mcpLifecyclePort{}
		s := NewMCPLifecycle(p, p)
		var err error
		switch name {
		case "add":
			_, err = s.Add(ctx, MCPAddInput{})
		case "attach":
			_, err = s.Attach(ctx, MCPRefInput{})
		case "detach":
			_, err = s.Detach(ctx, MCPRefInput{})
		case "enabled":
			_, err = s.SetEnabled(ctx, MCPSetEnabledInput{})
		case "remove":
			_, err = s.Remove(ctx, MCPRefInput{})
		}
		if err != context.Canceled || len(p.calls) != 0 || p.views != 0 {
			t.Fatalf("EXPECTED:direct canceled writer blocked ACTUAL:%s err=%v calls=%v", name, err, p.calls)
		}
	}
}
func TestMCPLifecycleOwnedCorrelationWinsExplicitFallback(t *testing.T) {
	for _, owned := range []string{"", "host-owned"} {
		ctx := opapi.WithCorrelation(context.Background(), owned)
		p := &mcpLifecyclePort{}
		s := NewMCPLifecycle(p, p)
		want := "explicit"
		if owned != "" {
			want = owned
		}
		for _, name := range []string{"add", "attach", "detach", "enabled", "remove"} {
			switch name {
			case "add":
				s.Add(ctx, MCPAddInput{CorrelationID: "explicit"})
			case "attach":
				s.Attach(ctx, MCPRefInput{CorrelationID: "explicit"})
			case "detach":
				s.Detach(ctx, MCPRefInput{CorrelationID: "explicit"})
			case "enabled":
				s.SetEnabled(ctx, MCPSetEnabledInput{CorrelationID: "explicit"})
			case "remove":
				s.Remove(ctx, MCPRefInput{CorrelationID: "explicit"})
			}
			if p.corr != want {
				t.Fatalf("EXPECTED:owned identity wins ACTUAL:%s corr=%q", name, p.corr)
			}
		}
	}
}

func TestMCPLifecycleAuditSettlementFailureDoesNotUndoWriter(t *testing.T) {
	for _, name := range []string{"mcp_add", "mcp_attach", "mcp_detach", "mcp_set_enabled", "mcp_remove"} {
		p := &mcpLifecyclePort{}
		failure := errors.New("owned settlement failure")
		a := &forgeAuditProbe{endErr: failure}
		f := 0
		d, _ := mcpLifecycleDispatcher(t, p, a, &f, opapi.Operator)
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, name, mcpLifecycleRaw(name), nil)
		if !errors.Is(err, failure) || f != 1 || len(p.calls) != 1 || a.begins != 1 || a.ends != 1 || a.cause != nil {
			t.Fatal(name, err, a, p)
		}
	}
}
