// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// runResponder answers every run with a fixed text.
func runResponder(_ llm.CompletionRequest) llm.CompletionResponse {
	return mock.FinalText("the answer")
}

// fakeTool is a registered tool that does nothing — the run's tool scope is
// what these tests assert, not the tool's behavior.
type fakeTool struct{ name string }

func (f fakeTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: f.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (f fakeTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{}, nil
}

// runKernel opens a kernel with a scripted provider.
func runKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = runResponder
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "run-model",
		Tools:    map[string]toolapi.Tool{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// runKernelWithTools opens a kernel with two registered tools.
func runKernelWithTools(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = runResponder
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "run-model",
		Tools: map[string]toolapi.Tool{
			"shell": fakeTool{name: "shell"},
			"file":  fakeTool{name: "file"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestRunScopesTheOfferedTools: a "tools" argument scopes the RUN, not just
// the plan — the model is offered only the named tools.
func TestRunScopesTheOfferedTools(t *testing.T) {
	k := runKernelWithTools(t)
	var offered []string
	k.Provider().(*mock.Provider).Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		for _, def := range req.Tools {
			offered = append(offered, def.Name)
		}
		return mock.FinalText("the answer")
	}
	service := NewRunService(RunPortsFromKernel(k))
	if _, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"say hi","tools":["shell"]}`), func(*event.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(offered) != 1 || offered[0] != "shell" {
		t.Fatalf("offered tools = %v, want [shell]", offered)
	}
}

// runKernelWithToolsAndCeiling opens a kernel with two registered tools and a
// daemon-wide wall-clock ceiling.
//
// The ceiling is what the dry run reports as the daemon's own timeout, and a
// daemon with NO ceiling cannot say whether the port carried it.
func runKernelWithToolsAndCeiling(t *testing.T) *runtime.Kernel {
	t.Helper()
	prov := mock.New()
	prov.Responder = runResponder
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    prov,
		Model:       "run-model",
		MaxDuration: 5 * time.Minute,
		Tools: map[string]toolapi.Tool{
			"shell": fakeTool{name: "shell"},
			"file":  fakeTool{name: "file"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestRunOperations: the run is an audited, tenant-routed operation that
// streams the run's own event frames.
func TestRunOperations(t *testing.T) {
	if _, err := RunOperations(nil); err == nil {
		t.Fatal("a run provider is required")
	}
	ops, err := RunOperations(func(context.Context) *RunService { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("the run registered %d operations, want 1", len(ops))
	}
	spec := ops[0].Spec()
	if spec.Name != "run" {
		t.Errorf("name = %q, want run", spec.Name)
	}
	if spec.Stream != opapi.StreamEvents {
		t.Errorf("stream = %v, want StreamEvents", spec.Stream)
	}
	if spec.ReadOnly {
		t.Error("a run mutates state — it must be audited")
	}
	if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
		t.Errorf("authz/tenancy = %v/%v, want OwnTenant/CallerTenant", spec.Authz, spec.Tenancy)
	}
	if !spec.AllowUnknownInput {
		t.Error("the run's arguments are read leniently — unknown keys must be tolerated")
	}
	if !strings.Contains(string(spec.OutputSchema), `"type":"object"`) {
		t.Errorf("the output schema is not an object: %s", spec.OutputSchema)
	}
	if !strings.Contains(string(spec.EmissionSchema), `"correlation_id"`) {
		t.Errorf("the emission schema is not the event frame: %s", spec.EmissionSchema)
	}
}

// TestRunResolveRejectsBadArguments: a mistyped flag is a usage error, never a
// silent default — a mistyped dry_run must not execute the run.
func TestRunResolveRejectsBadArguments(t *testing.T) {
	k := runKernel(t)
	service := NewRunService(RunPortsFromKernel(k))
	cases := []struct {
		name string
		in   opapi.RunRequest
		want string
	}{
		{"missing intent", decode[opapi.RunRequest](t, `{}`), "args.intent required"},
		{"blank intent", decode[opapi.RunRequest](t, `{"intent":"  "}`), "args.intent required"},
		{"unknown agent", decode[opapi.RunRequest](t, `{"intent":"x","agent":"ghost"}`), "unknown agent: ghost"},
		{"bad timeout", decode[opapi.RunRequest](t, `{"intent":"x","timeout":"notaduration"}`), "invalid timeout"},
		{"zero timeout", decode[opapi.RunRequest](t, `{"intent":"x","timeout":"0s"}`), "invalid timeout"},
		{"peer without remote profile", decode[opapi.RunRequest](t, `{"intent":"x","remote_peer":"nodeB"}`), `remote_peer requires execution_profile "remote-agezt"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.resolve(context.Background(), tc.in)
			if err == nil {
				t.Fatalf("want the error %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
	// A mistyped field is refused by the row itself, before the service runs:
	// the family's readers read the values, not their types.
	for _, raw := range []string{
		`{"intent":7}`,
		`{"intent":"x","tenant":7}`,
		`{"intent":"x","model":7}`,
		`{"intent":"x","images":"a"}`,
		`{"intent":"x","images":[7]}`,
		`{"intent":"x","system":7}`,
		`{"intent":"x","tools":"shell"}`,
		`{"intent":"x","tools":[7]}`,
		`{"intent":"x","max_cost":"5"}`,
		`{"intent":"x","assure":"2"}`,
		`{"intent":"x","dry_run":"true"}`,
		`{"intent":"x","auto_approve_caps":[7]}`,
	} {
		decodeError[opapi.RunRequest](t, raw)
	}
}

// TestRunDryRunResolvesWithoutRunning: a dry run reports the plan and starts
// no run.
func TestRunDryRunResolvesWithoutRunning(t *testing.T) {
	k := runKernel(t)
	service := NewRunService(RunPortsFromKernel(k))
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"what is the weather","dry_run":true}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.DryRun == nil || !*out.DryRun {
		t.Fatal("the dry run did not report a plan")
	}
	if out.Answer != nil || out.CorrelationID != nil {
		t.Fatalf("the dry run reported a run's answer: %+v", out)
	}
	if out.Intent == nil || *out.Intent != "what is the weather" {
		t.Fatalf("intent = %v", out.Intent)
	}
	if out.Model == nil || *out.Model != "run-model" {
		t.Fatalf("model = %v, want run-model", out.Model)
	}
	if out.ToolsMode == nil || *out.ToolsMode != "all" {
		t.Fatalf("tools_mode = %v, want all", out.ToolsMode)
	}
}

// TestRunDryRunNeverEmitsEvents: the plan is a terminal answer, not a stream.
func TestRunDryRunNeverEmitsEvents(t *testing.T) {
	k := runKernel(t)
	service := NewRunService(RunPortsFromKernel(k))
	emitted := 0
	if _, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","dry_run":true}`), func(*event.Event) error { emitted++; return nil }); err != nil {
		t.Fatal(err)
	}
	if emitted != 0 {
		t.Fatalf("the dry run emitted %d events", emitted)
	}
}

// TestRunExecuteAnswersWithTheRun: a plain run answers with the run's own
// answer and correlation.
func TestRunExecuteAnswersWithTheRun(t *testing.T) {
	k := runKernel(t)
	service := NewRunService(RunPortsFromKernel(k))
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"say hi"}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer == nil || *out.Answer != "the answer" {
		t.Fatalf("answer = %v", out.Answer)
	}
	if out.CorrelationID == nil || !strings.HasPrefix(*out.CorrelationID, "run-") {
		t.Fatalf("correlation_id = %v", out.CorrelationID)
	}
	if out.Agent != nil {
		t.Fatalf("agent = %v, want none for an ad-hoc run", out.Agent)
	}
}

// TestRunExecuteEnrichesTheAnswer: the terminal answer carries the run's own
// accounting when the journal fold knows the run.
func TestRunExecuteEnrichesTheAnswer(t *testing.T) {
	ports := RunPortsFromKernel(runKernel(t))
	ports.Run = func(corr string) (Run, bool, error) {
		return Run{Iters: 3, SpentMicrocents: 42, Model: "run-model"}, true, nil
	}
	service := NewRunService(ports)
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"say hi"}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.Iters == nil || *out.Iters != 3 {
		t.Fatalf("iters = %v, want 3", out.Iters)
	}
	if out.SpentMC == nil || *out.SpentMC != 42 {
		t.Fatalf("spent_mc = %v, want 42", out.SpentMC)
	}
	if out.Model == nil || *out.Model != "run-model" {
		t.Fatalf("model = %v, want run-model", out.Model)
	}
}

// TestRunAnswerStaysBareWhenTheFoldIsUnknown: a fold that does not know the
// run leaves the answer bare — never a wrong number.
func TestRunAnswerStaysBareWhenTheFoldIsUnknown(t *testing.T) {
	ports := RunPortsFromKernel(runKernel(t))
	ports.Run = func(corr string) (Run, bool, error) {
		return Run{Iters: 9, SpentMicrocents: 99}, false, nil
	}
	service := NewRunService(ports)
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"say hi"}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.Iters != nil || out.SpentMC != nil {
		t.Fatalf("the answer carries a foreign run's accounting: %+v", out)
	}
}

