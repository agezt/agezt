// SPDX-License-Identifier: MIT

// Package approvals owns the human-in-the-loop approval history: the journal's
// approval requests joined with their grants, denials and timeouts.
package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	defaultLimit = 20
	maxLimit     = 1_000
)

// History folds one kernel's approval records.
type History struct {
	journal journalview.Reader
	now     func() time.Time
}

func New(journal journalview.Reader, now func() time.Time) *History {
	return &History{journal: journal, now: now}
}

type LogRequest struct {
	Limit   json.RawMessage `json:"limit,omitempty"`
	Cursor  json.RawMessage `json:"cursor,omitempty"`
	Denied  json.RawMessage `json:"denied,omitempty"`
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
}

type StatsRequest struct {
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
}

type Row struct {
	TSUnixMS      int64  `json:"ts_unix_ms"`
	Seq           int64  `json:"seq"`
	ApprovalID    string `json:"approval_id"`
	Capability    string `json:"capability"`
	Tool          string `json:"tool"`
	Reason        string `json:"reason"`
	Actor         string `json:"actor"`
	CorrelationID string `json:"correlation_id"`
	Status        string `json:"status"`
	ResolvedBy    string `json:"resolved_by"`
}

type LogOutput struct {
	Approvals  []Row  `json:"approvals"`
	Count      int    `json:"count"`
	NextCursor string `json:"next_cursor"`
}

type StatsOutput struct {
	Total              int            `json:"total"`
	Granted            int            `json:"granted"`
	Denied             int            `json:"denied"`
	Timeout            int            `json:"timeout"`
	Pending            int            `json:"pending"`
	Resolved           int            `json:"resolved"`
	GrantRate          float64        `json:"grant_rate"`
	DeniedByCapability map[string]int `json:"denied_by_capability"`
	WindowMS           int64          `json:"window_ms"`
}

