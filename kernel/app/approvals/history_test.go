// SPDX-License-Identifier: MIT

package approvals

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

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

var clock = time.UnixMilli(10_000)

func ev(seq, ts int64, kind event.Kind, actor, corr, payload string) *event.Event {
	return &event.Event{Seq: seq, TSUnixMS: ts, Kind: kind, Actor: actor, CorrelationID: corr, Payload: json.RawMessage(payload)}
}

// history: a granted, a denied, a timed-out and a pending request; a resolution
// that arrives before its request; an orphan resolution; malformed and id-less
// records.
func history() fakeJournal {
	return fakeJournal{events: []*event.Event{
		ev(1, 1_000, event.KindApprovalRequested, "agent", "c1", `{"approval_id":"a1","capability":"shell","tool_name":"shell","reason":"rm"}`),
		ev(2, 1_500, event.KindApprovalGranted, "operator", "c1x", `{"approval_id":"a1","resolved_by":"ops"}`),
		ev(3, 2_000, event.KindApprovalRequested, "agent", "c2", `{"approval_id":"a2","capability":"http.post","tool_name":"http","reason":"post"}`),
		ev(4, 2_500, event.KindApprovalDenied, "operator", "c2", `{"approval_id":"a2","resolved_by":"lead"}`),
		ev(5, 3_000, event.KindApprovalRequested, "agent", "c3", `{"approval_id":"a3","tool_name":"x"}`),
		ev(6, 8_500, event.KindApprovalTimeout, "kernel", "", `{"approval_id":"a3"}`),
		ev(7, 8_000, event.KindApprovalRequested, "agent", "c4", `{"approval_id":"a4","capability":"file.write"}`),
		ev(8, 8_200, event.KindApprovalGranted, "operator", "c5x", `{"approval_id":"a5","resolved_by":"early"}`),
		ev(9, 8_600, event.KindApprovalRequested, "agent", "c5", `{"approval_id":"a5","capability":"memory","tool_name":"mem"}`),
		ev(10, 9_000, event.KindApprovalDenied, "operator", "c6", `{"approval_id":"a6","resolved_by":"x"}`),
		ev(11, 9_100, event.KindApprovalRequested, "agent", "c7", `{"capability":"shell"}`),
		ev(12, 9_200, event.KindApprovalGranted, "agent", "c8", `[1]`),
		ev(13, 9_300, event.KindPolicyDecision, "agent", "c9", `{"approval_id":"a9"}`),
	}}
}

