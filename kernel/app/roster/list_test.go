// SPDX-License-Identifier: MIT
package roster

import (
	"context"
	"encoding/json"
	"errors"
	core "github.com/agezt/agezt/kernel/roster"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRosterListCacheOwnershipTTLAndInvalidation(t *testing.T) {
	profiles := []core.Profile{{Slug: "a", CreatedMS: 10, UpdatedMS: 1, Enabled: true}, {Slug: "b", CreatedMS: 20, UpdatedMS: 2}, {Slug: "c", CreatedMS: 20, UpdatedMS: 3, Enabled: true}}
	now := time.Unix(1, 0)
	reads, statusCalls := 0, 0
	s := NewList(func() []core.Profile { reads++; return profiles }, func(p []core.Profile) map[string]map[string]any {
		statusCalls++
		out := map[string]map[string]any{}
		for _, row := range p {
			out[row.Slug] = map[string]any{"owned_status": statusCalls}
		}
		return out
	}, func() time.Time { return now })
	first, err := s.List(context.Background(), ListInput{Limit: 1})
	if err != nil || first["count"] != 1 || first["total"] != 3 || first["enabled_count"] != 2 || first["next_cursor"] != "20:c" {
		t.Fatal(first, err)
	}
	all, err := s.List(context.Background(), ListInput{})
	rows := all["profiles"].([]any)
	if err != nil || len(rows) != 3 || rows[0].(map[string]any)["slug"] != "c" || rows[2].(map[string]any)["slug"] != "a" || statusCalls != 1 || reads != 2 {
		t.Fatal(all, reads, statusCalls)
	}
	page, err := s.List(context.Background(), ListInput{Limit: 1, Cursor: "20:c"})
	if err != nil || page["profiles"].([]any)[0].(map[string]any)["slug"] != "b" || page["next_cursor"] != "20:b" || page["total"] != 3 {
		t.Fatal(page, err)
	}
	now = now.Add(ListCacheTTL)
	s.List(context.Background(), ListInput{})
	if statusCalls != 1 {
		t.Fatal("TTL boundary expired early")
	}
	now = now.Add(time.Nanosecond)
	s.List(context.Background(), ListInput{})
	if statusCalls != 2 {
		t.Fatal("TTL expiry missed")
	}
	profiles[0].UpdatedMS++
	s.List(context.Background(), ListInput{})
	if statusCalls != 3 {
		t.Fatal("profile edit missed")
	}
	s.Invalidate()
	s.List(context.Background(), ListInput{})
	if statusCalls != 4 {
		t.Fatal("same-key invalidation missed")
	}
	profiles = nil
	s.Invalidate()
	empty, err := s.List(context.Background(), ListInput{})
	if err != nil || empty["total"] != 0 || empty["enabled_count"] != 0 || len(empty["profiles"].([]any)) != 0 || statusCalls != 5 {
		t.Fatal("last-profile cache retained", empty, err)
	}
	raw, _ := json.Marshal(empty)
	if string(raw) != `{"count":0,"enabled_count":0,"profiles":null,"total":0}` {
		t.Fatal("legacy empty shape", string(raw))
	}
}
func TestRosterListPreparationBeforeDecodeAndCursorCompatibility(t *testing.T) {
	profiles := []core.Profile{{Slug: "a", CreatedMS: 10, Enabled: true}, {Slug: "b", CreatedMS: 20, Enabled: true}}
	statusCalls := 0
	s := NewList(func() []core.Profile { return profiles }, func([]core.Profile) map[string]map[string]any { statusCalls++; return nil }, nil)
	cause := errors.New("owned codec failure")
	out, err := s.List(context.Background(), ListInput{DecodeError: cause})
	if out != nil || !errors.Is(err, cause) || statusCalls != 1 {
		t.Fatal("admission order drift", out, err, statusCalls)
	}
	for _, cursor := range []string{"", "bad", "9223372036854775808:a"} {
		out, err := s.List(context.Background(), ListInput{Cursor: cursor})
		if err != nil || out["count"] != 2 || statusCalls != 1 {
			t.Fatal(cursor, out, err)
		}
	}
	for _, tc := range []struct {
		cursor string
		want   int
	}{{"20", 1}, {"20:b", 1}, {"20:z", 2}, {"-1:a", 0}, {"20:", 1}} {
		out, err := s.List(context.Background(), ListInput{Cursor: tc.cursor})
		if err != nil || out["count"] != tc.want {
			t.Fatal(tc, out, err)
		}
	}
	for _, limit := range []int{-1, 0, 1, 2, 1001} {
		out, err := s.List(context.Background(), ListInput{Limit: limit})
		want := 2
		if limit == 1 {
			want = 1
		}
		if err != nil || out["count"] != want {
			t.Fatal(limit, out, err)
		}
	}
}
func TestRosterProfileViewPreservesAllFieldsAndFloatNumbers(t *testing.T) {
	direct := false
	p := core.Profile{ID: "owned", Slug: "owned", Name: "name", Soul: "soul", Instructions: []string{"instruction"}, Model: "model", Fallbacks: []string{"fallback"}, MaxCostMc: 9007199254740993, CreatedMS: 9007199254740993, Enabled: true, DirectCallable: &direct}
	raw, _ := json.Marshal(p)
	var want map[string]any
	json.Unmarshal(raw, &want)
	want["kind"] = p.Kind()
	want["managed"] = true
	actual := ProfileView(p)
	if !reflect.DeepEqual(actual, want) || actual["created_ms"] != float64(9007199254740993) {
		t.Fatal(actual, want)
	}
}
func TestRosterListConcurrentReadsAndInvalidation(t *testing.T) {
	profiles := []core.Profile{{Slug: "a", CreatedMS: 1, Enabled: true}}
	var calls atomic.Int64
	s := NewList(func() []core.Profile { return profiles }, func([]core.Profile) map[string]map[string]any { calls.Add(1); return nil }, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 64; j++ {
				out, err := s.List(context.Background(), ListInput{Limit: 1})
				if err != nil || out["total"] != 1 || out["count"] != 1 {
					t.Error(out, err)
				}
			}
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 64; j++ {
				s.Invalidate()
			}
		}()
	}
	wg.Wait()
	if calls.Load() == 0 {
		t.Fatal("no projection")
	}
}
