// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeJournal struct {
	events []*event.Event
	err    error
}

func (f fakeJournal) Range(fn func(*event.Event) error) error {
	for _, e := range f.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return f.err
}

var clock = time.UnixMilli(10_000)

func decisions() fakeJournal {
	ev := func(seq, ts int64, kind event.Kind, actor, corr, payload string) *event.Event {
		return &event.Event{Seq: seq, TSUnixMS: ts, Kind: kind, Actor: actor, CorrelationID: corr, Payload: json.RawMessage(payload)}
	}
	return fakeJournal{events: []*event.Event{
		ev(1, 1_000, event.KindPolicyDecision, "agent", "c1", `{"tool":"shell","capability":"shell","allow":true,"reason":"L4"}`),
		ev(2, 2_000, event.KindPolicyDecision, "agent", "c2", `{"tool":"shell","capability":"shell","allow":false,"reason":"hard deny","hard_denied":true}`),
		ev(3, 3_000, event.KindPolicyChanged, "operator", "", `{"action":"mode.set"}`),
		ev(4, 8_000, event.KindPolicyDecision, "agent", "c3", `{"tool":"http","capability":"http.post","allow":false,"reason":"L0"}`),
		ev(5, 9_000, event.KindPolicyDecision, "agent", "c4", `{"tool":"http","capability":"http.post","allow":"yes","reason":"bad"}`),
		ev(6, 9_500, event.KindPolicyDecision, "agent", "c5", ``),
	}}
}

