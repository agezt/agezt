// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

type LogInput struct {
	Limit    int
	CutoffMS int64
	Cursor   any
	OpFilter string
}
type LogRow struct {
	Op       string `json:"op"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Subject  string `json:"subject"`
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
		var op, id, subject, mtyp string
		switch e.Kind {
		case event.KindMemoryWritten:
			var p struct{ Action, ID, Type, Subject string }
			_ = json.Unmarshal(e.Payload, &p)
			op = p.Action
			if op == "" {
				op = "write"
			}
			id, subject, mtyp = p.ID, p.Subject, p.Type
		case event.KindMemoryForgotten:
			var p struct{ ID, Subject string }
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "forget", p.ID, p.Subject
		case event.KindMemorySuperseded:
			var p struct {
				OldID string `json:"old_id"`
				NewID string `json:"new_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "supersede", p.OldID, "→ "+p.NewID
		case event.KindMemoryPromoted:
			var p struct {
				ID        string `json:"id"`
				Subject   string `json:"subject"`
				FromScope string `json:"from_scope"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			op, id, subject = "promote", p.ID, p.Subject+" (was private to "+p.FromScope+")"
		default:
			return nil, false
		}
		if in.OpFilter != "" && !memOpMatches(in.OpFilter, e.Kind, op) {
			return nil, false
		}
		return map[string]any{"op": op, "id": id, "type": mtyp, "subject": subject}, true
	})
	if err != nil {
		return LogOutput{}, err
	}
	rows := make([]LogRow, 0, len(output.Rows))
	for _, row := range output.Rows {
		rows = append(rows, LogRow{Op: row["op"].(string), ID: row["id"].(string), Type: row["type"].(string), Subject: row["subject"].(string), TSUnixMS: row["ts_unix_ms"].(int64), Seq: row["seq"].(int64)})
	}
	return LogOutput{Ops: rows, Count: output.Count, NextCursor: output.NextCursor}, nil
}

func memOpMatches(filter string, kind event.Kind, op string) bool {
	switch filter {
	case "written", "write":
		return kind == event.KindMemoryWritten
	case "forgotten", "forget":
		return kind == event.KindMemoryForgotten
	case "superseded", "supersede":
		return kind == event.KindMemorySuperseded
	case "promoted", "promote":
		return kind == event.KindMemoryPromoted
	default:
		return op == filter
	}
}
