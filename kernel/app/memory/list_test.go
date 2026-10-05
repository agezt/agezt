// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	store "github.com/agezt/agezt/kernel/memory"
)

type snapshotStore struct {
	store.Store
	records []store.Record
	cause   error
}

func (s snapshotStore) All() ([]store.Record, error) {
	return append([]store.Record(nil), s.records...), s.cause
}

func TestPreparedListRetainsCursorOrderingTotalAndIndependentPages(t *testing.T) {
	rs := []store.Record{
		{ID: "a", Content: "a", CreatedMS: 100}, {ID: "b", Content: "b", CreatedMS: 100}, {ID: "c", Content: "c", CreatedMS: 100},
		{ID: "y", Content: "y", CreatedMS: 99}, {ID: "z", Content: "z", CreatedMS: 99}, {ID: "x", Content: "x", CreatedMS: 98},
		{ID: "hidden", Content: "hidden", CreatedMS: 101, Tombstoned: true},
	}
	service := appmemory.New(store.NewManager(snapshotStore{records: rs}, nil))
	ctx := context.Background()
	prepared, err := service.PrepareList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cursor := ""
	all := []string{}
	for _, want := range [][]string{{"c", "b"}, {"a", "z"}, {"y", "x"}} {
		out, err := prepared.Page(ctx, appmemory.ListInput{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, r := range out.Records {
			ids = append(ids, r.ID)
		}
		if !reflect.DeepEqual(ids, want) || out.Count != 2 || out.Total != 6 {
			t.Fatalf("page cursor=%s out=%+v want=%v", cursor, out, want)
		}
		all = append(all, ids...)
		cursor = out.NextCursor
	}
	if cursor != "" || len(all) != 6 {
		t.Fatalf("terminal page cursor=%s all=%v", cursor, all)
	}
	for _, in := range []appmemory.ListInput{{Limit: 2}, {Limit: 2, Cursor: "invalid"}} {
		out, err := prepared.Page(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Records) != 2 || out.Records[0].ID != "c" || out.Records[1].ID != "b" || out.NextCursor != "100:b" {
			t.Fatalf("prepared read changed: %+v", out)
		}
	}
}

func TestPreparedListRetainsLegacyLimitAndEmptyWire(t *testing.T) {
	rs := make([]store.Record, 1005)
	for i := range rs {
		rs[i] = store.Record{ID: fmt.Sprintf("%04d", i), Content: "fixture", CreatedMS: int64(i)}
	}
	service := appmemory.New(store.NewManager(snapshotStore{records: rs}, nil))
	ctx := context.Background()
	prepared, err := service.PrepareList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw   float64
		count int
	}{{0, 100}, {-1, 100}, {1, 1}, {1.9, 1}, {.5, 1005}, {2000, 1000}} {
		out, err := prepared.Page(ctx, appmemory.ListInput{Limit: tc.raw})
		if err != nil {
			t.Fatal(err)
		}
		if out.Count != tc.count || len(out.Records) != tc.count || out.Total != 1005 {
			t.Errorf("limit=%v count=%d/%d total=%d", tc.raw, out.Count, len(out.Records), out.Total)
		}
	}
	empty, err := appmemory.New(store.NewManager(snapshotStore{}, nil)).PrepareList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := empty.Page(ctx, appmemory.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"records":[],"count":0,"total":0}` {
		t.Fatalf("empty wire=%s", raw)
	}
	cause := errors.New("fixture active read failed")
	if _, err := appmemory.New(store.NewManager(snapshotStore{cause: cause}, nil)).PrepareList(ctx); !errors.Is(err, cause) {
		t.Fatalf("prepare cause=%v", err)
	}
}
