// SPDX-License-Identifier: MIT

package providers_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type observationAuth struct{}

func (observationAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type observationRouter struct{}

func (observationRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

func TestObservationSpecsRetainTypedRowsMapsAndRelevantFields(t *testing.T) {
	ops, err := providers.ObservationOperations(func(context.Context) *providers.Observations { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 3 {
		t.Fatalf("operations = %d", len(ops))
	}
	for _, op := range ops {
		spec := op.Spec()
		if !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatalf("metadata = %+v", spec)
		}
		zero, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, zero); err != nil {
			t.Fatal(err)
		}
		var wrong json.RawMessage
		switch spec.Name {
		case "provider_log":
			wrong = json.RawMessage(`{"events":[{"kind":42,"ts_unix_ms":1,"seq":1}],"count":1,"next_cursor":""}`)
		case "provider_stats":
			wrong = json.RawMessage(`{"routed":1,"fallbacks":0,"fallback_rate":0,"window_ms":0,"by_primary":{"alpha":"wrong"},"fallbacks_by_primary":{}}`)
		case "provider_rejections":
			wrong = json.RawMessage(`{"rejections":[{"kind":"rejected","capability":"vision","ts_unix_ms":1,"model":42}],"count":1}`)
		default:
			t.Fatalf("unexpected spec %s", spec.Name)
		}
		if schema.ValidateJSON(spec.OutputSchema, wrong) == nil {
			t.Fatalf("%s erased row/map type", spec.Name)
		}
		ignored := json.RawMessage(`{"ignored":true}`)
		if spec.Name == "provider_stats" {
			ignored = json.RawMessage(`{"limit":"unused","cursor":42,"fallbacks":"unused"}`)
		}
		if spec.Name == "provider_rejections" {
			ignored = json.RawMessage(`{"cursor":42,"fallbacks":"unused"}`)
		}
		if err := schema.ValidateJSON(spec.InputSchema, ignored); err != nil {
			t.Fatalf("unused legacy input rejected: %v", err)
		}
	}
}

func TestObservationDispatchPreservesLimitAdmissionAndRejectsBeforeRead(t *testing.T) {
	events := make([]*event.Event, 1005)
	for i := range events {
		events[i] = observation(event.KindRoutingDecision, 10, int64(i+1), `{"primary":"alpha"}`)
	}
	calls := 0
	ops, err := providers.ObservationOperations(func(context.Context) *providers.Observations {
		calls++
		return providers.NewObservations(observationReader{events: events})
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := app.NewDispatcher(ops, app.Dependencies{Auth: observationAuth{}, Router: observationRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw   string
		count int
	}{{`{}`, 20}, {`{"limit":null}`, 20}, {`{"limit":0}`, 1}, {`{"limit":-5}`, 1}, {`{"limit":2.9}`, 2}, {`{"limit":2000}`, 1000}, {`{"limit":2,"cursor":"10:3"}`, 2}, {`{"fallbacks":true}`, 0}} {
		out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "provider_log", json.RawMessage(tc.raw), nil)
		if err != nil {
			t.Fatal(err)
		}
		if count := out.(providers.ObservationLogOutput).Count; count != tc.count {
			t.Fatalf("%s count = %d; want %d", tc.raw, count, tc.count)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"provider_log", `{"limit":"bad"}`},
		{"provider_log", `{"fallbacks":null}`},
		{"provider_log", `{"fallbacks":"bad"}`},
		{"provider_stats", `{"since_ms":"bad"}`},
		{"provider_rejections", `{"limit":true}`},
	} {
		before := calls
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err == nil || calls != before {
			t.Fatalf("%s invalid input entered reader: err=%v calls=%d/%d", tc.name, err, calls, before)
		}
	}
}

func TestObservationDispatchPreservesRelativeWindow(t *testing.T) {
	now := time.Now().UnixMilli()
	ops, err := providers.ObservationOperations(func(context.Context) *providers.Observations {
		return providers.NewObservations(observationReader{events: []*event.Event{
			observation(event.KindRoutingDecision, now-60000, 1, `{"primary":"old"}`),
			observation(event.KindRoutingDecision, now, 2, `{"primary":"recent"}`),
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := app.NewDispatcher(ops, app.Dependencies{Auth: observationAuth{}, Router: observationRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "provider_stats", json.RawMessage(`{"since_ms":10000.9}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	stats := out.(providers.ObservationStatsOutput)
	if stats.Routed != 1 || stats.WindowMS != 10000 || stats.ByPrimary["recent"] != 1 || stats.ByPrimary["old"] != 0 {
		t.Fatalf("window changed: %+v", stats)
	}
}
