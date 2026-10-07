// SPDX-License-Identifier: MIT
package journalview_test

import (
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"reflect"
	"testing"
)

func TestProjectValuesMatchesMapPagingAndPreservesDecoderBeforeCutoff(t *testing.T) {
	type item struct {
		ID  string
		Seq int64
	}
	events := []*event.Event{{ID: "old", Seq: 0, TSUnixMS: 10}, {ID: "lower-tie", Seq: 1, TSUnixMS: 20}, {ID: "upper-tie", Seq: 2, TSUnixMS: 20}}
	decoded := 0
	in := journalview.Input{Limit: 1, CutoffMS: 20}
	typed, err := journalview.ProjectValues(reader{events: events}, in, func(e *event.Event) (item, bool) { decoded++; return item{ID: e.ID, Seq: e.Seq}, true })
	if err != nil || typed.Count != 1 || decoded != 3 || typed.Rows[0].ID != "upper-tie" || typed.NextCursor == "" {
		t.Fatal(typed, err, decoded)
	}
	maps, err := journalview.Project(reader{events: events}, in, decode)
	if err != nil || maps.Count != typed.Count || maps.NextCursor != typed.NextCursor || maps.Rows[0]["id"] != typed.Rows[0].ID || maps.Rows[0]["seq"] != typed.Rows[0].Seq {
		t.Fatal(maps, typed, err)
	}
	next, err := journalview.ProjectValues(reader{events: events}, journalview.Input{Limit: 1, CutoffMS: 20, Cursor: typed.NextCursor}, func(e *event.Event) (item, bool) { return item{ID: e.ID, Seq: e.Seq}, true })
	if err != nil || next.Count != 1 || next.Rows[0].ID != "lower-tie" {
		t.Fatal(next, err)
	}
	// The compatibility map bridge stamps only rows that survive the cutoff.
	saved := map[string]any{"id": "old"}
	_, err = journalview.Project(reader{events: events[:1]}, in, func(*event.Event) (map[string]any, bool) { return saved, true })
	if err != nil || !reflect.DeepEqual(saved, map[string]any{"id": "old"}) {
		t.Fatal(saved, err)
	}
	cause := errors.New("owned range failure")
	failed, err := journalview.ProjectValues(reader{events: events, err: cause}, in, func(e *event.Event) (item, bool) { return item{ID: e.ID}, true })
	if err != cause || !reflect.DeepEqual(failed, journalview.ValuesOutput[item]{}) {
		t.Fatal(failed, err)
	}
}
