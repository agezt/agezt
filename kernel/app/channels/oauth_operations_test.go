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

func TestChannelOAuthOperationMetadataAndPresence(t *testing.T) {
	if _, err := OAuthOperations(nil); err == nil {
		t.Fatal("nil provider registered")
	}
	ops, err := OAuthOperations(func(context.Context) *OAuth { return nil })
	if err != nil || len(ops) != 3 {
		t.Fatal(ops, err)
	}
	names := []string{"channel_oauth_start", "channel_oauth_callback", "channel_oauth_status"}
	inputs := []reflect.Type{reflect.TypeFor[OAuthStartRequest](), reflect.TypeFor[OAuthCallbackRequest](), reflect.TypeFor[OAuthStatusRequest]()}
	outputs := []reflect.Type{reflect.TypeFor[OAuthStartOutput](), reflect.TypeFor[OAuthCallbackOutput](), reflect.TypeFor[OAuthStatusOutput]()}
	counts := []int{2, 5, 4}
	for i, op := range ops {
		s := op.Spec()
		if s.Name != names[i] || s.ReadOnly != (i == 2) || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.Input != inputs[i] || s.Output != outputs[i] {
			t.Fatal(s)
		}
		if i == 1 {
			if s.HTTP != (opapi.HTTP{}) {
				t.Fatal("internal callback declared public route", s.HTTP)
			}
		} else {
			path := "/api/channel/oauth/start"
			if i == 2 {
				path = "/api/channel/oauth/status"
			}
			if s.HTTP.Method != "POST" || s.HTTP.Path != path {
				t.Fatal(s.HTTP)
			}
		}
		var node map[string]any
		json.Unmarshal(s.OutputSchema, &node)
		if len(node["properties"].(map[string]any)) != counts[i] {
			t.Fatal(string(s.OutputSchema))
		}
		if schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"kind":null,"label":false,"code":{},"state":42,"unknown":[]}`)) != nil {
			t.Fatal("lenient admission changed")
		}
		var out any = OAuthStartOutput{}
		if i == 1 {
			out = OAuthCallbackOutput{OK: true, Applied: "restart"}
		}
		if i == 2 {
			out = OAuthStatusOutput{Status: "unknown"}
		}
		raw, _ := json.Marshal(out)
		if schema.ValidateJSON(s.OutputSchema, raw) != nil {
			t.Fatal(string(raw))
		}
		var wire map[string]any
		json.Unmarshal(raw, &wire)
		if i == 2 {
			if len(wire) != 1 || wire["status"] != "unknown" {
				t.Fatal(wire)
			}
		} else if len(wire) != counts[i] {
			t.Fatal(wire)
		}
		field := []string{"state", "ok", "status"}[i]
		wire[field] = 3
		raw, _ = json.Marshal(wire)
		if schema.ValidateJSON(s.OutputSchema, raw) == nil {
			t.Fatal("output type erased", s.Name)
		}
	}
	p := newOAuthProbe()
	p.flow = OAuthFlow{}
	out, _ := NewOAuth(p).Status(context.Background(), OAuthStatusInput{})
	raw, _ := json.Marshal(out)
	var wire map[string]any
	json.Unmarshal(raw, &wire)
	if len(wire) != 4 || wire["error"] != "" || wire["kind"] != "" || wire["label"] != "" {
		t.Fatal("known empty members lost", wire)
	}
	*out.Error = "changed"
	*out.Kind = "changed"
	*out.Label = "changed"
	fresh, _ := NewOAuth(p).Status(context.Background(), OAuthStatusInput{})
	if *fresh.Error != "" || *fresh.Kind != "" || *fresh.Label != "" {
		t.Fatal("borrowed status output", fresh)
	}
}

func TestChannelOAuthOperationAdmissionBeforeProviderAndLenientInputs(t *testing.T) {
	for _, command := range []string{"channel_oauth_start", "channel_oauth_callback", "channel_oauth_status"} {
		for _, mode := range []string{"success", "canceled", "audit-error", "audit-missing", "tenant", "agent"} {
			p := newOAuthProbe()
			audit := &accountAudit{}
			providers := 0
			ops, _ := OAuthOperations(func(ctx context.Context) *OAuth {
				providers++
				if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
					t.Fatal("route lost")
				}
				return NewOAuth(p)
			})
			principal := opapi.Principal{Kind: opapi.Operator}
			if mode == "tenant" {
				principal = opapi.Principal{Kind: opapi.Tenant, Tenant: "owned"}
			}
			if mode == "agent" {
				principal.Kind = opapi.Agent
			}
			deps := app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}, Audit: audit}
			sentinel := errors.New("owned audit admission")
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
			out, err := d.Dispatch(ctx, opapi.Caller{Tenant: "ignored"}, command, json.RawMessage(`{"kind":" SLACK ","label":" work ","client_id":" id ","client_secret":" secret ","redirect_uri":"https://owned.example/cb","code":" code ","state":" state ","unknown":true}`), nil)
			cancel()
			denied := mode == "canceled" || mode == "tenant" || mode == "agent" || command != "channel_oauth_status" && (mode == "audit-error" || mode == "audit-missing")
			if denied {
				if err == nil || out != nil || providers != 0 || len(p.calls) != 0 || audit.end != 0 {
					t.Fatal(command, mode, out, err, p.calls)
				}
				if mode == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				continue
			}
			if err != nil || providers != 1 {
				t.Fatal(command, mode, out, err)
			}
			if command == "channel_oauth_status" {
				if audit.begin != 0 || audit.end != 0 {
					t.Fatal("status audited")
				}
			} else if audit.begin != 1 || audit.end != 1 || audit.terminal != nil {
				t.Fatal(audit)
			}
		}
	}
	for _, invalid := range []any{nil, false, 3, []any{}, map[string]any{}} {
		p := newOAuthProbe()
		audit := &accountAudit{}
		ops, _ := OAuthOperations(func(context.Context) *OAuth { return NewOAuth(p) })
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: opapi.Principal{Kind: opapi.Operator}}, Router: ownedACPRoute{}, Audit: audit})
		raw, _ := json.Marshal(map[string]any{"kind": invalid, "code": invalid, "state": invalid, "unknown": true})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "channel_oauth_start", raw, nil); err == nil || err.Error() != "client_id and client_secret are required" || p.kind != "" || audit.begin != 1 || audit.end != 1 {
			t.Fatal("lenient type input changed", err, p.kind, audit)
		}
		// The original domain error wins over any stricter JSON type admission.
	}
}
