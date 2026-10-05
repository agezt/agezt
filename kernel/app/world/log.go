// SPDX-License-Identifier: MIT

package world

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// LogInput contains the adapter-admitted page size and absolute window cutoff.
type LogInput struct {
	Limit      int
	CutoffMS   int64
	Cursor     any
	KindFilter string
}
type LogRow struct {
	Op       string `json:"op"`
	What     string `json:"what"`
	Label    string `json:"label"`
	TSUnixMS int64  `json:"ts_unix_ms"`
	Seq      int64  `json:"seq"`
}
type LogOutput struct {
	Ops        []LogRow `json:"ops"`
	Count      int      `json:"count"`
	NextCursor string   `json:"next_cursor"`
}
type LogService struct{ journal journalview.Reader }

func NewLog(reader journalview.Reader) *LogService { return &LogService{journal: reader} }

func (s *LogService) Log(_ context.Context, in LogInput) (LogOutput, error) {
	output, err := journalview.Project(s.journal, journalview.Input{Limit: in.Limit, CutoffMS: in.CutoffMS, Cursor: in.Cursor}, func(e *event.Event) (map[string]any, bool) {
		var op, what, label string
		switch e.Kind {
		case event.KindWorldEntityUpserted:
			var p struct{ Action, Name, Kind string }
			_ = json.Unmarshal(e.Payload, &p)
			op, what, label = p.Action, "entity", p.Name
			if p.Kind != "" {
				label += " [" + p.Kind + "]"
			}
		case event.KindWorldRelationUpserted:
			var p struct{ Action, From, Verb, To string }
			_ = json.Unmarshal(e.Payload, &p)
			op, what, label = p.Action, "relation", p.From+" "+p.Verb+" "+p.To
		case event.KindWorldForgotten:
			var p struct{ Name, Verb, What string }
			_ = json.Unmarshal(e.Payload, &p)
			op, what = "forget", p.What
			if p.Name != "" {
				label = p.Name
			} else {
				label = p.Verb
			}
		default:
			return nil, false
		}
		if op == "" {
			op = "upsert"
		}
		if in.KindFilter != "" && what != in.KindFilter {
			return nil, false
		}
		return map[string]any{"op": op, "what": what, "label": label}, true
	})
	if err != nil {
		return LogOutput{}, err
	}
	rows := make([]LogRow, 0, len(output.Rows))
	for _, row := range output.Rows {
		rows = append(rows, LogRow{Op: row["op"].(string), What: row["what"].(string), Label: row["label"].(string), TSUnixMS: row["ts_unix_ms"].(int64), Seq: row["seq"].(int64)})
	}
	return LogOutput{Ops: rows, Count: output.Count, NextCursor: output.NextCursor}, nil
}
