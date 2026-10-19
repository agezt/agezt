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
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/scheduler"
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

// TestPlanRunRefusesTheMissingPlan: the plan JSON is required and must be a
// string.
func TestPlanRunRefusesTheMissingPlan(t *testing.T) {
	p := &planRunner{}
	for _, raw := range []string{``, `null`, `7`, `{}`} {
		_, err := p.runner().Run(context.Background(), PlanRunRequest{PlanJSON: json.RawMessage(raw)}, func(*event.Event) error { return nil })
		if err == nil {
			t.Fatalf("%s: a refused plan answers nothing", raw)
		}
		if raw == `` && err.Error() != "args.plan_json required (JSON string)" {
			t.Fatalf("%s: %v", raw, err)
		}
		if raw != `` && !strings.HasPrefix(err.Error(), "args.plan_json must be a JSON string") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if p.pattern != "" {
		t.Fatal("a refused plan never subscribes", p.pattern)
	}
}

// TestPlanRunRefusesABrokenPlan: a plan that is not JSON is refused with the
// parse prefix, and a plan without nodes is refused as empty.
func TestPlanRunRefusesABrokenPlan(t *testing.T) {
	p := &planRunner{}
	if _, err := p.runner().Run(context.Background(), PlanRunRequest{PlanJSON: json.RawMessage(`"not a plan"`)}, func(*event.Event) error { return nil }); err == nil || !strings.HasPrefix(err.Error(), "plan_json parse: ") {
		t.Fatalf("err=%v", err)
	}
	if _, err := p.runner().Run(context.Background(), PlanRunRequest{PlanJSON: json.RawMessage(`"{\"nodes\":[]}"`)}, func(*event.Event) error { return nil }); err == nil || err.Error() != "plan has no nodes" {
		t.Fatalf("err=%v", err)
	}
}

// TestPlanRunRefusesAnUnknownNodeKind: a node the runner does not know is
// refused with its id and kind.
func TestPlanRunRefusesAnUnknownNodeKind(t *testing.T) {
	p := &planRunner{}
	_, err := p.runner().Run(context.Background(), PlanRunRequest{PlanJSON: planArg(`{"nodes":[{"id":"a","kind":"nope"}]}`)}, func(*event.Event) error { return nil })
	if err == nil || err.Error() != `node "a": unknown kind "nope" (want loop|gate)` {
		t.Fatalf("err=%v", err)
	}
}

// planArg encodes a plan JSON as the string argument a client sends.
func planArg(plan string) json.RawMessage {
	raw, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}
	return raw
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
	out, err := p.runner().Run(app.WithRequestID(context.Background(), "q1"), PlanRunRequest{
		PlanJSON: json.RawMessage(`"{\"nodes\":[{\"id\":\"a\",\"kind\":\"loop\",\"intent\":\"x\"}]}"`),
	}, func(ev *event.Event) error {
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
	_, err := p.runner().Run(context.Background(), PlanRunRequest{
		PlanJSON: json.RawMessage(`"{\"nodes\":[{\"id\":\"a\",\"kind\":\"loop\"}]}"`),
	}, func(*event.Event) error { return nil })
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
	if s.Name != "plan" || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamEvents || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != "/api/plan/run" || s.Output != reflect.TypeFor[PlanRunOutput]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("plan spec: %+v", s)
	}
	if len(s.EmissionSchema) == 0 {
		t.Fatal("plan declares no event frames")
	}
}
