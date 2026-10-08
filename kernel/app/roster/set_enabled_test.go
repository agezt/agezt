// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

type setEnabledCalls struct {
	sets       []string
	counters   int
	invalidate int
}

func setEnabledFixture(fail error) (*SetEnabledService, *setEnabledCalls) {
	calls := &setEnabledCalls{}
	return NewSetEnabled(func(ref string, enabled bool) (core.Profile, error) {
		calls.sets = append(calls.sets, fmt.Sprintf("%s=%v", ref, enabled))
		if fail != nil {
			return core.Profile{}, fail
		}
		return core.Profile{ID: "id-" + ref, Slug: "slug-" + ref, Enabled: enabled, CreatedMS: 9007199254740993}, nil
	}, func(slug string) int { calls.counters++; return len(slug) }, func(slug string) int { calls.counters++; return 2 }, func() { calls.invalidate++ }), calls
}

func setEnabledRun(t *testing.T, s *SetEnabledService, raw string) (SetEnabledOutput, error) {
	t.Helper()
	var in SetEnabledRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.SetEnabled(context.Background(), in)
}

func TestRosterSetEnabledCodecAndEffects(t *testing.T) {
	for raw, want := range map[string]string{
		`{"ref":"a","enabled":true}`: "a=true", `{"ref":"a","enabled":false}`: "a=false", `{"ref":"a","enabled":"TRUE"}`: "a=true", `{"ref":"a","enabled":"1"}`: "a=true",
		`{"ref":"a","enabled":"yes"}`: "a=false", `{"ref":"a","enabled":"0"}`: "a=false", `{"ref":"a","enabled":1}`: "a=false", `{"ref":"a","enabled":null}`: "a=false", `{"ref":"a"}`: "a=false", `{"ref":" a "}`: " a =false",
	} {
		s, calls := setEnabledFixture(nil)
		if _, err := setEnabledRun(t, s, raw); err != nil || !reflect.DeepEqual(calls.sets, []string{want}) || calls.invalidate != 1 {
			t.Fatal(raw, calls.sets, err)
		}
	}
	s, calls := setEnabledFixture(nil)
	resumed, err := setEnabledRun(t, s, `{"ref":"a","enabled":true}`)
	raw, _ := json.Marshal(resumed)
	var wire map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	if err != nil || string(wire["standing_paused"]) != "6" || string(wire["schedules_paused"]) != "2" || calls.counters != 2 {
		t.Fatal("resume reports paused triggers", string(raw), err)
	}
	var profile map[string]json.RawMessage
	json.Unmarshal(wire["profile"], &profile)
	if string(profile["slug"]) != `"slug-a"` || string(profile["kind"]) != `"custom"` || string(profile["managed"]) != "false" || string(profile["created_ms"]) != "9007199254740992" || profile["status"] != nil {
		t.Fatal("legacy profile view", string(wire["profile"]))
	}
	paused, _ := setEnabledRun(t, s, `{"ref":"a","enabled":false}`)
	raw, _ = json.Marshal(paused)
	if paused.StandingPaused != nil || paused.SchedulesPaused != nil || calls.counters != 2 || string(raw) == "" {
		t.Fatal("pause reports no trigger counts", string(raw))
	}
}

func TestRosterSetEnabledErrors(t *testing.T) {
	for raw, want := range map[string]string{`{}`: "args.ref required", `{"ref":"  "}`: "args.ref required", `{"ref":3,"enabled":true}`: "args.ref must be a string"} {
		s, calls := setEnabledFixture(nil)
		if _, err := setEnabledRun(t, s, raw); err == nil || err.Error() != want || calls.sets != nil || calls.invalidate != 0 {
			t.Fatal(raw, err, calls)
		}
	}
	other := errors.New("disk full")
	for cause, want := range map[error]string{
		fmt.Errorf("wrap: %w", core.ErrNotFound): "unknown agent: ghost",
		fmt.Errorf("wrap: %w", core.ErrRetired):  "agent ghost is retired — revive it first",
		other:                                    "disk full",
	} {
		s, calls := setEnabledFixture(cause)
		_, err := setEnabledRun(t, s, `{"ref":"ghost","enabled":true}`)
		if err == nil || err.Error() != want || calls.invalidate != 0 || calls.counters != 0 {
			t.Fatal(cause, err, calls)
		}
	}
}

type recordingAudit struct {
	begins []opapi.AuditRecord
	ends   []error
	fail   error
}

func (a *recordingAudit) Begin(_ context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begins = append(a.begins, record)
	if a.fail != nil {
		return nil, a.fail
	}
	return recordingSpan{a}, nil
}

type recordingSpan struct{ a *recordingAudit }

func (s recordingSpan) End(_ context.Context, err error) error {
	s.a.ends = append(s.a.ends, err)
	return nil
}

func TestRosterSetEnabledOperationIsAudited(t *testing.T) {
	if _, err := SetEnabledOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := setEnabledFixture(nil)
	providers := 0
	ops, err := SetEnabledOperations(func(ctx context.Context) *SetEnabledService {
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
	if spec.Name != "agent_set_enabled" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[SetEnabledRequest]() || spec.Output != reflect.TypeFor[SetEnabledOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/enable"}) {
		t.Fatal(spec)
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"profile":{"id":"i","slug":"s","enabled":true,"created_ms":0,"updated_ms":0,"kind":"custom","managed":false},"standing_paused":"x"}`)) == nil {
		t.Fatal("untyped counter accepted")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_set_enabled", json.RawMessage(`{"ref":"a","enabled":true}`), nil); err == nil || err.Error() != "audit unavailable" || providers != 0 || calls.sets != nil {
		t.Fatal("failed audit admission must block the write", err, providers, calls.sets)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_set_enabled", json.RawMessage(`{"ref":"a"}`), nil); err == nil || calls.sets != nil {
			t.Fatal("non-primary write", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_set_enabled", json.RawMessage(`{"ref":"a"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.sets != nil {
		t.Fatal("canceled write", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_set_enabled", json.RawMessage(`{"ref":"a","enabled":"1"}`), nil)
	if _, ok := out.(SetEnabledOutput); err != nil || !ok || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_set_enabled" || string(audit.begins[0].Input) != `{"ref":"a","enabled":"1"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_set_enabled", json.RawMessage(`{}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "args.ref required" {
		t.Fatal("audited failure", err, audit.ends)
	}
}
