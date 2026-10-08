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

type wakeLaunch struct {
	corr, slug, intent, reason string
	lineage                    IncidentLineage
}

type wakeCalls struct {
	steps     []string
	published []map[string]any
	launches  []wakeLaunch
}

func wakeFixture() (*WakeService, *wakeCalls) {
	managed := false
	profiles := map[string]core.Profile{
		"ops":    {Slug: "ops", Enabled: true, Soul: "s", Lifecycle: core.AgentLifecycle{Mode: core.LifecycleCycle, MaxCycles: 3}},
		"gone":   {Slug: "gone", Enabled: true, Retired: true},
		"off":    {Slug: "off"},
		"sub":    {Slug: "sub", Enabled: true, DirectCallable: &managed, ParentAgent: " lead ", OwnerAgent: "owner"},
		"owned":  {Slug: "owned", Enabled: true, DirectCallable: &managed, OwnerAgent: " owner "},
		"orphan": {Slug: "orphan", Enabled: true, DirectCallable: &managed},
		"retsub": {Slug: "retsub", Retired: true, DirectCallable: &managed},
		"pausub": {Slug: "pausub", DirectCallable: &managed},
	}
	calls := &wakeCalls{}
	return NewWake(func(ref string) (core.Profile, bool) {
		calls.steps = append(calls.steps, "get:"+ref)
		p, ok := profiles[strings.ToLower(ref)]
		return p, ok
	}, func() string {
		calls.steps = append(calls.steps, "corr")
		return "corr-1"
	}, func(subject, corr string, payload map[string]any) {
		calls.steps = append(calls.steps, "publish:"+subject+":"+corr)
		calls.published = append(calls.published, payload)
	}, func(corr string, p core.Profile, intent, reason string, lineage IncidentLineage) {
		calls.steps = append(calls.steps, "launch:"+corr)
		calls.launches = append(calls.launches, wakeLaunch{corr, p.Slug, intent, reason, lineage})
	}), calls
}

func wakeRun(t *testing.T, s *WakeService, raw string) (WakeOutput, error) {
	t.Helper()
	var in WakeRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.Wake(context.Background(), in)
}

func TestRosterWakeRejectsWithoutEffects(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                                    "args.ref required",
		`{"ref":"  ","intent":3}`:               "args.ref required",
		`{"ref":3}`:                             "args.ref must be a string",
		`{"ref":"ghost","intent":3}`:            "unknown agent: ghost",
		`{"ref":"gone","intent":3}`:             "agent gone is retired — revive it first",
		`{"ref":"retsub"}`:                      "agent retsub is retired — revive it first",
		`{"ref":"off","intent":3}`:              "agent off is paused",
		`{"ref":"pausub"}`:                      "agent pausub is paused",
		`{"ref":"sub","intent":3}`:              "agent sub is a managed sub-agent and cannot be called directly; wake lead or delegate through it",
		`{"ref":"owned"}`:                       "agent owned is a managed sub-agent and cannot be called directly; wake owner or delegate through it",
		`{"ref":"orphan"}`:                      "agent orphan is a managed sub-agent and cannot be called directly; route the work through its parent/owner agent",
		`{"ref":"ops","intent":3,"reason":"r"}`: "args.intent must be a string",
		`{"ref":"ops","intent":null}`:           "args.intent must be a string",
		`{"ref":"ops","intent":["x"]}`:          "args.intent must be a string",
	} {
		s, calls := wakeFixture()
		if _, err := wakeRun(t, s, raw); err == nil || err.Error() != want || calls.published != nil || calls.launches != nil {
			t.Fatal(raw, err, calls.steps)
		}
		for _, step := range calls.steps {
			if !strings.HasPrefix(step, "get:") {
				t.Fatal("effect before rejection", raw, calls.steps)
			}
		}
	}
}

