// SPDX-License-Identifier: MIT

package providers_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/event"
)

type observationReader struct {
	events []*event.Event
	err    error
}

func (r observationReader) Range(fn func(*event.Event) error) error {
	for _, e := range r.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return r.err
}

func observation(kind event.Kind, ms, seq int64, payload string) *event.Event {
	return &event.Event{Kind: kind, TSUnixMS: ms, Seq: seq, Payload: json.RawMessage(payload)}
}

func assertObservationJSON(t *testing.T, result map[string]any, want string) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var gotJSON, wantJSON any
	if err := json.Unmarshal(raw, &gotJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("result = %s; want %s", raw, want)
	}
}

func TestObservationsStatsPreservesWindowAndProviderDimension(t *testing.T) {
	svc := providers.NewObservations(observationReader{events: []*event.Event{
		observation(event.KindRoutingDecision, 5, 1, `{"primary":"old"}`),
		observation(event.KindRoutingDecision, 10, 2, `{"primary":"alpha"}`),
		observation(event.KindRoutingDecision, 11, 3, `{`),
		observation(event.KindProviderFallback, 12, 4, `{"failed":"alpha","next":"beta"}`),
		observation(event.KindProviderFallback, 13, 5, `{"scope":"model-chain","failed_model":"small","next_model":"large"}`),
		observation(event.KindCapabilityRejected, 14, 6, `{"model":"ignored"}`),
	}})
	out, err := svc.Stats(providers.ObservationInput{CutoffMS: 10, WindowMS: 100})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"routed":2,"fallbacks":1,"fallback_rate":0.5,"by_primary":{"alpha":1},"fallbacks_by_primary":{"alpha":1},"window_ms":100}`)
	empty, err := providers.NewObservations(observationReader{}).Stats(providers.ObservationInput{WindowMS: -5})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, empty, `{"routed":0,"fallbacks":0,"fallback_rate":0,"by_primary":{},"fallbacks_by_primary":{},"window_ms":-5}`)
}

func TestObservationsLogPreservesModelHopsAndPagination(t *testing.T) {
	svc := providers.NewObservations(observationReader{events: []*event.Event{
		observation(event.KindRoutingDecision, 10, 1, `{"primary":"alpha","chain":["alpha","beta"],"task_type":"chat"}`),
		observation(event.KindProviderFallback, 20, 2, `{"failed":"alpha","next":"beta","reason":"429"}`),
		observation(event.KindProviderFallback, 20, 3, `{"scope":"model-chain","failed_model":"small","next_model":"large","task_type":"code","reason":"capacity"}`),
	}})
	out, err := svc.Log(providers.ObservationInput{Limit: 1, FallbacksOnly: true, CutoffMS: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"events":[{"kind":"fallback","scope":"model-chain","failed":"small","next":"large","task_type":"code","reason":"capacity","ts_unix_ms":20,"seq":3}],"count":1,"next_cursor":"20:3"}`)
	out, err = svc.Log(providers.ObservationInput{Limit: 5, Cursor: "20:3"})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"events":[{"kind":"fallback","failed":"alpha","next":"beta","reason":"429","ts_unix_ms":20,"seq":2},{"kind":"route","primary":"alpha","chain":"alpha,beta","task_type":"chat","ts_unix_ms":10,"seq":1}],"count":2,"next_cursor":""}`)
	out, err = svc.Log(providers.ObservationInput{Limit: 5, Cursor: "20:2", FallbacksOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"events":[],"count":0,"next_cursor":""}`)
}

func TestObservationsRejectionsPreservesOrderWindowAndWireShape(t *testing.T) {
	svc := providers.NewObservations(observationReader{events: []*event.Event{
		observation(event.KindCapabilityRejected, 5, 1, `{"model":"old","capability":"vision"}`),
		observation(event.KindCapabilityRejected, 10, 2, `{"model":"text","capability":"vision"}`),
		observation(event.KindCapabilityRerouted, 10, 3, `{"from_model":"small","to_model":"large","capability":"tool_call"}`),
	}})
	out, err := svc.Rejections(providers.ObservationInput{Limit: 1, CutoffMS: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"rejections":[{"kind":"rerouted","ts_unix_ms":10,"capability":"tool_call","from_model":"small","to_model":"large"}],"count":1}`)
	out, err = svc.Rejections(providers.ObservationInput{Limit: 5, CutoffMS: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"rejections":[{"kind":"rerouted","ts_unix_ms":10,"capability":"tool_call","from_model":"small","to_model":"large"},{"kind":"rejected","ts_unix_ms":10,"capability":"vision","model":"text"}],"count":2}`)
	out, err = svc.Rejections(providers.ObservationInput{Limit: 5, CutoffMS: 11})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"rejections":[],"count":0}`)
}

func TestObservationsPreservesReaderFailureAndIsolation(t *testing.T) {
	cause := errors.New("journal unavailable")
	svc := providers.NewObservations(observationReader{events: []*event.Event{observation(event.KindRoutingDecision, 1, 1, `{"primary":"private"}`)}, err: cause})
	for _, read := range []func(providers.ObservationInput) (map[string]any, error){svc.Log, svc.Stats, svc.Rejections} {
		out, err := read(providers.ObservationInput{Limit: 20})
		if !errors.Is(err, cause) || out != nil {
			t.Fatalf("partial output or lost error: %v, %v", out, err)
		}
	}
	out, err := providers.NewObservations(observationReader{}).Log(providers.ObservationInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertObservationJSON(t, out, `{"events":[],"count":0,"next_cursor":""}`)
}
