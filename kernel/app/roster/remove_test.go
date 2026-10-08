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

type removeCalls struct {
	steps     []string
	published []map[string]any
}

type removeOpts struct {
	failAt  string // step prefix whose port fails
	removed bool
}

func removeFixture(o removeOpts) (*RemoveService, *removeCalls) {
	profiles := map[string]core.Profile{
		"lead":  {Slug: "lead"},
		"solo":  {Slug: "solo"},
		"guard": {Slug: "guard", System: true},
	}
	children := map[string][]core.Profile{"lead": {{Slug: "a", ParentAgent: "lead"}, {Slug: "b", OwnerAgent: "lead"}}}
	calls := &removeCalls{}
	step := func(s string) error {
		calls.steps = append(calls.steps, s)
		if o.failAt != "" && strings.HasPrefix(s, o.failAt) {
			return errors.New("fail " + s)
		}
		return nil
	}
	n := func(on bool, v int) int {
		if on {
			return v
		}
		return 0
	}
	return NewRemove(RemovePorts{
		Get: func(ref string) (core.Profile, bool) {
			step("get:" + ref)
			p, ok := profiles[ref]
			return p, ok
		},
		Subagents: func(slug string) []core.Profile { step("subagents:" + slug); return children[slug] },
		Retained: func(slug string, subs []core.Profile, include bool) []string {
			step(fmt.Sprintf("retained:%s:%d:%v", slug, len(subs), include))
			return []string{"m1", "m2"}
		},
		Workflows: func(p core.Profile) []string { step("workflows:" + p.Slug); return []string{"wf-" + p.Slug} },
		Retire: func(parent string, kids []core.Profile, on bool) (int, []string, error) {
			err := step(fmt.Sprintf("retire:%s:%d:%v", parent, len(kids), on))
			if !on {
				return 0, nil, err
			}
			return len(kids), []string{"a", "b"}, err
		},
		Standing: func(slug string, on bool) (int, error) {
			err := step(fmt.Sprintf("standing:%s:%v", slug, on))
			return n(on, 1), err
		},
		Schedules: func(slug string, on bool) (int, error) {
			err := step(fmt.Sprintf("schedules:%s:%v", slug, on))
			return n(on, 2), err
		},
		Memory: func(p core.Profile, on bool) (int, error) {
			err := step(fmt.Sprintf("memory:%s:%v", p.Slug, on))
			return n(on, 3), err
		},
		Authored: func(slug string, on bool) (int, error) {
			err := step(fmt.Sprintf("authored:%s:%v", slug, on))
			return n(on, 4), err
		},
		Skills: func(slug string, on bool) (int, error) {
			err := step(fmt.Sprintf("skills:%s:%v", slug, on))
			return n(on, 5), err
		},
		Config: func(slug string, on bool) (int, int, error) {
			err := step(fmt.Sprintf("config:%s:%v", slug, on))
			return n(on, 6), n(on, 7), err
		},
		Workspace: func(p core.Profile, on bool) (int, error) {
			err := step(fmt.Sprintf("workspace:%s:%v", p.Slug, on))
			return n(on, 8), err
		},
		Remove:  func(ref string) (bool, error) { return o.removed, step("remove:" + ref) },
		NewCorr: func() string { step("corr"); return "corr-1" },
		Publish: func(subject, corr string, payload map[string]any) {
			step("publish:" + subject + ":" + corr)
			calls.published = append(calls.published, payload)
		},
		Invalidate: func() { step("invalidate") },
	}), calls
}

func removeRun(t *testing.T, s *RemoveService, raw string) (RemoveOutput, error) {
	t.Helper()
	var in RemoveRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.Remove(context.Background(), in)
}

func TestRosterParseRemoveCascade(t *testing.T) {
	for raw, want := range map[string]RemoveCascade{
		`null`: {}, `"all"`: {}, `[true]`: {}, `{}`: {},
		`{"standing":true,"schedules":"YES","memory":" on ","authored_memory":"1","skills":"true","config":1,"workspace":"no","subagents":false}`: {Standing: true, Schedules: true, Memory: true, AuthoredMemory: true, Skills: true},
		`{"authored_shared_memory":true,"workdir":"on","config":"On","subagents":"1"}`:                                                            {AuthoredMemory: true, Workspace: true, Config: true, Subagents: true},
	} {
		if got := ParseRemoveCascade(json.RawMessage(raw)); got != want {
			t.Fatalf("%s: %+v", raw, got)
		}
	}
	if got := ParseRemoveCascade(nil); got != (RemoveCascade{}) {
		t.Fatal(got)
	}
}

