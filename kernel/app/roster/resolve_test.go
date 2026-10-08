// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

type resolveCalls struct {
	steps     []string
	published []map[string]any
	lineages  []IncidentLineage
	chains    [][]string
}

type resolveFixtureOpts struct {
	effectErr error
	exhausted []string
	gen       int
	applied   RoutingChainResult
	helpID    string
}

func resolveFixture(o resolveFixtureOpts) (*ResolveService, *resolveCalls) {
	managed := false
	profiles := map[string]core.Profile{
		"ops":   {Slug: "ops", Enabled: true, ParentAgent: " Lead ", OwnerAgent: "owner"},
		"solo":  {Slug: "solo", Enabled: true, OwnerAgent: "Owner"},
		"gone":  {Slug: "gone", Retired: true},
		"lead":  {Slug: "lead", Enabled: true},
		"owner": {Slug: "owner", Enabled: true},
		"peer":  {Slug: "peer", Enabled: true},
		"dead":  {Slug: "dead", Retired: true},
		"sub":   {Slug: "sub", Enabled: true, DirectCallable: &managed, ParentAgent: "lead"},
	}
	calls := &resolveCalls{}
	step := func(s string) { calls.steps = append(calls.steps, s) }
	return NewResolve(ResolvePorts{
		Get: func(ref string) (core.Profile, bool) {
			step("get:" + ref)
			p, ok := profiles[strings.ToLower(ref)]
			return p, ok
		},
		NewCorrelation: func() string { step("corr"); return "corr-1" },
		Publish: func(subject, corr string, payload map[string]any) {
			step("publish:" + subject + ":" + corr + ":" + payload["phase"].(string))
			calls.published = append(calls.published, payload)
		},
		Pause:  func(slug string) error { step("pause:" + slug); return o.effectErr },
		Retire: func(slug, reason string) error { step("retire:" + slug + ":" + reason); return o.effectErr },
		HelpRequest: func(target, text string) (string, error) {
			step("help:" + target + ":" + text)
			return o.helpID, o.effectErr
		},
		ExhaustedChain: func(slug string, lineage IncidentLineage, taskType string) []string {
			step("exhausted:" + slug + ":" + taskType)
			calls.lineages = append(calls.lineages, lineage)
			return o.exhausted
		},
		ForceGeneration: func(slug, taskType string) int { step("gen:" + slug + ":" + taskType); return o.gen },
		ApplyChain: func(slug, taskType string, chain []string, reason string) (RoutingChainResult, error) {
			step("apply:" + slug + ":" + taskType + ":" + reason)
			calls.chains = append(calls.chains, chain)
			return o.applied, o.effectErr
		},
	}), calls
}

func resolveRun(t *testing.T, s *ResolveService, raw string) (ResolveOutput, error) {
	t.Helper()
	var in ResolveRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.Resolve(context.Background(), in)
}

func TestRosterResolveRejectsBeforeJournaling(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                                      "args.ref required",
		`{"ref":3,"resolution":"paused"}`:         "args.ref must be a string",
		`{"ref":"ghost","resolution":"bogus"}`:    "unknown agent: ghost",
		`{"ref":"ops"}`:                           "args.resolution must be paused, retired, delegated, or force_chain",
		`{"ref":"ops","resolution":"PAUSED"}`:     "args.resolution must be paused, retired, delegated, or force_chain",
		`{"ref":"ops","resolution":7}`:            "args.resolution must be paused, retired, delegated, or force_chain",
		`{"ref":"ops","resolution":"forcechain"}`: "args.resolution must be paused, retired, delegated, or force_chain",
	} {
		s, calls := resolveFixture(resolveFixtureOpts{})
		if _, err := resolveRun(t, s, raw); err == nil || err.Error() != want || calls.published != nil {
			t.Fatal(raw, err, calls.steps)
		}
		for _, st := range calls.steps {
			if !strings.HasPrefix(st, "get:") {
				t.Fatal("effect before rejection", raw, calls.steps)
			}
		}
	}
}

