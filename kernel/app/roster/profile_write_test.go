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
	core "github.com/agezt/agezt/kernel/roster"
)

type profileWriteFixture struct {
	profiles    map[string]core.Profile
	added       []core.Profile
	updates     int
	invalidates int
	addErr      error
	updateErr   error
	updateMiss  bool
}

func newProfileWriteFixture() *profileWriteFixture {
	return &profileWriteFixture{profiles: map[string]core.Profile{
		"a":    {ID: "id-a", Slug: "a", Name: "A", Soul: "soul", Model: "m", MaxCostMc: 5},
		"boss": {ID: "id-boss", Slug: "boss"},
		"gone": {ID: "id-gone", Slug: "gone", Retired: true},
	}}
}

func (f *profileWriteFixture) service() *ProfileWriteService {
	return NewProfileWrite(func(ref string) (core.Profile, bool) {
		p, ok := f.profiles[ref]
		return p, ok
	}, func(p core.Profile) (core.Profile, error) {
		f.added = append(f.added, p)
		if f.addErr != nil {
			return core.Profile{}, f.addErr
		}
		p.ID = "new-id"
		return p, nil
	}, func(ref string, mutate func(*core.Profile)) (core.Profile, bool, error) {
		f.updates++
		if f.updateErr != nil {
			return core.Profile{}, false, f.updateErr
		}
		p, ok := f.profiles[ref]
		if !ok || f.updateMiss {
			return core.Profile{}, false, nil
		}
		mutate(&p)
		f.profiles[ref] = p
		return p, true, nil
	}, func() { f.invalidates++ })
}

func TestRosterProfileAddValidationAndEffects(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                                 "args.profile required",
		`{"profile":"x"}`:                    "args.profile: json: cannot unmarshal string into Go value of type roster.Profile",
		`{"profile":{"max_cost_mc":"many"}}`: "args.profile: json: cannot unmarshal string into Go struct field Profile.max_cost_mc of type int64",
		`{"profile":{"slug":"n","owner_agent":"ghost"}}`:                         `roster: owner_agent "ghost" does not exist`,
		`{"profile":{"slug":"n","parent_agent":"gone"}}`:                         `roster: parent_agent "gone" is retired`,
		`{"profile":{"slug":"n","owner_agent":" N "}}`:                           "roster: owner_agent cannot point to the same agent",
		`{"profile":{"slug":"n","owner_agent":"ghost","parent_agent":"ghost2"}}`: `roster: owner_agent "ghost" does not exist`,
	} {
		f := newProfileWriteFixture()
		for range 5 { // owner is always reported before parent
			if _, err := f.service().Add(context.Background(), addRequest(t, raw)); err == nil || err.Error() != want || f.added != nil || f.invalidates != 0 {
				t.Fatal(raw, err)
			}
		}
	}
	f := newProfileWriteFixture()
	out, err := f.service().Add(context.Background(), addRequest(t, `{"profile":{"slug":"n","kind":" SubAgent ","system":true,"owner_agent":"boss","max_cost_mc":9007199254740993}}`))
	if err != nil || len(f.added) != 1 || f.added[0].System || f.added[0].DirectCallable == nil || *f.added[0].DirectCallable || f.invalidates != 1 {
		t.Fatal("added profile", f.added, err)
	}
	raw, _ := json.Marshal(out)
	var wire map[string]map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	if string(wire["profile"]["kind"]) != `"subagent"` || string(wire["profile"]["managed"]) != "true" || string(wire["profile"]["id"]) != `"new-id"` || string(wire["profile"]["max_cost_mc"]) != "9007199254740992" {
		t.Fatal("legacy profile wire", string(raw))
	}
	f = newProfileWriteFixture()
	f.addErr = errors.New("roster: slug required")
	if _, err := f.service().Add(context.Background(), addRequest(t, `{"profile":null}`)); err == nil || err.Error() != "roster: slug required" || len(f.added) != 1 || f.invalidates != 0 {
		t.Fatal("store error passes through", err)
	}
}

func addRequest(t *testing.T, raw string) AddRequest {
	t.Helper()
	var in AddRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return in
}

func editRun(t *testing.T, f *profileWriteFixture, raw string) (ProfileWriteOutput, error) {
	t.Helper()
	var in EditRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return f.service().Edit(context.Background(), in)
}

