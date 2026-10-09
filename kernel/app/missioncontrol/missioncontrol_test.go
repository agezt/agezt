// SPDX-License-Identifier: MIT

package missioncontrol

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

var clock = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func attention(t *testing.T, raw string) AttentionRequest {
	t.Helper()
	var in AttentionRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestSpendToday(t *testing.T) {
	for _, c := range []struct {
		total int64
		ok    bool
		want  string
	}{{9007199254740993, true, `{"total":9007199254740993}`}, {5, false, `{"total":5}`}, {0, false, `{"total":0}`}} {
		out, err := New(Ports{Spend: func() (int64, bool) { return c.total, c.ok }}).SpendToday(context.Background(), SpendRequest{})
		if raw, _ := json.Marshal(out); err != nil || string(raw) != c.want {
			t.Fatal(string(raw), err)
		}
	}
}

func TestAttentionArgs(t *testing.T) {
	for raw, want := range map[string]struct {
		window time.Duration
		limit  int
	}{
		`{}`:                              {24 * time.Hour, 8},
		`{"window":" 5m ","limit":" 3 "}`: {5 * time.Minute, 3},
		`{"window":"-5m","limit":"0"}`:    {24 * time.Hour, 8},
		`{"window":"soon","limit":"x"}`:   {24 * time.Hour, 8},
		`{"window":90,"limit":2.9}`:       {90 * time.Second, 2},
		`{"window":0.5,"limit":0.5}`:      {500 * time.Millisecond, 0},
		`{"window":-1,"limit":-1}`:        {24 * time.Hour, 8},
		`{"window":true,"limit":null}`:    {24 * time.Hour, 8},
		`{"limit":51}`:                    {24 * time.Hour, 50},
		`{"limit":"500"}`:                 {24 * time.Hour, 50},
	} {
		window, limit := attentionArgs(attention(t, raw))
		if window != want.window || limit != want.limit {
			t.Fatal(raw, window, limit)
		}
	}
}

func TestApprovalSummary(t *testing.T) {
	for want, p := range map[string]approval.Request{
		"shell — risky":                     {ID: "a", ToolName: "shell", Reason: "risky"},
		"shell":                             {ID: "a", ToolName: "shell", Capability: "x"},
		"net.post requested by bot — exfil": {ID: "a", Capability: "net.post", Actor: "bot", Reason: "exfil"},
		"net.post requested by agent":       {ID: "a", Capability: "net.post"},
		"ap-1":                              {ID: "ap-1", Reason: "ignored"},
	} {
		if got := approvalSummary(p); got != want {
			t.Fatal(got, want)
		}
	}
}

func TestAttention(t *testing.T) {
	ms := func(d time.Duration) int64 { return clock.Add(-d).UnixMilli() }
	ports := Ports{
		Now: func() time.Time { return clock },
		Pending: func() []approval.Request {
			return []approval.Request{
				{ID: "ap-old", ToolName: "shell", CreatedAt: clock.Add(-72 * time.Hour)},
				{ID: "ap-b", Capability: "net", CreatedAt: clock.Add(-time.Hour)},
				{ID: "ap-a", Capability: "net", CreatedAt: clock.Add(-time.Hour)},
			}
		},
		Asks: func() []map[string]any {
			return []map[string]any{
				{"issue_key": "disk", "summary": "disk filling", "ts_unix_ms": ms(time.Minute)},
				{"issue_key": "stale", "summary": "old", "ts_unix_ms": ms(25 * time.Hour)},
				{"issue_key": "", "summary": "no key", "ts_unix_ms": ms(time.Minute)},
				{"issue_key": "float", "summary": "wrong type", "ts_unix_ms": float64(ms(time.Minute))},
				{"issue_key": "edge", "summary": "at cutoff", "ts_unix_ms": ms(24 * time.Hour)},
			}
		},
	}
	out, err := New(ports).Attention(context.Background(), AttentionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, it := range out.Items {
		ids = append(ids, it.ID)
	}
	if !reflect.DeepEqual(ids, []string{"disk", "ap-a", "ap-b", "edge", "ap-old"}) || out.Count != 5 {
		t.Fatalf("approvals ignore the window, asks need a key, an int64 time and the window; newest first with id ties: %v", ids)
	}
	if out.Items[0] != (Item{ID: "disk", Kind: "pulse_ask", Summary: "disk filling", TS: ms(time.Minute), HRef: "/jarvis#ask-disk"}) || out.Items[4] != (Item{ID: "ap-old", Kind: "approval", Summary: "shell", TS: ms(72 * time.Hour), HRef: "/approvals"}) {
		t.Fatalf("%+v", out.Items)
	}
	out, _ = New(ports).Attention(context.Background(), attention(t, `{"limit":2,"window":"30m"}`))
	if out.Count != 2 || out.Items[0].ID != "disk" || out.Items[1].ID != "ap-a" {
		t.Fatal("the window narrows asks only, then the limit truncates", out)
	}
	out, _ = New(ports).Attention(context.Background(), attention(t, `{"limit":0.5}`))
	if raw, _ := json.Marshal(out); string(raw) != `{"items":[],"count":0}` {
		t.Fatal("a sub-one numeric limit truncates to nothing", string(raw))
	}
	none := Ports{Now: time.Now, Pending: func() []approval.Request { return nil }, Asks: func() []map[string]any { return nil }}
	if raw, _ := json.Marshal(func() AttentionOutput {
		o, _ := New(none).Attention(context.Background(), AttentionRequest{})
		return o
	}()); string(raw) != `{"items":[],"count":0}` {
		t.Fatal(string(raw))
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	svc := New(Ports{Spend: func() (int64, bool) { return 1, true }, Now: time.Now, Pending: func() []approval.Request { return nil }, Asks: func() []map[string]any { return nil }})
	ops, err := Operations(func(context.Context) *Service { return svc })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, want := range []struct{ name, path string }{{"spend_today", "/api/spend/today"}, {"attention", "/api/attention"}} {
		spec := ops[i].Spec()
		if spec.Name != want.name || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: "GET", Path: want.path}) {
			t.Fatal(spec)
		}
	}
	spend, _ := svc.SpendToday(context.Background(), SpendRequest{})
	feed, _ := svc.Attention(context.Background(), AttentionRequest{})
	for i, out := range []any{spend, feed} {
		raw, _ := json.Marshal(out)
		if err := schema.ValidateJSON(ops[i].Spec().OutputSchema, raw); err != nil {
			t.Fatal(i, err, string(raw))
		}
	}
}
