// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	store "github.com/agezt/agezt/kernel/memory"
)

func readFixture(t *testing.T) (*store.FileStore, *appmemory.Service) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, appmemory.New(store.NewManager(s, nil))
}

func TestGetRetainsLifecycleAndWirePresence(t *testing.T) {
	s, service := readFixture(t)
	r := store.Record{ID: "state", Type: store.TypeFact, Subject: "", Content: "retained record", Confidence: 0,
		CreatedMS: 0, LastSeenMS: -1, HalfLifeMS: 1, Evidence: store.EvidenceCurated,
		Tags: map[string]string{"scope": "private"}, SourceEvent: "event", AddedBy: "operator", UpdatedBy: "agent",
		SupersededBy: "new", Tombstoned: true, SuspendedMS: 1, SuspendedReason: "review"}
	if err := s.Put(r); err != nil {
		t.Fatal(err)
	}
	out, err := service.Get(context.Background(), appmemory.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	rec := wire["record"].(map[string]any)
	want := map[string]any{"id": "state", "type": "FACT", "subject": "", "content": "retained record", "confidence": float64(0),
		"created_ms": float64(0), "last_seen_ms": float64(-1), "half_life_ms": float64(1), "expires_ms": float64(0), "expired": true,
		"evidence": "curated", "source_event": "event", "added_by": "operator", "updated_by": "agent", "superseded_by": "new",
		"tombstoned": true, "suspended_ms": float64(1), "suspended": true, "suspended_reason": "review"}
	for key, value := range want {
		if rec[key] != value {
			t.Errorf("%s=%v want %v", key, rec[key], value)
		}
	}
	if !out.Found || out.Record.Tags["scope"] != "private" {
		t.Fatalf("lost found/tags: %+v", out)
	}
	out, err = service.Get(context.Background(), appmemory.GetInput{ID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"found":false}` {
		t.Fatalf("missing record wire=%s", raw)
	}
	if err := s.Put(store.Record{ID: "plain", Content: "plain"}); err != nil {
		t.Fatal(err)
	}
	out, err = service.Get(context.Background(), appmemory.GetInput{ID: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	rec = wire["record"].(map[string]any)
	for _, key := range []string{"tags", "half_life_ms", "expires_ms", "expired", "tombstoned", "suspended", "suspended_ms", "suspended_reason", "superseded_by"} {
		if _, exists := rec[key]; exists {
			t.Errorf("plain record includes %s", key)
		}
	}
}

func TestSearchAndRelatedRetainLimitsAndSeedExclusion(t *testing.T) {
	s, service := readFixture(t)
	now := time.Now().UnixMilli()
	for i := 0; i < 105; i++ {
		r := store.Record{ID: fmt.Sprintf("%03d", i), Type: store.TypeFact, Content: "shared fixture words", Confidence: 1, CreatedMS: now, LastSeenMS: now}
		if err := s.Put(r); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		raw   float64
		count int
	}{{0, 10}, {-1, 10}, {1, 1}, {1.9, 1}, {1000, 100}} {
		out, err := service.Search(context.Background(), appmemory.SearchInput{Query: "fixture", Limit: tc.raw})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Results) != tc.count {
			t.Errorf("search limit=%v got %d/%d", tc.raw, out.Count, len(out.Results))
		}
		out, err = service.FindRelated(context.Background(), appmemory.RelatedInput{ID: "000", Limit: tc.raw})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Results) != tc.count {
			t.Errorf("related limit=%v got %d/%d", tc.raw, out.Count, len(out.Results))
		}
		for _, hit := range out.Results {
			if hit.Record.ID == "000" || hit.Score <= 0 {
				t.Fatalf("seed/score drift: %+v", hit)
			}
		}
	}
	out, err := service.Search(context.Background(), appmemory.SearchInput{Query: "absent-token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"results":[],"count":0}` {
		t.Fatalf("empty search=%s", raw)
	}
	if _, err := service.FindRelated(context.Background(), appmemory.RelatedInput{ID: "missing"}); err == nil || err.Error() != "seed record id not found" {
		t.Fatalf("missing seed=%v", err)
	}
}

type failingStore struct {
	store.Store
	cause   error
	failGet bool
}

func (s failingStore) Get(id string) (store.Record, bool, error) {
	if s.failGet {
		return store.Record{}, false, s.cause
	}
	return s.Store.Get(id)
}
func (s failingStore) All() ([]store.Record, error) { return nil, s.cause }

func TestReadErrorsRetainStoreCause(t *testing.T) {
	s, _ := readFixture(t)
	if err := s.Put(store.Record{ID: "seed", Content: "seed"}); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("fixture store unavailable")
	for _, failGet := range []bool{true, false} {
		service := appmemory.New(store.NewManager(failingStore{Store: s, cause: cause, failGet: failGet}, nil))
		_, err := service.FindRelated(context.Background(), appmemory.RelatedInput{ID: "seed"})
		if !errors.Is(err, cause) {
			t.Errorf("related failGet=%v cause=%v", failGet, err)
		}
		_, err = service.Search(context.Background(), appmemory.SearchInput{Query: "seed"})
		if !errors.Is(err, cause) {
			t.Errorf("search cause=%v", err)
		}
		if failGet {
			_, err = service.Get(context.Background(), appmemory.GetInput{ID: "seed"})
			if !errors.Is(err, cause) {
				t.Errorf("get cause=%v", err)
			}
		}
	}
}
