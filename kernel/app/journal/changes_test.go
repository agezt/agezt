// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func TestTrimFloat(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 1: "1", -5: "-5", 1000: "1000", 1.5: "1.5", -2.25: "-2.25", 0.5: "0.5"} {
		if got := trimFloat(in); got != want {
			t.Errorf("trimFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

// The labels are a stable operator-facing contract, and membership is what puts
// an event in the changelog.
func TestChangelogKinds(t *testing.T) {
	want := map[event.Kind]string{
		event.KindHalt: "system HALTED", event.KindAnomalyDetected: "anomaly auto-halt", event.KindResume: "system resumed",
		event.KindPolicyChanged: "policy changed", event.KindSkillCreated: "skill created", event.KindSkillPromoted: "skill promoted",
		event.KindSkillQuarantined: "skill quarantined", event.KindSkillReverted: "skill reverted", event.KindSkillRestored: "skill restored",
		event.KindWorkflowRestored: "workflow restored", event.KindReflectionCompleted: "reflection completed",
		event.KindCatalogSynced: "model catalog synced", event.KindCatalogSyncFailed: "model catalog sync FAILED",
		event.KindCatalogDiscoveryCompleted: "provider discovery completed", event.KindCatalogDiscoveryFailed: "provider discovery FAILED",
		event.KindPulsePaused: "pulse paused", event.KindPulseResumed: "pulse resumed",
	}
	if !reflect.DeepEqual(changelogKinds, want) {
		t.Fatalf("changelog kinds changed:\n%v\nwant %v", changelogKinds, want)
	}
}

func TestChangelogDetail(t *testing.T) {
	for raw, want := range map[string]string{
		``:                                "",
		`not json`:                        "",
		`[1]`:                             "",
		`{}`:                              "",
		`{"summary":"s","name":"n"}`:      "s",
		`{"summary":"","name":"n"}`:       "n",
		`{"summary":null,"skill_id":"k"}`: "k",
		`{"count":3,"model":"m"}`:         "m",
		`{"count":3}`:                     "3",
		`{"count":2.5}`:                   "2.5",
		`{"id":7,"rule":"r"}`:             "7",
		`{"reason":true,"subject":"x"}`:   "x",
		`{"change":"c","reason":"r"}`:     "c",
		`{"provider":"p","model":"m"}`:    "p",
		`{"other":"o","summary":{"a":1}}`: "",
	} {
		if got := changelogDetail(json.RawMessage(raw)); got != want {
			t.Errorf("changelogDetail(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestChangelog(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return time.UnixMilli(10_000) }
	events := []*event.Event{
		{ID: "e0", Seq: 0, TSUnixMS: 1_000, Kind: event.KindHalt, Payload: json.RawMessage(`{"reason":"operator"}`)},
		{ID: "e1", Seq: 1, TSUnixMS: 2_000, Kind: "task.received"},
		{ID: "e2", Seq: 2, TSUnixMS: 5_000, Kind: event.KindSkillPromoted, CorrelationID: "c", Payload: json.RawMessage(`{"name":"deploy"}`)},
		{ID: "e3", Seq: 3, TSUnixMS: 5_000, Kind: event.KindResume},
		{ID: "e4", Seq: 4, TSUnixMS: 9_000, Kind: event.KindCatalogSyncFailed, Payload: json.RawMessage(`{"count":2}`)},
	}
	ids := func(out ChangelogOutput) string {
		var parts []string
		for _, e := range out.Entries {
			parts = append(parts, e.EventID)
		}
		return strings.Join(parts, ",")
	}
	for raw, want := range map[string]string{
		`{}`:                            "e4,e3,e2,e0",
		`{"limit":2}`:                   "e4,e3",
		`{"limit":2.9}`:                 "e4,e3",
		`{"limit":0}`:                   "e4,e3,e2,e0",
		`{"limit":-1}`:                  "e4,e3,e2,e0",
		`{"limit":"1"}`:                 "e4,e3,e2,e0",
		`{"since_ms":6000}`:             "e4,e3,e2",
		`{"since_ms":1000}`:             "e4",
		`{"since_ms":-1}`:               "e4,e3,e2,e0",
		`{"since_ms":"1000"}`:           "e4,e3,e2,e0",
		`{"since_ms":5000.7,"limit":1}`: "e4",
	} {
		out, err := New(&fakeJournal{events: events}, nil, clock).Changelog(ctx, decode[WindowRequest](t, raw))
		if err != nil || ids(out) != want || out.Count != len(out.Entries) {
			t.Fatal(raw, ids(out), err)
		}
	}
	all, _ := New(&fakeJournal{events: events}, nil, clock).Changelog(ctx, WindowRequest{})
	if !reflect.DeepEqual(all.Entries[2], ChangelogEntry{TSUnixMS: 5_000, Kind: string(event.KindSkillPromoted), Label: "skill promoted", Detail: "deploy", EventID: "e2", CorrelationID: "c"}) || all.Entries[0].Detail != "2" || all.Entries[3].Label != "system HALTED" {
		t.Fatalf("%+v", all.Entries)
	}
	raw, _ := json.Marshal(all.Entries[1])
	if string(raw) != `{"ts_unix_ms":5000,"kind":"`+string(event.KindResume)+`","label":"system resumed","detail":"","event_id":"e3","correlation_id":""}` {
		t.Fatal(string(raw))
	}
	var many []*event.Event
	for i := range 1_200 {
		many = append(many, &event.Event{ID: "x", Seq: int64(i), TSUnixMS: int64(i), Kind: event.KindPolicyChanged})
	}
	if out, _ := New(&fakeJournal{events: many}, nil, clock).Changelog(ctx, decode[WindowRequest](t, `{"limit":5000}`)); out.Count != 1_000 || out.Entries[0].TSUnixMS != 1_199 {
		t.Fatal(out.Count)
	}
	if out, _ := New(&fakeJournal{events: many}, nil, clock).Changelog(ctx, WindowRequest{}); out.Count != 20 || out.Entries[19].TSUnixMS != 1_180 {
		t.Fatal("the default page is the newest 20", out.Count)
	}
	empty, _ := New(&fakeJournal{}, nil, clock).Changelog(ctx, WindowRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"entries":[],"count":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, nil, clock).Changelog(ctx, WindowRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestCacheStats(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return time.UnixMilli(10_000) }
	var priced []string
	cost := func(model string, in, out int) int64 {
		priced = append(priced, model)
		return int64(10*in + out)
	}
	budget := func(ts int64, payload string) *event.Event {
		return &event.Event{TSUnixMS: ts, Kind: event.KindBudgetConsumed, Payload: json.RawMessage(payload)}
	}
	events := []*event.Event{
		budget(1_000, `{"model":"a","input_tokens":100,"output_tokens":5,"cached_input_tokens":80,"cache_write_input_tokens":0,"cost_microcents":300}`),
		budget(6_000, `{"model":"b","input_tokens":10,"output_tokens":1,"cached_input_tokens":0,"cache_write_input_tokens":40,"cost_microcents":500}`),
		budget(8_000, `not json`),
		budget(9_000, `{"model":"c","input_tokens":20,"cached_input_tokens":5,"cost_microcents":50}`),
		{TSUnixMS: 9_500, Kind: "task.received", Payload: json.RawMessage(`{"cached_input_tokens":999}`)},
	}
	out, err := New(&fakeJournal{events: events}, nil, clock).WithCost(cost).CacheStats(ctx, WindowRequest{})
	want := CacheStatsOutput{CachedInputTokens: 85, CacheWriteInputTokens: 40, SavedMicrocents: (1005 - 300) + (200 - 50), Calls: 3}
	if err != nil || out != want || strings.Join(priced, ",") != "a,b,c" {
		t.Fatalf("%+v %v %v", out, priced, err)
	}
	windowed, _ := New(&fakeJournal{events: events}, nil, clock).WithCost(cost).CacheStats(ctx, decode[WindowRequest](t, `{"since_ms":4000.5}`))
	if windowed != (CacheStatsOutput{CachedInputTokens: 5, CacheWriteInputTokens: 40, SavedMicrocents: 150, Calls: 2, WindowMS: 4000}) {
		t.Fatalf("%+v", windowed)
	}
	for raw, echo := range map[string]int64{`{"since_ms":-3}`: -3, `{"since_ms":"9"}`: 0, `{"since_ms":0}`: 0} {
		if all, _ := New(&fakeJournal{events: events}, nil, clock).WithCost(cost).CacheStats(ctx, decode[WindowRequest](t, raw)); all.Calls != 3 || all.WindowMS != echo {
			t.Fatal("a non-positive window covers everything and echoes the number", raw, all)
		}
	}
	empty, _ := New(&fakeJournal{}, nil, clock).WithCost(cost).CacheStats(ctx, WindowRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"cached_input_tokens":0,"cache_write_input_tokens":0,"saved_microcents":0,"calls":0,"window_ms":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, nil, clock).WithCost(cost).CacheStats(ctx, WindowRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}
