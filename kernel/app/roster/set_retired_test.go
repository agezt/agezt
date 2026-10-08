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

type setRetiredCalls struct {
	steps     []string
	published []map[string]any
}

type setRetiredOpts struct {
	setErr, pauseStandingErr, pauseSchedulesErr error
	missingBefore                               bool // Get misses although the write succeeds
}

func setRetiredFixture(o setRetiredOpts) (*SetRetiredService, *setRetiredCalls) {
	profiles := map[string]core.Profile{
		"ops":    {Slug: "ops", Enabled: true},
		"orphan": {Slug: "orphan", Retired: true, OwnerAgent: "ghost"},
		"lead":   {Slug: "lead", Enabled: true},
		"child":  {Slug: "child", Retired: true, ParentAgent: "lead"},
	}
	calls := &setRetiredCalls{}
	step := func(s string) { calls.steps = append(calls.steps, s) }
	return NewSetRetired(SetRetiredPorts{
		Get: func(ref string) (core.Profile, bool) {
			step("get:" + ref)
			if o.missingBefore {
				return core.Profile{}, false
			}
			p, ok := profiles[ref]
			return p, ok
		},
		Impact: func(p core.Profile) ImpactOutput {
			step("impact:" + p.Slug)
			return ImpactOutput{Slug: p.Slug, StandingOrders: []string{"order"}, StandingCount: 1, Subagents: []string{}}
		},
		SetRetired: func(ref string, retired bool, reason string) (core.Profile, error) {
			step(fmt.Sprintf("set:%s:%v:%s", ref, retired, reason))
			if o.setErr != nil {
				return core.Profile{}, o.setErr
			}
			p := profiles[strings.TrimSpace(ref)]
			p.Retired, p.RetiredReason, p.RetiredMS = retired, "stored "+reason, 42
			return p, nil
		},
		PauseStanding:        func(slug string) (int, error) { step("pause-standing:" + slug); return 2, o.pauseStandingErr },
		PauseSchedules:       func(slug string) (int, error) { step("pause-schedules:" + slug); return 3, o.pauseSchedulesErr },
		CountPausedStanding:  func(slug string) int { step("count-standing:" + slug); return 4 },
		CountPausedSchedules: func(slug string) int { step("count-schedules:" + slug); return 5 },
		NewCorrelation:       func() string { step("corr"); return "corr-1" },
		Publish: func(subject, corr string, payload map[string]any) {
			step("publish:" + subject + ":" + corr)
			calls.published = append(calls.published, payload)
		},
		Invalidate: func() { step("invalidate") },
	}), calls
}

func setRetiredReq(t *testing.T, raw string) SetRetiredRequest {
	t.Helper()
	var in SetRetiredRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return in
}

func TestRosterRetireReportsImpactBeforeTheChange(t *testing.T) {
	s, calls := setRetiredFixture(setRetiredOpts{})
	out, err := s.Retire(context.Background(), setRetiredReq(t, `{"ref":"ops","reason":"  done  "}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"get:ops", "impact:ops", "set:ops:true:done", "pause-standing:ops", "pause-schedules:ops", "corr", "publish:agent.retire:corr-1", "invalidate"}; !reflect.DeepEqual(calls.steps, want) {
		t.Fatal(calls.steps)
	}
	if out.Profile.Slug != "ops" || !out.Profile.Retired || !reflect.DeepEqual(out.Impact, []string{"order"}) || out.StandingPaused != 2 || out.SchedulesPaused != 3 {
		t.Fatal(out)
	}
	if sum := out.ImpactSummary; sum == nil || sum.Slug != "ops" || sum.StandingCount != 1 || sum.StandingPaused != 2 || sum.SchedulesPaused != 3 {
		t.Fatal("summary", out.ImpactSummary)
	}
	pl := calls.published[0]
	if pl["agent"] != "ops" || pl["reason"] != "stored done" || pl["retired_ms"] != int64(42) || pl["standing_paused"] != 2 || pl["schedules_paused"] != 3 {
		t.Fatal(pl)
	}
	journaled, ok := pl["impact_summary"].(map[string]any)
	if !ok || journaled["standing_paused"] != float64(2) || journaled["schedules_paused"] != float64(3) || journaled["standing_count"] != float64(1) || journaled["slug"] != "ops" || journaled["skills"] != nil {
		t.Fatal("journaled summary is the generic object", pl["impact_summary"])
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"impact_summary":{"slug":"ops","subagents":[],`) || !strings.Contains(string(raw), `"standing_paused":2,"schedules_paused":3},"standing_paused":2`) {
		t.Fatal("wire", string(raw))
	}
	// Lenient reason; a miss before the write leaves the impact null.
	s, calls = setRetiredFixture(setRetiredOpts{missingBefore: true})
	out, err = s.Retire(context.Background(), setRetiredReq(t, `{"ref":"ops","reason":7}`))
	if err != nil || out.Impact != nil || out.ImpactSummary != nil || calls.steps[1] != "set:ops:true:" || calls.published[0]["impact_summary"] != nil {
		t.Fatal("no impact", out, calls.steps, err)
	}
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), `"impact":null,"impact_summary":null`) {
		t.Fatal(string(raw))
	}
}

