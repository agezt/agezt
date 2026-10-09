// SPDX-License-Identifier: MIT

// Package missioncontrol owns the console's Mission Control status reads: the
// spend-today tile and the needs-your-attention feed.
package missioncontrol

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	DefaultWindow = 24 * time.Hour
	DefaultLimit  = 8
	MaxLimit      = 50
)

// Ports reads the primary kernel. Spend is false when no governor tracks spend;
// Pending and Asks return nil when there is no approval registry or no pulse.
type Ports struct {
	Spend   func() (int64, bool)
	Pending func() []approval.Request
	Asks    func() []map[string]any
	Now     func() time.Time
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

type SpendRequest struct{}

// SpendOutput is the tile's whole shape: today's spend in microcents, zero when
// spend is not tracked.
type SpendOutput struct {
	Total int64 `json:"total"`
}

func (s *Service) SpendToday(_ context.Context, _ SpendRequest) (SpendOutput, error) {
	total, _ := s.ports.Spend()
	return SpendOutput{Total: total}, nil
}

type AttentionRequest struct {
	Window json.RawMessage `json:"window,omitempty"`
	Limit  json.RawMessage `json:"limit,omitempty"`
}

type Item struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	TS      int64  `json:"ts"`
	HRef    string `json:"href"`
}

type AttentionOutput struct {
	Items []Item `json:"items"`
	Count int    `json:"count"`
}

func rawAny(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v, true
}

// attentionArgs never rejects: a bad window or limit falls back to the
// defaults, because the feed is a status read. A window is a duration string or
// a number of seconds; a limit is a count or its string, capped at MaxLimit.
func attentionArgs(in AttentionRequest) (time.Duration, int) {
	window := DefaultWindow
	if raw, ok := rawAny(in.Window); ok {
		switch v := raw.(type) {
		case string:
			if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
				window = d
			}
		case float64:
			if v > 0 {
				window = time.Duration(v * float64(time.Second))
			}
		}
	}
	limit := DefaultLimit
	if raw, ok := rawAny(in.Limit); ok {
		switch v := raw.(type) {
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				limit = n
			}
		case float64:
			if v > 0 {
				limit = int(v)
			}
		}
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	return window, limit
}

// approvalSummary is one line: "<tool> — <reason>" for tool approvals,
// "<capability> requested by <actor> — <reason>" otherwise, and the approval id
// when there is nothing else, so a card is never empty.
func approvalSummary(p approval.Request) string {
	if p.ToolName != "" {
		if p.Reason != "" {
			return p.ToolName + " — " + p.Reason
		}
		return p.ToolName
	}
	if p.Capability != "" {
		who := p.Actor
		if who == "" {
			who = "agent"
		}
		if p.Reason != "" {
			return p.Capability + " requested by " + who + " — " + p.Reason
		}
		return p.Capability + " requested by " + who
	}
	return p.ID
}

// Attention merges every pending approval (an open task has no window) with the
// pulse asks raised inside the window, newest first with id order breaking
// ties, then keeps the first limit.
func (s *Service) Attention(_ context.Context, in AttentionRequest) (AttentionOutput, error) {
	window, limit := attentionArgs(in)
	cutoff := s.ports.Now().Add(-window).UnixMilli()
	items := make([]Item, 0, 16)
	for _, p := range s.ports.Pending() {
		items = append(items, Item{ID: p.ID, Kind: "approval", Summary: approvalSummary(p), TS: p.CreatedAt.UnixMilli(), HRef: "/approvals"})
	}
	for _, raw := range s.ports.Asks() {
		issue, _ := raw["issue_key"].(string)
		summary, _ := raw["summary"].(string)
		ts, _ := raw["ts_unix_ms"].(int64)
		if ts < cutoff || issue == "" {
			continue
		}
		items = append(items, Item{ID: issue, Kind: "pulse_ask", Summary: summary, TS: ts, HRef: "/jarvis#ask-" + issue})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TS != items[j].TS {
			return items[i].TS > items[j].TS
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return AttentionOutput{Items: items, Count: len(items)}, nil
}

// Operations declares the two unaudited, operator-only Mission Control reads on
// their Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("mission control provider required")
	}
	spendOut, err := schema.FromType(reflect.TypeFor[SpendOutput](), false)
	if err != nil {
		return nil, err
	}
	attentionOut, err := schema.FromType(reflect.TypeFor[AttentionOutput](), false)
	if err != nil {
		return nil, err
	}
	spend, err := app.NewOperation(opapi.Spec{Name: "spend_today", ReadOnly: true, OutputSchema: spendOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/spend/today"}}, func(ctx context.Context, in SpendRequest) (SpendOutput, error) {
		return provider(ctx).SpendToday(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	attention, err := app.NewOperation(opapi.Spec{Name: "attention", ReadOnly: true, OutputSchema: attentionOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"window":{},"limit":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/attention"}}, func(ctx context.Context, in AttentionRequest) (AttentionOutput, error) {
		return provider(ctx).Attention(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{spend, attention}, nil
}
