// SPDX-License-Identifier: MIT

package world_test

import (
	"context"
	"testing"

	appworld "github.com/agezt/agezt/kernel/app/world"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	graph "github.com/agezt/agezt/kernel/worldmodel"
)

func TestEveryGraphMutationRetainsAdmittedCorrelation(t *testing.T) {
	for _, name := range []string{"add", "edit", "relate", "forget"} {
		t.Run(name, func(t *testing.T) {
			s, err := graph.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			j, err := journal.Open(t.TempDir(), journal.Options{})
			if err != nil {
				t.Fatal(err)
			}
			b := bus.New(j)
			t.Cleanup(func() { b.Close(); _ = j.Close(); _ = s.Close() })
			if err := s.PutEntity(graph.Entity{ID: "seed", Name: "Seed", Kind: graph.KindTopic, Weight: 1}); err != nil {
				t.Fatal(err)
			}
			service := appworld.New(graph.NewGraph(s, b))
			ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
			expected := event.KindWorldEntityUpserted
			switch name {
			case "add":
				_, err = service.Add(ctx, appworld.AddInput{Name: "New fixture"})
			case "edit":
				_, err = service.Edit(ctx, appworld.EditInput{ID: "seed", Aliases: []string{"new alias"}})
			case "relate":
				_, err = service.Relate(ctx, appworld.RelateInput{From: "Seed", To: "New endpoint", Verb: "owns"})
				expected = event.KindWorldRelationUpserted
			case "forget":
				_, err = service.Forget(ctx, appworld.GetInput{ID: "seed"})
				expected = event.KindWorldForgotten
			}
			if err != nil {
				t.Fatal(err)
			}
			found := false
			if err := j.Range(func(e *event.Event) error {
				if e.CorrelationID != "owned-operation" {
					t.Errorf("%s effect %s correlation=%q", name, e.Kind, e.CorrelationID)
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