func TestLog(t *testing.T) {
	ctx := context.Background()
	h := New(history(), func() time.Time { return clock })
	out, err := h.Log(ctx, LogRequest{})
	if err != nil || out.Count != 6 || out.NextCursor != "" {
		t.Fatalf("%+v %v", out, err)
	}
	ids := []string{}
	for _, r := range out.Approvals {
		ids = append(ids, r.ApprovalID)
	}
	if !reflect.DeepEqual(ids, []string{"a6", "a5", "a4", "a3", "a2", "a1"}) {
		t.Fatal("newest request first; an orphan resolution anchors at itself", ids)
	}
	if out.Approvals[1] != (Row{TSUnixMS: 8_600, Seq: 9, ApprovalID: "a5", Capability: "memory", Tool: "mem", Actor: "agent", CorrelationID: "c5", Status: "granted", ResolvedBy: "early"}) {
		t.Fatal("a late request re-anchors the row and takes over actor and correlation", out.Approvals[1])
	}
	if out.Approvals[0] != (Row{TSUnixMS: 9_000, Seq: 10, ApprovalID: "a6", Actor: "operator", CorrelationID: "c6", Status: "denied", ResolvedBy: "x"}) {
		t.Fatal("an orphan resolution supplies actor and correlation", out.Approvals[0])
	}
	if out.Approvals[3] != (Row{TSUnixMS: 3_000, Seq: 5, ApprovalID: "a3", Tool: "x", Actor: "agent", CorrelationID: "c3", Status: "timeout"}) {
		t.Fatal(out.Approvals[3])
	}
	if out.Approvals[5] != (Row{TSUnixMS: 1_000, Seq: 1, ApprovalID: "a1", Capability: "shell", Tool: "shell", Reason: "rm", Actor: "agent", CorrelationID: "c1", Status: "granted", ResolvedBy: "ops"}) {
		t.Fatal(out.Approvals[5])
	}
	if out.Approvals[2].Status != "pending" || out.Approvals[2].ResolvedBy != "" {
		t.Fatal(out.Approvals[2])
	}
	denied, _ := h.Log(ctx, decode[LogRequest](t, `{"denied":true}`))
	if denied.Count != 3 || denied.Approvals[0].ApprovalID != "a6" || denied.Approvals[2].ApprovalID != "a2" {
		t.Fatal("denied keeps denials and timeouts", denied)
	}
	window, _ := h.Log(ctx, decode[LogRequest](t, `{"since_ms":2500.9}`))
	if window.Count != 3 {
		t.Fatal("the window is the daemon clock minus a truncated since_ms, on the request time", window)
	}
	paged, _ := h.Log(ctx, decode[LogRequest](t, `{"limit":2.9}`))
	if paged.Count != 2 || paged.NextCursor == "" {
		t.Fatal(paged)
	}
	next, _ := h.Log(ctx, decode[LogRequest](t, `{"limit":2,"cursor":"`+paged.NextCursor+`"}`))
	if next.Count != 2 || next.Approvals[0].ApprovalID != "a4" {
		t.Fatal(next)
	}
	last, _ := h.Log(ctx, decode[LogRequest](t, `{"limit":2,"cursor":"`+next.NextCursor+`"}`))
	if last.Count != 2 || last.Approvals[1].ApprovalID != "a1" || last.NextCursor == "" {
		t.Fatal(last)
	}
	for raw, want := range map[string]int{`{"limit":0}`: 1, `{"limit":-3}`: 1, `{"limit":"5"}`: 6, `{"cursor":3}`: 6, `{"cursor":"garbage"}`: 6} {
		if got, _ := h.Log(ctx, decode[LogRequest](t, raw)); got.Count != want {
			t.Fatal(raw, got.Count)
		}
	}
	for _, raw := range []string{`{"denied":"yes"}`, `{"denied":null}`, `{"denied":1,"limit":"x"}`} {
		if _, err := h.Log(ctx, decode[LogRequest](t, raw)); err == nil || err.Error() != "args.denied must be a boolean" {
			t.Fatal(raw, err)
		}
	}
	empty, _ := New(fakeJournal{}, time.Now).Log(ctx, LogRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"approvals":[],"count":0,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := New(fakeJournal{err: boom}, time.Now).Log(ctx, LogRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestLogLimits(t *testing.T) {
	var j fakeJournal
	for i := range 1_005 {
		j.events = append(j.events, ev(int64(i+1), int64(i+1), event.KindApprovalRequested, "agent", "", `{"approval_id":"a`+string(rune('a'+i%26))+`-`+time.UnixMilli(int64(i)).Format("150405.000")+`"}`))
	}
	h := New(j, time.Now)
	for raw, want := range map[string]int{`{}`: 20, `{"limit":25}`: 25, `{"limit":1000}`: 1_000, `{"limit":2000}`: 1_000} {
		out, err := h.Log(context.Background(), decode[LogRequest](t, raw))
		if err != nil || out.Count != want || out.NextCursor == "" {
			t.Fatal(raw, out.Count, err)
		}
	}
}

func TestStats(t *testing.T) {
	ctx := context.Background()
	h := New(history(), func() time.Time { return clock })
	out, err := h.Stats(ctx, StatsRequest{})
	want := StatsOutput{Total: 6, Granted: 2, Denied: 2, Timeout: 1, Pending: 1, Resolved: 5, GrantRate: 0.4, DeniedByCapability: map[string]int{"http.post": 1, "unknown": 2}}
	if err != nil || !reflect.DeepEqual(out, want) {
		t.Fatalf("%+v %v", out, err)
	}
	window, _ := h.Stats(ctx, decode[StatsRequest](t, `{"since_ms":2500.9}`))
	if !reflect.DeepEqual(window, StatsOutput{Total: 2, Granted: 1, Pending: 1, Resolved: 1, GrantRate: 1, DeniedByCapability: map[string]int{}, WindowMS: 2500}) {
		t.Fatal("a windowed fold counts by request time and drops the requestless resolution", window)
	}
	negative, _ := h.Stats(ctx, decode[StatsRequest](t, `{"since_ms":-5}`))
	if negative.Total != 6 || negative.WindowMS != -5 {
		t.Fatal("a non-positive window is all-time but echoed", negative)
	}
	if lenient, _ := h.Stats(ctx, decode[StatsRequest](t, `{"since_ms":"x"}`)); lenient.Total != 6 || lenient.WindowMS != 0 {
		t.Fatal(lenient)
	}
	empty, _ := New(fakeJournal{}, time.Now).Stats(ctx, StatsRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"total":0,"granted":0,"denied":0,"timeout":0,"pending":0,"resolved":0,"grant_rate":0,"denied_by_capability":{},"window_ms":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := New(fakeJournal{err: boom}, time.Now).Stats(ctx, StatsRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	h := New(history(), func() time.Time { return clock })
	ops, err := Operations(func(context.Context) *History { return h })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	log, _ := h.Log(context.Background(), LogRequest{})
	stats, _ := h.Stats(context.Background(), StatsRequest{})
	for i, w := range []struct {
		name string
		http opapi.HTTP
		out  any
	}{
		{"approvals_log", opapi.HTTP{Method: "GET", Path: "/api/approvals_log"}, log},
		{"approvals_stats", opapi.HTTP{}, stats},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}

func TestLogSameMillisecond(t *testing.T) {
	j := fakeJournal{events: []*event.Event{
		ev(1, 5_000, event.KindApprovalRequested, "agent", "", `{"approval_id":"first"}`),
		ev(2, 5_000, event.KindApprovalRequested, "agent", "", `{"approval_id":"second"}`),
	}}
	out, err := New(j, time.Now).Log(context.Background(), LogRequest{})
	if err != nil || out.Count != 2 || out.Approvals[0].ApprovalID != "second" || out.Approvals[1].ApprovalID != "first" {
		t.Fatal("a same-millisecond tie orders by sequence, newest first", out, err)
	}
}
