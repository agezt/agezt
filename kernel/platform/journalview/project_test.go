// SPDX-License-Identifier: MIT

package journalview_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

type reader struct {
	events []*event.Event
	err    error
}

func (r reader) Range(fn func(*event.Event) error) error {
	for _, e := range r.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return r.err
}

func decode(e *event.Event) (map[string]any, bool) {
	return map[string]any{"id": e.ID}, true
}

func TestProjectPagination(t *testing.T) {
	r := reader{events: []*event.Event{
		{ID: "old", TSUnixMS: 10, Seq: 1},
		{ID: "tie-low", TSUnixMS: 20, Seq: 2},
		{ID: "new", TSUnixMS: 30, Seq: 4},
		{ID: "tie-high", TSUnixMS: 20, Seq: 3},
	}}
	for _, tc := range []struct {
		name   string
		cursor any
		ids    []string
		next   string
	}{
		{"first", nil, []string{"new", "tie-high"}, "20:3"},
		{"second", "20:3", []string{"tie-low", "old"}, "10:1"},
		{"short", "20:2", []string{"old"}, ""},
		{"empty", "10:1", []string{}, ""},
		{"malformed", "not-a-cursor", []string{"new", "tie-high"}, "20:3"},
		{"wrong-type", 123, []string{"new", "tie-high"}, "20:3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := journalview.Project(r, journalview.Input{Limit: 2, Cursor: tc.cursor}, decode)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(out.Rows))
			for _, row := range out.Rows {
				ids = append(ids, row["id"].(string))
				if _, ok := row["ts_unix_ms"].(int64); !ok {
					t.Fatalf("missing timestamp: %#v", row)
				}
				if _, ok := row["seq"].(int64); !ok {
					t.Fatalf("missing sequence: %#v", row)
				}
			}
			if !reflect.DeepEqual(ids, tc.ids) || out.Count != len(tc.ids) || out.NextCursor != tc.next || out.Rows == nil {
				t.Fatalf("output = %#v, ids = %v; want ids %v, cursor %q", out, ids, tc.ids, tc.next)
			}
		})
	}
}

func TestProjectDecodesOutsideWindowBeforeCutoff(t *testing.T) {
	r := reader{events: []*event.Event{
		{ID: "invocation", TSUnixMS: 5, Seq: 1},
		{ID: "old-result", TSUnixMS: 9, Seq: 2},
		{ID: "result", TSUnixMS: 10, Seq: 3},
	}}
	var visited []string
	var input string
	out, err := journalview.Project(r, journalview.Input{Limit: 5, CutoffMS: 10}, func(e *event.Event) (map[string]any, bool) {
		visited = append(visited, e.ID)
		if e.ID == "invocation" {
			input = "original input"
			return nil, false
		}
		return map[string]any{"input": input, "id": e.ID}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(visited, []string{"invocation", "old-result", "result"}) {
		t.Fatalf("decoder visits = %v", visited)
	}
	if out.Count != 1 || out.Rows[0]["id"] != "result" || out.Rows[0]["input"] != "original input" || out.NextCursor != "" {
		t.Fatalf("window lost cross-event input or cutoff boundary: %#v", out)
	}
}

func TestProjectRangeFailurePreservesCauseAndDiscardsPartialRows(t *testing.T) {
	cause := errors.New("journal read failed")
	out, err := journalview.Project(reader{events: []*event.Event{{TSUnixMS: 10, Seq: 1}}, err: cause}, journalview.Input{Limit: 1}, decode)
	if !errors.Is(err, cause) || out.Rows != nil || out.Count != 0 || out.NextCursor != "" {
		t.Fatalf("partial result or lost failure: %#v, %v", out, err)
	}
}
