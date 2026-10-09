// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	defaultListLimit = 20
	maxListLimit     = 1_000
)

// Run is one folded run of the routed kernel's journal.
type Run struct {
	CorrelationID, Intent, FailReason, ParentCorrelation string
	AnswerPreview, Model, Agent, Phase, Tool             string
	StartedUnixMS, StartedSeq, CompletedUnixMS           int64
	FailedUnixMS, SpentMicrocents                        int64
	Iters                                                int
	Completed, Failed, Abandoned                         bool
}

// Status reports a run's status. Precedence: completed > failed > abandoned >
// running, so a run carrying several terminal markers reports the most
// authoritative one.
func (r Run) Status() string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}

// ListRequest keeps the arguments raw so the legacy lenient numbers and strict
// strings hold.
type ListRequest struct {
	Limit     json.RawMessage `json:"limit,omitempty"`
	Cursor    json.RawMessage `json:"cursor,omitempty"`
	Status    json.RawMessage `json:"status,omitempty"`
	Intent    json.RawMessage `json:"intent,omitempty"`
	Model     json.RawMessage `json:"model,omitempty"`
	MinCostMC json.RawMessage `json:"min_cost_mc,omitempty"`
	MaxCostMC json.RawMessage `json:"max_cost_mc,omitempty"`
}

type StatsRequest struct {
	SinceMS json.RawMessage `json:"since_ms,omitempty"`
	Intent  json.RawMessage `json:"intent,omitempty"`
}

type RunRow struct {
	CorrelationID     string `json:"correlation_id"`
	Intent            string `json:"intent"`
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	StartedUnixMS     int64  `json:"started_unix_ms"`
	CompletedUnixMS   int64  `json:"completed_unix_ms"`
	DurationMS        int64  `json:"duration_ms"`
	Iters             int    `json:"iters"`
	ParentCorrelation string `json:"parent_correlation"`
	SpentMC           int64  `json:"spent_mc"`
	Model             string `json:"model"`
	AnswerPreview     string `json:"answer_preview"`
	Agent             string `json:"agent"`
	// Phase and Tool are the live activity of a running run only.
	Phase string `json:"phase,omitempty"`
	Tool  string `json:"tool,omitempty"`
}

type ListOutput struct {
	Runs       []RunRow `json:"runs"`
	Count      int      `json:"count"`
	NextCursor string   `json:"next_cursor"`
}

type Distribution struct {
	Count int   `json:"count"`
	Avg   int64 `json:"avg"`
	Min   int64 `json:"min"`
	Max   int64 `json:"max"`
	P50   int64 `json:"p50"`
	P95   int64 `json:"p95"`
}

type ModelSpend struct {
	Runs            int   `json:"runs"`
	SpentMicrocents int64 `json:"spent_microcents"`
}

type StatsOutput struct {
	Total                    int                   `json:"total"`
	Completed                int                   `json:"completed"`
	Failed                   int                   `json:"failed"`
	Running                  int                   `json:"running"`
	Abandoned                int                   `json:"abandoned"`
	Terminal                 int                   `json:"terminal"`
	SuccessRate              float64               `json:"success_rate"`
	AvgIters                 float64               `json:"avg_iters"`
	FailedByReason           map[string]int        `json:"failed_by_reason"`
	WindowMS                 int64                 `json:"window_ms"`
	Delegations              int                   `json:"delegations"`
	DelegatingRuns           int                   `json:"delegating_runs"`
	MaxFanout                int                   `json:"max_fanout"`
	SpentMicrocents          int64                 `json:"spent_microcents"`
	DelegatedSpentMicrocents int64                 `json:"delegated_spent_microcents"`
	ByModel                  map[string]ModelSpend `json:"by_model"`
	SpendMicrocents          Distribution          `json:"spend_microcents"`
	DurationMS               Distribution          `json:"duration_ms"`
}

