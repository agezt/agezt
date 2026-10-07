// SPDX-License-Identifier: MIT
package artifacts_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	appartifacts "github.com/agezt/agezt/kernel/app/artifacts"
	blobs "github.com/agezt/agezt/kernel/artifact"
	"reflect"
	"testing"
	"time"
)

type blob struct {
	ref   string
	data  []byte
	cause error
}

func (b *blob) Get(ref string) ([]byte, error) { b.ref = ref; return b.data, b.cause }

type index struct {
	rows                       []blobs.Entry
	filter                     blobs.Filter
	staleCutoff, collectCutoff int64
	staleCalls, collectCalls   int
	deleted                    string
	cause                      error
}

func (i *index) List(filter blobs.Filter) []blobs.Entry { i.filter = filter; return i.rows }
func (i *index) StaleEntries(cutoff int64) []blobs.Entry {
	i.staleCutoff = cutoff
	i.staleCalls++
	return i.rows
}
func (i *index) Collect(cutoff int64) (int, int64) {
	i.collectCutoff = cutoff
	i.collectCalls++
	return 2, 9
}
func (i *index) Delete(id string) error { i.deleted = id; return i.cause }
func TestArtifactGetRetainsBytesReferenceAndErrorMapping(t *testing.T) {
	store := &blob{data: []byte{0, 255, 1, 2}}
	service := appartifacts.New(store, nil, nil)
	out, err := service.Get(context.Background(), appartifacts.GetInput{Ref: "owned-ref"})
	if err != nil || out.Ref != "owned-ref" || store.ref != "owned-ref" || out.Size != 4 || out.Data != base64.StdEncoding.EncodeToString(store.data) {
		t.Fatal(out, err, store.ref)
	}
	cause := errors.New("owned cause")
	for _, item := range []struct {
		cause error
		want  string
	}{{blobs.ErrBadRef, "malformed ref (want a 64-hex content address)"}, {blobs.ErrNotFound, "artifact not found: owned-ref"}, {blobs.ErrCorrupt, "artifact CORRUPT (bytes do not match ref): owned-ref"}, {cause, "owned cause"}} {
		store.cause = item.cause
		out, err = service.Get(context.Background(), appartifacts.GetInput{Ref: "owned-ref"})
		if err == nil || err.Error() != item.want || !reflect.DeepEqual(out, appartifacts.GetOutput{}) {
			t.Fatal(out, err, item.want)
		}
		if item.cause == cause && err != cause {
			t.Fatal("original cause changed")
		}
	}
}
func TestArtifactListRetainsRequiredEmptyFieldsAndFilters(t *testing.T) {
	idx := &index{rows: []blobs.Entry{{ID: "owned", Ref: "ref", Size: 3, CreatedMs: 100}}}
	out, err := appartifacts.New(nil, idx, nil).List(context.Background(), appartifacts.ListInput{Kind: "image", Source: "run", CorrelationID: "corr"})
	if err != nil || out.Count != 1 || !reflect.DeepEqual(idx.filter, blobs.Filter{Kind: "image", Source: "run", Corr: "corr"}) {
		t.Fatal(out, err, idx.filter)
	}
	raw, _ := json.Marshal(out)
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	entry := values["entries"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "ref", "name", "mime", "kind", "source", "sender", "corr", "size", "created_ms", "caption"} {
		if _, exists := entry[key]; !exists {
			t.Fatal("required empty field lost", key, string(raw))
		}
	}
	idx.rows = nil
	out, err = appartifacts.New(nil, idx, nil).List(context.Background(), appartifacts.ListInput{})
	raw, _ = json.Marshal(out)
	if err != nil || string(raw) != "{\"count\":0,\"entries\":[]}" {
		t.Fatal(out, err, string(raw))
	}
}
func TestArtifactCollectRetainsCutoffProjectionAndDryRunSeparation(t *testing.T) {
	now := time.UnixMilli(100 * 24 * 60 * 60 * 1000)
	for _, days := range []int{0, -1, 7} {
		for _, dry := range []bool{true, false} {
			idx := &index{rows: []blobs.Entry{{ID: "first", Name: "one", Kind: "file", Source: "run", Size: 4, CreatedMs: 1}, {ID: "second", Name: "two", Size: 5, CreatedMs: 2}}}
			out, err := appartifacts.New(nil, idx, func() time.Time { return now }).Collect(context.Background(), appartifacts.CollectInput{OlderThanDays: days, DryRun: dry})
			wantDays := days
			if wantDays <= 0 {
				wantDays = 30
			}
			wantCutoff := now.Add(-time.Duration(wantDays) * 24 * time.Hour).UnixMilli()
			if err != nil || out.OlderThanDays != wantDays || out.CutoffMS != wantCutoff || out.DryRun != dry || out.Count != 2 || out.Bytes != 9 {
				t.Fatal(out, err)
			}
			raw, _ := json.Marshal(out)
			var values map[string]any
			_ = json.Unmarshal(raw, &values)
			_, present := values["candidates"]
			if present != dry {
				t.Fatal("candidate presence", dry, string(raw))
			}
			if dry {
				if idx.collectCalls != 0 || idx.staleCalls != 1 || idx.staleCutoff != wantCutoff || out.Candidates == nil || (*out.Candidates)[0].Name != "one" || (*out.Candidates)[1].CreatedMS != 2 {
					t.Fatal(out, idx)
				}
			} else if idx.staleCalls != 0 || idx.collectCalls != 1 || idx.collectCutoff != wantCutoff || out.Candidates != nil {
				t.Fatal(out, idx)
			}
		}
	}
	idx := &index{}
	out, err := appartifacts.New(nil, idx, func() time.Time { return now }).Collect(context.Background(), appartifacts.CollectInput{DryRun: true})
	raw, _ := json.Marshal(out)
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	if err != nil || out.Candidates == nil || values["candidates"] == nil || len(values["candidates"].([]any)) != 0 {
		t.Fatal(out, err, string(raw))
	}
}
func TestArtifactDeleteRetainsSelectedIDAndCauseWithoutPartialOutput(t *testing.T) {
	idx := &index{}
	service := appartifacts.New(nil, idx, nil)
	out, err := service.Delete(context.Background(), appartifacts.DeleteInput{ID: "owned"})
	if err != nil || !out.Deleted || out.ID != "owned" || idx.deleted != "owned" {
		t.Fatal(out, err, idx)
	}
	cause := errors.New("owned delete cause")
	idx.cause = cause
	out, err = service.Delete(context.Background(), appartifacts.DeleteInput{ID: "owned"})
	if err != cause || !reflect.DeepEqual(out, appartifacts.DeleteOutput{}) {
		t.Fatal(out, err)
	}
	idx.cause = blobs.ErrNotFound
	if _, err := service.Delete(context.Background(), appartifacts.DeleteInput{ID: "missing"}); err == nil || err.Error() != "artifact not found: missing" {
		t.Fatal(err)
	}
}
func TestArtifactServicesRetainUnavailableStoreAndIndexErrors(t *testing.T) {
	s := appartifacts.New(nil, nil, nil)
	if _, err := s.Get(context.Background(), appartifacts.GetInput{}); err == nil || err.Error() != "artifact store unavailable" {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), appartifacts.ListInput{}); err == nil || err.Error() != "artifact index unavailable" {
		t.Fatal(err)
	}
	if _, err := s.Delete(context.Background(), appartifacts.DeleteInput{}); err == nil || err.Error() != "artifact index unavailable" {
		t.Fatal(err)
	}
	if _, err := s.Collect(context.Background(), appartifacts.CollectInput{}); err == nil || err.Error() != "artifact index unavailable" {
		t.Fatal(err)
	}
}
