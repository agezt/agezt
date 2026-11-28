// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/scheduler"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// fakeSubscription delivers scripted events and counts the cancellation.
type fakeSubscription struct {
	events    chan *event.Event
	cancelled bool
}

func (f *fakeSubscription) Events() <-chan *event.Event { return f.events }
func (f *fakeSubscription) Cancel()                     { f.cancelled = true }

// planRunner is a scripted plan runner.
type planRunner struct {
	sub          *fakeSubscription
	pattern      string
	plan         scheduler.Plan
	planID       string
	result       *scheduler.PlanResult
	err          error
	event        *event.Event
	subscribeErr error
}

func (p *planRunner) runner() *PlanRunner {
	return &PlanRunner{
		loopRunner: func() scheduler.LoopRunner { return nil },
		approvals:  func() *approval.Registry { return nil },
		runPlan: func(_ context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
			p.plan = plan
			p.planID = planID
			if p.sub != nil && p.event != nil {
				p.sub.events <- p.event
			}
			return p.result, p.err
		},
		subscribe: func(pattern string, _ int) (PlanSubscription, error) {
			p.pattern = pattern
			if p.subscribeErr != nil {
				return nil, p.subscribeErr
			}
			return p.sub, nil
		},
	}
}

// TestPlanRunRefusesTheMissingPlan: the plan JSON is required, and the row
// refuses a plan that is not a text.
func TestPlanRunRefusesTheMissingPlan(t *testing.T) {
	p := &planRunner{}
	if _, err := p.runner().Run(context.Background(), opapi.PlanRunRequest{}, func(*event.Event) error { return nil }); err == nil || err.Error() != "args.plan_json required (JSON string)" {
		t.Fatalf("err=%v", err)
	}
	// A plan that was sent as something other than a text is refused by the
	// row itself, before the runner reads it.
	for _, raw := range []string{`{"plan_json":7}`, `{"plan_json":{}}`, `{"plan_json":["a"]}`} {
		decodeError[opapi.PlanRunRequest](t, raw)
	}
	// A plan that was sent empty is the empty text, and the plan reader says
	// so in its own words.
	if _, err := p.runner().Run(context.Background(), decode[opapi.PlanRunRequest](t, `{"plan_json":null}`), func(*event.Event) error { return nil }); err == nil || !strings.HasPrefix(err.Error(), "plan_json parse: ") {
		t.Fatalf("err=%v", err)
	}
	if p.pattern != "" {
		t.Fatal("a refused plan never subscribes", p.pattern)
	}
}

// TestPlanRunRefusesABrokenPlan: a plan that is not JSON is refused with the
// parse prefix, and a plan without nodes is refused as empty.
func TestPlanRunRefusesABrokenPlan(t *testing.T) {
	p := &planRunner{}
	if _, err := p.runner().Run(context.Background(), planRequest(t, "not a plan"), func(*event.Event) error { return nil }); err == nil || !strings.HasPrefix(err.Error(), "plan_json parse: ") {
		t.Fatalf("err=%v", err)
	}
	if _, err := p.runner().Run(context.Background(), planRequest(t, `{"nodes":[]}`), func(*event.Event) error { return nil }); err == nil || err.Error() != "plan has no nodes" {
		t.Fatalf("err=%v", err)
	}
}

// TestPlanRunRefusesAnUnknownNodeKind: a node the runner does not know is
// refused with its id and kind.
func TestPlanRunRefusesAnUnknownNodeKind(t *testing.T) {
	p := &planRunner{}
	_, err := p.runner().Run(context.Background(), planRequest(t, `{"nodes":[{"id":"a","kind":"nope"}]}`), func(*event.Event) error { return nil })
	if err == nil || err.Error() != `node "a": unknown kind "nope" (want loop|gate)` {
		t.Fatalf("err=%v", err)
	}
}

// planRequest builds the plan operation's row from the plan JSON a client
// sends.
func planRequest(t *testing.T, plan string) opapi.PlanRunRequest {
	t.Helper()
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return decode[opapi.PlanRunRequest](t, `{"plan_json":`+string(raw)+`}`)
}

// TestPlanRunStreamsTheEventsAndAnswers: the plan's events reach the caller
// and the terminal answer carries the plan id and the node outputs.
func TestPlanRunStreamsTheEventsAndAnswers(t *testing.T) {
	events := make(chan *event.Event, 4)
	sub := &fakeSubscription{events: events}
	p := &planRunner{sub: sub, event: &event.Event{Kind: event.KindPlanStarted}, result: &scheduler.PlanResult{PlanID: "plan-1", NodeResults: map[string]scheduler.Result{
		"a": {Output: "done"},
	}}}
	var seen []string
	out, err := p.runner().Run(app.WithRequestID(context.Background(), "q1"), planRequest(t, `{"nodes":[{"id":"a","kind":"loop","intent":"x"}]}`), func(ev *event.Event) error {
		seen = append(seen, string(ev.Kind))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("the plan's events never reached the caller")
	}
	if out.PlanID != "plan-1" || len(out.NodeOutputs) != 1 {
		t.Fatalf("output=%+v", out)
	}
	if p.pattern != "plan.>" {
		t.Fatalf("the plan subscribes to %q", p.pattern)
	}
	if p.planID != "plan-1" {
		t.Fatalf("the plan is named after the request: %q", p.planID)
	}
	if !sub.cancelled {
		t.Fatal("the subscription is released")
	}
}

// TestPlanRunNamesThePlanAfterTheRequest: the plan id follows the request id,
// and a request without one leaves the plan unnamed.
func TestPlanRunNamesThePlanAfterTheRequest(t *testing.T) {
	for request, want := range map[string]string{"q1": "plan-1", "abc": "plan-abc", "": "", "q": ""} {
		if got := planIDFor(request); got != want {
			t.Fatalf("planIDFor(%q) = %q, want %q", request, got, want)
		}
	}
}

// TestPlanRunReportsThePlanFailure: a plan failure is the operation's failure.
func TestPlanRunReportsThePlanFailure(t *testing.T) {
	p := &planRunner{sub: &fakeSubscription{events: make(chan *event.Event, 1)}, err: errors.New("plan: node failed")}
	_, err := p.runner().Run(context.Background(), planRequest(t, `{"nodes":[{"id":"a","kind":"loop"}]}`), func(*event.Event) error { return nil })
	if err == nil || err.Error() != "plan: node failed" {
		t.Fatalf("err=%v", err)
	}
}

// TestPlanRunOperations: the plan execution is primary-only, live and audited,
// on its own route.
func TestPlanRunOperations(t *testing.T) {
	if _, err := PlanRunOperations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := PlanRunOperations(func(context.Context) *PlanRunner { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(len(ops), err)
	}
	s := ops[0].Spec()
	out, err := schema.FromType(reflect.TypeFor[PlanRunOutput](), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "plan" || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamEvents || !s.AllowUnknownInput || s.Output != reflect.TypeFor[PlanRunOutput]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("plan spec: %+v", s)
	}
	if len(s.EmissionSchema) == 0 {
		t.Fatal("plan declares no event frames")
	}
}
