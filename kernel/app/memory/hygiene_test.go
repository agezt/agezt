// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	store "github.com/agezt/agezt/kernel/memory"
)

func TestPruneRetainsAgeDryRunAndFieldPresence(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	old := now - int64(31*24*time.Hour/time.Millisecond)
	for _, rec := range []store.Record{
		{ID: "old-tomb", Content: "old forgotten", LastSeenMS: old, Tombstoned: true},
		{ID: "old-super", Content: "old superseded", LastSeenMS: old, SupersededBy: "successor"},
		{ID: "old-active", Content: "old active", LastSeenMS: old},
		{ID: "recent-tomb", Content: "recent forgotten", LastSeenMS: now, Tombstoned: true},
	} {
		if err := s.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	before := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	out, err := service.Prune(ctx, appmemory.PruneInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	if !out.DryRun || out.OlderThanDays != 30 || out.CutoffMS < before || out.CutoffMS > after || out.Prunable == nil || *out.Prunable != 2 || out.Pruned != nil || out.Stats.Total != 4 {
		t.Fatalf("dry prune=%+v", out)
	}
	if s.Count() != 4 {
		t.Fatal("dry-run changed store")
	}
	out, err = service.Prune(ctx, appmemory.PruneInput{OlderThanDays: 30, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if out.DryRun || out.Pruned == nil || *out.Pruned != 2 || out.Prunable != nil || out.Stats.Total != 4 {
		t.Fatalf("execute prune=%+v", out)
	}
	for _, id := range []string{"old-active", "recent-tomb"} {
		if _, found, err := s.Get(id); err != nil || !found {
			t.Fatalf("retained %s found=%v err=%v", id, found, err)
		}
	}
	for _, dry := range []bool{true, false} {
		out, err = service.Prune(ctx, appmemory.PruneInput{OlderThanDays: -1, DryRun: dry})
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
		present, absent := "pruned", "prunable"
		if dry {
			present, absent = absent, present
		}
		if value, exists := wire[present]; !exists || value != float64(0) {
			t.Errorf("present zero %s=%v exists=%v", present, value, exists)
		}
		if _, exists := wire[absent]; exists {
			t.Errorf("unexpected %s", absent)
		}
	}
}

func TestTidyRetainsDryRunAndCuratedRecords(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	for _, rec := range []store.Record{
		{ID: "strong", Type: store.TypeFact, Subject: "topic", Content: "strong distilled note", Confidence: 1, LastSeenMS: now, Tags: map[string]string{"source": "distill"}},
		{ID: "weak", Type: store.TypeFact, Subject: "topic", Content: "weak distilled note", Confidence: .3, LastSeenMS: now, Tags: map[string]string{"source": "distill"}},
		{ID: "curated", Type: store.TypeFact, Subject: "topic", Content: "curated note", LastSeenMS: now, Tags: map[string]string{"source": "operator"}},
	} {
		if err := s.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	out, err := service.Tidy(ctx, appmemory.HygieneInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun || out.Collapsed != 1 {
		t.Fatalf("dry tidy=%+v", out)
	}
	r, _, err := s.Get("weak")
	if err != nil || r.Tombstoned {
		t.Fatalf("dry-run mutation=%+v err=%v", r, err)
	}
	out, err = service.Tidy(ctx, appmemory.HygieneInput{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if out.DryRun || out.Collapsed != 1 {
		t.Fatalf("execute tidy=%+v", out)
	}
	for _, id := range []string{"strong", "curated"} {
		r, found, err := s.Get(id)
		if err != nil || !found || r.Tombstoned {
			t.Fatalf("retained %s=%+v err=%v", id, r, err)
		}
	}
	r, _, err = s.Get("weak")
	if err != nil || !r.Tombstoned {
		t.Fatalf("weak not forgotten=%+v err=%v", r, err)
	}
}

func TestAuditAndCleanRetainReportsAndDryRun(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	for _, rec := range []store.Record{
		{ID: "low", Content: "x", Tags: map[string]string{"source": "agent"}},
		{ID: "curated", Content: "x", Tags: map[string]string{"source": "operator"}},
		{ID: "expired", Content: "expired fact", LastSeenMS: 1, HalfLifeMS: 1, Tags: map[string]string{"source": "operator"}},
		{ID: "suspended", Content: "suspended fact", SuspendedMS: 1, Tags: map[string]string{"source": "operator"}},
	} {
		if err := s.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	audit, err := service.Audit(ctx, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if audit.Total != 4 || audit.Usable != 2 || audit.Expired != 1 || audit.Suspended != 1 {
		t.Fatalf("audit=%+v", audit)
	}
	out, err := service.Clean(ctx, appmemory.HygieneInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun || out.HardDeleted || out.Scanned != 4 || out.Rejected != 1 || out.Removed != 0 || len(out.Decisions) != 1 || s.Count() != 4 {
		t.Fatalf("dry clean=%+v count=%d", out, s.Count())
	}
	out, err = service.Clean(ctx, appmemory.HygieneInput{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if out.DryRun || !out.HardDeleted || out.Removed != 1 || s.Count() != 3 {
		t.Fatalf("execute clean=%+v count=%d", out, s.Count())
	}
}

func TestHygieneUnavailableAndStoreCauses(t *testing.T) {
	ctx := context.Background()
	nilService := appmemory.New(nil)
	if _, err := nilService.Prune(ctx, appmemory.PruneInput{}); err == nil || err.Error() != "memory unavailable" {
		t.Errorf("nil prune=%v", err)
	}
	if _, err := nilService.Tidy(ctx, appmemory.HygieneInput{}); err == nil || err.Error() != "memory unavailable" {
		t.Errorf("nil tidy=%v", err)
	}
	s, _ := readFixture(t)
	cause := errors.New("fixture hygiene unavailable")
	service := appmemory.New(store.NewManager(failingStore{Store: s, cause: cause}, nil))
	_, err := service.Prune(ctx, appmemory.PruneInput{DryRun: true})
	if !errors.Is(err, cause) {
		t.Errorf("prune cause=%v", err)
	}
	_, err = service.Tidy(ctx, appmemory.HygieneInput{DryRun: true})
	if !errors.Is(err, cause) {
		t.Errorf("tidy cause=%v", err)
	}
	_, err = service.Audit(ctx, struct{}{})
	if !errors.Is(err, cause) {
		t.Errorf("audit cause=%v", err)
	}
	_, err = service.Clean(ctx, appmemory.HygieneInput{DryRun: true})
	if !errors.Is(err, cause) {
		t.Errorf("clean cause=%v", err)
	}
}
