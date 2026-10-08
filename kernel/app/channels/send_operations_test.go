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

type sendCleanupProbe struct {
	accepted  bool
	callbacks []func()
}

func (p *sendCleanupProbe) Defer(cleanup func()) bool {
	if !p.accepted {
		return false
	}
	p.callbacks = append(p.callbacks, cleanup)
	return true
}

func TestChannelSendOperationSpecAndTerminalOwnership(t *testing.T) {
	if _, err := SendOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	for _, mode := range []string{"success", "sender-error", "sender-panic", "rejected-scope", "absent-scope"} {
		var sentContext context.Context
		cause := errors.New("owned sender error")
		cleanup := &sendCleanupProbe{accepted: mode != "rejected-scope"}
		ops, err := SendOperations(func(ctx context.Context) *Outbound {
			if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
				t.Fatal("route lost")
			}
			return NewOutbound(func(call context.Context, kind, to, text string) error {
				sentContext = call
				if kind != "slack" || to != "owned" || text != "text" {
					t.Fatal(kind, to, text)
				}
				if mode == "sender-panic" {
					panic("owned sender panic")
				}
				if mode == "sender-error" {
					return cause
				}
				return nil
			})
		})
		if err != nil || len(ops) != 1 {
			t.Fatal(ops, err)
		}
		s := ops[0].Spec()
		if s.Name != "send" || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || !s.AllowUnknownInput || s.Stream != opapi.StreamNone || s.Input != reflect.TypeFor[SendRequest]() || s.Output != reflect.TypeFor[SendOutput]() || s.HTTP.Method != "POST" || s.HTTP.Path != "/api/send" {
			t.Fatal(s)
		}
		var declaration map[string]any
		json.Unmarshal(s.OutputSchema, &declaration)
		if len(declaration["properties"].(map[string]any)) != 3 {
			t.Fatal("terminal schema must have three public fields")
		}
		rawOutput, _ := json.Marshal(SendOutput{Sent: true, Channel: "slack", To: "owned"})
		if string(rawOutput) != `{"sent":true,"channel":"slack","to":"owned"}` {
			t.Fatal("result presence/privacy", string(rawOutput))
		}
		if schema.ValidateJSON(s.OutputSchema, json.RawMessage(`{"sent":"wrong","channel":"slack","to":"owned"}`)) == nil {
			t.Fatal("untyped result")
		}
		audit := &accountAudit{}
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: opapi.Principal{Kind: opapi.Operator}}, Router: ownedACPRoute{}, Audit: audit})
		ctx := context.Background()
		if mode != "absent-scope" {
			ctx = opapi.WithTerminalCleanup(ctx, cleanup)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, "send", json.RawMessage(`{"channel":" SLACK ","to":" owned ","text":" text ","unknown":true}`), nil)
		if audit.begin != 1 || audit.end != 1 || sentContext == nil {
			t.Fatal(audit, sentContext)
		}
		if mode == "sender-error" {
			if !errors.Is(err, cause) || out != nil {
				t.Fatal(out, err)
			}
		} else if mode == "sender-panic" {
			if err == nil || err.Error() != "internal error" || out != nil || len(cleanup.callbacks) != 0 || !errors.Is(sentContext.Err(), context.Canceled) {
				t.Fatal("panic cleanup not immediate", out, err, sentContext.Err())
			}
			continue
		} else if err != nil || out != (SendOutput{Sent: true, Channel: "slack", To: "owned"}) {
			t.Fatal(out, err)
		}
		if mode == "rejected-scope" || mode == "absent-scope" {
			if !errors.Is(sentContext.Err(), context.Canceled) || len(cleanup.callbacks) != 0 {
				t.Fatal("fallback cleanup lost", sentContext.Err())
			}
			continue
		}
		if sentContext.Err() != nil || len(cleanup.callbacks) != 1 {
			t.Fatal("released before delivery", sentContext.Err(), len(cleanup.callbacks))
		}
		cleanup.callbacks[0]()
		if !errors.Is(sentContext.Err(), context.Canceled) {
			t.Fatal("terminal cleanup lost")
		}
	}
}

func TestChannelSendOperationAdmissionAndLenientRequiredFields(t *testing.T) {
	for _, mode := range []string{"canceled", "tenant", "audit-error", "audit-missing", "invalid"} {
		providers, sends := 0, 0
		ops, _ := SendOperations(func(context.Context) *Outbound {
			providers++
			return NewOutbound(func(context.Context, string, string, string) error { sends++; return nil })
		})
		audit := &accountAudit{}
		principal := opapi.Principal{Kind: opapi.Operator}
		if mode == "tenant" {
			principal = opapi.Principal{Kind: opapi.Tenant, Tenant: "owned"}
		}
		deps := app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}, Audit: audit}
		if mode == "audit-error" {
			audit.err = errors.New("owned audit failure")
		}
		if mode == "audit-missing" {
			deps.Audit = nil
		}
		d, _ := app.NewDispatcher(ops, deps)
		ctx, cancel := context.WithCancel(context.Background())
		if mode == "canceled" {
			cancel()
		}
		raw := json.RawMessage(`{"channel":"slack","to":"owned","text":"text"}`)
		if mode == "invalid" {
			raw = json.RawMessage(`{"channel":false,"to":{},"text":null}`)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, "send", raw, nil)
		cancel()
		if out != nil || err == nil || sends != 0 {
			t.Fatal(mode, out, err, sends)
		}
		if mode == "invalid" {
			if providers != 1 || err.Error() != "send requires channel, to, and text" || audit.begin != 1 || audit.end != 1 {
				t.Fatal(mode, err, providers, audit)
			}
		} else if providers != 0 {
			t.Fatal("admission entered provider", mode, providers)
		}
	}
}