func TestRosterResolvePauseAndRetire(t *testing.T) {
	s, calls := resolveFixture(resolveFixtureOpts{})
	out, err := resolveRun(t, s, `{"ref":"OPS","resolution":" paused ","summary":"  s  ","incident_id":" i ","root_incident_id":"r","parent_incident_id":7}`)
	if err != nil || out != (ResolveOutput{Applied: true, Agent: "ops", Resolution: "paused", CorrelationID: "corr-1"}) {
		t.Fatal(out, err)
	}
	if !reflect.DeepEqual(calls.steps, []string{"get:OPS", "corr", "publish:agent.resolve:corr-1:requested", "pause:ops", "publish:agent.resolve:corr-1:completed"}) {
		t.Fatal(calls.steps)
	}
	base := map[string]any{"agent": "ops", "resolution": "paused", "resolution_summary": "s", "incident_id": "i", "root_incident_id": "r", "parent_incident_id": ""}
	with := func(phase string, extra map[string]any) map[string]any {
		m := map[string]any{"phase": phase}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if !reflect.DeepEqual(calls.published, []map[string]any{with("requested", nil), with("completed", nil)}) {
		t.Fatal(calls.published)
	}
	// A retired agent cannot be paused; the failure is journaled with the reason.
	s, calls = resolveFixture(resolveFixtureOpts{})
	if _, err := resolveRun(t, s, `{"ref":"gone","resolution":"paused"}`); err == nil || err.Error() != "agent gone is retired — revive it first" || len(calls.published) != 2 || calls.published[1]["phase"] != "failed" || calls.published[1]["reason"] != err.Error() {
		t.Fatal("retired pause", err, calls.published)
	}
	for _, st := range calls.steps {
		if strings.HasPrefix(st, "pause:") {
			t.Fatal("paused a retired agent")
		}
	}
	s, calls = resolveFixture(resolveFixtureOpts{effectErr: errors.New("disk full")})
	if _, err := resolveRun(t, s, `{"ref":"ops","resolution":"paused"}`); err == nil || err.Error() != "disk full" || calls.published[1]["reason"] != "disk full" {
		t.Fatal("pause error", err)
	}
	for raw, want := range map[string]string{
		`{"ref":"ops","resolution":"retired","summary":" done "}`: "retire:ops:done",
		`{"ref":"gone","resolution":"retired"}`:                   "retire:gone:retired by operator incident resolution",
	} {
		s, calls = resolveFixture(resolveFixtureOpts{})
		if _, err := resolveRun(t, s, raw); err != nil || calls.steps[3] != want {
			t.Fatal(raw, err, calls.steps)
		}
	}
}

func TestRosterResolveDelegation(t *testing.T) {
	for raw, want := range map[string]string{
		`{"ref":"ops","resolution":"delegated"}`:                        "delegated resolution requires delegate_to",
		`{"ref":"ops","resolution":"delegated","delegate_to":3}`:        "delegated resolution requires delegate_to",
		`{"ref":"ops","resolution":"delegated","delegate_to":" OPS "}`:  "delegated resolution points back to the root agent ops",
		`{"ref":"ops","resolution":"delegated","delegate_to":"lead"}`:   "delegated resolution points back to the current owner Lead",
		`{"ref":"solo","resolution":"delegated","delegate_to":"owner"}`: "delegated resolution points back to the current owner Owner",
		`{"ref":"ops","resolution":"delegated","delegate_to":"nobody"}`: "delegated resolution target nobody does not exist",
		`{"ref":"ops","resolution":"delegated","delegate_to":"dead"}`:   "delegated resolution target dead is retired",
		`{"ref":"ops","resolution":"delegated","delegate_to":"SUB"}`:    "delegated resolution target sub is a managed sub-agent",
	} {
		s, calls := resolveFixture(resolveFixtureOpts{})
		if _, err := resolveRun(t, s, raw); err == nil || err.Error() != want || len(calls.published) != 2 || calls.published[1]["phase"] != "failed" || calls.published[1]["reason"] != want {
			t.Fatal(raw, err, calls.published)
		}
		for _, st := range calls.steps {
			if strings.HasPrefix(st, "help:") {
				t.Fatal("posted a refused delegation", raw)
			}
		}
	}
	s, calls := resolveFixture(resolveFixtureOpts{helpID: "  m-1  "})
	// The parent wins over the owner, so the owner is a fine target here.
	if _, err := resolveRun(t, s, `{"ref":"ops","resolution":"delegated","delegate_to":"  owner  "}`); err != nil {
		t.Fatal(err)
	}
	if calls.steps[3] != "get:owner" || calls.steps[4] != "help:owner:Operator delegated this incident for ownership review." {
		t.Fatal(calls.steps)
	}
	if got := calls.published[0]["delegate_to"]; got != "owner" {
		t.Fatal("requested delegate_to", calls.published[0])
	}
	if c := calls.published[1]; c["phase"] != "completed" || c["delegate_to"] != "owner" || c["message_id"] != "m-1" {
		t.Fatal("completed delegation", c)
	}
	s, calls = resolveFixture(resolveFixtureOpts{effectErr: errors.New("board down")})
	if _, err := resolveRun(t, s, `{"ref":"ops","resolution":"delegated","delegate_to":"peer","summary":"look"}`); err == nil || err.Error() != "board down" || calls.steps[4] != "help:peer:look" || calls.published[1]["delegate_to"] != nil {
		t.Fatal("help error", err, calls.steps, calls.published)
	}
}

func TestRosterResolveForceChain(t *testing.T) {
	for raw, want := range map[string]string{
		`{"ref":"ops","resolution":"force_chain","task_model_chain":["m"]}`:                      "force_chain resolution requires task_type and task_model_chain",
		`{"ref":"ops","resolution":"force_chain","task_type":"code"}`:                            "force_chain resolution requires task_type and task_model_chain",
		`{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":[1," "]}`: "force_chain resolution requires task_type and task_model_chain",
		`{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":"m"}`:     "force_chain resolution requires task_type and task_model_chain",
	} {
		s, calls := resolveFixture(resolveFixtureOpts{})
		if _, err := resolveRun(t, s, raw); err == nil || err.Error() != want || calls.lineages != nil || calls.chains != nil {
			t.Fatal(raw, err, calls.steps)
		}
	}
	// A non-empty array is always journaled, even when no model survives.
	s, calls := resolveFixture(resolveFixtureOpts{})
	resolveRun(t, s, `{"ref":"ops","resolution":"force_chain","task_type":" code ","task_model_chain":[1," "]}`)
	if got, ok := calls.published[0]["routing_task_model_chain"].([]string); !ok || got == nil || len(got) != 0 || calls.published[0]["routing_task_type"] != "code" {
		t.Fatal("requested chain", calls.published[0])
	}
	for raw, want := range map[string]any{
		`"m"`: nil, `[]`: nil, `[" m "]`: []string{"m"},
	} {
		s, calls = resolveFixture(resolveFixtureOpts{})
		resolveRun(t, s, `{"ref":"ops","resolution":"paused","task_model_chain":`+raw+`}`)
		if got, ok := calls.published[0]["routing_task_model_chain"]; (want == nil) == ok || (ok && !reflect.DeepEqual(got, want)) {
			t.Fatal("requested chain", raw, calls.published[0])
		}
	}

	s, calls = resolveFixture(resolveFixtureOpts{exhausted: []string{" GPT-5 ", "b"}})
	if _, err := resolveRun(t, s, `{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":["gpt-5","B"],"incident_id":"i","root_incident_id":"r","parent_incident_id":"p"}`); err == nil || err.Error() != "force_chain resolution must choose a new chain for exhausted routing policy" || calls.chains != nil || !reflect.DeepEqual(calls.lineages, []IncidentLineage{{"i", "r", "p"}}) {
		t.Fatal("exhausted chain", err, calls.steps)
	}
	for _, st := range calls.steps {
		if strings.HasPrefix(st, "gen:") {
			t.Fatal("generation read for a refused chain")
		}
	}

	s, calls = resolveFixture(resolveFixtureOpts{exhausted: []string{"gpt-5"}, gen: 2, applied: RoutingChainResult{Previous: []string{"old"}}})
	out, err := resolveRun(t, s, `{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":[" gpt-5 ", 4, "b"],"summary":"why"}`)
	if err != nil || out.Resolution != "force_chain" || !reflect.DeepEqual(calls.chains, [][]string{{"gpt-5", "b"}}) {
		t.Fatal(out, err, calls.chains)
	}
	if !reflect.DeepEqual(calls.steps[3:], []string{"exhausted:ops:code", "gen:ops:code", "apply:ops:code:why", "publish:agent.resolve:corr-1:completed"}) {
		t.Fatal(calls.steps)
	}
	c := calls.published[1]
	if c["routing_task_type"] != "code" || !reflect.DeepEqual(c["routing_task_model_chain"], []string{"gpt-5", "b"}) || !reflect.DeepEqual(c["previous_routing_task_model_chain"], []string{"old"}) || c["routing_force_generation"] != 3 || c["previous_routing_force_generation"] != 2 {
		t.Fatal("completed force payload falls back to the request", c)
	}
	s, calls = resolveFixture(resolveFixtureOpts{applied: RoutingChainResult{TaskType: "code2", Chain: []string{"x"}}})
	resolveRun(t, s, `{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":["y"]}`)
	if c := calls.published[1]; c["routing_task_type"] != "code2" || !reflect.DeepEqual(c["routing_task_model_chain"], []string{"x"}) || c["routing_force_generation"] != 1 {
		t.Fatal("completed force payload prefers the result", c)
	}
	for _, k := range []string{"previous_routing_task_model_chain", "previous_routing_force_generation", "delegate_to", "message_id"} {
		if _, ok := calls.published[1][k]; ok {
			t.Fatal("empty field journaled", k, calls.published[1])
		}
	}
	s, calls = resolveFixture(resolveFixtureOpts{effectErr: fmt.Errorf("routing target for code already matches the current chain")})
	if _, err := resolveRun(t, s, `{"ref":"ops","resolution":"force_chain","task_type":"code","task_model_chain":["y"]}`); err == nil || calls.published[1]["phase"] != "failed" || calls.published[1]["reason"] != err.Error() || calls.published[1]["routing_task_type"] != nil {
		t.Fatal("apply error", err, calls.published)
	}
}

func TestRosterResolveOperationIsAudited(t *testing.T) {
	if _, err := ResolveOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := resolveFixture(resolveFixtureOpts{})
	providers := 0
	ops, err := ResolveOperations(func(ctx context.Context) *ResolveService {
		providers++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "agent_resolve" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[ResolveRequest]() || spec.Output != reflect.TypeFor[ResolveOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/resolve"}) {
		t.Fatal(spec)
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"applied":true,"agent":"a","resolution":"paused","correlation_id":"c"}`)) != nil || schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"applied":true,"agent":"a","correlation_id":"c"}`)) == nil {
		t.Fatal("output schema")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_resolve", json.RawMessage(`{"ref":"ops","resolution":"paused"}`), nil); err == nil || err.Error() != "audit unavailable" || providers != 0 || calls.steps != nil {
		t.Fatal("failed audit admission must block the resolution", err, providers, calls.steps)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_resolve", json.RawMessage(`{"ref":"ops","resolution":"paused"}`), nil); err == nil || calls.steps != nil {
			t.Fatal("non-primary resolution", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_resolve", json.RawMessage(`{"ref":"ops","resolution":"paused"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.steps != nil {
		t.Fatal("canceled resolution", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_resolve", json.RawMessage(`{"ref":"ops","resolution":"paused"}`), nil)
	if got, ok := out.(ResolveOutput); err != nil || !ok || got.Resolution != "paused" || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_resolve" || string(audit.begins[0].Input) != `{"ref":"ops","resolution":"paused"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_resolve", json.RawMessage(`{"ref":"gone","resolution":"paused"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "agent gone is retired — revive it first" {
		t.Fatal("audited failure", err, audit.ends)
	}
}