// TestRunResolveUsesTheAgentsModel: an agent's own model heads the run unless
// the caller overrides it.
func TestRunResolveUsesTheAgentsModel(t *testing.T) {
	k := runKernel(t)
	if _, err := k.RosterService().Add(roster.Profile{Slug: "modeled", Model: "agent-model"}); err != nil {
		t.Fatal(err)
	}
	service := NewRunService(RunPortsFromKernel(k))
	resolved, err := service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","agent":"modeled"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.model != "agent-model" {
		t.Fatalf("model = %q, want agent-model", resolved.model)
	}
	// An explicit override still wins.
	resolved, err = service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","agent":"modeled","model":"explicit-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.model != "explicit-model" {
		t.Fatalf("model = %q, want explicit-model", resolved.model)
	}
	if !resolved.overridden {
		t.Fatal("an explicit model must be marked as overridden")
	}
}

// TestRunDryRunScopesTheTools: a "tools" argument scopes the run to the named
// tools — the plan reports exactly the tool set the agent loop would see.
func TestRunDryRunScopesTheTools(t *testing.T) {
	k := runKernelWithTools(t)
	service := NewRunService(RunPortsFromKernel(k))
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","tools":["shell"],"dry_run":true}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.ToolsMode == nil || *out.ToolsMode != "restricted" {
		t.Fatalf("tools_mode = %v, want restricted", out.ToolsMode)
	}
	if out.Tools == nil || len(*out.Tools) != 1 || (*out.Tools)[0] != "shell" {
		t.Fatalf("tools = %v, want [shell]", out.Tools)
	}
}

