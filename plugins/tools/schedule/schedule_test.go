// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
)

// fakeStore records calls so the tool's mapping (op → store method + args) is
// asserted without a real on-disk schedule store.
type fakeStore struct {
	added    []cadence.Entry
	lastOnce time.Time
	lastIntv time.Duration
	lastAtMin, lastDays int
	removed  string
	removeOK bool
	entries  []cadence.Entry
}

func (f *fakeStore) Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastIntv = interval
	e := cadence.Entry{ID: "ev1", Intent: intent, Mode: cadence.ModeInterval, IntervalSec: int64(interval / time.Second), Source: source, Enabled: true, NextRunUnix: now.Add(interval).Unix()}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastAtMin, f.lastDays = atMinutes, days
	e := cadence.Entry{ID: "day1", Intent: intent, Mode: cadence.ModeDaily, AtMinutes: atMinutes, Days: days, Source: source, Enabled: true}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) AddOnce(intent string, at time.Time, model, source string, now time.Time) (cadence.Entry, error) {
	f.lastOnce = at
	e := cadence.Entry{ID: "once1", Intent: intent, Mode: cadence.ModeOnce, Source: source, Enabled: true, NextRunUnix: at.Unix()}
	f.added = append(f.added, e)
	return e, nil
}
func (f *fakeStore) Remove(id string) (bool, error) { f.removed = id; return f.removeOK, nil }
func (f *fakeStore) List() []cadence.Entry          { return f.entries }

// fixedNow pins the clock so the one-shot's absolute fire time is assertable.
var fixedNow = time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

func newTool(f *fakeStore) *Tool {
	t := New()
	t.store = f
	t.now = func() time.Time { return fixedNow }
	return t
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "schedule" {
		t.Fatalf("name = %q", d.Name)
	}
	if !json.Valid(d.InputSchema) {
		t.Fatal("schema invalid")
	}
}

func TestOpIn_OneShotAtDelay(t *testing.T) {
	f := &fakeStore{}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "in", "delay": "30m", "intent": "check the deploy"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if len(f.added) != 1 || f.added[0].Intent != "check the deploy" {
		t.Fatalf("AddOnce not called as expected: %+v", f.added)
	}
	if f.added[0].Source != "agent" {
		t.Errorf("source = %q, want agent", f.added[0].Source)
	}
	if want := fixedNow.Add(30 * time.Minute); !f.lastOnce.Equal(want) {
		t.Errorf("fire time = %v, want now+30m = %v", f.lastOnce, want)
	}
}

func TestOpEvery_Interval(t *testing.T) {
	f := &fakeStore{}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "every", "interval": "1h", "intent": "hourly digest"})
	if isErr {
		t.Fatal("unexpected error")
	}
	if f.lastIntv != time.Hour {
		t.Errorf("interval = %v, want 1h", f.lastIntv)
	}
}

func TestOpDaily_ParsesTimeAndDays(t *testing.T) {
	f := &fakeStore{}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "daily", "at": "09:30", "days": "mon-fri", "intent": "standup"})
	if isErr {
		t.Fatal("unexpected error")
	}
	if f.lastAtMin != 9*60+30 {
		t.Errorf("atMinutes = %d, want %d", f.lastAtMin, 9*60+30)
	}
	if f.lastDays == 0 {
		t.Error("days should be a non-zero weekday mask for mon-fri")
	}
}

func TestOpRemove(t *testing.T) {
	f := &fakeStore{removeOK: true}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "remove", "id": "once1"})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if f.removed != "once1" {
		t.Errorf("removed = %q, want once1", f.removed)
	}
}

func TestOpRemove_NotFound(t *testing.T) {
	f := &fakeStore{removeOK: false}
	_, isErr := invoke(t, newTool(f), map[string]any{"op": "remove", "id": "nope"})
	if !isErr {
		t.Error("removing an unknown id should be an error result")
	}
}

func TestOpList(t *testing.T) {
	f := &fakeStore{entries: []cadence.Entry{
		{ID: "a", Intent: "x", Mode: cadence.ModeInterval, IntervalSec: 3600, Enabled: true, Source: "agent"},
	}}
	out, _ := invoke(t, newTool(f), map[string]any{"op": "list"})
	if out["count"].(float64) != 1 {
		t.Fatalf("count = %v, want 1", out["count"])
	}
}

func TestBadInputs(t *testing.T) {
	f := &fakeStore{}
	cases := []map[string]any{
		{"op": "in", "delay": "nope", "intent": "x"},
		{"op": "in", "delay": "-5m", "intent": "x"},
		{"op": "every", "interval": "", "intent": "x"},
		{"op": "daily", "at": "25:00", "intent": "x"},
		{"op": "daily", "at": "notime", "intent": "x"},
		{"op": "bogus"},
		{"op": ""},
	}
	for _, c := range cases {
		if _, isErr := invoke(t, newTool(f), c); !isErr {
			t.Errorf("expected error result for %v", c)
		}
	}
}

func TestUnboundStoreIsSafe(t *testing.T) {
	tool := New() // never Bound
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result, not succeed")
	}
}
