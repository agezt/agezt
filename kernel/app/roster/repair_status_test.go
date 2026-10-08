// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/roster"
)

func repairEvent(seq int64, pl map[string]any) *event.Event {
	raw, _ := json.Marshal(pl)
	return &event.Event{Seq: seq, Kind: event.KindInfo, Subject: "doctor.auto_repair", TSUnixMS: seq * 1000, CorrelationID: "c" + string(rune('0'+seq)), Payload: raw}
}

func repairFixture(p core.Profile, events []*event.Event, now time.Time) (*RepairStatusService, *int) {
	reads := 0
	return NewRepairStatus(func(ref string) (core.Profile, bool) {
		if ref == p.Slug {
			return p, true
		}
		return core.Profile{}, false
	}, func(fn func(*event.Event) error) error {
		reads++
		for _, e := range events {
			_ = fn(e)
		}
		return nil
	}, func() time.Duration { return 10 * time.Second }, func() time.Time { return now }), &reads
}

func repairRun(t *testing.T, s *RepairStatusService, raw string) (RepairStatusOutput, error) {
	t.Helper()
	var in RepairStatusRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.RepairStatus(context.Background(), in)
}

func TestRosterRepairStatusRowsInflightAndPaging(t *testing.T) {
	events := []*event.Event{
		repairEvent(1, map[string]any{"agent": "a", "phase": "queued", "fingerprint": "fp-1", "mode": "degraded", "issues": []any{"bad", " "}}),
		repairEvent(2, map[string]any{"agent": "b", "phase": "queued", "fingerprint": "fp-x"}),
		repairEvent(3, map[string]any{"agent": "a", "phase": "completed", "fingerprint": "fp-1", "applied": []any{"model"}, "chain_depth": float64(2), "routing_task_model_chain": []any{"m1", "m2"}}),
		repairEvent(4, map[string]any{"agent": "a", "phase": "routing_rollback_queued", "fingerprint": "fp-2", "resolution": "force_chain"}),
		{Seq: 5, Kind: event.KindInfo, Subject: "doctor.auto_repair", Payload: json.RawMessage(`{"agent":"a"`)},
		{Seq: 6, Kind: event.KindInfo, Subject: "doctor.other", Payload: json.RawMessage(`{"agent":"a"}`)},
		repairEvent(7, map[string]any{"agent": "a", "phase": "failed", "error": "boom"}),
	}
	s, reads := repairFixture(core.Profile{Slug: "a", Enabled: true}, events, time.UnixMilli(1_000_000))
	out, err := repairRun(t, s, `{"ref":"a"}`)
	if err != nil || *reads != 1 {
		t.Fatal(err, *reads)
	}
	seqs := func(rows []RepairRowOutput) (got []int64) {
		for _, r := range rows {
			got = append(got, r.Seq)
		}
		return got
	}
	if !reflect.DeepEqual(seqs(out.History), []int64{7, 4, 3, 1}) || out.Count != 4 || out.Total != 4 || out.NextCursor != "" || out.Slug != "a" || out.CooldownSec != 10 {
		t.Fatal("history", seqs(out.History), out)
	}
	if !reflect.DeepEqual(seqs(out.Inflight), []int64{4}) || out.InflightCount != 1 {
		t.Fatal("inflight keeps only the latest queued row per fingerprint", seqs(out.Inflight))
	}
	if out.Latest == nil || out.Latest.Seq != 7 || out.NextEligibleMS == nil || *out.NextEligibleMS != 7000+10000 {
		t.Fatal("latest", out.Latest, out.NextEligibleMS)
	}
	row := out.History[2]
	if row.Applied[0] != "model" || row.ChainDepth != 2 || len(row.RoutingTaskModelChain) != 2 || row.Issues != nil || row.CorrelationID != "c3" || row.NextEligibleMS != 13000 {
		t.Fatal("row fields", row)
	}
	if out.History[3].Issues[0] != "bad" || len(out.History[3].Issues) != 1 {
		t.Fatal("issues skip blank entries", out.History[3].Issues)
	}
	if out.NextAction.Action != "wait_inflight" || *out.NextAction.Fingerprint != "fp-2" || *out.NextAction.CorrelationID != "c4" || *out.NextAction.Phase != "routing_rollback_queued" {
		t.Fatal("inflight decision", out.NextAction)
	}
	page, _ := repairRun(t, s, `{"ref":"a","limit":1,"cursor":"7"}`)
	if !reflect.DeepEqual(seqs(page.History), []int64{4}) || page.NextCursor != "4" || page.Count != 1 || page.Total != 4 {
		t.Fatal("cursor page", seqs(page.History), page.NextCursor)
	}
	if !reflect.DeepEqual(page.Latest, out.Latest) || *page.NextEligibleMS != *out.NextEligibleMS || !reflect.DeepEqual(page.NextAction, out.NextAction) || !reflect.DeepEqual(page.Inflight, out.Inflight) {
		t.Fatal("current state moved with the cursor", page.Latest, page.NextAction)
	}
	for raw, want := range map[string][]int64{`{"ref":"a","cursor":" 4 "}`: {3, 1}, `{"ref":"a","cursor":"0"}`: {7, 4, 3, 1}, `{"ref":"a","cursor":4}`: {7, 4, 3, 1}, `{"ref":"a","limit":2}`: {7, 4}, `{"ref":"a","limit":0.5}`: {7, 4, 3, 1}} {
		got, err := repairRun(t, s, raw)
		if err != nil || !reflect.DeepEqual(seqs(got.History), want) {
			t.Fatal(raw, seqs(got.History), err)
		}
	}
	if got, _ := repairRun(t, s, `{"ref":"a","cursor":"1"}`); got.History == nil || len(got.History) != 0 || got.Latest == nil {
		t.Fatal("cursor past the end keeps [] history and current state", got)
	}
	empty, reads2 := repairFixture(core.Profile{Slug: "a", Enabled: true}, nil, time.Now())
	got, _ := repairRun(t, empty, `{"ref":"a"}`)
	raw, _ := json.Marshal(got)
	var wire map[string]json.RawMessage
	json.Unmarshal(raw, &wire)
	if string(wire["history"]) != "[]" || string(wire["inflight"]) != "[]" || wire["latest"] != nil || wire["next_eligible_ms"] != nil || wire["next_cursor"] != nil || *reads2 != 1 {
		t.Fatal("empty shape", string(raw))
	}
	if string(wire["next_action"]) != `{"action":"manual_repair","label":"manual repair","detail":"no autonomous repair is currently queued","tone":"muted"}` {
		t.Fatal("empty decision", string(wire["next_action"]))
	}
}

