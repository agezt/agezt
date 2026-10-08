// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

type repairActionLaunch struct {
	corr, slug, reason string
	lineage            IncidentLineage
}

type repairActionCalls struct {
	steps     []string
	published []map[string]any
	launches  []repairActionLaunch
}

func repairActionFixture() (*RepairService, *repairActionCalls) {
	managed := false
	profiles := map[string]core.Profile{
		"ops":   {Slug: "ops", Enabled: true},
		"gone":  {Slug: "gone", Enabled: true, Retired: true},
		"off":   {Slug: "off"},
		"sub":   {Slug: "sub", Enabled: true, DirectCallable: &managed, ParentAgent: "lead"},
		"owned": {Slug: "owned", Enabled: true, DirectCallable: &managed, OwnerAgent: "owner"},
	}
	calls := &repairActionCalls{}
	return NewRepair(func(ref string) (core.Profile, bool) {
		calls.steps = append(calls.steps, "get:"+ref)
		p, ok := profiles[strings.ToLower(ref)]
		return p, ok
	}, func() string {
		calls.steps = append(calls.steps, "corr")
		return "corr-1"
	}, func(subject, corr string, payload map[string]any) {
		calls.steps = append(calls.steps, "publish:"+subject+":"+corr)
		calls.published = append(calls.published, payload)
	}, func(corr string, p core.Profile, reason string, lineage IncidentLineage) {
		calls.steps = append(calls.steps, "launch:"+corr)
		calls.launches = append(calls.launches, repairActionLaunch{corr, p.Slug, reason, lineage})
	}), calls
}

func repairActionRun(t *testing.T, s *RepairService, raw string) (RepairOutput, error) {
	t.Helper()
	var in RepairRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.Repair(context.Background(), in)
}

func TestRosterRepairRejectsWithoutEffects(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                          "args.ref required",
		`{"ref":" "}`:                 "args.ref required",
		`{"ref":3}`:                   "args.ref must be a string",
		`{"ref":"ghost","reason":3}`:  "unknown agent: ghost",
		`{"ref":"gone"}`:              "agent gone is retired — revive it first",
		`{"ref":"off"}`:               "agent off is paused",
		`{"ref":"sub","reason":"x"}`:  "agent sub is a managed sub-agent and cannot be repaired directly; wake lead or delegate through it",
		`{"ref":"owned","reason":""}`: "agent owned is a managed sub-agent and cannot be repaired directly; wake owner or delegate through it",
	} {
		s, calls := repairActionFixture()
		if _, err := repairActionRun(t, s, raw); err == nil || err.Error() != want || calls.published != nil || calls.launches != nil {
			t.Fatal(raw, err, calls.steps)
		}
		for _, step := range calls.steps {
			if !strings.HasPrefix(step, "get:") {
				t.Fatal("effect before rejection", raw, calls.steps)
			}
		}
	}
}

func TestRosterRepairJournalsThenLaunches(t *testing.T) {
	s, calls := repairActionFixture()
	out, err := repairActionRun(t, s, `{"ref":"OPS","reason":"  why  ","incident_id":" i ","root_incident_id":"r","parent_incident_id":" p ","intent":"ignored"}`)
	if err != nil || out != (RepairOutput{Accepted: true, Agent: "ops", CorrelationID: "corr-1"}) {
		t.Fatal(out, err)
	}
	if !reflect.DeepEqual(calls.steps, []string{"get:OPS", "corr", "publish:agent.repair:corr-1", "launch:corr-1"}) {
		t.Fatal("order", calls.steps)
	}
	want := map[string]any{"phase": "requested", "agent": "ops", "reason": "why", "incident_id": "i", "root_incident_id": "r", "parent_incident_id": "p"}
	if !reflect.DeepEqual(calls.published, []map[string]any{want}) {
		t.Fatal("requested payload", calls.published)
	}
	if !reflect.DeepEqual(calls.launches, []repairActionLaunch{{"corr-1", "ops", "why", IncidentLineage{"i", "r", "p"}}}) {
		t.Fatal("launch", calls.launches)
	}
	s, calls = repairActionFixture()
	if _, err := repairActionRun(t, s, `{"ref":"ops","reason":7,"incident_id":true,"root_incident_id":{},"parent_incident_id":null}`); err != nil || !reflect.DeepEqual(calls.launches[0], repairActionLaunch{"corr-1", "ops", "", IncidentLineage{}}) || calls.published[0]["reason"] != "" {
		t.Fatal("lenient args", calls.launches, err)
	}
}

func TestRosterRepairOperationIsAudited(t *testing.T) {
	if _, err := RepairOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := repairActionFixture()
	providers := 0
	ops, err := RepairOperations(func(ctx context.Context) *RepairService {
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
	if spec.Name != "agent_repair" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[RepairRequest]() || spec.Output != reflect.TypeFor[RepairOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/repair"}) {
		t.Fatal(spec)
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"accepted":true,"agent":"a","correlation_id":"c"}`)) != nil || schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"accepted":true,"agent":1,"correlation_id":"c"}`)) == nil {
		t.Fatal("output schema")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_repair", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || err.Error() != "audit unavailable" || providers != 0 || calls.steps != nil {
		t.Fatal("failed audit admission must block the repair", err, providers, calls.steps)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_repair", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || calls.steps != nil {
			t.Fatal("non-primary repair", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_repair", json.RawMessage(`{"ref":"ops"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.steps != nil {
		t.Fatal("canceled repair", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_repair", json.RawMessage(`{"ref":"ops","reason":"r"}`), nil)
	if got, ok := out.(RepairOutput); err != nil || !ok || got.CorrelationID != "corr-1" || len(calls.launches) != 1 || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_repair" || string(audit.begins[0].Input) != `{"ref":"ops","reason":"r"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_repair", json.RawMessage(`{"ref":"off"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "agent off is paused" {
		t.Fatal("audited failure", err, audit.ends)
	}
}