func TestRosterRemoveRejections(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`: "args.ref required", `{"ref":3}`: "args.ref must be a string",
		`{"ref":"guard","cascade":{"subagents":true}}`: "system agent guard cannot be removed; retire or pause it instead",
		`{"ref":"lead"}`: "agent lead has 2 dependent sub-agent(s); set cascade.subagents=true to retire them before removal",
		`{"ref":"lead","cascade":{"subagents":"no"}}`: "agent lead has 2 dependent sub-agent(s); set cascade.subagents=true to retire them before removal",
	} {
		s, calls := removeFixture(removeOpts{removed: true})
		if _, err := removeRun(t, s, raw); err == nil || err.Error() != want || calls.published != nil {
			t.Fatal(raw, err)
		}
		for _, st := range calls.steps {
			if !strings.HasPrefix(st, "get:") && !strings.HasPrefix(st, "subagents:") {
				t.Fatal("effect before rejection", raw, calls.steps)
			}
		}
	}
	s, calls := removeFixture(removeOpts{removed: true})
	out, err := removeRun(t, s, `{"ref":"ghost","cascade":{"standing":true}}`)
	raw, _ := json.Marshal(out)
	if err != nil || string(raw) != `{"removed":false}` || len(calls.steps) != 1 {
		t.Fatal("unknown agent reports only removed:false", string(raw), err, calls.steps)
	}
}

func TestRosterRemoveCascadeOrderAndTotals(t *testing.T) {
	s, calls := removeFixture(removeOpts{removed: true})
	out, err := removeRun(t, s, `{"ref":"lead","cascade":{"standing":true,"schedules":true,"memory":true,"authored_memory":true,"skills":true,"config":true,"workspace":true,"subagents":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"get:lead", "subagents:lead", "retained:lead:2:true", "workflows:lead", "workflows:a", "workflows:b", "retire:lead:2:true",
		"standing:lead:true", "schedules:lead:true", "standing:a:true", "schedules:a:true", "standing:b:true", "schedules:b:true",
		"memory:lead:true", "authored:lead:true", "skills:lead:true", "config:lead:true", "workspace:lead:true",
		"memory:a:true", "authored:a:true", "skills:a:true", "config:a:true", "workspace:a:true",
		"memory:b:true", "authored:b:true", "skills:b:true", "config:b:true", "workspace:b:true",
		"remove:lead", "corr", "publish:agent.remove:corr-1", "invalidate"}
	if !reflect.DeepEqual(calls.steps, want) {
		t.Fatalf("%v\nwant %v", calls.steps, want)
	}
	r := out.RemoveReport
	if !out.Removed || r == nil || r.StandingRemoved != 3 || r.SchedulesRemoved != 6 || r.MemoriesForgotten != 9 || r.AuthoredMemoriesForgotten != 12 || r.SkillsArchived != 15 || r.ConfigsDeleted != 18 || r.ConfigsAccessPruned != 21 || r.WorkspacesDeleted != 24 ||
		r.SubagentsRetired != 2 || !reflect.DeepEqual(r.SubagentsRetiredSlugs, []string{"a", "b"}) || r.MailboxMessagesRetained != 2 || !reflect.DeepEqual(r.MailboxMessagesRetainedRefs, []string{"m1", "m2"}) ||
		r.WorkflowRefsRetained != 1 || !reflect.DeepEqual(r.WorkflowRefsRetainedLabels, []string{"wf-lead"}) || r.SubagentWorkflowRefsRetained != 2 || !reflect.DeepEqual(r.SubagentWorkflowRefsRetainedLabels, []string{"a: wf-a", "b: wf-b"}) {
		t.Fatalf("%+v", r)
	}
	pl := calls.published[0]
	if pl["agent"] != "lead" || pl["removed"] != true || !reflect.DeepEqual(pl["cascade"], map[string]any{"standing": true, "schedules": true, "memory": true, "authored_memory": true, "skills": true, "config": true, "workspace": true, "subagents": true}) || pl["standing_removed"] != float64(3) || len(pl) != 19 {
		t.Fatal(pl)
	}
	raw, _ := json.Marshal(out)
	var wire map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	if len(wire) != 17 || string(wire["subagent_workflow_refs_retained_labels"]) != `["a: wf-a","b: wf-b"]` {
		t.Fatal(string(raw))
	}

	// Each flag reaches only its own port; nothing cascades to sub-agents of a
	// lone agent, and the sub-agent workflow labels stay null.
	for flag, port := range map[string]string{"standing": "standing", "schedules": "schedules", "memory": "memory", "authored_memory": "authored", "skills": "skills", "config": "config", "workspace": "workspace"} {
		s, calls := removeFixture(removeOpts{removed: true})
		out, err := removeRun(t, s, `{"ref":"solo","cascade":{"`+flag+`":true}}`)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range calls.steps {
			parts := strings.Split(st, ":")
			if len(parts) == 3 && parts[2] == "true" && parts[0] != port {
				t.Fatal(flag, "reached", st)
			}
		}
		if !strings.Contains(strings.Join(calls.steps, ","), port+":solo:true") || out.SubagentWorkflowRefsRetainedLabels != nil || out.SubagentsRetiredSlugs != nil {
			t.Fatal(flag, calls.steps, out.RemoveReport)
		}
		raw, _ := json.Marshal(out)
		if !strings.Contains(string(raw), `"subagent_workflow_refs_retained_labels":null`) || !strings.Contains(string(raw), `"subagents_retired_slugs":null`) {
			t.Fatal(string(raw))
		}
	}
}

func TestRosterRemoveStopsAtTheFirstFailure(t *testing.T) {
	for _, at := range []string{"retire:", "standing:lead", "schedules:lead", "standing:a", "schedules:b", "memory:lead", "authored:lead", "skills:lead", "config:lead", "workspace:lead", "memory:b", "authored:a", "skills:b", "config:a", "workspace:b", "remove:"} {
		s, calls := removeFixture(removeOpts{failAt: at, removed: true})
		_, err := removeRun(t, s, `{"ref":"lead","cascade":{"subagents":true}}`)
		last := calls.steps[len(calls.steps)-1]
		if err == nil || err.Error() != "fail "+last || !strings.HasPrefix(last, at) || calls.published != nil {
			t.Fatal(at, err, calls.steps)
		}
	}
	// A profile that vanished is reported, not journaled, and still invalidates.
	s, calls := removeFixture(removeOpts{removed: false})
	out, err := removeRun(t, s, `{"ref":"solo"}`)
	if err != nil || out.Removed || out.RemoveReport == nil || calls.published != nil || calls.steps[len(calls.steps)-1] != "invalidate" {
		t.Fatal(out, err, calls.steps)
	}
}

func TestRosterRemoveOperationIsAudited(t *testing.T) {
	if _, err := RemoveOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := removeFixture(removeOpts{removed: true})
	ops, err := RemoveOperations(func(ctx context.Context) *RemoveService {
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "agent_remove" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[RemoveRequest]() || spec.Output != reflect.TypeFor[RemoveOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/remove"}) {
		t.Fatal(spec)
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"removed":false}`)) != nil || schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"removed":true,"standing_removed":"x"}`)) == nil {
		t.Fatal("output schema")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_remove", json.RawMessage(`{"ref":"solo"}`), nil); err == nil || err.Error() != "audit unavailable" || calls.steps != nil {
		t.Fatal("failed audit admission must block the removal", err, calls.steps)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_remove", json.RawMessage(`{"ref":"solo"}`), nil); err == nil || calls.steps != nil {
			t.Fatal("non-primary removal", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_remove", json.RawMessage(`{"ref":"solo"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.steps != nil {
		t.Fatal("canceled removal", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_remove", json.RawMessage(`{"ref":"solo","cascade":{"memory":true}}`), nil)
	if _, ok := out.(RemoveOutput); err != nil || !ok || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_remove" || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_remove", json.RawMessage(`{"ref":"guard"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil {
		t.Fatal("audited failure", err, audit.ends)
	}
}
