// SPDX-License-Identifier: MIT
package config

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

type configAuth struct{ kind opapi.PrincipalKind }

func (a configAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "acme"}, nil
}

type configRoute struct{}

func (configRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type forbiddenConfigAudit struct{ calls int }

func (a *forbiddenConfigAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return nil, errors.New("config read must not audit")
}
func TestConfigTypedOperationMetadataContextNoAuditAndAdmission(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil factory")
	}
	p := &configProbe{}
	f, envCalls := 0, 0
	var seen context.Context
	ops, err := Operations(func(ctx context.Context) *Service {
		f++
		seen = ctx
		return New(p, []string{"owned"}, func(string) bool { envCalls++; return true })
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	sp := ops[0].Spec()
	if sp.Name != "config" || !sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != reflect.TypeFor[ShowInput]() || sp.Output != reflect.TypeFor[ShowOutput]() || sp.Emission != nil || len(sp.EmissionSchema) != 0 {
		t.Fatal(sp)
	}
	audit := &forbiddenConfigAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: configAuth{opapi.Operator}, Router: configRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	type callerKey struct{}
	ctx := context.WithValue(context.Background(), callerKey{}, "owned")
	for _, raw := range []string{`{}`, `{"unknown":true,"tenant":"spoof","model":null,"env":false}`} {
		out, err := d.Dispatch(ctx, opapi.Caller{}, sp.Name, json.RawMessage(raw), nil)
		encoded, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(sp.OutputSchema, encoded) != nil || seen != ctx {
			t.Fatal(out, err)
		}
	}
	if f != 2 || envCalls != 2 || len(p.calls) != 14 || audit.calls != 0 {
		t.Fatal(f, envCalls, p.calls, audit)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(canceled, opapi.Caller{}, sp.Name, json.RawMessage(`{}`), nil); err != context.Canceled {
		t.Fatal(err)
	}
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: configAuth{opapi.Tenant}, Router: configRoute{}, Audit: audit})
	if _, err := tenant.Dispatch(context.Background(), opapi.Caller{}, sp.Name, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant admitted")
	}
	if f != 2 || envCalls != 2 || len(p.calls) != 14 || audit.calls != 0 {
		t.Fatal(f, envCalls, p.calls, audit)
	}
}
func configJSON(t *testing.T, value any) map[string]any {
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
func TestConfigTypedSchemaRequiredRootsPathsAndOptionalRouting(t *testing.T) {
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	sp := ops[0].Spec()
	decl := configJSON(t, json.RawMessage(sp.OutputSchema))
	props := decl["properties"].(map[string]any)
	if len(props) != 8 || len(decl["required"].([]any)) != 7 {
		t.Fatal(decl)
	}
	paths := props["paths"].(map[string]any)
	if len(paths["properties"].(map[string]any)) != 6 || len(paths["required"].([]any)) != 6 {
		t.Fatal(paths)
	}
	routing := props["routing"].(map[string]any)
	routingRequired, _ := routing["required"].([]any)
	if len(routing["properties"].(map[string]any)) != 3 || len(routingRequired) != 0 {
		t.Fatal(routing)
	}
	zero := ShowOutput{Env: map[string]bool{}}
	value := configJSON(t, zero)
	raw, _ := json.Marshal(value)
	if schema.ValidateJSON(sp.OutputSchema, raw) != nil || len(value) != 7 {
		t.Fatal(string(raw))
	}
	for _, key := range []string{"paths", "model", "system_prompt_set", "tool_count", "plugin_count", "ask_policy", "env"} {
		copy := configJSON(t, value)
		delete(copy, key)
		raw, _ := json.Marshal(copy)
		if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
			t.Fatal("missing root", key)
		}
	}
	pathValue := value["paths"].(map[string]any)
	pathSchema, _ := json.Marshal(paths)
	for _, key := range []string{"base", "journal", "state", "runtime", "catalog", "vault"} {
		copy := configJSON(t, pathValue)
		delete(copy, key)
		raw, _ := json.Marshal(copy)
		if schema.ValidateJSON(pathSchema, raw) == nil {
			t.Fatal("missing path", key)
		}
	}
	for _, r := range []*Routing{nil, {}, {Routes: map[string][]string{"nil": {}, "raw": {"second", "first"}}}, {Requires: map[string][]string{"empty": {}}}, {ModelOverrides: map[string]string{"raw": "", "other": " raw model "}}} {
		zero.Routing = r
		raw, _ := json.Marshal(zero)
		if schema.ValidateJSON(sp.OutputSchema, raw) != nil {
			t.Fatal(string(raw))
		}
	}
	for _, bad := range []string{`{"owned":"private-value"}`, `{"owned":1}`, `{"owned":[]}`} {
		copy := configJSON(t, value)
		var env any
		json.Unmarshal([]byte(bad), &env)
		copy["env"] = env
		raw, _ := json.Marshal(copy)
		if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
			t.Fatal("non-boolean env admitted", bad)
		}
	}
	for _, key := range []string{"routes", "requires", "model_overrides"} {
		zero.Routing = &Routing{}
		encoded := configJSON(t, zero.Routing)
		if _, ok := encoded[key]; ok {
			t.Fatal("empty map emitted", key)
		}
	}
}
