// SPDX-License-Identifier: MIT
package roster

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
	"reflect"
	"testing"
	"time"
)

type listAuth struct{ p opapi.Principal }

func (a listAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.p, nil
}

type listRoute struct{}
type listRouteKey struct{}

func (listRoute) Route(ctx context.Context, _ opapi.Principal, s opapi.Spec) (context.Context, error) {
	if s.Tenancy != opapi.Primary {
		return nil, errors.New("lost primary")
	}
	return context.WithValue(ctx, listRouteKey{}, "selected"), nil
}
func TestRosterListOperationCompleteTypedSchema(t *testing.T) {
	if _, err := ListOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := ListOperations(func(context.Context) *ListService { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	s := ops[0].Spec()
	if s.Name != "agent_list" || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Input != reflect.TypeFor[ListRequest]() || s.Output != reflect.TypeFor[ListOutput]() || s.Stream != opapi.StreamNone || s.Emission != nil || !s.AllowUnknownInput || s.HTTP.Method != "GET" || s.HTTP.Path != "/api/agents" {
		t.Fatal(s)
	}
	var root map[string]any
	json.Unmarshal(s.OutputSchema, &root)
	var lower map[string]any
	rawLower, _ := schema.FromType(reflect.TypeFor[core.Profile](), false)
	json.Unmarshal(rawLower, &lower)
	props := root["properties"].(map[string]any)
	profile := props["profiles"].(map[string]any)["items"].(map[string]any)
	pp := profile["properties"].(map[string]any)
	status := pp["status"].(map[string]any)
	sp := status["properties"].(map[string]any)
	if len(props) != 5 || len(pp) != len(lower["properties"].(map[string]any))+3 || len(sp) != 81 || len(status["required"].([]any)) != 18 {
		t.Fatal("incomplete full shape", len(props), len(pp), len(sp), status["required"])
	}
	for _, key := range []string{"lifecycle", "tasklist", "self_repair", "retry_policy", "health_policy", "noise_policy", "config_overrides", "owner_agent", "direct_callable", "max_cost_mc", "retired_ms", "kind", "managed", "status"} {
		if _, ok := pp[key]; !ok {
			t.Fatal("missing profile field", key)
		}
	}
	for _, raw := range []string{`{"profiles":null,"count":0,"total":0,"enabled_count":0}`, `{"profiles":[],"count":0,"total":0,"enabled_count":0}`} {
		if err := schema.ValidateJSON(s.OutputSchema, json.RawMessage(raw)); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`{"profiles":null,"count":"wrong","total":0,"enabled_count":0}`, `{"profiles":null,"count":0,"total":0}`, `{"profiles":[{"id":"i","slug":"s","enabled":true,"created_ms":0,"updated_ms":0,"kind":"custom","managed":"wrong"}],"count":1,"total":1,"enabled_count":1}`} {
		if schema.ValidateJSON(s.OutputSchema, json.RawMessage(raw)) == nil {
			t.Fatal("untyped result", raw)
		}
	}
}
func TestRosterListTypedProfileStatusCompleteWireAndNumbers(t *testing.T) {
	p := core.Profile{ID: "owned", Slug: "a", Name: "A", Soul: "soul", Instructions: []string{"owned"}, Model: "model", Fallbacks: []string{"fallback"}, TaskType: "code", MaxCostMc: 9007199254740993, MaxDailyMc: 17, CreatedMS: 9007199254740993, UpdatedMS: 23, Enabled: true, ConfigOverrides: map[string]string{"AGEZT_MAX_ITER": "7"}, TaskList: []core.AgentTask{{Title: "owned task", CreatedMS: 9007199254740993}}}
	row := RenderStatus([]core.Profile{p}, richStatusSnapshot())["a"]
	status, err := statusOutput(row)
	if err != nil {
		t.Fatal(err)
	}
	out := ProfileOutput{Profile: p, Kind: p.Kind(), Managed: !p.AllowsDirectCall(), Status: status, StatusPresent: true}
	want := ProfileView(p)
	want["status"] = row
	before, _ := json.Marshal(want)
	after, _ := json.Marshal(out)
	var a, b any
	json.Unmarshal(before, &a)
	json.Unmarshal(after, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal(string(before), string(after))
	}
	var wire map[string]json.RawMessage
	json.Unmarshal(after, &wire)
	if string(wire["created_ms"]) != "9007199254740992" || string(wire["max_cost_mc"]) != "9007199254740992" {
		t.Fatal("legacy profile rounding", string(after))
	}
	var statusWire map[string]json.RawMessage
	json.Unmarshal(wire["status"], &statusWire)
	if string(statusWire["active_spent_mc"]) != "9007199254740993" {
		t.Fatal("status precision", string(after))
	}
	ops, _ := ListOperations(func(context.Context) *ListService { return nil })
	raw, _ := json.Marshal(ListOutput{Profiles: []ProfileOutput{out}, Count: 1, Total: 1, EnabledCount: 1})
	if err := schema.ValidateJSON(ops[0].Spec().OutputSchema, raw); err != nil {
		t.Fatal(err)
	}
	out.Status = nil
	after, _ = json.Marshal(out)
	json.Unmarshal(after, &wire)
	if string(wire["status"]) != "null" {
		t.Fatal("present-null status", string(after))
	}
}
func TestRosterListTypedStatusExplicitNullsAndByteParity(t *testing.T) {
	variants := []func(*StatusSnapshot){
		func(*StatusSnapshot) {},
		func(d *StatusSnapshot) { d.Wakes = map[string]WakeStatus{"a": {ScheduleCount: 1}} },
		func(d *StatusSnapshot) { d.Wakes = map[string]WakeStatus{"a": {EventSubjects: []string{}}} },
		func(d *StatusSnapshot) {
			d.Degraded, d.Misconfigured, d.ForcedExhausted, d.ForcedFailed = nil, nil, nil, nil
			d.Unstable = map[string]UnstableStatus{"a": {Count: 1}}
		},
		func(d *StatusSnapshot) {
			d.Degraded, d.Misconfigured, d.ForcedExhausted = nil, nil, map[string]ForcedStatus{"a": {}}
		},
		func(d *StatusSnapshot) { d.Runbooks = map[string]map[string]any{"a": nil} },
		func(d *StatusSnapshot) { *d = StatusSnapshot{} },
	}
	for i, mutate := range variants {
		d := richStatusSnapshot()
		mutate(&d)
		p := core.Profile{ID: "owned", Slug: "a", Enabled: true}
		row := RenderStatus([]core.Profile{p}, d)["a"]
		status, err := statusOutput(row)
		if err != nil {
			t.Fatal(i, err)
		}
		want, _ := json.Marshal(row)
		got, _ := json.Marshal(status)
		if string(want) != string(got) {
			t.Fatalf("variant %d status bytes\n%s\n%s", i, want, got)
		}
		managed := false
		for _, profile := range []core.Profile{p, {ID: "sub", Slug: "a", Enabled: true, DirectCallable: &managed, OwnerAgent: "owner"}, {ID: "sys", Slug: "a", System: true}} {
			legacy := map[string]any{}
			for slug, status := range RenderStatus([]core.Profile{profile}, d) {
				legacy[slug] = status
			}
			view := ProfileView(profile)
			view["status"] = legacy["a"]
			want, _ = json.Marshal(view)
			out, err := NewList(func() []core.Profile { return []core.Profile{profile} }, func(p []core.Profile) map[string]map[string]any { return RenderStatus(p, d) }, nil).List(context.Background(), ListInput{})
			if err != nil || out.Count != 1 {
				t.Fatal(i, out, err)
			}
			got, _ = json.Marshal(out.Profiles[0])
			if string(want) != string(got) {
				t.Fatalf("variant %d %s profile bytes\n%s\n%s", i, profile.ID, want, got)
			}
		}
	}
	for key, value := range map[string]any{"active_iter": nil, "health_state": nil, "unknown_status": "x"} {
		if _, err := statusOutput(map[string]any{key: value}); err == nil {
			t.Fatal("untyped status accepted", key)
		}
	}
}
func TestRosterListOperationAdmissionAndDelayedCodec(t *testing.T) {
	providers, reads, statuses := 0, 0, 0
	profiles := []core.Profile{{ID: "a", Slug: "a", CreatedMS: 10, Enabled: true}, {ID: "b", Slug: "b", CreatedMS: 20}}
	s := NewList(func() []core.Profile { reads++; return profiles }, func(p []core.Profile) map[string]map[string]any { statuses++; return RenderStatus(p, StatusSnapshot{}) }, nil)
	ops, _ := ListOperations(func(ctx context.Context) *ListService {
		providers++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}, {Kind: opapi.System}} {
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_list", json.RawMessage(`{}`), nil); err == nil || providers != 0 || reads != 0 {
			t.Fatal("non-primary effects", err)
		}
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_list", json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || providers != 0 || reads != 0 {
		t.Fatal("canceled effects", err)
	}
	for _, tc := range []struct{ raw, err string }{{`{"limit":"wrong","cursor":false}`, "args.limit must be a number"}, {`{"limit":null}`, "args.limit must be a number"}, {`{"cursor":null}`, "args.cursor must be a string"}} {
		before := reads
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_list", json.RawMessage(tc.raw), nil)
		if err == nil || err.Error() != tc.err || reads != before+1 || statuses != 1 {
			t.Fatal("codec ordering", tc, err, reads, statuses)
		}
	}
	for _, tc := range []struct {
		raw          string
		count        int
		cursor, next string
	}{{`{"limit":1.9,"cursor":"bad","unknown":true}`, 1, "", "20:b"}, {`{"limit":1}`, 1, "", "20:b"}, {`{"limit":0.5}`, 2, "", ""}, {`{"limit":-1}`, 2, "", ""}, {`{"limit":0}`, 2, "", ""}, {`{"cursor":"20:b"}`, 1, "20:b", ""}, {`{}`, 2, "", ""}} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_list", json.RawMessage(tc.raw), nil)
		page, ok := out.(ListOutput)
		if err != nil || !ok || page.Count != tc.count || page.Total != 2 || page.EnabledCount != 1 || page.NextCursor != tc.next || tc.cursor != "" && page.Profiles[0].Slug != "a" {
			t.Fatal(tc, out, err)
		}
	}
}
func BenchmarkRosterListDispatch(b *testing.B) {
	p := []core.Profile{{ID: "a", Slug: "a", CreatedMS: 1, Enabled: true}}
	s := NewList(func() []core.Profile { return p }, func(p []core.Profile) map[string]map[string]any { return RenderStatus(p, StatusSnapshot{}) }, func() time.Time { return time.Unix(1, 0) })
	ops, err := ListOperations(func(context.Context) *ListService { return s })
	if err != nil {
		b.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_list", json.RawMessage(`{"limit":1}`), nil); err != nil {
			b.Fatal(err)
		}
	}
}
