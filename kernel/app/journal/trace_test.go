// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeTracer struct {
	why      []*event.Event
	whyErr   error
	causes   []*event.Event
	causeErr error
	parents  map[string]string
	calls    []string
}

func (f *fakeTracer) Why(id string) ([]*event.Event, error) {
	f.calls = append(f.calls, "why:"+id)
	return f.why, f.whyErr
}
func (f *fakeTracer) ParentOf(corr string) string {
	f.calls = append(f.calls, "parent:"+corr)
	return f.parents[corr]
}
func (f *fakeTracer) Causes(id string) ([]*event.Event, error) {
	f.calls = append(f.calls, "causes:"+id)
	return f.causes, f.causeErr
}

func whyRequest(t *testing.T, raw string) WhyRequest {
	t.Helper()
	var in WhyRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestWhy(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{`{}`, `{"event_id":""}`, `{"event_id":3}`, `{"event_id":null}`} {
		f := &fakeTracer{}
		if _, err := NewTrace(f).Why(ctx, whyRequest(t, raw)); err == nil || err.Error() != "args.event_id required" || f.calls != nil {
			t.Fatal(raw, err, f.calls)
		}
	}
	a := &event.Event{ID: "e1", CorrelationID: "run-child"}
	b := &event.Event{ID: "e2", CorrelationID: "run-child"}
	root := &event.Event{ID: "e0", CorrelationID: "pulse-tick"}
	f := &fakeTracer{why: []*event.Event{a, b}, causes: []*event.Event{root, b}, parents: map[string]string{"run-child": "run-lead"}}
	out, err := NewTrace(f).Why(ctx, whyRequest(t, `{"event_id":" e2"}`))
	if err != nil || !reflect.DeepEqual(out, WhyOutput{Events: []*event.Event{a, b}, Correlation: "run-child", ParentCorrelation: "run-lead", CausationChain: []*event.Event{root, b}}) {
		t.Fatalf("%+v %v", out, err)
	}
	if !reflect.DeepEqual(f.calls, []string{"why: e2", "parent:run-child", "causes: e2"}) {
		t.Fatal("the id passes untrimmed", f.calls)
	}
	single := &fakeTracer{why: []*event.Event{a}, causes: []*event.Event{a}}
	if out, _ := NewTrace(single).Why(ctx, whyRequest(t, `{"event_id":"e1"}`)); len(out.CausationChain) != 0 || out.CausationChain == nil || out.ParentCorrelation != "" {
		t.Fatal("a one-event causation chain is not reported, but the list stays empty, not null", out)
	}
	broken := &fakeTracer{why: []*event.Event{a}, causes: []*event.Event{root, a}, causeErr: errors.New("journal unreadable")}
	if out, err := NewTrace(broken).Why(ctx, whyRequest(t, `{"event_id":"e1"}`)); err != nil || len(out.CausationChain) != 0 {
		t.Fatal("a failed causation walk never fails the trace", out, err)
	}
	empty := &fakeTracer{}
	out, err = NewTrace(empty).Why(ctx, whyRequest(t, `{"event_id":"ghost"}`))
	if raw, _ := json.Marshal(out); err != nil || string(raw) != `{"events":[],"correlation":"","parent_correlation":"","causation_chain":[]}` || reflect.DeepEqual(empty.calls, []string{"why:ghost", "parent:", "causes:ghost"}) {
		t.Fatal("an unknown event has no correlation, so no parent lookup", string(raw), err, empty.calls)
	}
	boom := errors.New("event not found")
	if _, err := NewTrace(&fakeTracer{whyErr: boom}).Why(ctx, whyRequest(t, `{"event_id":"x"}`)); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestTraceOperations(t *testing.T) {
	if _, err := TraceOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := TraceOperations(func(context.Context) *Trace { return NewTrace(&fakeTracer{}) })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "why" || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{}) || spec.Output != reflect.TypeOf(WhyOutput{}) {
		t.Fatal(spec)
	}
	out := WhyOutput{Events: []*event.Event{{ID: "e1", Kind: event.KindTaskReceived, Payload: json.RawMessage(`{"intent":"x"}`)}}, CausationChain: []*event.Event{}}
	raw, _ := json.Marshal(out)
	if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
}