func rawValue(raw json.RawMessage) any {
	var v any
	if len(raw) != 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// optionalString is a strict string: absent is empty, present must be a string.
func optionalString(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	s, ok := rawValue(raw).(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return s, nil
}

// lenientInt64 reads a JSON number truncated toward zero; anything else is 0.
func lenientInt64(raw json.RawMessage) int64 {
	n, _ := rawValue(raw).(float64)
	return int64(n)
}

// Service reads the routed kernel's folded runs.
type Service struct {
	runs func() (map[string]Run, error)
	now  func() time.Time
}

func New(runs func() (map[string]Run, error), now func() time.Time) *Service {
	return &Service{runs: runs, now: now}
}

// List filters before the limit, sorts newest first (journal seq breaks a
// same-millisecond tie) and pages strictly older than the cursor.
func (s *Service) List(_ context.Context, in ListRequest) (ListOutput, error) {
	limit := defaultListLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	limit = min(max(limit, 1), maxListLimit)
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(rawValue(in.Cursor))
	var filters [3]string
	for i, f := range []struct {
		raw json.RawMessage
		key string
	}{{in.Status, "status"}, {in.Intent, "intent"}, {in.Model, "model"}} {
		var err error
		if filters[i], err = optionalString(f.raw, f.key); err != nil {
			return ListOutput{}, err
		}
	}
	status, intent, model := filters[0], strings.ToLower(filters[1]), strings.ToLower(filters[2])
	minCost, maxCost := lenientInt64(in.MinCostMC), lenientInt64(in.MaxCostMC)
	runs, err := s.runs()
	if err != nil {
		return ListOutput{}, err
	}
	entries := make([]Run, 0, len(runs))
	for _, r := range runs {
		if status != "" && r.Status() != status ||
			intent != "" && !strings.Contains(strings.ToLower(r.Intent), intent) ||
			model != "" && !strings.Contains(strings.ToLower(r.Model), model) ||
			minCost > 0 && r.SpentMicrocents < minCost ||
			maxCost > 0 && r.SpentMicrocents > maxCost {
			continue
		}
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].StartedUnixMS != entries[j].StartedUnixMS {
			return entries[i].StartedUnixMS > entries[j].StartedUnixMS
		}
		return entries[i].StartedSeq > entries[j].StartedSeq
	})
	if cursorOK {
		kept := entries[:0]
		for _, r := range entries {
			if journal.KeepBeforeCursor(r.StartedUnixMS, r.StartedSeq, cursorMS, cursorSeq) {
				kept = append(kept, r)
			}
		}
		entries = kept
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := ListOutput{Runs: make([]RunRow, 0, len(entries)), Count: len(entries)}
	for _, r := range entries {
		row := RunRow{CorrelationID: r.CorrelationID, Intent: r.Intent, Status: r.Status(), StartedUnixMS: r.StartedUnixMS, CompletedUnixMS: r.CompletedUnixMS, Iters: r.Iters, ParentCorrelation: r.ParentCorrelation, SpentMC: r.SpentMicrocents, Model: r.Model, AnswerPreview: r.AnswerPreview, Agent: r.Agent}
		switch {
		case r.Completed:
			if r.StartedUnixMS > 0 {
				row.DurationMS = r.CompletedUnixMS - r.StartedUnixMS
			}
		case r.Failed:
			row.Reason = r.FailReason
			if r.StartedUnixMS > 0 && r.FailedUnixMS >= r.StartedUnixMS {
				row.DurationMS = r.FailedUnixMS - r.StartedUnixMS
			}
		case r.Abandoned:
		default:
			if r.Phase != "" {
				row.Phase, row.Tool = r.Phase, r.Tool
			}
		}
		out.Runs = append(out.Runs, row)
	}
	if n := len(entries); n > 0 {
		last := entries[n-1]
		out.NextCursor = journal.NextCursor(last.StartedUnixMS, last.StartedSeq, n, limit)
	}
	return out, nil
}

func distribution(values []int64) Distribution {
	d := journalview.SummarizeDurations(values)
	return Distribution{Count: len(values), Avg: d.Avg, Min: d.Min, Max: d.Max, P50: d.P50, P95: d.P95}
}

// Stats aggregates every run, optionally windowed to runs that started within
// since_ms of now and scoped to an intent substring. Durations cover completed
// runs only; the success rate counts failed and abandoned runs against it but
// not runs still in flight.
func (s *Service) Stats(_ context.Context, in StatsRequest) (StatsOutput, error) {
	runs, err := s.runs()
	if err != nil {
		return StatsOutput{}, err
	}
	sinceMS := lenientInt64(in.SinceMS)
	var cutoff int64
	if sinceMS > 0 {
		cutoff = s.now().UnixMilli() - sinceMS
	}
	intent, err := optionalString(in.Intent, "intent")
	if err != nil {
		return StatsOutput{}, err
	}
	intent = strings.ToLower(intent)
	out := StatsOutput{FailedByReason: map[string]int{}, WindowMS: sinceMS, ByModel: map[string]ModelSpend{}}
	var itersSum int
	durations, spends := make([]int64, 0, len(runs)), make([]int64, 0, len(runs))
	fanout := map[string]int{}
	for _, r := range runs {
		if cutoff > 0 && (r.StartedUnixMS == 0 || r.StartedUnixMS < cutoff) {
			continue
		}
		if intent != "" && !strings.Contains(strings.ToLower(r.Intent), intent) {
			continue
		}
		out.Total++
		out.SpentMicrocents += r.SpentMicrocents
		if r.SpentMicrocents > 0 {
			spends = append(spends, r.SpentMicrocents)
		}
		if r.Model != "" {
			m := out.ByModel[r.Model]
			m.Runs++
			m.SpentMicrocents += r.SpentMicrocents
			out.ByModel[r.Model] = m
		}
		if r.ParentCorrelation != "" {
			out.Delegations++
			fanout[r.ParentCorrelation]++
			out.DelegatedSpentMicrocents += r.SpentMicrocents
		}
		switch {
		case r.Completed:
			out.Completed++
			itersSum += r.Iters
			if r.StartedUnixMS > 0 && r.CompletedUnixMS >= r.StartedUnixMS {
				durations = append(durations, r.CompletedUnixMS-r.StartedUnixMS)
			}
		case r.Failed:
			out.Failed++
			reason := r.FailReason
			if reason == "" {
				reason = "unknown"
			}
			out.FailedByReason[reason]++
		case r.Abandoned:
			out.Abandoned++
		default:
			out.Running++
		}
	}
	out.Terminal = out.Completed + out.Failed + out.Abandoned
	if out.Terminal > 0 {
		out.SuccessRate = float64(out.Completed) / float64(out.Terminal)
	}
	if out.Completed > 0 {
		out.AvgIters = float64(itersSum) / float64(out.Completed)
	}
	out.DelegatingRuns = len(fanout)
	for _, n := range fanout {
		out.MaxFanout = max(out.MaxFanout, n)
	}
	out.SpendMicrocents, out.DurationMS = distribution(spends), distribution(durations)
	return out, nil
}

func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema = output
	spec.ReadOnly, spec.Authz, spec.Tenancy, spec.AllowUnknownInput = true, opapi.OwnTenant, opapi.CallerTenant, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the two unaudited run reads. Each routes to the caller's
// tenant kernel, so a tenant sees only its own runs.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("runs provider required")
	}
	var ops []app.Operation
	if err := bind(&ops, opapi.Spec{Name: "runs_list", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{},"cursor":{},"status":{},"intent":{},"model":{},"min_cost_mc":{},"max_cost_mc":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/runs"}}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, in)
	}); err != nil {
		return nil, err
	}
	if err := bind(&ops, opapi.Spec{Name: "runs_stats", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"since_ms":{},"intent":{}}}`)}, func(ctx context.Context, in StatsRequest) (StatsOutput, error) {
		return provider(ctx).Stats(ctx, in)
	}); err != nil {
		return nil, err
	}
	return ops, nil
}
