// SPDX-License-Identifier: MIT

// Package journalview owns the shared newest-first journal projection mechanics.
// Decoders see every event before row cutoff; callers own admission and view shaping.
package journalview

import (
	"sort"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

type Reader interface {
	Range(func(*event.Event) error) error
}
type Input struct {
	// Limit is the caller's admitted positive page size.
	Limit    int
	CutoffMS int64
	Cursor   any
}
type Output struct {
	Rows       []map[string]any
	Count      int
	NextCursor string
}

func Project(reader Reader, in Input, decode func(*event.Event) (map[string]any, bool)) (Output, error) {
	limit, cutoff := in.Limit, in.CutoffMS
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(in.Cursor)
	type row struct {
		ts, seq int64
		view    map[string]any
	}
	rows := make([]row, 0)
	if err := reader.Range(func(e *event.Event) error {
		view, ok := decode(e)
		if !ok {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		view["ts_unix_ms"] = e.TSUnixMS
		view["seq"] = e.Seq // A2: stable per-row id for the frontend cursor pager
		rows = append(rows, row{ts: e.TSUnixMS, seq: e.Seq, view: view})
		return nil
	}); err != nil {
		return Output{}, err
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if cursorOK { // A2: keep rows strictly older than the cursor, before the limit
		kept := rows[:0]
		for _, r := range rows {
			if journal.KeepBeforeCursor(r.ts, r.seq, cursorMS, cursorSeq) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view)
	}
	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(rows); n > 0 {
		nextCursor = journal.NextCursor(rows[n-1].ts, rows[n-1].seq, n, limit)
	}
	return Output{Rows: out, Count: len(out), NextCursor: nextCursor}, nil
}
