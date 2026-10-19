// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/planner"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// authoring is a scripted plan authoring surface.
type authoring struct {
	generated []string
	refined   []string
	raw       string
	nodes     int
	err       error
}

func (a *authoring) service(provider llm.Provider) *PlanAuthoring {
	return &PlanAuthoring{
		provider: func() llm.Provider { return provider },
		generate: func(_ context.Context, cfg planner.Config, intent string) (string, planner.Plan, error) {
			a.generated = append(a.generated, intent+"|"+cfg.Model)
			if a.err != nil {
				return "", planner.Plan{}, a.err
			}
			return a.raw, planner.Plan{Nodes: make([]planner.Node, a.nodes)}, nil
		},
		refine: func(_ context.Context, cfg planner.Config, original planner.Plan, feedback string) (string, planner.Plan, error) {
			a.refined = append(a.refined, feedback+"|"+cfg.Model)
			if a.err != nil {
				return "", planner.Plan{}, a.err
			}
			return a.raw, planner.Plan{Nodes: make([]planner.Node, a.nodes)}, nil
		},
	}
}

func testProvider() llm.Provider {
	prov := mock.New()
	prov.Responder = func(_ llm.CompletionRequest) llm.CompletionResponse { return mock.FinalText("a plan") }
	return prov
}

// TestPlanGenerateRefusesTheMissingIntent: the blank intent is refused
// before the planner runs.
func TestPlanGenerateRefusesTheMissingIntent(t *testing.T) {
	a := &authoring{}
	for _, raw := range []string{``, `""`, `"   "`} {
		out, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(raw)})
		if err == nil || err.Error() != "args.intent required" {
			t.Fatalf("%s: %v, want args.intent required", raw, err)
		}
		if !reflect.DeepEqual(out, PlanOutput{}) {
			t.Fatalf("%s: a refused intent answers nothing: %+v", raw, out)
		}
	}
	// An explicit null is a wrong type, not a missing argument.
	if _, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(`null`)}); err == nil || err.Error() != "args.intent must be a string" {
		t.Fatalf("null: %v, want args.intent must be a string", err)
	}
	if len(a.generated) != 0 {
		t.Fatal("a refused intent never reaches the planner", a.generated)
	}
}

// TestPlanGenerateRefusesANonStringArgument: an argument that is not a string
// is refused with the type error, not the required error.
func TestPlanGenerateRefusesANonStringArgument(t *testing.T) {
	a := &authoring{}
	for _, raw := range []string{`7`, `{}`, `[]`, `true`} {
		if _, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(raw)}); err == nil || err.Error() != "args.intent must be a string" {
			t.Fatalf("%s: %v, want args.intent must be a string", raw, err)
		}
	}
	if _, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(`"do it"`), Model: json.RawMessage(`7`)}); err == nil || err.Error() != "args.model must be a string" {
		t.Fatalf("the model: %v", err)
	}
}

