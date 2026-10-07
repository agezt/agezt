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

func TestChannelGatewayOperationSpecsAndResultPresence(t *testing.T) {
	if _, err := GatewayOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := GatewayOperations(func(context.Context) *Gateway { return nil })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, op := range ops {
		s := op.Spec()
		name, path, output, count := "whatsappgw_status", "/api/whatsappgw/status", reflect.TypeFor[GatewayStatusOutput](), 5
		if i == 1 {
			name, path, output, count = "whatsappgw_qr", "/api/whatsappgw/qr", reflect.TypeFor[GatewayQROutput](), 4
		}
		if s.Name != name || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != path || s.Input != reflect.TypeFor[GatewayRequest]() || s.Output != output {
			t.Fatal(s)
		}
		var node map[string]any
		json.Unmarshal(s.OutputSchema, &node)
		if len(node["properties"].(map[string]any)) != count {
			t.Fatal(string(s.OutputSchema))
		}
		if schema.ValidateJSON(s.InputSchema, json.RawMessage(`{"url":null,"key":false,"unknown":{}}`)) != nil {
			t.Fatal("lenient decoding changed")
		}
		if schema.ValidateJSON(s.OutputSchema, json.RawMessage(`{"ok":"wrong"}`)) == nil {
			t.Fatal("untyped bool")
		}
	}
	for _, code := range []int{0, 200, 503} {
		svc := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) { return nil, code, "", nil })
		status, _ := svc.Status(context.Background(), GatewayInput{URL: "http://owned.example"})
		wire := gatewayJSONView(t, status)
		if code == 200 {
			if len(wire) != 3 || status.Connected == nil || *status.Connected || status.Status == nil || *status.Status != "" || status.Error != nil || status.HTTPStatus != nil {
				t.Fatal(status, wire)
			}
			*status.Status = "mutated"
			*status.Connected = true
			fresh, _ := svc.Status(context.Background(), GatewayInput{URL: "http://owned.example"})
			if *fresh.Status != "" || *fresh.Connected {
				t.Fatal("borrowed output")
			}
		} else if len(wire) != 3 || status.OK || status.HTTPStatus == nil || *status.HTTPStatus != code || status.Connected != nil || status.Status != nil {
			t.Fatal(status, wire)
		}
		qr, _ := svc.QR(context.Background(), GatewayInput{URL: "http://owned.example"})
		wire = gatewayJSONView(t, qr)
		if code == 200 {
			if len(wire) != 2 || qr.Error == nil || qr.HTTPStatus != nil || qr.QR != nil {
				t.Fatal(qr, wire)
			}
		} else if len(wire) != 3 || qr.HTTPStatus == nil || *qr.HTTPStatus != code || qr.QR != nil {
			t.Fatal(qr, wire)
		}
		for i, out := range []any{status, qr} {
			raw, _ := json.Marshal(out)
			if schema.ValidateJSON(ops[i].Spec().OutputSchema, raw) != nil {
				t.Fatal(string(raw))
			}
		}
	}
	svc := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) {
		return nil, 0, "", errors.New("owned unavailable")
	})
	status, _ := svc.Status(context.Background(), GatewayInput{URL: "http://owned.example"})
	qr, _ := svc.QR(context.Background(), GatewayInput{URL: "http://owned.example"})
	if len(gatewayJSONView(t, status)) != 2 || len(gatewayJSONView(t, qr)) != 2 || status.HTTPStatus != nil || qr.HTTPStatus != nil {
		t.Fatal(status, qr)
	}
}

func TestChannelGatewayOperationPrimaryUnauditedCancelAndLenientURL(t *testing.T) {
	for _, command := range []string{"whatsappgw_status", "whatsappgw_qr"} {
		for _, mode := range []string{"success", "canceled", "tenant", "agent"} {
			providers, gets := 0, 0
			principal := opapi.Principal{Kind: opapi.Operator}
			if mode == "tenant" {
				principal = opapi.Principal{Kind: opapi.Tenant, Tenant: "owned"}
			}
			if mode == "agent" {
				principal.Kind = opapi.Agent
			}
			ops, _ := GatewayOperations(func(ctx context.Context) *Gateway {
				providers++
				if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
					t.Fatal("route lost")
				}
				return NewGateway(func(full, header, key string, max int64) ([]byte, int, string, error) {
					gets++
					if key != "owned" || header != "apikey" {
						t.Fatal(full, header, key, max)
					}
					return []byte(`{"status":"WORKING","base64":"raw"}`), 200, "application/json", nil
				})
			})
			d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}})
			ctx, cancel := context.WithCancel(context.Background())
			if mode == "canceled" {
				cancel()
			}
			out, err := d.Dispatch(ctx, opapi.Caller{Tenant: "ignored"}, command, json.RawMessage(`{"url":" http://owned.example/// ","backend":" EVOLUTION ","key":" owned ","unknown":true}`), nil)
			cancel()
			if mode != "success" {
				if err == nil || out != nil || providers != 0 || gets != 0 {
					t.Fatal(mode, out, err, providers, gets)
				}
				if mode == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				continue
			}
			if err != nil || providers != 1 || gets != 1 || gatewayJSONView(t, out)["ok"] != true {
				t.Fatal(out, err, providers, gets)
			}
		}
		for _, value := range []any{nil, false, 3, []any{}, map[string]any{}} {
			gets := 0
			ops, _ := GatewayOperations(func(context.Context) *Gateway {
				return NewGateway(func(string, string, string, int64) ([]byte, int, string, error) { gets++; return nil, 0, "", nil })
			})
			d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: opapi.Principal{Kind: opapi.Operator}}, Router: ownedACPRoute{}})
			raw, _ := json.Marshal(map[string]any{"url": value})
			_, err := d.Dispatch(context.Background(), opapi.Caller{}, command, raw, nil)
			if err == nil || err.Error() != "args.url (gateway URL) is required" || gets != 0 {
				t.Fatal(value, err, gets)
			}
		}
	}
}
