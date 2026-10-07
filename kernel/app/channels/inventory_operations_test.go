// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestChannelInventoryOperationSpecFullTypedGETAndSecretPublicPresence(t *testing.T) {
	if _, err := InventoryOperations(nil); err == nil {
		t.Fatal("nil inventory provider registered")
	}
	ops, err := InventoryOperations(func(context.Context) *Inventory { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "channel_list" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/channels" || spec.Input != reflect.TypeFor[ListInput]() || spec.Output != reflect.TypeFor[ListOutput]() {
		t.Fatal(spec)
	}
	p := &inventoryProbe{manifests: []channel.Manifest{{Kind: "owned", ConfigSection: "none", SetupSteps: nil}, {Kind: "empty", ConfigSection: "none", SetupSteps: []string{}}}}
	out, err := NewInventory(p).List(context.Background(), ListInput{})
	raw, _ := json.Marshal(out)
	if err != nil || schema.ValidateJSON(spec.OutputSchema, raw) != nil {
		t.Fatal(string(raw), err)
	}
	var roundtrip ListOutput
	if err := json.Unmarshal(raw, &roundtrip); err != nil || !reflect.DeepEqual(out, roundtrip) {
		t.Fatal(out, roundtrip, err)
	}
	var fields map[string]any
	json.Unmarshal(spec.OutputSchema, &fields)
	props := fields["properties"].(map[string]any)
	channelNode := props["channels"].(map[string]any)["items"].(map[string]any)
	channelProps := channelNode["properties"].(map[string]any)
	if len(props) != 4 || len(channelProps) != 15 || len(channelProps["accounts"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)) != 5 || len(channelProps["fields"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)) != 8 {
		t.Fatal("nested schema erased", string(spec.OutputSchema))
	}
	if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"force":true,"unknown":{"value":42}}`)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"count", "probe_matrix", "channels"} {
		bad := inventoryJSONView(t, out)
		bad[name] = "wrong"
		raw, _ := json.Marshal(bad)
		if schema.ValidateJSON(spec.OutputSchema, raw) == nil {
			t.Fatal("typed root accepts wrong value", name)
		}
	}
	bad := inventoryJSONView(t, out)
	rows := bad["channels"].([]map[string]any)
	rows[0]["configured"] = "wrong"
	raw, _ = json.Marshal(bad)
	if schema.ValidateJSON(spec.OutputSchema, raw) == nil {
		t.Fatal("typed row erased")
	}
	public := ""
	for _, value := range []ChannelField{{Env: "SEC", Secret: true}, {Env: "PUBLIC", Value: &public}} {
		raw, _ := json.Marshal(value)
		var field map[string]any
		json.Unmarshal(raw, &field)
		if value.Secret {
			if len(field) != 7 || field["value"] != nil {
				t.Fatal(field)
			}
		} else if len(field) != 8 || field["value"] != "" {
			t.Fatal(field)
		}
	}
}

func TestChannelInventoryOperationPrimaryNoAuditCanceledBeforePrepare(t *testing.T) {
	p := &inventoryProbe{}
	providerCalls := 0
	ops, err := InventoryOperations(func(ctx context.Context) *Inventory {
		providerCalls++
		if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
			t.Fatal("provider lost selected route")
		}
		return NewInventory(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Operator}, {Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		d, err := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}})
		if err != nil {
			t.Fatal(err)
		}
		before, calls := providerCalls, len(p.calls)
		out, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "channel_list", json.RawMessage(`{"unknown":true,"tenant":"ignored"}`), nil)
		if principal.Kind != opapi.Operator {
			if err == nil || providerCalls != before || len(p.calls) != calls {
				t.Fatal("non-operator entered reader", out, err)
			}
			continue
		}
		if err != nil || out.(ListOutput).Count != 0 || out.(ListOutput).Channels == nil || providerCalls != before+1 {
			t.Fatal("primary read lost empty array or required audit", out, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		before, calls = providerCalls, len(p.calls)
		if out, err := d.Dispatch(ctx, opapi.Caller{}, "channel_list", json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || out != nil || providerCalls != before || len(p.calls) != calls {
			t.Fatal("canceled inventory prepared storage", out, err)
		}
	}
}