func TestRosterRetireErrors(t *testing.T) {
	for raw, want := range map[string]string{`{}`: "args.ref required", `{"ref":3}`: "args.ref must be a string", `{"ref":"  "}`: "args.ref required"} {
		s, calls := setRetiredFixture(setRetiredOpts{})
		if _, err := s.Retire(context.Background(), setRetiredReq(t, raw)); err == nil || err.Error() != want || calls.steps != nil {
			t.Fatal(raw, err, calls.steps)
		}
		if _, err := s.Revive(context.Background(), setRetiredReq(t, raw)); err == nil || err.Error() != want || calls.steps != nil {
			t.Fatal(raw, err, calls.steps)
		}
	}
	for cause, want := range map[error]string{fmt.Errorf("wrap: %w", core.ErrNotFound): "unknown agent: ghost", errors.New("disk full"): "disk full"} {
		s, calls := setRetiredFixture(setRetiredOpts{setErr: cause})
		if _, err := s.Retire(context.Background(), setRetiredReq(t, `{"ref":"ghost"}`)); err == nil || err.Error() != want || calls.published != nil {
			t.Fatal(cause, err)
		}
		s, calls = setRetiredFixture(setRetiredOpts{setErr: cause})
		if _, err := s.Revive(context.Background(), setRetiredReq(t, `{"ref":"ghost"}`)); err == nil || err.Error() != want || calls.published != nil {
			t.Fatal(cause, err)
		}
	}
	for _, o := range []setRetiredOpts{{pauseStandingErr: errors.New("p1")}, {pauseSchedulesErr: errors.New("p2")}} {
		s, calls := setRetiredFixture(o)
		if _, err := s.Retire(context.Background(), setRetiredReq(t, `{"ref":"ops"}`)); err == nil || calls.published != nil || strings.Contains(strings.Join(calls.steps, ","), "invalidate") {
			t.Fatal("pause failure", err, calls.steps)
		}
	}
}

func TestRosterRevive(t *testing.T) {
	s, calls := setRetiredFixture(setRetiredOpts{})
	out, err := s.Revive(context.Background(), setRetiredReq(t, `{"ref":"child","reason":" back "}`))
	if err != nil || out.Profile.Retired || out.StandingPaused != 4 || out.SchedulesPaused != 5 {
		t.Fatal(out, err)
	}
	if want := []string{"get:child", "get:lead", "set:child:false:back", "count-standing:child", "count-schedules:child", "corr", "publish:agent.revive:corr-1", "invalidate"}; !reflect.DeepEqual(calls.steps, want) {
		t.Fatal(calls.steps)
	}
	if !reflect.DeepEqual(calls.published[0], map[string]any{"agent": "child", "standing_paused": 4, "schedules_paused": 5}) {
		t.Fatal(calls.published)
	}
	// A revival re-checks the hierarchy before the change.
	s, calls = setRetiredFixture(setRetiredOpts{})
	if _, err := s.Revive(context.Background(), setRetiredReq(t, `{"ref":"orphan"}`)); err == nil || !strings.Contains(err.Error(), "ghost") || strings.Contains(strings.Join(calls.steps, ","), "set:") {
		t.Fatal("revive hierarchy", err, calls.steps)
	}
	// An agent unknown before the change goes straight to the write.
	s, calls = setRetiredFixture(setRetiredOpts{missingBefore: true})
	if _, err := s.Revive(context.Background(), setRetiredReq(t, `{"ref":"orphan"}`)); err != nil || calls.steps[1] != "set:orphan:false:" {
		t.Fatal(err, calls.steps)
	}
}

func TestRosterSetRetiredOperationsAreAudited(t *testing.T) {
	if _, err := SetRetiredOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, calls := setRetiredFixture(setRetiredOpts{})
	ops, err := SetRetiredOperations(func(ctx context.Context) *SetRetiredService {
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, want := range []struct {
		name, path string
		out        reflect.Type
	}{{"agent_retire", "/api/agents/retire", reflect.TypeFor[RetireOutput]()}, {"agent_revive", "/api/agents/revive", reflect.TypeFor[ReviveOutput]()}} {
		spec := ops[i].Spec()
		if spec.Name != want.name || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[SetRetiredRequest]() || spec.Output != want.out || spec.HTTP != (opapi.HTTP{Method: "POST", Path: want.path}) {
			t.Fatal(spec)
		}
	}
	profile := `{"id":"i","slug":"s","enabled":true,"created_ms":0,"updated_ms":0,"kind":"custom","managed":false}`
	if schema.ValidateJSON(ops[1].Spec().OutputSchema, json.RawMessage(`{"profile":`+profile+`,"standing_paused":1,"schedules_paused":2}`)) != nil || schema.ValidateJSON(ops[0].Spec().OutputSchema, json.RawMessage(`{"profile":`+profile+`,"impact":null,"impact_summary":null,"standing_paused":"x","schedules_paused":0}`)) == nil {
		t.Fatal("schemas")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	for _, name := range []string{"agent_retire", "agent_revive"} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"ref":"ops"}`), nil); err == nil || err.Error() != "audit unavailable" || calls.steps != nil {
			t.Fatal("failed audit admission must block the write", name, err, calls.steps)
		}
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_retire", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || calls.steps != nil {
			t.Fatal("non-primary write", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_revive", json.RawMessage(`{"ref":"ops"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.steps != nil {
		t.Fatal("canceled write", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_retire", json.RawMessage(`{"ref":"ops","reason":"r"}`), nil)
	if _, ok := out.(RetireOutput); err != nil || !ok || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_retire" || string(audit.begins[0].Input) != `{"ref":"ops","reason":"r"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_revive", json.RawMessage(`{"ref":"orphan"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil {
		t.Fatal("audited failure", err, audit.ends)
	}
}