func rawValue(raw json.RawMessage) any {
	var v any
	if len(raw) != 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// lenientInt64 reads a JSON number truncated toward zero; anything else is 0.
func lenientInt64(raw json.RawMessage) int64 {
	n, _ := rawValue(raw).(float64)
	return int64(n)
}

func (h *History) cutoff(since int64) int64 {
	if since > 0 {
		return h.now().UnixMilli() - since
	}
	return 0
}

// terminal maps a resolution kind to its status.
var terminal = map[event.Kind]string{
	event.KindApprovalGranted: "granted",
	event.KindApprovalDenied:  "denied",
	event.KindApprovalTimeout: "timeout",
}

// Log lists one row per approval, newest request first. A resolution without a
// request still yields a row anchored at the resolution; the request, when it
// comes, re-anchors it. denied keeps the denials and timeouts.
func (h *History) Log(_ context.Context, in LogRequest) (LogOutput, error) {
	limit := defaultLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	limit = min(max(limit, 1), maxLimit)
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(rawValue(in.Cursor))
	var deniedOnly bool
	if len(in.Denied) != 0 {
		b, ok := rawValue(in.Denied).(bool)
		if !ok {
			return LogOutput{}, errors.New("args.denied must be a boolean")
		}
		deniedOnly = b
	}
	cutoff := h.cutoff(lenientInt64(in.SinceMS))
	byID := map[string]*Row{}
	order := make([]*Row, 0)
	get := func(id string, e *event.Event) *Row {
		r := byID[id]
		if r == nil {
			r = &Row{ApprovalID: id, TSUnixMS: e.TSUnixMS, Seq: e.Seq, Status: "pending"}
			byID[id] = r
			order = append(order, r)
		}
		return r
	}
	if err := h.journal.Range(func(e *event.Event) error {
		if e.Kind == event.KindApprovalRequested {
			var p struct {
				ApprovalID string `json:"approval_id"`
				Capability string `json:"capability"`
				ToolName   string `json:"tool_name"`
				Reason     string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			r := get(p.ApprovalID, e)
			r.TSUnixMS, r.Seq = e.TSUnixMS, e.Seq
			r.Capability, r.Tool, r.Reason = p.Capability, p.ToolName, p.Reason
			r.Actor, r.CorrelationID = e.Actor, e.CorrelationID
			return nil
		}
		status, ok := terminal[e.Kind]
		if !ok {
			return nil
		}
		var p struct {
			ApprovalID string `json:"approval_id"`
			ResolvedBy string `json:"resolved_by"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		if p.ApprovalID == "" {
			return nil
		}
		r := get(p.ApprovalID, e)
		if r.Actor == "" {
			r.Actor = e.Actor
		}
		if r.CorrelationID == "" {
			r.CorrelationID = e.CorrelationID
		}
		r.Status, r.ResolvedBy = status, p.ResolvedBy
		return nil
	}); err != nil {
		return LogOutput{}, err
	}
	rows := make([]Row, 0, len(order))
	for _, r := range order {
		if cutoff > 0 && r.TSUnixMS < cutoff || deniedOnly && r.Status != "denied" && r.Status != "timeout" {
			continue
		}
		if cursorOK && !journal.KeepBeforeCursor(r.TSUnixMS, r.Seq, cursorMS, cursorSeq) {
			continue
		}
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TSUnixMS != rows[j].TSUnixMS {
			return rows[i].TSUnixMS > rows[j].TSUnixMS
		}
		return rows[i].Seq > rows[j].Seq
	})
	rows = rows[:min(len(rows), limit)]
	out := LogOutput{Approvals: rows, Count: len(rows)}
	if n := len(rows); n > 0 {
		out.NextCursor = journal.NextCursor(rows[n-1].TSUnixMS, rows[n-1].Seq, n, limit)
	}
	return out, nil
}

// Stats counts the approvals requested in the window by final status, the grant
// rate over resolved ones, and the denials and timeouts by capability ("unknown"
// when none was recorded). A resolution without a request has no request time,
// so any window excludes it.
func (h *History) Stats(_ context.Context, in StatsRequest) (StatsOutput, error) {
	since := lenientInt64(in.SinceMS)
	cutoff := h.cutoff(since)
	type approval struct {
		ts                 int64
		capability, status string
	}
	byID := map[string]*approval{}
	get := func(id string) *approval {
		a := byID[id]
		if a == nil {
			a = &approval{status: "pending"}
			byID[id] = a
		}
		return a
	}
	if err := h.journal.Range(func(e *event.Event) error {
		if e.Kind == event.KindApprovalRequested {
			var p struct {
				ApprovalID string `json:"approval_id"`
				Capability string `json:"capability"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.ApprovalID == "" {
				return nil
			}
			a := get(p.ApprovalID)
			a.ts, a.capability = e.TSUnixMS, p.Capability
			return nil
		}
		status, ok := terminal[e.Kind]
		if !ok {
			return nil
		}
		var p struct {
			ApprovalID string `json:"approval_id"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		if p.ApprovalID != "" {
			get(p.ApprovalID).status = status
		}
		return nil
	}); err != nil {
		return StatsOutput{}, err
	}
	out := StatsOutput{DeniedByCapability: map[string]int{}, WindowMS: since}
	for _, a := range byID {
		if cutoff > 0 && a.ts < cutoff {
			continue
		}
		out.Total++
		switch a.status {
		case "granted":
			out.Granted++
		case "denied":
			out.Denied++
		case "timeout":
			out.Timeout++
		default:
			out.Pending++
		}
		if a.status == "denied" || a.status == "timeout" {
			name := a.capability
			if name == "" {
				name = "unknown"
			}
			out.DeniedByCapability[name]++
		}
	}
	out.Resolved = out.Granted + out.Denied + out.Timeout
	if out.Resolved > 0 {
		out.GrantRate = float64(out.Granted) / float64(out.Resolved)
	}
	return out, nil
}

func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema, spec.ReadOnly, spec.Authz, spec.Tenancy, spec.AllowUnknownInput = output, true, opapi.OwnTenant, opapi.CallerTenant, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the two unaudited approval-history reads. Each routes to
// the caller's tenant kernel, so a tenant sees only its own approvals.
func Operations(provider func(context.Context) *History) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("approvals history provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "approvals_log", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"cursor":{},"denied":{},"since_ms":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/approvals_log"}}, func(ctx context.Context, in LogRequest) (LogOutput, error) {
				return provider(ctx).Log(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "approvals_stats", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"since_ms":{}}}`)}, func(ctx context.Context, in StatsRequest) (StatsOutput, error) {
				return provider(ctx).Stats(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