func TestDecisionLog(t *testing.T) {
	ctx := context.Background()
	d := NewDecisions(decisions(), func() time.Time { return clock })
	out, err := d.Log(ctx, DecisionLogRequest{})
	if err != nil || out.Count != 5 || out.NextCursor != "" || len(out.Decisions) != 5 {
		t.Fatalf("%+v %v", out, err)
	}
	if out.Decisions[0].Seq != 6 || out.Decisions[4].Seq != 1 {
		t.Fatal("newest first", out.Decisions)
	}
	if out.Decisions[1] != (DecisionRow{Actor: "agent", CorrelationID: "c4", TSUnixMS: 9_000, Seq: 5}) {
		t.Fatal("a malformed field zeroes the whole decision", out.Decisions[1])
	}
	if out.Decisions[3] != (DecisionRow{Actor: "agent", CorrelationID: "c2", Tool: "shell", Capability: "shell", Reason: "hard deny", HardDenied: true, TSUnixMS: 2_000, Seq: 2}) {
		t.Fatal(out.Decisions[3])
	}
	denied, _ := d.Log(ctx, decode[DecisionLogRequest](t, `{"denied":true}`))
	if denied.Count != 4 {
		t.Fatal("denied keeps the denials, malformed ones included", denied)
	}
	scoped, _ := d.Log(ctx, decode[DecisionLogRequest](t, `{"tool":"http","capability":"http.post"}`))
	if scoped.Count != 1 || scoped.Decisions[0].Seq != 4 {
		t.Fatal(scoped)
	}
	window, _ := d.Log(ctx, decode[DecisionLogRequest](t, `{"since_ms":2500.9}`))
	if window.Count != 3 {
		t.Fatal("the window is the daemon clock minus a truncated since_ms", window)
	}
	paged, _ := d.Log(ctx, decode[DecisionLogRequest](t, `{"limit":2.7}`))
	if paged.Count != 2 || paged.NextCursor == "" {
		t.Fatal(paged)
	}
	next, _ := d.Log(ctx, decode[DecisionLogRequest](t, `{"limit":2,"cursor":"`+paged.NextCursor+`"}`))
	if next.Count != 2 || next.Decisions[0].Seq != 4 {
		t.Fatal(next)
	}
	for _, raw := range []string{`{"limit":0}`, `{"limit":-3}`, `{"limit":"5"}`} {
		got, _ := d.Log(ctx, decode[DecisionLogRequest](t, raw))
		if want := map[string]int{`{"limit":0}`: 1, `{"limit":-3}`: 1, `{"limit":"5"}`: 5}[raw]; got.Count != want {
			t.Fatal(raw, got.Count)
		}
	}
	for raw, want := range map[string]string{
		`{"denied":"yes"}`:              "args.denied must be a boolean",
		`{"denied":null,"tool":3}`:      "args.denied must be a boolean",
		`{"tool":3,"capability":3}`:     "args.tool must be a string",
		`{"capability":null}`:           "args.capability must be a string",
		`{"capability":3,"cursor":"x"}`: "args.capability must be a string",
	} {
		if _, err := d.Log(ctx, decode[DecisionLogRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	empty, _ := NewDecisions(fakeJournal{}, time.Now).Log(ctx, DecisionLogRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"decisions":[],"count":0,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := NewDecisions(fakeJournal{err: boom}, time.Now).Log(ctx, DecisionLogRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestDecisionStats(t *testing.T) {
	ctx := context.Background()
	d := NewDecisions(decisions(), func() time.Time { return clock })
	out, err := d.Stats(ctx, DecisionStatsRequest{})
	if err != nil || !reflect.DeepEqual(out, DecisionStatsOutput{Total: 5, Allowed: 1, Denied: 4, HardDenied: 1, DenialRate: 0.8, DeniedByCapability: map[string]int{"shell": 1, "http.post": 1, "unknown": 2}}) {
		t.Fatalf("%+v %v", out, err)
	}
	window, _ := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"since_ms":2500.9}`))
	if window.Total != 3 || window.WindowMS != 2500 {
		t.Fatal(window)
	}
	negative, _ := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"since_ms":-5}`))
	if negative.Total != 5 || negative.WindowMS != -5 {
		t.Fatal("a non-positive window is all-time but echoed", negative)
	}
	scoped, _ := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"tool":"shell"}`))
	if scoped.Total != 2 || scoped.DenialRate != 0.5 || !reflect.DeepEqual(scoped.DeniedByCapability, map[string]int{"shell": 1}) {
		t.Fatal(scoped)
	}
	byCap, _ := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"capability":"http.post"}`))
	if byCap.Total != 1 {
		t.Fatal(byCap)
	}
	if _, err := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"since_ms":"x","tool":3}`)); err == nil || err.Error() != "args.tool must be a string" {
		t.Fatal(err)
	}
	if _, err := d.Stats(ctx, decode[DecisionStatsRequest](t, `{"capability":[1]}`)); err == nil || err.Error() != "args.capability must be a string" {
		t.Fatal(err)
	}
	empty, _ := NewDecisions(fakeJournal{}, time.Now).Stats(ctx, DecisionStatsRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"total":0,"allowed":0,"denied":0,"hard_denied":0,"denial_rate":0,"denied_by_capability":{},"window_ms":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := NewDecisions(fakeJournal{err: boom}, time.Now).Stats(ctx, DecisionStatsRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestDecisionOperations(t *testing.T) {
	if _, err := DecisionOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	d := NewDecisions(decisions(), func() time.Time { return clock })
	ops, err := DecisionOperations(func(context.Context) *Decisions { return d })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	log, _ := d.Log(context.Background(), DecisionLogRequest{})
	stats, _ := d.Stats(context.Background(), DecisionStatsRequest{})
	for i, w := range []struct {
		name, path string
		out        any
	}{
		{"edict_log", "/api/policy_log", log},
		{"edict_stats", "/api/policy", stats},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: "GET", Path: w.path}) || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}

func TestDecisionLogLimits(t *testing.T) {
	var j fakeJournal
	for i := range 1_005 {
		j.events = append(j.events, &event.Event{Seq: int64(i + 1), TSUnixMS: int64(i + 1), Kind: event.KindPolicyDecision, Payload: json.RawMessage(`{"allow":true}`)})
	}
	d := NewDecisions(j, time.Now)
	for raw, want := range map[string]int{`{}`: 20, `{"limit":25}`: 25, `{"limit":1000}`: 1_000, `{"limit":2000}`: 1_000, `{"limit":null}`: 20} {
		out, err := d.Log(context.Background(), decode[DecisionLogRequest](t, raw))
		if err != nil || out.Count != want || out.NextCursor == "" {
			t.Fatal(raw, out.Count, err)
		}
	}
}
