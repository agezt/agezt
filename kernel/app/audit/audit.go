// SPDX-License-Identifier: MIT

package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	defaultLimit = 20
	maxLimit     = 1_000
)

// Service folds one kernel's journal.
type Service struct {
	journal journalview.Reader
	now     func() time.Time
}

func New(journal journalview.Reader, now func() time.Time) *Service {
	return &Service{journal: journal, now: now}
}

// PageRequest is the shared log page: a lenient limit, a since_ms window and an
// opaque cursor.
type PageRequest struct {
	Limit   json.RawMessage `json:"limit,omitempty"`
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
	Cursor  json.RawMessage `json:"cursor,omitempty"`
}

type WardenLogRequest struct {
	PageRequest
	Issues json.RawMessage `json:"issues,omitempty"`
}

type WindowRequest struct {
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
}

// Stamp is every log row's stable pager identity.
type Stamp struct {
	TSUnixMS int64 `json:"ts_unix_ms"`
	Seq      int64 `json:"seq"`
}

type BlockRow struct {
	IP     string `json:"ip"`
	Reason string `json:"reason"`
	Tool   string `json:"tool"`
	Stamp
}

type ThrottleRow struct {
	Used        int `json:"used"`
	LimitPerMin int `json:"limit_per_min"`
	Stamp
}

