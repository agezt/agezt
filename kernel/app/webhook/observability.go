// SPDX-License-Identifier: MIT
// Package webhook owns outbound delivery observability use cases.
package webhook

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

type Observability struct {
	journal journalview.Reader
	now     func() int64
}

func NewObservability(reader journalview.Reader, now func() int64) *Observability {
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return &Observability{journal: reader, now: now}
}

type LogInput struct {
	Limit      *int
	SinceMS    int64
	Cursor     any
	FailedOnly bool
}
type Delivery struct {
	Attempts  int     `json:"attempts"`
	Error     *string `json:"error,omitempty"`
	EventKind string  `json:"event_kind"`
	OK        bool    `json:"ok"`
	Seq       int64   `json:"seq"`
	Status    *int    `json:"status,omitempty"`
	TSUnixMS  int64   `json:"ts_unix_ms"`
	URL       string  `json:"url"`
}
type LogOutput struct {
	Deliveries []Delivery `json:"deliveries"`
	Count      int        `json:"count"`
	NextCursor string     `json:"next_cursor"`
}
type URLCounts struct {
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
}
type StatsOutput struct {
	ByURL       map[string]URLCounts `json:"by_url"`
	Delivered   int                  `json:"delivered"`
	Failed      int                  `json:"failed"`
	FailureRate float64              `json:"failure_rate"`
	Total       int                  `json:"total"`
	WindowMS    int64                `json:"window_ms"`
}

func (s *Observability) cutoff(since int64) int64 {
	if since <= 0 {
		return 0
	}
	return s.now() - since
}

func (s *Observability) Log(_ context.Context, in LogInput) (LogOutput, error) {
	limit := 20
	if in.Limit != nil {
		limit = *in.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	out, err := journalview.ProjectValues(s.journal, journalview.Input{Limit: limit, CutoffMS: s.cutoff(in.SinceMS), Cursor: in.Cursor}, func(e *event.Event) (Delivery, bool) {
		delivered, failed := e.Kind == event.KindWebhookDelivered, e.Kind == event.KindWebhookFailed
		if !delivered && !failed || in.FailedOnly && !failed {
			return Delivery{}, false
		}
		var payload struct {
			URL       string `json:"url"`
			EventKind string `json:"event_kind"`
			Status    int    `json:"status"`
			Attempts  int    `json:"attempts"`
			Error     string `json:"error"`
		}
		_ = json.Unmarshal(e.Payload, &payload)
		row := Delivery{Attempts: payload.Attempts, EventKind: payload.EventKind, OK: delivered, Seq: e.Seq, TSUnixMS: e.TSUnixMS, URL: payload.URL}
		if delivered {
			row.Status = &payload.Status
		} else {
			row.Error = &payload.Error
		}
		return row, true
	})
	if err != nil {
		return LogOutput{}, err
	}
	return LogOutput{Deliveries: out.Rows, Count: out.Count, NextCursor: out.NextCursor}, nil
}

func (s *Observability) Stats(_ context.Context, since int64) (StatsOutput, error) {
	cutoff := s.cutoff(since)
	out := StatsOutput{ByURL: map[string]URLCounts{}, WindowMS: since}
	if err := s.journal.Range(func(e *event.Event) error {
		delivered, failed := e.Kind == event.KindWebhookDelivered, e.Kind == event.KindWebhookFailed
		if !delivered && !failed || cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var payload struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(e.Payload, &payload)
		count := out.ByURL[payload.URL]
		if delivered {
			out.Delivered++
			count.Delivered++
		} else {
			out.Failed++
			count.Failed++
		}
		out.ByURL[payload.URL] = count
		return nil
	}); err != nil {
		return StatsOutput{}, err
	}
	out.Total = out.Delivered + out.Failed
	if out.Total > 0 {
		out.FailureRate = float64(out.Failed) / float64(out.Total)
	}
	return out, nil
}
