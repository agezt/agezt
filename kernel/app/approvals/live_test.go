// SPDX-License-Identifier: MIT

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeBroker struct {
	pending  []approval.Request
	err      error
	resolved []string
}

func (f *fakeBroker) Pending() []approval.Request { return f.pending }
func (f *fakeBroker) Resolve(id string, d approval.Decision, reason, by string) error {
	f.resolved = append(f.resolved, id+"|"+string(d)+"|"+reason+"|"+by)
	return f.err
}

func TestPending(t *testing.T) {
	created, timeout := time.Unix(1_700_000_000, 900_000_000), time.Unix(1_700_000_300, 0)
	b := &fakeBroker{pending: []approval.Request{
		{ID: "a1", Capability: "shell", ToolName: "shell", Input: `{"cmd":"rm"}`, Reason: "L1", Actor: "agent", CorrelationID: "c1", CreatedAt: created, Timeout: timeout,
			EffectClass: "destructive", PredictedEffects: []string{"deletes"}, AffectedResources: []string{"/srv"}, RollbackNotes: "none", Confidence: 0.25,
			CanonicalIntent: "clean", HarmfulInterpretation: "wipe", AmbiguityScore: 0.5, RegretAxes: map[string]float64{"data": 0.9}, ConfirmationPrompt: "sure?"},
		{ID: "a2"},
	}}
	out, err := NewLive(b).Pending(context.Background(), PendingRequest{})
	if err != nil || out.Count != 2 || !reflect.DeepEqual(out.Pending[0], PendingRow{
		ID: "a1", Capability: "shell", ToolName: "shell", Input: `{"cmd":"rm"}`, Reason: "L1", Actor: "agent", CorrelationID: "c1", CreatedUnix: 1_700_000_000, TimeoutUnix: 1_700_000_300,
		EffectClass: "destructive", PredictedEffects: []string{"deletes"}, AffectedResources: []string{"/srv"}, RollbackNotes: "none", Confidence: 0.25,
		CanonicalIntent: "clean", HarmfulInterpretation: "wipe", AmbiguityScore: 0.5, RegretAxes: map[string]float64{"data": 0.9}, ConfirmationPrompt: "sure?"}) {
		t.Fatalf("%+v %v", out, err)
	}
	raw, _ := json.Marshal(out.Pending[1])
	if want := `{"id":"a2","capability":"","tool_name":"","input":"","reason":"","actor":"","correlation_id":"","created_unix":` + jsonInt(time.Time{}.Unix()) + `,"timeout_unix":` + jsonInt(time.Time{}.Unix()) + `,"effect_class":"","predicted_effects":null,"affected_resources":null,"rollback_notes":"","confidence":0,"canonical_intent":"","harmful_interpretation":"","ambiguity_score":0,"regret_axes":null,"confirmation_prompt":""}`; string(raw) != want {
		t.Fatal("unset metadata stays null", string(raw))
	}
	empty, _ := NewLive(&fakeBroker{}).Pending(context.Background(), PendingRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"pending":[],"count":0}` {
		t.Fatal(string(raw))
	}
}

func jsonInt(n int64) string { raw, _ := json.Marshal(n); return string(raw) }

func TestDecide(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{
		`{}`:                               "args.id required",
		`{"id":3,"decision":"grant"}`:      "args.id required",
		`{"id":"","decision":"bogus"}`:     "args.id required",
		`{"id":"a1"}`:                      `args.decision must be "grant" or "deny"`,
		`{"id":"a1","decision":"Grant"}`:   `args.decision must be "grant" or "deny"`,
		`{"id":"a1","decision":["grant"]}`: `args.decision must be "grant" or "deny"`,
	} {
		b := &fakeBroker{}
		if _, err := NewLive(b).Decide(ctx, decode[DecideRequest](t, raw)); err == nil || err.Error() != want || b.resolved != nil {
			t.Fatal(raw, err, b.resolved)
		}
	}
	b := &fakeBroker{}
	out, err := NewLive(b).Decide(ctx, decode[DecideRequest](t, `{"id":" a1 ","decision":"grant","reason":"ok"}`))
	if err != nil || out != (DecideOutput{OK: true, ID: " a1 ", Decision: "grant"}) || !reflect.DeepEqual(b.resolved, []string{" a1 |" + string(approval.DecisionGrant) + "|ok|operator"}) {
		t.Fatal("the id passes untrimmed and the operator resolves it", out, err, b.resolved)
	}
	out, _ = NewLive(b).Decide(ctx, decode[DecideRequest](t, `{"id":"a2","decision":"deny","reason":7}`))
	if out.Decision != "deny" || b.resolved[1] != "a2|"+string(approval.DecisionDeny)+"||operator" {
		t.Fatal("a non-string reason is empty", b.resolved)
	}
	boom := approval.ErrUnknownApproval
	if _, err := NewLive(&fakeBroker{err: boom}).Decide(ctx, decode[DecideRequest](t, `{"id":"gone","decision":"deny"}`)); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestLiveOperations(t *testing.T) {
	if _, err := LiveOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	b := &fakeBroker{pending: []approval.Request{{ID: "a1"}}}
	ops, err := LiveOperations(func(context.Context) *Live { return NewLive(b) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	pending, _ := NewLive(b).Pending(context.Background(), PendingRequest{})
	for i, w := range []struct {
		name     string
		readOnly bool
		http     opapi.HTTP
		out      any
	}{
		{"approvals", true, opapi.HTTP{Method: "GET", Path: "/api/approvals"}, pending},
		{"decide", false, opapi.HTTP{Method: "POST", Path: "/api/decide"}, DecideOutput{OK: true, ID: "a1", Decision: "grant"}},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || spec.ReadOnly != w.readOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