func TestRosterWakeJournalsThenLaunches(t *testing.T) {
	s, calls := wakeFixture()
	long := strings.Repeat("é", 300)
	out, err := wakeRun(t, s, `{"ref":"ops","intent":"  `+long+`  ","reason":"  why  ","incident_id":" i ","root_incident_id":" r ","parent_incident_id":" p "}`)
	if err != nil || out != (WakeOutput{Accepted: true, Agent: "ops", CorrelationID: "corr-1"}) {
		t.Fatal(out, err)
	}
	if !reflect.DeepEqual(calls.steps, []string{"get:ops", "corr", "publish:agent.wake:corr-1", "launch:corr-1"}) {
		t.Fatal("order", calls.steps)
	}
	want := map[string]any{
		"phase": "requested", "agent": "ops", "reason": "why", "intent": truncate(long, 240),
		"autonomy_runbook": core.AutonomyRunbook(core.Profile{Slug: "ops", Enabled: true, Soul: "s", Lifecycle: core.AgentLifecycle{Mode: core.LifecycleCycle, MaxCycles: 3}}),
		"incident_id":      "i", "root_incident_id": "r", "parent_incident_id": "p",
	}
	if !reflect.DeepEqual(calls.published, []map[string]any{want}) || !strings.HasSuffix(calls.published[0]["intent"].(string), "…") || len([]rune(calls.published[0]["intent"].(string))) > 241 {
		t.Fatal("requested payload", calls.published)
	}
	if !reflect.DeepEqual(calls.launches, []wakeLaunch{{"corr-1", "ops", long, "why", IncidentLineage{"i", "r", "p"}}}) {
		t.Fatal("launch gets the full intent", calls.launches)
	}
	// The result names the resolved slug, not the caller's ref.
	s, calls = wakeFixture()
	if out, err := wakeRun(t, s, `{"ref":"OPS","reason":"r"}`); err != nil || out.Agent != "ops" || calls.launches[0].slug != "ops" || calls.published[0]["agent"] != "ops" {
		t.Fatal("resolved slug", out, err)
	}
	// Lenient reason and incident ids: anything but a string is empty.
	s, calls = wakeFixture()
	if _, err := wakeRun(t, s, `{"ref":"ops","reason":7,"incident_id":true,"root_incident_id":{},"parent_incident_id":null}`); err != nil {
		t.Fatal(err)
	}
	if calls.published[0]["reason"] != "" || calls.published[0]["incident_id"] != "" || calls.published[0]["root_incident_id"] != "" || calls.published[0]["parent_incident_id"] != "" || calls.launches[0].intent != BuildOperatorWakeIntent("", "ops", "", IncidentLineage{}) {
		t.Fatal("lenient args", calls.published, calls.launches)
	}
}

func TestRosterWakeDefaultIntent(t *testing.T) {
	const tail = "Inspect your durable instructions, memory, mailbox, tasklist, and current health context. Do the next concrete recovery step and then stop."
	for _, tc := range []struct {
		explicit, reason string
		lineage          IncidentLineage
		want             string
	}{
		{" do it ", "r", IncidentLineage{IncidentID: "i"}, "do it"},
		{"  ", "", IncidentLineage{}, "Manual wake-up.\nYou are agent ops. You were explicitly woken by the operator/control plane.\n" + tail},
		{"", " r ", IncidentLineage{IncidentID: " i ", RootIncidentID: " root ", ParentIncidentID: "p"}, "Manual wake-up.\nYou are agent ops. You were explicitly woken by the operator/control plane.\nReason: r\nIncident root: root\nIncident hop: i\n" + tail},
		{"", "", IncidentLineage{RootIncidentID: "root"}, "Manual wake-up.\nYou are agent ops. You were explicitly woken by the operator/control plane.\nIncident root: root\n" + tail},
	} {
		if got := BuildOperatorWakeIntent(tc.explicit, "ops", tc.reason, tc.lineage); got != tc.want {
			t.Fatalf("%q\nwant %q", got, tc.want)
		}
	}
	s, calls := wakeFixture()
	if _, err := wakeRun(t, s, `{"ref":"ops","intent":"   ","reason":"why","root_incident_id":"r"}`); err != nil || calls.launches[0].intent != BuildOperatorWakeIntent("", "ops", "why", IncidentLineage{RootIncidentID: "r"}) {
		t.Fatal("blank intent falls back", calls.launches, err)
	}
}

func TestRosterWakeOperationIsAudited(t *testing.T) {
	if _, err := WakeOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := wakeFixture()
	providers := 0
	ops, err := WakeOperations(func(ctx context.Context) *WakeService {
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
	if spec.Name != "agent_wake" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[WakeRequest]() || spec.Output != reflect.TypeFor[WakeOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/wake"}) {
		t.Fatal(spec)
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"accepted":true,"agent":"a","correlation_id":"c"}`)) != nil || schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"accepted":"yes","agent":"a","correlation_id":"c"}`)) == nil {
		t.Fatal("output schema")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_wake", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || err.Error() != "audit unavailable" || providers != 0 || calls.steps != nil {
		t.Fatal("failed audit admission must block the wake", err, providers, calls.steps)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_wake", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || calls.steps != nil {
			t.Fatal("non-primary wake", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_wake", json.RawMessage(`{"ref":"ops"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.steps != nil {
		t.Fatal("canceled wake", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_wake", json.RawMessage(`{"ref":"ops","reason":"r"}`), nil)
	if got, ok := out.(WakeOutput); err != nil || !ok || got.CorrelationID != "corr-1" || len(calls.launches) != 1 || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_wake" || string(audit.begins[0].Input) != `{"ref":"ops","reason":"r"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_wake", json.RawMessage(`{"ref":"off"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "agent off is paused" {
		t.Fatal("audited failure", err, audit.ends)
	}
}
