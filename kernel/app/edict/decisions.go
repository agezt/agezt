// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

const (
	defaultDecisionLimit = 20
	maxDecisionLimit     = 1_000
)

// Decisions folds one kernel's policy.decision records: the decisions the
// rules produced, one per gated tool call.
type Decisions struct {
	journal journalview.Reader
	now     func() time.Time
}

func NewDecisions(journal journalview.Reader, now func() time.Time) *Decisions {
	return &Decisions{journal: journal, now: now}
}

type DecisionLogRequest struct {
	Denied     json.RawMessage `json:"denied,omitempty"`
	Tool       json.RawMessage `json:"tool,omitempty"`
	Capability json.RawMessage `json:"capability,omitempty"`
	Limit      json.RawMessage `json:"limit,omitempty"`
	SinceMS    json.RawMessage `json:"since_ms,omitempty"`
	Cursor     json.RawMessage `json:"cursor,omitempty"`
}

type DecisionStatsRequest struct {
	SinceMS    json.RawMessage `json:"since_ms,omitempty"`
	Tool       json.RawMessage `json:"tool,omitempty"`
	Capability json.RawMessage `json:"capability,omitempty"`
}

type DecisionRow struct {
	Actor         string `json:"actor"`
	CorrelationID string `json:"correlation_id"`
	Tool          string `json:"tool"`
	Capability    string `json:"capability"`
	Allow         bool   `json:"allow"`
	Reason        string `json:"reason"`
	HardDenied    bool   `json:"hard_denied"`
	TSUnixMS      int64  `json:"ts_unix_ms"`
	Seq           int64  `json:"seq"`
}

type DecisionsOutput struct {
	Decisions  []DecisionRow `json:"decisions"`
	Count      int           `json:"count"`
	NextCursor string        `json:"next_cursor"`
}

type DecisionStatsOutput struct {
	Total              int            `json:"total"`
	Allowed            int            `json:"allowed"`
	Denied             int            `json:"denied"`
	HardDenied         int            `json:"hard_denied"`
	DenialRate         float64        `json:"denial_rate"`
	DeniedByCapability map[string]int `json:"denied_by_capability"`
	WindowMS           int64          `json:"window_ms"`
}

type decision struct {
	Tool       string `json:"tool"`
	Capability string `json:"capability"`
	Reason     string `json:"reason"`
	Allow      bool   `json:"allow"`
	HardDenied bool   `json:"hard_denied"`
}

// decodeDecision reads a policy.decision payload; any malformed field zeroes
// the whole decision, which then counts as a denial.
func decodeDecision(payload json.RawMessage) decision {
	var d decision
	if len(payload) == 0 || json.Unmarshal(payload, &d) != nil {
		return decision{}
	}
	return d
}

// lenientInt64 reads a JSON number truncated toward zero; anything else is 0.
func lenientInt64(raw json.RawMessage) int64 {
	n, _ := rawValue(raw).(float64)
	return int64(n)
}

func (d *Decisions) cutoff(since int64) int64 {
	if since > 0 {
		return d.now().UnixMilli() - since
	}
	return 0
}

// scope reads the strict tool and capability filters, in that order.
func scope(tool, capability json.RawMessage) (string, string, error) {
	t, err := optionalString(tool, "tool")
	if err != nil {
		return "", "", err
	}
	c, err := optionalString(capability, "capability")
	if err != nil {
		return "", "", err
	}
	return t, c, nil
}

func (s decision) matches(tool, capability string) bool {
	return (tool == "" || s.Tool == tool) && (capability == "" || s.Capability == capability)
}

// Log lists policy decisions newest first: a strict denied flag, then the
// strict tool and capability filters, then the lenient page.
func (d *Decisions) Log(_ context.Context, in DecisionLogRequest) (DecisionsOutput, error) {
	var deniedOnly bool
	if len(in.Denied) != 0 {
		b, ok := rawValue(in.Denied).(bool)
		if !ok {
			return DecisionsOutput{}, errors.New("args.denied must be a boolean")
		}
		deniedOnly = b
	}
	tool, capability, err := scope(in.Tool, in.Capability)
	if err != nil {
		return DecisionsOutput{}, err
	}
	limit := defaultDecisionLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	page := journalview.Input{Limit: min(max(limit, 1), maxDecisionLimit), CutoffMS: d.cutoff(lenientInt64(in.SinceMS)), Cursor: rawValue(in.Cursor)}
	out, err := journalview.ProjectValues(d.journal, page, func(e *event.Event) (DecisionRow, bool) {
		if e.Kind != event.KindPolicyDecision {
			return DecisionRow{}, false
		}
		p := decodeDecision(e.Payload)
		if deniedOnly && p.Allow || !p.matches(tool, capability) {
			return DecisionRow{}, false
		}
		return DecisionRow{Actor: e.Actor, CorrelationID: e.CorrelationID, Tool: p.Tool, Capability: p.Capability, Allow: p.Allow, Reason: p.Reason, HardDenied: p.HardDenied, TSUnixMS: e.TSUnixMS, Seq: e.Seq}, true
	})
	if err != nil {
		return DecisionsOutput{}, err
	}
	return DecisionsOutput{Decisions: out.Rows, Count: out.Count, NextCursor: out.NextCursor}, nil
}

// Stats aggregates the decisions in the window: totals, the denial rate and the
// denials by capability ("unknown" when none was recorded).
func (d *Decisions) Stats(_ context.Context, in DecisionStatsRequest) (DecisionStatsOutput, error) {
	since := lenientInt64(in.SinceMS)
	tool, capability, err := scope(in.Tool, in.Capability)
	if err != nil {
		return DecisionStatsOutput{}, err
	}
	cutoff := d.cutoff(since)
	out := DecisionStatsOutput{DeniedByCapability: map[string]int{}, WindowMS: since}
	if err := d.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision || cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		p := decodeDecision(e.Payload)
		if !p.matches(tool, capability) {
			return nil
		}
		out.Total++
		if p.Allow {
			out.Allowed++
			return nil
		}
		out.Denied++
		if p.HardDenied {
			out.HardDenied++
		}
		name := p.Capability
		if name == "" {
			name = "unknown"
		}
		out.DeniedByCapability[name]++
		return nil
	}); err != nil {
		return DecisionStatsOutput{}, err
	}
	if out.Total > 0 {
		out.DenialRate = float64(out.Denied) / float64(out.Total)
	}
	return out, nil
}

// DecisionOperations declares the two unaudited decision reads. Each routes to
// the caller's tenant kernel, so a tenant sees only its own decisions.
func DecisionOperations(provider func(context.Context) *Decisions) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("edict decisions provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "edict_log", ReadOnly: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"denied":{},"tool":{},"capability":{},"limit":{},"since_ms":{},"cursor":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/policy_log"}}, func(ctx context.Context, in DecisionLogRequest) (DecisionsOutput, error) {
				return provider(ctx).Log(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "edict_stats", ReadOnly: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"since_ms":{},"tool":{},"capability":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/policy"}}, func(ctx context.Context, in DecisionStatsRequest) (DecisionStatsOutput, error) {
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