// ExecutionRow carries the keys of its kind only: an exec row has exit code,
// duration and flags, a downgrade the requested profile, a limit breach the
// argv0 and limit; profile is always present.
type ExecutionRow struct {
	Kind       string  `json:"kind"`
	Profile    string  `json:"profile"`
	Argv0      *string `json:"argv0,omitempty"`
	ExitCode   *int    `json:"exit_code,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Downgraded *bool   `json:"downgraded,omitempty"`
	TimedOut   *bool   `json:"timed_out,omitempty"`
	Requested  *string `json:"requested,omitempty"`
	Reason     *string `json:"reason,omitempty"`
	Stamp
}

type BlocksOutput struct {
	Blocks     []BlockRow `json:"blocks"`
	Count      int        `json:"count"`
	NextCursor string     `json:"next_cursor"`
}

type ThrottlesOutput struct {
	Throttles  []ThrottleRow `json:"throttles"`
	Count      int           `json:"count"`
	NextCursor string        `json:"next_cursor"`
}

type ExecutionsOutput struct {
	Executions []ExecutionRow `json:"executions"`
	Count      int            `json:"count"`
	NextCursor string         `json:"next_cursor"`
}

type RateLimitStatsOutput struct {
	Throttled   int   `json:"throttled"`
	LimitPerMin int   `json:"limit_per_min"`
	WorstUsed   int   `json:"worst_used"`
	WindowMS    int64 `json:"window_ms"`
}

type WardenStatsOutput struct {
	Executions    int            `json:"executions"`
	Downgraded    int            `json:"downgraded"`
	DowngradeRate float64        `json:"downgrade_rate"`
	TimedOut      int            `json:"timed_out"`
	LimitBreaches int            `json:"limit_breaches"`
	ByProfile     map[string]int `json:"by_profile"`
	WindowMS      int64          `json:"window_ms"`
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

func (s *Service) cutoff(raw json.RawMessage) int64 {
	if since := lenientInt64(raw); since > 0 {
		return s.now().UnixMilli() - since
	}
	return 0
}

// page admits the shared log page: limit 20 unless a JSON number, clamped to
// 1..1,000.
func (s *Service) page(in PageRequest) journalview.Input {
	limit := defaultLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	return journalview.Input{Limit: min(max(limit, 1), maxLimit), CutoffMS: s.cutoff(in.SinceMS), Cursor: rawValue(in.Cursor)}
}

func stamp(e *event.Event) Stamp { return Stamp{TSUnixMS: e.TSUnixMS, Seq: e.Seq} }

// decode reads a payload leniently: malformed fields stay zero.
func decode(e *event.Event, v any) { _ = json.Unmarshal(e.Payload, v) }

// NetguardLog lists the egress guard's refusals, newest first.
func (s *Service) NetguardLog(_ context.Context, in PageRequest) (BlocksOutput, error) {
	out, err := journalview.ProjectValues(s.journal, s.page(in), func(e *event.Event) (BlockRow, bool) {
		if e.Kind != event.KindNetguardBlocked {
			return BlockRow{}, false
		}
		var p struct {
			IP     string `json:"ip"`
			Reason string `json:"reason"`
			Tool   string `json:"tool"`
		}
		decode(e, &p)
		return BlockRow{IP: p.IP, Reason: p.Reason, Tool: p.Tool, Stamp: stamp(e)}, true
	})
	if err != nil {
		return BlocksOutput{}, err
	}
	return BlocksOutput{Blocks: out.Rows, Count: out.Count, NextCursor: out.NextCursor}, nil
}

type throttle struct {
	Used        int `json:"used"`
	LimitPerMin int `json:"limit_per_min"`
}

// RateLimitLog lists the run-rate throttles, newest first.
func (s *Service) RateLimitLog(_ context.Context, in PageRequest) (ThrottlesOutput, error) {
	out, err := journalview.ProjectValues(s.journal, s.page(in), func(e *event.Event) (ThrottleRow, bool) {
		if e.Kind != event.KindRateLimited {
			return ThrottleRow{}, false
		}
		var p throttle
		decode(e, &p)
		return ThrottleRow{Used: p.Used, LimitPerMin: p.LimitPerMin, Stamp: stamp(e)}, true
	})
	if err != nil {
		return ThrottlesOutput{}, err
	}
	return ThrottlesOutput{Throttles: out.Rows, Count: out.Count, NextCursor: out.NextCursor}, nil
}

// RateLimitStats counts the throttles in the window, the last positive limit
// seen in journal order and the worst usage.
func (s *Service) RateLimitStats(_ context.Context, in WindowRequest) (RateLimitStatsOutput, error) {
	cutoff := s.cutoff(in.SinceMS)
	out := RateLimitStatsOutput{WindowMS: lenientInt64(in.SinceMS)}
	if err := s.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindRateLimited || cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p throttle
		decode(e, &p)
		out.Throttled++
		if p.LimitPerMin > 0 {
			out.LimitPerMin = p.LimitPerMin
		}
		out.WorstUsed = max(out.WorstUsed, p.Used)
		return nil
	}); err != nil {
		return RateLimitStatsOutput{}, err
	}
	return out, nil
}

// WardenLog lists sandboxed executions, profile downgrades and limit breaches,
// newest first; issues keeps only the downgrades and breaches.
func (s *Service) WardenLog(_ context.Context, in WardenLogRequest) (ExecutionsOutput, error) {
	var issuesOnly bool
	if len(in.Issues) != 0 {
		b, ok := rawValue(in.Issues).(bool)
		if !ok {
			return ExecutionsOutput{}, errors.New("args.issues must be a boolean")
		}
		issuesOnly = b
	}
	out, err := journalview.ProjectValues(s.journal, s.page(in.PageRequest), func(e *event.Event) (ExecutionRow, bool) {
		switch e.Kind {
		case event.KindWardenExecuted:
			if issuesOnly {
				return ExecutionRow{}, false
			}
			var p struct {
				ProfileEffective string `json:"profile_effective"`
				Argv0            string `json:"argv0"`
				ExitCode         int    `json:"exit_code"`
				DurationMS       int64  `json:"duration_ms"`
				Downgraded       bool   `json:"downgraded"`
				TimedOut         bool   `json:"timed_out"`
			}
			decode(e, &p)
			return ExecutionRow{Kind: "exec", Profile: p.ProfileEffective, Argv0: new(p.Argv0), ExitCode: new(p.ExitCode), DurationMS: new(p.DurationMS), Downgraded: new(p.Downgraded), TimedOut: new(p.TimedOut), Stamp: stamp(e)}, true
		case event.KindWardenProfileDowngraded:
			var p struct {
				Requested string `json:"requested"`
				Effective string `json:"effective"`
				Reason    string `json:"reason"`
			}
			decode(e, &p)
			return ExecutionRow{Kind: "downgrade", Profile: p.Effective, Requested: new(p.Requested), Reason: new(p.Reason), Stamp: stamp(e)}, true
		case event.KindWardenLimitExceeded:
			var p struct {
				Limit string `json:"limit"`
				Argv0 string `json:"argv0"`
			}
			decode(e, &p)
			return ExecutionRow{Kind: "limit", Argv0: new(p.Argv0), Reason: new(p.Limit), Stamp: stamp(e)}, true
		default:
			return ExecutionRow{}, false
		}
	})
	if err != nil {
		return ExecutionsOutput{}, err
	}
	return ExecutionsOutput{Executions: out.Rows, Count: out.Count, NextCursor: out.NextCursor}, nil
}

// WardenStats aggregates the sandboxed executions in the window: totals, the
// downgrade rate, timeouts, a per-effective-profile breakdown and breaches.
func (s *Service) WardenStats(_ context.Context, in WindowRequest) (WardenStatsOutput, error) {
	cutoff := s.cutoff(in.SinceMS)
	out := WardenStatsOutput{ByProfile: map[string]int{}, WindowMS: lenientInt64(in.SinceMS)}
	if err := s.journal.Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindWardenExecuted:
			var p struct {
				ProfileEffective string `json:"profile_effective"`
				Downgraded       bool   `json:"downgraded"`
				TimedOut         bool   `json:"timed_out"`
			}
			decode(e, &p)
			out.Executions++
			profile := p.ProfileEffective
			if profile == "" {
				profile = "unknown"
			}
			out.ByProfile[profile]++
			if p.Downgraded {
				out.Downgraded++
			}
			if p.TimedOut {
				out.TimedOut++
			}
		case event.KindWardenLimitExceeded:
			out.LimitBreaches++
		}
		return nil
	}); err != nil {
		return WardenStatsOutput{}, err
	}
	if out.Executions > 0 {
		out.DowngradeRate = float64(out.Downgraded) / float64(out.Executions)
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

// Operations declares the five unaudited guard audit reads. Each routes to the
// caller's tenant kernel, so a tenant sees only its own guards' records.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("audit provider required")
	}
	pageSchema := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"since_ms":{},"cursor":{}}}`)
	windowSchema := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"since_ms":{}}}`)
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "netguard_log", InputSchema: pageSchema, HTTP: opapi.HTTP{Method: "GET", Path: "/api/netguard_log"}}, func(ctx context.Context, in PageRequest) (BlocksOutput, error) {
				return provider(ctx).NetguardLog(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "ratelimit_log", InputSchema: pageSchema, HTTP: opapi.HTTP{Method: "GET", Path: "/api/ratelimit_log"}}, func(ctx context.Context, in PageRequest) (ThrottlesOutput, error) {
				return provider(ctx).RateLimitLog(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "ratelimit_stats", InputSchema: windowSchema}, func(ctx context.Context, in WindowRequest) (RateLimitStatsOutput, error) {
				return provider(ctx).RateLimitStats(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "warden_log", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"since_ms":{},"cursor":{},"issues":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/warden_log"}}, func(ctx context.Context, in WardenLogRequest) (ExecutionsOutput, error) {
				return provider(ctx).WardenLog(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "warden_stats", InputSchema: windowSchema}, func(ctx context.Context, in WindowRequest) (WardenStatsOutput, error) {
				return provider(ctx).WardenStats(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
