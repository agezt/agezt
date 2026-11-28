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

func (d *Decisions) cutoff(since int64) int64 {
	if since > 0 {
		return d.now().UnixMilli() - since
	}
	return 0
}
func (s decision) matches(tool, capability string) bool {
	return (tool == "" || s.Tool == tool) && (capability == "" || s.Capability == capability)
}

// Log lists policy decisions newest first: a strict denied flag, then the
// tool and capability filters, then the lenient page.
func (d *Decisions) Log(_ context.Context, in opapi.EdictDecisionLogRequest) (DecisionsOutput, error) {
	tool, capability := in.Tool, in.Capability
	limit := defaultDecisionLimit
	if in.Limit.Present() {
		limit = int(in.Limit.Number())
	}
	page := journalview.Input{Limit: min(max(limit, 1), maxDecisionLimit), CutoffMS: d.cutoff(int64(in.SinceMS.Number())), Cursor: in.Cursor}
	out, err := journalview.ProjectValues(d.journal, page, func(e *event.Event) (DecisionRow, bool) {
		if e.Kind != event.KindPolicyDecision {
			return DecisionRow{}, false
		}
		p := decodeDecision(e.Payload)
		if in.Denied && p.Allow || !p.matches(tool, capability) {
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
func (d *Decisions) Stats(_ context.Context, in opapi.EdictDecisionStatsRequest) (DecisionStatsOutput, error) {
	since := int64(in.SinceMS.Number())
	tool, capability := in.Tool, in.Capability
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
			return bind(&ops, opapi.Spec{Name: opapi.EdictLog, ReadOnly: true}, func(ctx context.Context, in opapi.EdictDecisionLogRequest) (DecisionsOutput, error) {
				return provider(ctx).Log(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.EdictStats, ReadOnly: true}, func(ctx context.Context, in opapi.EdictDecisionStatsRequest) (DecisionStatsOutput, error) {
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