// TestPlanGenerateAnswersThePlan: the plan JSON and the node count answer the
// request, and the model override reaches the planner.
func TestPlanGenerateAnswersThePlan(t *testing.T) {
	a := &authoring{raw: `{"nodes":[]}`, nodes: 3}
	out, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{
		Intent: json.RawMessage(`"do it"`), Model: json.RawMessage(`"fast"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.PlanJSON != `{"nodes":[]}` || out.NodeCount != 3 {
		t.Fatalf("output=%+v", out)
	}
	if a.generated[0] != "do it|fast" {
		t.Fatalf("the planner call: %v", a.generated)
	}
}

// TestPlanGenerateKeepsTheIntentAsGiven: the intent is not trimmed on the way
// to the planner, as before.
func TestPlanGenerateKeepsTheIntentAsGiven(t *testing.T) {
	a := &authoring{raw: `{}`}
	if _, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(`"  do it  "`)}); err != nil {
		t.Fatal(err)
	}
	if a.generated[0] != "  do it  |" {
		t.Fatalf("the intent reaches the planner as given: %q", a.generated[0])
	}
}

// TestPlanGenerateRefusesADaemonWithoutAProvider: the daemon's missing
// provider is the operator's answer, not a planner error.
func TestPlanGenerateRefusesADaemonWithoutAProvider(t *testing.T) {
	a := &authoring{}
	if _, err := a.service(nil).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(`"do it"`)}); err == nil || err.Error() != "daemon has no provider configured" {
		t.Fatalf("err=%v", err)
	}
}

// TestPlanGenerateReportsThePlannerFailure: a planner failure is the
// operation's failure.
func TestPlanGenerateReportsThePlannerFailure(t *testing.T) {
	a := &authoring{err: errors.New("planner: intent required")}
	if _, err := a.service(testProvider()).Generate(context.Background(), PlanGenerateRequest{Intent: json.RawMessage(`"do it"`)}); err == nil || err.Error() != "planner: intent required" {
		t.Fatal(err)
	}
}

// TestPlanRefineRefusesTheMissingArguments: the plan and the feedback are
// both required.
func TestPlanRefineRefusesTheMissingArguments(t *testing.T) {
	a := &authoring{}
	plan := json.RawMessage(`"{\"nodes\":[]}"`)
	for _, raw := range []string{``, `""`, `"   "`} {
		if _, err := a.service(testProvider()).Refine(context.Background(), PlanRefineRequest{PlanJSON: json.RawMessage(raw), Feedback: json.RawMessage(`"change it"`)}); err == nil || err.Error() != "args.plan_json required" {
			t.Fatalf("%s: %v, want args.plan_json required", raw, err)
		}
	}
	for _, raw := range []string{``, `""`, `"   "`} {
		if _, err := a.service(testProvider()).Refine(context.Background(), PlanRefineRequest{PlanJSON: plan, Feedback: json.RawMessage(raw)}); err == nil || err.Error() != "args.feedback required" {
			t.Fatalf("%s: %v, want args.feedback required", raw, err)
		}
	}
	// An explicit null is a wrong type, not a missing argument.
	if _, err := a.service(testProvider()).Refine(context.Background(), PlanRefineRequest{PlanJSON: plan, Feedback: json.RawMessage(`null`)}); err == nil || err.Error() != "args.feedback must be a string" {
		t.Fatalf("null: %v, want args.feedback must be a string", err)
	}
	if len(a.refined) != 0 {
		t.Fatal("a refused revision never reaches the planner", a.refined)
	}
}

// TestPlanRefineRefusesABrokenPlan: a plan that is not JSON is refused with
// the plan_json prefix.
func TestPlanRefineRefusesABrokenPlan(t *testing.T) {
	a := &authoring{}
	_, err := a.service(testProvider()).Refine(context.Background(), PlanRefineRequest{
		PlanJSON: json.RawMessage(`"not a plan"`), Feedback: json.RawMessage(`"change it"`),
	})
	if err == nil || !strings.HasPrefix(err.Error(), "plan_json: ") {
		t.Fatalf("err=%v", err)
	}
}

// TestPlanRefineAnswersTheRevisedPlan: the revised plan JSON and its node
// count answer the request.
func TestPlanRefineAnswersTheRevisedPlan(t *testing.T) {
	a := &authoring{raw: `{"nodes":[{"id":"a"}]}`, nodes: 1}
	out, err := a.service(testProvider()).Refine(context.Background(), PlanRefineRequest{
		PlanJSON: json.RawMessage(`"{\"nodes\":[{\"id\":\"a\"}]}"`), Feedback: json.RawMessage(`"change it"`), Model: json.RawMessage(`"fast"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.PlanJSON != `{"nodes":[{"id":"a"}]}` || out.NodeCount != 1 {
		t.Fatalf("output=%+v", out)
	}
	if a.refined[0] != "change it|fast" {
		t.Fatalf("the planner call: %v", a.refined)
	}
}

// TestPlanAuthoringOperations: the plan authoring surface is primary-only,
// live and audited, on its own routes.
func TestPlanAuthoringOperations(t *testing.T) {
	if _, err := PlanAuthoringOperations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := PlanAuthoringOperations(func(context.Context) *PlanAuthoring { return nil })
	if err != nil || len(ops) != 2 {
		t.Fatal(len(ops), err)
	}
	for i, want := range []struct {
		name, path string
	}{
		{"plan_generate", "/api/plan/generate"},
		{"plan_refine", "/api/plan/refine"},
	} {
		s := ops[i].Spec()
		out, err := schema.FromType(reflect.TypeFor[PlanOutput](), false)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != want.name || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamLive || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != want.path || s.Output != reflect.TypeFor[PlanOutput]() || string(s.OutputSchema) != string(out) {
			t.Fatalf("%s spec: %+v", want.name, s)
		}
		if len(s.EmissionSchema) == 0 {
			t.Fatalf("%s declares no event frames", want.name)
		}
	}
}
