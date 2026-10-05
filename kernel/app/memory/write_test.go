// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	store "github.com/agezt/agezt/kernel/memory"
)

func TestCurationRetainsOperatorProvenanceAndRevision(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	in := appmemory.RememberInput{Content: "x", Subject: "curation", Tags: map[string]string{"source": "agent", "scope": "private"}, HalfLifeMS: 100000.9}
	added, err := service.Remember(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !added.Created || added.ID == "" || added.Type != "FACT" || added.Subject != "curation" {
		t.Fatalf("add=%+v", added)
	}
	rec, found, err := s.Get(added.ID)
	if err != nil || !found {
		t.Fatalf("store found=%v err=%v", found, err)
	}
	if rec.AddedBy != "operator" || rec.UpdatedBy != "operator" || rec.Tags["source"] != "agent" || rec.HalfLifeMS != 100000 {
		t.Fatalf("operator/force/tags/half-life drift: %+v", rec)
	}
	if in.Tags["source"] != "agent" || len(in.Tags) != 2 {
		t.Fatalf("caller tags mutated: %v", in.Tags)
	}
	again, err := service.Remember(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.ID != added.ID {
		t.Fatalf("dedupe=%+v", again)
	}
	unchanged, err := service.Supersede(ctx, appmemory.SupersedeInput{RememberInput: in, OldID: added.ID})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Superseded || unchanged.NewID != added.ID || unchanged.OldID != added.ID {
		t.Fatalf("same content=%+v", unchanged)
	}
	in.Content = "revised operator content"
	revised, err := service.Supersede(ctx, appmemory.SupersedeInput{RememberInput: in, OldID: added.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !revised.Superseded || revised.NewID == added.ID || revised.OldID != added.ID {
		t.Fatalf("revision=%+v", revised)
	}
	rec, _, err = s.Get(added.ID)
	if err != nil || rec.SupersededBy != revised.NewID {
		t.Fatalf("old record=%+v err=%v", rec, err)
	}
	forgotten, err := service.Forget(ctx, appmemory.GetInput{ID: revised.NewID})
	if err != nil || !forgotten.Forgotten {
		t.Fatalf("forget=%+v err=%v", forgotten, err)
	}
	rec, _, err = s.Get(revised.NewID)
	if err != nil || !rec.Tombstoned {
		t.Fatalf("retained forgotten=%+v err=%v", rec, err)
	}
	if _, err := service.Remember(ctx, appmemory.RememberInput{Content: "default source"}); err != nil {
		t.Fatal(err)
	}
	recs, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Content == "default source" && r.Tags["source"] != "operator" {
			t.Fatalf("default tags=%v", r.Tags)
		}
	}
}

func TestPromoteAndBulkForgetRetainWireAndCounts(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	added, err := service.Remember(ctx, appmemory.RememberInput{Content: "private record", Tags: map[string]string{"scope": "agent"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.Promote(ctx, appmemory.GetInput{ID: added.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Promoted || out.Subject == nil || *out.Subject != "" {
		t.Fatalf("present empty subject=%+v", out)
	}
	rec, _, err := s.Get(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := rec.Tags["scope"]; exists {
		t.Fatalf("scope retained: %v", rec.Tags)
	}
	missing, err := service.Promote(ctx, appmemory.GetInput{ID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"promoted":false,"id":"missing"}` {
		t.Fatalf("missing promote=%s", raw)
	}
	bulk, err := service.BulkForget(ctx, appmemory.BulkForgetInput{IDs: []string{added.ID, "missing", added.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if bulk.Forgotten != 2 || bulk.NotFound != 1 {
		t.Fatalf("bulk counts=%+v", bulk)
	}
	bulk, err = service.BulkForget(ctx, appmemory.BulkForgetInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(bulk)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"forgotten":0,"not_found":0}` {
		t.Fatalf("empty bulk=%s", raw)
	}
	if _, err := service.BulkForget(ctx, appmemory.BulkForgetInput{IDs: make([]string, 501)}); err == nil || err.Error() != "args.ids exceeds 500 — use smaller batches" {
		t.Fatalf("bulk guard=%v", err)
	}
}

type writeFailStore struct {
	store.Store
	cause  error
	failID string
}

func (s writeFailStore) Put(r store.Record) error {
	if s.failID == "" || r.ID == s.failID {
		return s.cause
	}
	return s.Store.Put(r)
}

func TestCurationErrorsRetainCauseAndPartialBatchOrder(t *testing.T) {
	s, service := readFixture(t)
	ctx := context.Background()
	cause := errors.New("fixture write failed")
	ids := []string{}
	for _, content := range []string{"first record", "second record", "third record"} {
		out, err := service.Remember(ctx, appmemory.RememberInput{Content: content, Tags: map[string]string{"scope": "agent"}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, out.ID)
	}
	fail := appmemory.New(store.NewManager(writeFailStore{Store: s, cause: cause}, nil))
	_, err := fail.Remember(ctx, appmemory.RememberInput{Content: "new record"})
	if !errors.Is(err, cause) {
		t.Errorf("remember cause=%v", err)
	}
	_, err = fail.Supersede(ctx, appmemory.SupersedeInput{RememberInput: appmemory.RememberInput{Content: "new revision"}, OldID: ids[0]})
	if !errors.Is(err, cause) {
		t.Errorf("supersede cause=%v", err)
	}
	_, err = fail.Forget(ctx, appmemory.GetInput{ID: ids[0]})
	if !errors.Is(err, cause) {
		t.Errorf("forget cause=%v", err)
	}
	_, err = fail.Promote(ctx, appmemory.GetInput{ID: ids[0]})
	if !errors.Is(err, cause) {
		t.Errorf("promote cause=%v", err)
	}
	fail = appmemory.New(store.NewManager(writeFailStore{Store: s, cause: cause, failID: ids[1]}, nil))
	_, err = fail.BulkForget(ctx, appmemory.BulkForgetInput{IDs: ids})
	if !errors.Is(err, cause) {
		t.Fatalf("batch cause=%v", err)
	}
	for i, id := range ids {
		rec, _, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Tombstoned != (i == 0) {
			t.Errorf("partial batch index=%d record=%+v", i, rec)
		}
	}
}