func TestRosterRepairStatusArgumentsAndLimits(t *testing.T) {
	var events []*event.Event
	for i := int64(1); i <= 150; i++ {
		events = append(events, repairEvent(i, map[string]any{"agent": "a", "phase": "completed"}))
	}
	s, reads := repairFixture(core.Profile{Slug: "a", Enabled: true}, events, time.Now())
	for raw, want := range map[string]string{`{}`: "args.ref required", `{"ref":5}`: "args.ref must be a string", `{"ref":"ghost","limit":"x"}`: "unknown agent: ghost", `{"ref":"a","limit":"x"}`: "args.limit must be a number"} {
		if _, err := repairRun(t, s, raw); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if *reads != 0 {
		t.Fatal("journal read before admission")
	}
	for raw, want := range map[string]int{`{"ref":"a"}`: 20, `{"ref":"a","limit":100}`: 100, `{"ref":"a","limit":150}`: 100, `{"ref":"a","limit":60}`: 60, `{"ref":"a","limit":1}`: 1, `{"ref":"a","limit":-1}`: 20} {
		out, err := repairRun(t, s, raw)
		if err != nil || out.Count != want || out.Total != 150 {
			t.Fatal(raw, out.Count, err)
		}
	}
}

func TestRosterRepairStatusContractAndDecisions(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	contract := repairContract(core.Profile{}, 90*time.Second)
	if !reflect.DeepEqual(contract, RepairContract{RetryAttempts: 1, RetryBackoff: "none", RetryOn: []string{"error", "timeout"}, CooldownSec: 90, AuthorityBoundary: contract.AuthorityBoundary}) || contract.AuthorityBoundary == "" {
		t.Fatal("default contract", contract)
	}
	rich := core.Profile{Enabled: true, RetryPolicy: &core.RetryPolicy{MaxAttempts: 3, Backoff: " exponential ", RetryOn: []string{"timeout"}}, SelfRepairPolicy: &core.SelfRepairPolicy{Enabled: true, MaxAttempts: 2, EscalateTo: " lead "}, HealthPolicy: &core.HealthPolicy{DoctorAgent: " doc ", FailureThreshold: 4}}
	if c := repairContract(rich, time.Minute); c.RetryAttempts != 3 || c.RetryBackoff != "exponential" || c.RetryOn[0] != "timeout" || !c.SelfRepairEnabled || c.SelfRepairAttempts != 2 || c.EscalateTo != "lead" || c.DoctorAgent != "doc" || c.FailureThreshold != 4 || c.CooldownSec != 60 {
		t.Fatal("rich contract", c)
	}
	failed := RepairRow{Seq: 1, Phase: "attempts_exhausted", Mode: "degraded", Fingerprint: "fp", Error: "boom", SelfRepairAttempt: 2, SelfRepairMaxAttempts: 2, NextEligibleMS: 10}
	cases := []struct {
		p    core.Profile
		rows []RepairRow
		want string
	}{
		{core.Profile{Retired: true, Enabled: true}, nil, `{"action":"revive_required","label":"revive required","detail":"graveyard agent cannot repair until revived","tone":"muted"}`},
		{core.Profile{}, nil, `{"action":"resume_required","label":"resume required","detail":"paused agent cannot repair until resumed","tone":"warn"}`},
		{core.Profile{Enabled: true}, []RepairRow{{Phase: "completed", NextEligibleMS: 2_000_000}}, `{"action":"cooldown","label":"cooldown active","detail":"wait before another autonomous repair attempt · phase completed","tone":"warn","fingerprint":"","phase":"completed","next_eligible_ms":2000000}`},
		{core.Profile{Enabled: true, ParentAgent: " boss "}, []RepairRow{failed}, `{"action":"escalate_owner","label":"escalate owner","detail":"self-repair failed; owner should take over · mode degraded · phase attempts_exhausted · fingerprint fp · boom · attempt 2/2","tone":"bad","phase":"attempts_exhausted","delegate_to":"boss"}`},
		{core.Profile{Enabled: true}, []RepairRow{failed}, `{"action":"operator_resolution","label":"operator resolution","detail":"repair failed and no owner escalation target is configured · mode degraded · phase attempts_exhausted · fingerprint fp · boom · attempt 2/2","tone":"bad","phase":"attempts_exhausted"}`},
		{core.Profile{Enabled: true}, []RepairRow{{Phase: "failed", SelfRepairAttempt: 1, SelfRepairMaxAttempts: 3}}, `{"action":"operator_resolution","label":"operator resolution","detail":"repair failed and no owner escalation target is configured · phase failed · attempt 1/3","tone":"bad","phase":"failed"}`},
		{core.Profile{Enabled: true}, []RepairRow{{Phase: "failed", Error: "x"}}, `{"action":"operator_resolution","label":"operator resolution","detail":"repair failed and no owner escalation target is configured · phase failed · x","tone":"bad","phase":"failed"}`},
		{core.Profile{Enabled: true, OwnerAgent: "own"}, []RepairRow{{Phase: " resolution_failed "}}, `{"action":"escalate_owner","label":"escalate owner","detail":"self-repair failed; owner should take over · phase  resolution_failed ","tone":"bad","phase":" resolution_failed ","delegate_to":"own"}`},
		{rich, []RepairRow{{Phase: "completed", Reason: "fixed"}}, `{"action":"run_self_repair","label":"self-repair eligible","detail":"next failure can trigger autonomous self-repair · phase completed · fixed","tone":"good"}`},
		{core.Profile{Enabled: true, HealthPolicy: &core.HealthPolicy{DoctorAgent: "doc"}}, nil, `{"action":"doctor_monitor","label":"doctor monitoring","detail":"doctor can queue repair after health threshold","tone":"good"}`},
	}
	for i, c := range cases {
		raw, _ := json.Marshal(repairNextAction(c.p, c.rows, nil, now.UnixMilli()))
		if string(raw) != c.want {
			t.Errorf("case %d\n got %s\nwant %s", i, raw, c.want)
		}
	}
	delegated := failed
	delegated.DelegateTo = " helper "
	if got := repairNextAction(rich, []RepairRow{delegated}, nil, now.UnixMilli()); *got.DelegateTo != "helper" {
		t.Fatal("row delegate wins over owner", *got.DelegateTo)
	}
	if got := repairNextAction(rich, []RepairRow{failed}, nil, now.UnixMilli()); *got.DelegateTo != "lead" {
		t.Fatal("self-repair escalation target wins over parent", *got.DelegateTo)
	}
	inflight := RepairRow{CorrelationID: "", Fingerprint: "", Phase: "queued"}
	raw, _ := json.Marshal(repairNextAction(core.Profile{Enabled: true}, nil, []RepairRow{inflight}, now.UnixMilli()))
	if string(raw) != `{"action":"wait_inflight","label":"repair in flight","detail":"doctor/self-repair run is already queued · phase queued","tone":"accent","correlation_id":"","fingerprint":"","phase":"queued"}` {
		t.Fatal("inflight keeps present empty fields", string(raw))
	}
}

func TestRosterRepairStatusOperationSpecAndAdmission(t *testing.T) {
	if _, err := RepairStatusOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, reads := repairFixture(core.Profile{Slug: "a", Enabled: true}, []*event.Event{repairEvent(1, map[string]any{"agent": "a", "phase": "completed"})}, time.Now())
	calls := 0
	ops, err := RepairStatusOperations(func(ctx context.Context) *RepairStatusService {
		calls++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "agent_repair_status" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[RepairStatusRequest]() || spec.Output != reflect.TypeFor[RepairStatusOutput]() || spec.HTTP != (opapi.HTTP{Method: "GET", Path: "/api/agents/repair_status"}) {
		t.Fatal(spec)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_repair_status", json.RawMessage(`{"ref":"a"}`), nil); err == nil || calls != 0 {
			t.Fatal("non-primary effects", err)
		}
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_repair_status", json.RawMessage(`{"ref":"a"}`), nil); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled effects", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_repair_status", json.RawMessage(`{"ref":"a","unknown":1}`), nil)
	page, ok := out.(RepairStatusOutput)
	if err != nil || !ok || page.Count != 1 || *reads != 1 || page.Latest == nil || page.Latest.Seq != 1 || page.NextEligibleMS == nil {
		t.Fatal(out, err)
	}
}
