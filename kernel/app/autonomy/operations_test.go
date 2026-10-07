// SPDX-License-Identifier: MIT
package autonomy

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

type feedAuth struct{ kind opapi.PrincipalKind }

func (a feedAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "acme"}, nil
}

type feedRoute struct{}

func (feedRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}
func TestAutonomyTypedOperationDefaultsSchemaAndCanceledAdmission(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("missing provider admitted")
	}
	port := &feedJournal{events: []*event.Event{feedEvent(0, event.KindInfo, "doctor.auto_repair", map[string]any{"routing_force_generation": float64(0), "chain_depth": float64(0)})}}
	providers := 0
	operations, err := Operations(func(context.Context) *Feed { providers++; return NewFeed(port) })
	if err != nil || len(operations) != 1 {
		t.Fatal(operations, err)
	}
	spec := operations[0].Spec()
	if spec.Name != "autonomy_feed" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input != reflect.TypeFor[FeedRequest]() || spec.Output != reflect.TypeFor[FeedOutput]() || !spec.AllowUnknownInput {
		t.Fatal(spec)
	}
	d, err := app.NewDispatcher(operations, app.Dependencies{Auth: feedAuth{opapi.Operator}, Router: feedRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `{"limit":null}`, `{"limit":true}`, `{"limit":"1"}`, `{"limit":{}}`, `{"limit":[]}`, `{"limit":0}`, `{"limit":1.9}`, `{"limit":1000}`, `{"unused":true}`} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(raw), nil)
		encoded, _ := json.Marshal(out)
		if err != nil {
			t.Fatal(raw, err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, encoded); err != nil {
			t.Fatal(string(encoded), err)
		}
	}
	if providers != 10 || len(port.calls) != 10 {
		t.Fatal(providers, port.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, spec.Name, json.RawMessage(`{}`), nil); err != context.Canceled {
		t.Fatal(err)
	}
	tenantDispatcher, _ := app.NewDispatcher(operations, app.Dependencies{Auth: feedAuth{opapi.Tenant}, Router: feedRoute{}})
	if _, err := tenantDispatcher.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, spec.Name, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("tenant admitted")
	}
	if providers != 10 || len(port.calls) != 10 {
		t.Fatal(providers, port.calls)
	}
	cause := errors.New("owned tail error")
	port.cause = cause
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(`{}`), nil); err != cause {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw  string
		want int
	}{{"", 60}, {"null", 60}, {`"1"`, 60}, {"true", 60}, {"0", 0}, {"-10", -10}, {"1.9", 1}, {"500", 500}} {
		if got := nativeFeedLimit(json.RawMessage(tc.raw)); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestAutonomyTypedRowSchemaRequiresIdentityAndRetainsOptionalZero(t *testing.T) {
	operations, err := Operations(func(context.Context) *Feed { return NewFeed(&feedJournal{}) })
	if err != nil {
		t.Fatal(err)
	}
	declared := operations[0].Spec().OutputSchema
	zero := 0
	row := FeedItem{RoutingForceGeneration: &zero, ChainDepth: &zero}
	out := FeedOutput{Items: []FeedItem{row}, Count: 1}
	encoded, _ := json.Marshal(out)
	if err := schema.ValidateJSON(declared, encoded); err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	_ = json.Unmarshal(encoded, &wire)
	item := wire["items"].([]any)[0].(map[string]any)
	if len(item) != 9 || item["routing_force_generation"] != float64(0) || item["chain_depth"] != float64(0) {
		t.Fatal(item)
	}
	for _, field := range []string{"seq", "ts_unix_ms", "kind", "subject", "category", "title", "correlation_id"} {
		value, present := item[field]
		if !present {
			t.Fatal("required field absent", field, item)
		}
		delete(item, field)
		raw, _ := json.Marshal(wire)
		if err := schema.ValidateJSON(declared, raw); err == nil {
			t.Fatal("schema accepted missing required identity", field)
		}
		item[field] = value
	}
	delete(wire, "count")
	raw, _ := json.Marshal(wire)
	if err := schema.ValidateJSON(declared, raw); err == nil {
		t.Fatal("schema accepted missing count")
	}
}

func TestAutonomyTypedDoctorNumericMetadataHasIndependentTruncatedValues(t *testing.T) {
	events := []*event.Event{feedEvent(0, event.KindInfo, "doctor.auto_repair", map[string]any{"routing_force_generation": float64(2.9), "chain_depth": float64(-4.9)}), feedEvent(1, event.KindInfo, "doctor.auto_repair", map[string]any{"routing_force_generation": float64(7), "chain_depth": float64(9)})}
	out, err := NewFeed(&feedJournal{events: events}).List(context.Background(), FeedInput{Limit: 2})
	if err != nil || out.Count != 2 {
		t.Fatal(out, err)
	}
	for index, want := range [][2]int{{7, 9}, {2, -4}} {
		row := out.Items[index]
		if row.RoutingForceGeneration == nil || row.ChainDepth == nil || *row.RoutingForceGeneration != want[0] || *row.ChainDepth != want[1] {
			t.Fatal(index, row, want)
		}
	}
}