func TestRosterProfileEditPatchSemantics(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`: "args.ref required", `{"ref":1,"profile":{}}`: "args.ref must be a string", `{"ref":"a"}`: "args.profile required",
		`{"ref":"a","profile":[1]}`:    "args.profile: json: cannot unmarshal array into Go value of type roster.Profile",
		`{"ref":"ghost","profile":{}}`: "unknown agent: ghost", `{"ref":"a","profile":{"parent_agent":"gone"}}`: `roster: parent_agent "gone" is retired`,
	} {
		f := newProfileWriteFixture()
		if _, err := editRun(t, f, raw); err == nil || err.Error() != want || f.updates != 0 || f.invalidates != 0 {
			t.Fatal(raw, err, f.updates)
		}
	}
	f := newProfileWriteFixture()
	out, err := editRun(t, f, `{"ref":"a","profile":{"name":"","model":"m2","slug":"ignored","system":true,"enabled":true}}`)
	got := f.profiles["a"]
	if err != nil || got.Name != "" || got.Model != "m2" || got.Soul != "soul" || got.MaxCostMc != 5 || got.Slug != "a" || got.System || got.Enabled || f.invalidates != 1 || out.Profile.Slug != "a" {
		t.Fatal("only provided mutable fields change", got, err)
	}
	f = newProfileWriteFixture()
	if _, err := editRun(t, f, `{"ref":"a","profile":{"kind":"subagent"}}`); err != nil || f.profiles["a"].DirectCallable == nil || *f.profiles["a"].DirectCallable {
		t.Fatal("kind subagent applies the managed flag", f.profiles["a"].DirectCallable, err)
	}
	f = newProfileWriteFixture()
	yes := true
	f.profiles["a"] = core.Profile{ID: "id-a", Slug: "a", DirectCallable: &yes}
	if _, err := editRun(t, f, `{"ref":"a","profile":{"kind":"custom"}}`); err != nil || f.profiles["a"].DirectCallable == nil || !*f.profiles["a"].DirectCallable {
		t.Fatal("other kinds leave direct_callable alone", err)
	}
	f = newProfileWriteFixture()
	f.updateMiss = true
	if _, err := editRun(t, f, `{"ref":"a","profile":{}}`); err == nil || err.Error() != "unknown agent: a" || f.invalidates != 0 {
		t.Fatal("update miss", err)
	}
	f = newProfileWriteFixture()
	f.updateErr = errors.New("disk full")
	if _, err := editRun(t, f, `{"ref":"a","profile":{}}`); err == nil || err.Error() != "disk full" || f.invalidates != 0 {
		t.Fatal("update error", err)
	}
}

func TestRosterApplyMutableProfilePatchFieldByField(t *testing.T) {
	no := false
	full := core.Profile{Name: "n", Soul: "s", Instructions: []string{"i"}, Model: "m", Fallbacks: []string{"f"}, TaskType: "t", MaxCostMc: 1, MaxDailyMc: 2, MemoryScope: "ms", Workdir: "w", OwnerAgent: "o", ParentAgent: "p", DirectCallable: &no, RetryPolicy: &core.RetryPolicy{MaxAttempts: 2}, HealthPolicy: &core.HealthPolicy{FailureThreshold: 3}, SelfRepairPolicy: &core.SelfRepairPolicy{Enabled: true}, NoisePolicy: &core.NoisePolicy{}, ToolAllow: []string{"a"}, ToolDeny: []string{"d"}, TrustCeiling: "tc", ExecutionProfile: " ep ", ConfigOverrides: map[string]string{"k": "v"}, Lifecycle: core.AgentLifecycle{Mode: "lm"}, TaskList: []core.AgentTask{{Title: "x"}}, Description: "desc"}
	raw, _ := json.Marshal(full)
	var keys map[string]json.RawMessage
	json.Unmarshal(raw, &keys)
	for key := range keys {
		switch key {
		case "id", "slug", "enabled", "created_ms", "updated_ms", "system", "retired", "retired_ms", "retired_reason":
			continue
		}
		var dst core.Profile
		ApplyMutableProfilePatch(&dst, full, map[string]bool{key: true})
		gotRaw, _ := json.Marshal(dst)
		var got map[string]json.RawMessage
		json.Unmarshal(gotRaw, &got)
		want := string(keys[key])
		if key == "execution_profile" {
			want = `"ep"`
		}
		if string(got[key]) != want {
			t.Errorf("provided %q copied %s, want %s", key, got[key], want)
		}
		var others core.Profile
		ApplyMutableProfilePatch(&others, full, map[string]bool{"unrelated": true, strings.ToUpper(key): true})
		if !reflect.DeepEqual(others, core.Profile{}) {
			t.Errorf("unprovided %q changed the profile", key)
		}
	}
	var trimmed core.Profile
	ApplyMutableProfilePatch(&trimmed, full, map[string]bool{"execution_profile": true})
	if trimmed.ExecutionProfile != "ep" {
		t.Fatal("execution profile trimmed", trimmed.ExecutionProfile)
	}
}

func TestRosterProfileWriteOperationsAreAudited(t *testing.T) {
	if _, err := ProfileWriteOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	f := newProfileWriteFixture()
	ops, err := ProfileWriteOperations(func(ctx context.Context) *ProfileWriteService {
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return f.service()
	})
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, want := range []struct{ name, path string }{{"agent_add", "/api/agents/add"}, {"agent_edit", "/api/agents/edit"}} {
		spec := ops[i].Spec()
		if spec.Name != want.name || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || len(spec.OutputSchema) == 0 || spec.HTTP != (opapi.HTTP{Method: "POST", Path: want.path}) {
			t.Fatal(spec)
		}
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_add", json.RawMessage(`{"profile":{"slug":"n"}}`), nil); err == nil || f.added != nil {
		t.Fatal("failed audit admission must block add", err)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_edit", json.RawMessage(`{"ref":"a","profile":{"model":"x"}}`), nil); err == nil || f.updates != 0 {
		t.Fatal("failed audit admission must block edit", err)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_add", json.RawMessage(`{"profile":{"slug":"n"}}`), nil); err == nil || f.added != nil {
			t.Fatal("non-primary add", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_add", json.RawMessage(`{"profile":{"slug":"n"}}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || f.added != nil {
		t.Fatal("canceled add", err)
	}
	if out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_add", json.RawMessage(`{"profile":{"slug":"n"}}`), nil); err != nil || len(f.added) != 1 || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited add", out, err)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_edit", json.RawMessage(`{"ref":"ghost","profile":{}}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "unknown agent: ghost" {
		t.Fatal("audited edit failure", err, audit.ends)
	}
}
