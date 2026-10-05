// SPDX-License-Identifier: MIT

package taste_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apptaste "github.com/agezt/agezt/kernel/app/taste"
	curated "github.com/agezt/agezt/kernel/taste"
)

func tasteFixture(t *testing.T) (*curated.Store, *apptaste.Service, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := curated.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return store, apptaste.New(store), dir
}

func TestTasteServiceRetainsCurationFiltersAndWire(t *testing.T) {
	store, service, _ := tasteFixture(t)
	ctx := context.Background()
	first, err := service.Create(ctx, apptaste.CreateInput{Title: " First ", Body: " body ", Scope: " Agent ", Tags: []string{"Writing", "writing", " "}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Exemplar.ID == "" || first.Exemplar.Title != "First" || first.Exemplar.Body != "body" || first.Exemplar.Scope != "Agent" || len(first.Exemplar.Tags) != 2 || first.Exemplar.CreatedMS <= 0 || first.Exemplar.UpdatedMS != first.Exemplar.CreatedMS {
		t.Fatalf("create=%+v", first)
	}
	second, err := service.Create(ctx, apptaste.CreateInput{Title: "Global", Body: "global body", Tags: []string{"coding"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in    apptaste.ListInput
		count int
	}{{apptaste.ListInput{Limit: 200}, 2}, {apptaste.ListInput{Scope: "agent", Limit: 200}, 1}, {apptaste.ListInput{Tag: "coding", Limit: 200}, 1}, {apptaste.ListInput{Limit: 1}, 1}, {apptaste.ListInput{Limit: 0}, 2}} {
		out, err := service.List(ctx, tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Exemplars) != tc.count {
			t.Errorf("filter=%+v out=%+v", tc.in, out)
		}
	}
	all, err := service.List(ctx, apptaste.ListInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	all.Exemplars[0].Tags = append(all.Exemplars[0].Tags, "caller-mutated")
	for _, ex := range store.List(curated.Filter{}) {
		for _, tag := range ex.Tags {
			if tag == "caller-mutated" {
				t.Fatal("returned tags alias store state")
			}
		}
	}
	raw, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	exemplar := wire["exemplar"].(map[string]any)
	if _, exists := exemplar["scope"]; exists {
		t.Fatal("empty optional scope emitted")
	}
	for _, key := range []string{"id", "title", "body", "created_ms", "updated_ms"} {
		if _, exists := exemplar[key]; !exists {
			t.Errorf("missing exemplar field %s", key)
		}
	}
	deleted, err := service.Delete(ctx, apptaste.DeleteInput{ID: first.Exemplar.ID})
	if err != nil || deleted.Deleted != first.Exemplar.ID {
		t.Fatalf("delete=%+v err=%v", deleted, err)
	}
	if _, err := service.Delete(ctx, apptaste.DeleteInput{ID: first.Exemplar.ID}); !errors.Is(err, curated.ErrNotFound) {
		t.Fatalf("missing delete cause=%v", err)
	}
	if _, err := service.Delete(ctx, apptaste.DeleteInput{ID: second.Exemplar.ID}); err != nil {
		t.Fatal(err)
	}
	out, err := service.List(ctx, apptaste.ListInput{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"exemplars":[],"count":0}` {
		t.Fatalf("empty taste wire=%s", raw)
	}
}

func TestTasteServiceRetainsValidationAndPersistenceFailure(t *testing.T) {
	store, service, dir := tasteFixture(t)
	ctx := context.Background()
	for _, in := range []apptaste.CreateInput{{}, {Title: "valid"}, {Title: " ", Body: "body"}, {Title: "title", Body: strings.Repeat("x", 8001)}} {
		if _, err := service.Create(ctx, in); err == nil {
			t.Errorf("invalid create admitted: %+v", in)
		}
	}
	if _, err := service.Delete(ctx, apptaste.DeleteInput{}); err == nil || err.Error() != "taste_delete requires id" {
		t.Fatalf("missing ID error=%v", err)
	}
	created, err := service.Create(ctx, apptaste.CreateInput{Title: "retained", Body: "retained body"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "taste.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "owned-blocker"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, apptaste.CreateInput{Title: "failed", Body: "failed body"}); err == nil {
		t.Fatal("create ignored persistence failure")
	}
	if _, err := service.Delete(ctx, apptaste.DeleteInput{ID: created.Exemplar.ID}); err == nil {
		t.Fatal("delete ignored persistence failure")
	}
	all := store.List(curated.Filter{})
	if len(all) != 1 || all[0].ID != created.Exemplar.ID {
		t.Fatalf("failed write changed store state: %+v", all)
	}
}