// TestRunDryRunReportsTheModelOverride: the plan reports the model the run
// WOULD use — an override, not the daemon default.
func TestRunDryRunReportsTheModelOverride(t *testing.T) {
	k := runKernel(t)
	service := NewRunService(RunPortsFromKernel(k))
	out, err := service.Execute(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","model":"override-model","dry_run":true}`), func(*event.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out.Model == nil || *out.Model != "override-model" {
		t.Fatalf("model = %v, want override-model", out.Model)
	}
	if out.ModelSource == nil || *out.ModelSource != "per-run (--model)" {
		t.Fatalf("model_source = %v, want per-run (--model)", out.ModelSource)
	}
}

// TestRunResolveUsesTheAgentsExecutionProfile: an agent's own execution
// profile applies to a direct run too — an explicit argument still wins.
func TestRunResolveUsesTheAgentsExecutionProfile(t *testing.T) {
	k := runKernel(t)
	if _, err := k.RosterService().Add(roster.Profile{Slug: "ssh-agent", ExecutionProfile: "ssh"}); err != nil {
		t.Fatal(err)
	}
	service := NewRunService(RunPortsFromKernel(k))
	// The agent's profile is applied, and the inactive SSH backend is refused.
	_, err := service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","agent":"ssh-agent"}`))
	if err == nil || !strings.Contains(err.Error(), "requires an active SSH backend") {
		t.Fatalf("error = %v, want the inactive SSH backend refusal", err)
	}
	// Without the agent, the profile does not apply and the run resolves.
	if _, err := service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x"}`)); err != nil {
		t.Fatalf("the ad-hoc run must resolve: %v", err)
	}
}

// TestRunResolveRejectsAManagedSubAgent: a managed sub-agent is not called
// directly — the work goes through its parent/owner.
func TestRunResolveRejectsAManagedSubAgent(t *testing.T) {
	k := runKernel(t)
	no := false
	if _, err := k.RosterService().Add(roster.Profile{
		Slug:           "worker",
		ParentAgent:    "lead",
		DirectCallable: &no,
	}); err != nil {
		t.Fatal(err)
	}
	service := NewRunService(RunPortsFromKernel(k))
	_, err := service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","agent":"worker"}`))
	if err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("error = %v, want the managed sub-agent rejection", err)
	}
	if !strings.Contains(err.Error(), "lead") {
		t.Fatalf("error = %v, want the manager hint", err)
	}
}

// TestRunResolveRejectsARetiredOrPausedAgent: an agent that is not callable
// now is a usage error, not a silent run as the default identity.
func TestRunResolveRejectsARetiredOrPausedAgent(t *testing.T) {
	k := runKernel(t)
	if _, err := k.RosterService().Add(roster.Profile{Slug: "retired-one", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.RosterService().Add(roster.Profile{Slug: "paused-one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Roster().SetEnabled("paused-one", false); err != nil {
		t.Fatal(err)
	}
	service := NewRunService(RunPortsFromKernel(k))
	for _, tc := range []struct{ agent, want string }{
		{"retired-one", "is retired"},
		{"paused-one", "is paused"},
	} {
		_, err := service.resolve(context.Background(), decode[opapi.RunRequest](t, `{"intent":"x","agent":"`+tc.agent+`"}`))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want it to contain %q", tc.agent, err, tc.want)
		}
	}
}

// TestModelPriced: the authoritative "can a cost cap bind?" check — a priced
// model binds, a KNOWN free/local model does not.
//
// An unrecognised model used to land in the second group, and this test
// asserted it. BIZ-001 moved it: billing an unknown model $0 defeated every
// spend ceiling at once, so it now bills at the unpriced fallback rate — which
// means a cost cap binds for it, and the "cap inert" advisory must no longer
// fire. The cases below are the two groups that remain genuinely distinct.
func TestModelPriced(t *testing.T) {
	for _, priced := range []string{"claude-sonnet-4-6", "totally-unknown-model"} {
		if !modelPriced(priced) {
			t.Errorf("%q should be priced — a cost cap must be able to bind for it", priced)
		}
	}
	for _, free := range []string{"mock", "llama3.2"} {
		if modelPriced(free) {
			t.Errorf("%q is known-free and should price to $0 (cap inert)", free)
		}
	}
}
