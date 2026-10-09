// SPDX-License-Identifier: MIT

package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeJournal struct {
	events []*event.Event
	err    error
	ranged bool
}

func (f *fakeJournal) Range(fn func(*event.Event) error) error {
	f.ranged = true
	if f.err != nil {
		return f.err
	}
	for _, e := range f.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func decodeInto[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

var clock = func() time.Time { return time.UnixMilli(10_000) }

func ev(seq, ts int64, kind event.Kind, payload string) *event.Event {
	e := &event.Event{Seq: seq, TSUnixMS: ts, Kind: kind}
	if payload != "" {
		e.Payload = json.RawMessage(payload)
	}
	return e
}

func guardJournal() *fakeJournal {
	return &fakeJournal{events: []*event.Event{
		ev(0, 1_000, event.KindNetguardBlocked, `{"ip":"169.254.169.254","reason":"metadata","tool":"http"}`),
		ev(1, 2_000, event.KindRateLimited, `{"used":12,"limit_per_min":10}`),
		ev(2, 3_000, event.KindWardenExecuted, `{"profile_effective":"strict","argv0":"python","exit_code":0,"duration_ms":40,"downgraded":false,"timed_out":false}`),
		ev(3, 4_000, event.KindWardenProfileDowngraded, `{"requested":"strict","effective":"permissive","reason":"no sandbox"}`),
		ev(4, 5_000, event.KindWardenExecuted, `{"profile_effective":"","argv0":"sh","exit_code":137,"duration_ms":900,"downgraded":true,"timed_out":true}`),
		ev(5, 6_000, event.KindWardenLimitExceeded, `{"limit":"memory","argv0":"sh"}`),
		ev(6, 7_000, event.KindRateLimited, `{"used":30,"limit_per_min":0}`),
		ev(7, 8_000, event.KindNetguardBlocked, `not json`),
		ev(8, 9_000, event.KindRateLimited, `{"used":5,"limit_per_min":20}`),
		ev(9, 9_000, event.KindWardenExecuted, ``),
	}}
}

func TestPage(t *testing.T) {
	s := New(nil, clock)
	for raw, want := range map[string]pageWant{
		`{}`:                            {20, 0},
		`{"limit":0}`:                   {1, 0},
		`{"limit":-5}`:                  {1, 0},
		`{"limit":3.9}`:                 {3, 0},
		`{"limit":"3"}`:                 {20, 0},
		`{"limit":5000}`:                {1_000, 0},
		`{"since_ms":4000}`:             {20, 6_000},
		`{"since_ms":4000.9,"limit":2}`: {2, 6_000},
		`{"since_ms":-1}`:               {20, 0},
		`{"since_ms":"4000"}`:           {20, 0},
	} {
		in := s.page(decodeInto[PageRequest](t, raw))
		if in.Limit != want.limit || in.CutoffMS != want.cutoff {
			t.Fatal(raw, in)
		}
	}
	if in := s.page(decodeInto[PageRequest](t, `{"cursor":"5000:3"}`)); in.Cursor != "5000:3" {
		t.Fatal("the cursor passes through raw", in.Cursor)
	}
}

type pageWant struct {
	limit  int
	cutoff int64
}

func TestNetguardAndRateLimitLogs(t *testing.T) {
	ctx := context.Background()
	blocks, err := New(guardJournal(), clock).NetguardLog(ctx, PageRequest{})
	if err != nil || blocks.Count != 2 || blocks.NextCursor != "" {
		t.Fatalf("%+v %v", blocks, err)
	}
	raw, _ := json.Marshal(blocks)
	if string(raw) != `{"blocks":[{"ip":"","reason":"","tool":"","ts_unix_ms":8000,"seq":7},{"ip":"169.254.169.254","reason":"metadata","tool":"http","ts_unix_ms":1000,"seq":0}],"count":2,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
	throttles, _ := New(guardJournal(), clock).RateLimitLog(ctx, decodeInto[PageRequest](t, `{"limit":2}`))
	raw, _ = json.Marshal(throttles)
	if string(raw) != `{"throttles":[{"used":5,"limit_per_min":20,"ts_unix_ms":9000,"seq":8},{"used":30,"limit_per_min":0,"ts_unix_ms":7000,"seq":6}],"count":2,"next_cursor":"`+journal.NextCursor(7000, 6, 2, 2)+`"}` {
		t.Fatal(string(raw))
	}
	next, _ := New(guardJournal(), clock).RateLimitLog(ctx, decodeInto[PageRequest](t, `{"limit":2,"cursor":"`+throttles.NextCursor+`"}`))
	if next.Count != 1 || next.Throttles[0].Seq != 1 {
		t.Fatalf("%+v", next)
	}
	if windowed, _ := New(guardJournal(), clock).RateLimitLog(ctx, decodeInto[PageRequest](t, `{"since_ms":2500}`)); windowed.Count != 1 {
		t.Fatalf("%+v", windowed)
	}
	empty, _ := New(&fakeJournal{}, clock).NetguardLog(ctx, PageRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"blocks":[],"count":0,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, clock).NetguardLog(ctx, PageRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := New(&fakeJournal{err: boom}, clock).RateLimitLog(ctx, PageRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestRateLimitStats(t *testing.T) {
	ctx := context.Background()
	all, err := New(guardJournal(), clock).RateLimitStats(ctx, WindowRequest{})
	if err != nil || all != (RateLimitStatsOutput{Throttled: 3, LimitPerMin: 20, WorstUsed: 30}) {
		t.Fatalf("the last positive limit in journal order wins: %+v %v", all, err)
	}
	if w, _ := New(guardJournal(), clock).RateLimitStats(ctx, decodeInto[WindowRequest](t, `{"since_ms":2500.5}`)); w != (RateLimitStatsOutput{Throttled: 1, LimitPerMin: 20, WorstUsed: 5, WindowMS: 2500}) {
		t.Fatalf("%+v", w)
	}
	zeroLast := &fakeJournal{events: []*event.Event{ev(0, 1, event.KindRateLimited, `{"used":1,"limit_per_min":10}`), ev(1, 2, event.KindRateLimited, `{"used":2,"limit_per_min":0}`)}}
	if w, _ := New(zeroLast, clock).RateLimitStats(ctx, WindowRequest{}); w.LimitPerMin != 10 || w.WorstUsed != 2 {
		t.Fatalf("a zero limit does not overwrite the last positive one: %+v", w)
	}
	if w, _ := New(guardJournal(), clock).RateLimitStats(ctx, decodeInto[WindowRequest](t, `{"since_ms":-7}`)); w.Throttled != 3 || w.WindowMS != -7 {
		t.Fatalf("%+v", w)
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, clock).RateLimitStats(ctx, WindowRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestWardenLog(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{`{"issues":"yes"}`: "args.issues must be a boolean", `{"issues":null}`: "args.issues must be a boolean", `{"issues":1,"limit":"x"}`: "args.issues must be a boolean"} {
		j := guardJournal()
		if _, err := New(j, clock).WardenLog(ctx, decodeInto[WardenLogRequest](t, raw)); err == nil || err.Error() != want || j.ranged {
			t.Fatal(raw, err)
		}
	}
	all, err := New(guardJournal(), clock).WardenLog(ctx, WardenLogRequest{})
	if err != nil || all.Count != 5 {
		t.Fatalf("%+v %v", all, err)
	}
	var rows []string
	for _, r := range all.Executions {
		raw, _ := json.Marshal(r)
		rows = append(rows, string(raw))
	}
	want := []string{
		`{"kind":"exec","profile":"","argv0":"","exit_code":0,"duration_ms":0,"downgraded":false,"timed_out":false,"ts_unix_ms":9000,"seq":9}`,
		`{"kind":"limit","profile":"","argv0":"sh","reason":"memory","ts_unix_ms":6000,"seq":5}`,
		`{"kind":"exec","profile":"","argv0":"sh","exit_code":137,"duration_ms":900,"downgraded":true,"timed_out":true,"ts_unix_ms":5000,"seq":4}`,
		`{"kind":"downgrade","profile":"permissive","requested":"strict","reason":"no sandbox","ts_unix_ms":4000,"seq":3}`,
		`{"kind":"exec","profile":"strict","argv0":"python","exit_code":0,"duration_ms":40,"downgraded":false,"timed_out":false,"ts_unix_ms":3000,"seq":2}`,
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("each kind carries its own keys:\n%s", strings.Join(rows, "\n"))
	}
	issues, _ := New(guardJournal(), clock).WardenLog(ctx, decodeInto[WardenLogRequest](t, `{"issues":true,"limit":1}`))
	if issues.Count != 1 || issues.Executions[0].Kind != "limit" || issues.NextCursor == "" {
		t.Fatalf("issues keeps downgrades and breaches only: %+v", issues)
	}
	if off, _ := New(guardJournal(), clock).WardenLog(ctx, decodeInto[WardenLogRequest](t, `{"issues":false,"since_ms":5500}`)); off.Count != 3 {
		t.Fatalf("%+v", off)
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, clock).WardenLog(ctx, WardenLogRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestWardenStats(t *testing.T) {
	ctx := context.Background()
	all, err := New(guardJournal(), clock).WardenStats(ctx, WardenLogRequest{}.window())
	want := WardenStatsOutput{Executions: 3, Downgraded: 1, DowngradeRate: 1.0 / 3, TimedOut: 1, LimitBreaches: 1, ByProfile: map[string]int{"strict": 1, "unknown": 2}}
	if err != nil || !reflect.DeepEqual(all, want) {
		t.Fatalf("%+v %v", all, err)
	}
	split := &fakeJournal{events: []*event.Event{
		ev(0, 1, event.KindWardenExecuted, `{"profile_effective":"strict","downgraded":true}`),
		ev(1, 2, event.KindWardenExecuted, `{"profile_effective":"strict","timed_out":true}`),
		ev(2, 3, event.KindWardenExecuted, `{"profile_effective":"strict","timed_out":true}`),
		ev(3, 4, event.KindWardenExecuted, `{"profile_effective":"strict"}`),
	}}
	if got, _ := New(split, clock).WardenStats(ctx, WindowRequest{}); got.Downgraded != 1 || got.TimedOut != 2 || got.DowngradeRate != 0.25 || got.ByProfile["strict"] != 4 {
		t.Fatalf("downgrades and timeouts count separately: %+v", got)
	}
	w, _ := New(guardJournal(), clock).WardenStats(ctx, decodeInto[WindowRequest](t, `{"since_ms":4500}`))
	if !reflect.DeepEqual(w, WardenStatsOutput{Executions: 1, LimitBreaches: 1, ByProfile: map[string]int{"unknown": 1}, WindowMS: 4500}) {
		t.Fatalf("%+v", w)
	}
	empty, _ := New(&fakeJournal{}, clock).WardenStats(ctx, decodeInto[WindowRequest](t, `{"since_ms":"9"}`))
	if raw, _ := json.Marshal(empty); string(raw) != `{"executions":0,"downgraded":0,"downgrade_rate":0,"timed_out":0,"limit_breaches":0,"by_profile":{},"window_ms":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, clock).WardenStats(ctx, WindowRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func (WardenLogRequest) window() WindowRequest { return WindowRequest{} }

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(guardJournal(), clock) })
	if err != nil || len(ops) != 5 {
		t.Fatal(ops, err)
	}
	ctx := context.Background()
	s := New(guardJournal(), clock)
	blocks, _ := s.NetguardLog(ctx, PageRequest{})
	throttles, _ := s.RateLimitLog(ctx, PageRequest{})
	rl, _ := s.RateLimitStats(ctx, WindowRequest{})
	execs, _ := s.WardenLog(ctx, WardenLogRequest{})
	ws, _ := s.WardenStats(ctx, WindowRequest{})
	for i, w := range []struct {
		name  string
		route string
		out   any
	}{
		{"netguard_log", "/api/netguard_log", blocks},
		{"ratelimit_log", "/api/ratelimit_log", throttles},
		{"ratelimit_stats", "", rl},
		{"warden_log", "/api/warden_log", execs},
		{"warden_stats", "", ws},
	} {
		spec := ops[i].Spec()
		http := opapi.HTTP{}
		if w.route != "" {
			http = opapi.HTTP{Method: "GET", Path: w.route}
		}
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
