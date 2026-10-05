// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"testing"
	"time"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	store "github.com/agezt/agezt/kernel/memory"
)

func TestEveryMemoryStoreMutationRetainsAdmittedCorrelation(t *testing.T) {
	for _, name := range []string{"remember", "supersede", "forget", "promote", "bulk", "prune", "tidy", "clean"} {
		t.Run(name, func(t *testing.T) {
			s, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			j, err := journal.Open(t.TempDir(), journal.Options{})
			if err != nil {
				t.Fatal(err)
			}
			b := bus.New(j)
			t.Cleanup(func() { b.Close(); _ = j.Close(); _ = s.Close() })
			now := time.Now().UnixMilli()
			seed := store.Record{ID: "seed", Type: store.TypeFact, Subject: "topic", Content: "baseline durable fact", Confidence: 1, LastSeenMS: now, Tags: map[string]string{"source": "agent", "scope": "agent"}}
			if name == "prune" {
				seed.Tombstoned = true
				seed.LastSeenMS = now - int64(31*24*time.Hour/time.Millisecond)
			}
			if name == "tidy" {
				seed.Tags["source"] = "distill"
			}
			if name == "clean" {
				seed.Content = "x"
			}
			if err := s.Put(seed); err != nil {
				t.Fatal(err)
			}
			if name == "tidy" {
				weak := seed
				weak.ID = "weak"
				weak.Content = "weaker durable note"
				weak.Confidence = .3
				if err := s.Put(weak); err != nil {
					t.Fatal(err)
				}
			}
			service := appmemory.New(store.NewManager(s, b))
			ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
			expected := event.KindMemoryWritten
			switch name {
			case "remember":
				_, err = service.Remember(ctx, appmemory.RememberInput{Content: "new durable fixture"})
			case "supersede":
				_, err = service.Supersede(ctx, appmemory.SupersedeInput{OldID: "seed", RememberInput: appmemory.RememberInput{Content: "replacement durable fixture"}})
				expected = event.KindMemorySuperseded
			case "forget":
				_, err = service.Forget(ctx, appmemory.GetInput{ID: "seed"})
				expected = event.KindMemoryForgotten
			case "promote":
				_, err = service.Promote(ctx, appmemory.GetInput{ID: "seed"})
				expected = event.KindMemoryPromoted
			case "bulk":
				_, err = service.BulkForget(ctx, appmemory.BulkForgetInput{IDs: []string{"seed"}})
				expected = event.KindMemoryForgotten
			case "prune":
				_, err = service.Prune(ctx, appmemory.PruneInput{DryRun: false})
				expected = event.KindMemoryPruned
			case "tidy":
				_, err = service.Tidy(ctx, appmemory.HygieneInput{DryRun: false})
				expected = event.KindMemoryForgotten
			case "clean":
				_, err = service.Clean(ctx, appmemory.HygieneInput{DryRun: false})
				expected = event.KindMemoryCleaned
			}
			if err != nil {
				t.Fatal(err)
			}
			found := false
			if err := j.Range(func(e *event.Event) error {
				if e.CorrelationID != "owned-operation" {
					t.Errorf("%s domain event %s correlation=%q", name, e.Kind, e.CorrelationID)
				}
				if e.Kind == expected {
					found = true
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("%s missing effect event %s", name, expected)
			}
		})
	}
}
