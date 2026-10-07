// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type accountAudit struct {
	begin, end    int
	err, terminal error
	record        opapi.AuditRecord
}

func (a *accountAudit) Begin(_ context.Context, r opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begin++
	a.record = r
	if a.err != nil {
		return nil, a.err
	}
	return a, nil
}
func (a *accountAudit) End(_ context.Context, err error) error { a.end++; a.terminal = err; return nil }

func TestChannelAccountOperationTypedSpecsAndOutputPresence(t *testing.T) {
	if _, err := AccountOperations(nil); err == nil {
		t.Fatal("nil provider accepted")
	}
	ops, err := AccountOperations(func(context.Context) *Accounts { return nil })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, op := range ops {
		s := op.Spec()
		name, path, input, output, count := "channel_account_set", "/api/channel/account/set", reflect.TypeFor[SetAccountRequest](), reflect.TypeFor[SetAccountOutput](), 5
		if i == 1 {
			name, path, input, output, count = "channel_account_remove", "/api/channel/account/remove", reflect.TypeFor[RemoveAccountRequest](), reflect.TypeFor[RemoveAccountOutput](), 4
		}
		if s.Name != name || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != path || s.Input != input || s.Output != output {
			t.Fatal(s)
		}
		var node map[string]any
		json.Unmarshal(s.OutputSchema, &node)
		if len(node["properties"].(map[string]any)) != count {
			t.Fatal(string(s.OutputSchema))
		}
		var out any = SetAccountOutput{Saved: true, Applied: "restart"}
		if i == 1 {
			out = RemoveAccountOutput{Applied: "restart"}
		}
		raw, _ := json.Marshal(out)
		if err := schema.ValidateJSON(s.OutputSchema, raw); err != nil {
			t.Fatal(string(raw), err)
		}
		var wire map[string]any
		json.Unmarshal(raw, &wire)
		if len(wire) != count || wire["label"] != "" {
			t.Fatal(wire)
		}
		if i == 1 && wire["removed"] != float64(0) {
			t.Fatal("zero count omitted", wire)
		}
		wire["applied"] = true
		raw, _ = json.Marshal(wire)
		if schema.ValidateJSON(s.OutputSchema, raw) == nil {
			t.Fatal("output schema erased")
		}
		if schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"kind":null,"label":false,"unknown":true}`)) != nil {
			t.Fatal("type errors moved before audit")
		}
	}
}

func TestChannelAccountOperationArgumentOrderAndAuditedValidation(t *testing.T) {
	for _, command := range []string{"channel_account_set", "channel_account_remove"} {
		keys := []string{"kind", "label"}
		if command == "channel_account_set" {
			keys = append(keys, "name", "value")
		}
		for _, key := range keys {
			for _, invalid := range []any{nil, false, float64(3), []any{}, map[string]any{}} {
				p := newAccountProbe(false)
				audit := &accountAudit{}
				providers := 0
				ops, _ := AccountOperations(func(ctx context.Context) *Accounts { providers++; return NewAccounts(p) })
				d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: opapi.Principal{Kind: opapi.Operator}}, Router: ownedACPRoute{}, Audit: audit})
				args := map[string]any{key: invalid, "unknown": true}
				raw, _ := json.Marshal(args)
				_, err := d.Dispatch(context.Background(), opapi.Caller{}, command, raw, nil)
				if err == nil || err.Error() != "args."+key+" must be a string" || providers != 0 || len(p.calls) != 0 || audit.begin != 1 || audit.end != 1 || !errors.Is(err, audit.terminal) {
					t.Fatal(command, key, err, providers, p.calls, audit)
				}
			}
		}
		p := newAccountProbe(false)
		audit := &accountAudit{}
		ops, _ := AccountOperations(func(context.Context) *Accounts { return NewAccounts(p) })
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: opapi.Principal{Kind: opapi.Operator}}, Router: ownedACPRoute{}, Audit: audit})
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, command, json.RawMessage(`{"kind":false,"label":null,"name":3,"value":false}`), nil)
		if err == nil || err.Error() != "args.kind must be a string" || len(p.calls) != 0 {
			t.Fatal(err, p.calls)
		}
	}
}

func TestChannelAccountOperationAdmissionAndSelectedRoute(t *testing.T) {
	for _, command := range []string{"channel_account_set", "channel_account_remove"} {
		for _, mode := range []string{"success", "audit-error", "audit-missing", "canceled", "tenant", "agent"} {
			p := newAccountProbe(false)
			p.config.values["AGEZT_PUBLIC#work"] = "before"
			audit := &accountAudit{}
			providers := 0
			ops, _ := AccountOperations(func(ctx context.Context) *Accounts {
				providers++
				if ctx.Value(ownedACPRouteKey{}) != "owned-route" || audit.begin != 1 || audit.end != 0 {
					t.Fatal("provider before admission/route", audit)
				}
				return NewAccounts(p)
			})
			principal := opapi.Principal{Kind: opapi.Operator}
			if mode == "tenant" {
				principal = opapi.Principal{Kind: opapi.Tenant, Tenant: "owned"}
			}
			if mode == "agent" {
				principal.Kind = opapi.Agent
			}
			deps := app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}, Audit: audit}
			sentinel := errors.New("owned audit failure")
			if mode == "audit-error" {
				audit.err = sentinel
			}
			if mode == "audit-missing" {
				deps.Audit = nil
			}
			d, _ := app.NewDispatcher(ops, deps)
			ctx, cancel := context.WithCancel(context.Background())
			if mode == "canceled" {
				cancel()
			}
			out, err := d.Dispatch(ctx, opapi.Caller{Tenant: "owned"}, command, json.RawMessage(`{"kind":" OWNED-KIND ","label":" work ","name":" AGEZT_PUBLIC ","value":" raw ","unknown":false}`), nil)
			cancel()
			if mode != "success" {
				if err == nil || out != nil || providers != 0 || len(p.calls) != 0 || p.config.values["AGEZT_PUBLIC#work"] != "before" || audit.end != 0 {
					t.Fatal(mode, out, err, p.calls, audit)
				}
				if mode == "audit-error" && !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
				if mode == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				continue
			}
			if err != nil || providers != 1 || audit.end != 1 || audit.terminal != nil || audit.record.Operation != command || audit.record.Principal.Tenant != "" {
				t.Fatal(out, err, audit)
			}
			if command == "channel_account_set" {
				if out != (SetAccountOutput{Kind: "owned-kind", Label: "work", Env: "AGEZT_PUBLIC#work", Saved: true, Applied: "restart"}) || p.config.values["AGEZT_PUBLIC#work"] != "raw" {
					t.Fatal(out, p.config.values)
				}
			} else {
				if out != (RemoveAccountOutput{Kind: "owned-kind", Label: "work", Removed: 1, Applied: "restart"}) || len(p.config.values) != 0 {
					t.Fatal(out, p.config.values)
				}
			}
		}
	}
}
