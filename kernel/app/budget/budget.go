// SPDX-License-Identifier: MIT

// Package budget owns the operator's view of the governor's daily spend: the
// snapshot behind `agt budget` and the console's budget panel, and the runtime
// knob that adjusts the global daily ceiling (M607).
package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// TaskSnapshot is one configured per-task-type cap and today's spend against it.
type TaskSnapshot struct {
	TaskType        string
	SpentMicrocents int64
	CapMicrocents   int64
}

// Snapshot is the governor's counters for the current UTC day.
type Snapshot struct {
	UTCDate           string
	SpentMicrocents   int64
	CeilingMicrocents int64
	PerTask           []TaskSnapshot
	StrictPricing     bool
}

// Governor is the primary kernel's spend governor. A daemon whose provider is
// not a governor has none, and both operations refuse.
type Governor interface {
	Snapshot() Snapshot
	SetDailyCeiling(microcents int64)
}

// Service reads and adjusts one governor; a nil governor means none is wired.
type Service struct{ gov Governor }

func New(gov Governor) *Service { return &Service{gov: gov} }

type TaskRow struct {
	TaskType  string `json:"task_type"`
	SpentMC   int64  `json:"spent_mc"`
	CeilingMC int64  `json:"ceiling_mc"`
}

// Output is the snapshot both operations return. per_task is sorted by task
// type and is an empty array, never null, when no per-task caps are set.
type Output struct {
	UTCDate       string    `json:"utc_date"`
	SpentMC       int64     `json:"spent_mc"`
	CeilingMC     int64     `json:"ceiling_mc"`
	PerTask       []TaskRow `json:"per_task"`
	StrictPricing bool      `json:"strict_pricing"`
}

func output(snap Snapshot) Output {
	sort.Slice(snap.PerTask, func(i, j int) bool {
		return snap.PerTask[i].TaskType < snap.PerTask[j].TaskType
	})
	rows := make([]TaskRow, 0, len(snap.PerTask))
	for _, row := range snap.PerTask {
		rows = append(rows, TaskRow{TaskType: row.TaskType, SpentMC: row.SpentMicrocents, CeilingMC: row.CapMicrocents})
	}
	return Output{UTCDate: snap.UTCDate, SpentMC: snap.SpentMicrocents, CeilingMC: snap.CeilingMicrocents, PerTask: rows, StrictPricing: snap.StrictPricing}
}

type GetRequest struct{}

func (s *Service) Get(_ context.Context, _ GetRequest) (Output, error) {
	if s.gov == nil {
		return Output{}, errors.New("budget: daemon's provider is not a governor (likely a test or future provider variant); no budget state to report")
	}
	return output(s.gov.Snapshot()), nil
}

type SetRequest struct {
	CeilingMC json.RawMessage `json:"ceiling_mc,omitempty"`
}

// wholeNumber reads a ceiling sent as a JSON number (CLI) or a string (the Web
// UI's query-string proxy), refusing fractions and unparseable values rather
// than truncating them.
func wholeNumber(raw json.RawMessage) (int64, error) {
	var v any
	_ = json.Unmarshal(raw, &v)
	switch n := v.(type) {
	case float64:
		if n != float64(int64(n)) {
			return 0, fmt.Errorf("must be a whole number, got %v", n)
		}
		return int64(n), nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("not an integer: %q", n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}

// Set replaces the global daily ceiling in microcents (0 is unlimited; the
// governor clamps a negative value to 0) and returns the post-set snapshot.
func (s *Service) Set(_ context.Context, in SetRequest) (Output, error) {
	if s.gov == nil {
		return Output{}, errors.New("budget_set: daemon's provider is not a governor; cannot adjust the ceiling")
	}
	if len(in.CeilingMC) == 0 {
		return Output{}, errors.New("args.ceiling_mc required (microcents; 0 = unlimited)")
	}
	mc, err := wholeNumber(in.CeilingMC)
	if err != nil {
		return Output{}, errors.New("args.ceiling_mc: " + err.Error())
	}
	s.gov.SetDailyCeiling(mc)
	return output(s.gov.Snapshot()), nil
}

// Operations declares the read-only snapshot on its Web UI route and the
// audited ceiling write, which has no Web UI route; both are operator-only.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("budget provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[Output](), false)
	if err != nil {
		return nil, err
	}
	get, err := app.NewOperation(opapi.Spec{Name: "budget", ReadOnly: true, OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/budget"}}, func(ctx context.Context, in GetRequest) (Output, error) {
		return provider(ctx).Get(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	set, err := app.NewOperation(opapi.Spec{Name: "budget_set", OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ceiling_mc":{}}}`)}, func(ctx context.Context, in SetRequest) (Output, error) {
		return provider(ctx).Set(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{get, set}, nil
}
