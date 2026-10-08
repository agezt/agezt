// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/roster"
)

// GraveyardRequest keeps older_than_days raw: legacy callers send a number or a
// numeric string, and any other value silently means "no age filter".
type GraveyardRequest struct {
	OlderThanDays json.RawMessage `json:"older_than_days,omitempty"`
}

func (r GraveyardRequest) days() float64 {
	var v any
	if json.Unmarshal(r.OlderThanDays, &v) != nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case string:
		// The parse error is ignored but its value kept (range errors yield ±Inf).
		days, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return days
	}
	return 0
}

type GraveyardRow struct {
	AgeDays       int    `json:"age_days"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	RetiredMS     int64  `json:"retired_ms"`
	RetiredReason string `json:"retired_reason"`
	Slug          string `json:"slug"`
	System        bool   `json:"system"`
}

type GraveyardOutput struct {
	Graveyard     []GraveyardRow `json:"graveyard"`
	Count         int            `json:"count"`
	OlderThanDays int            `json:"older_than_days"`
}

// GraveyardService reports retired agents with their retirement age. It only
// reads: archiving or hard removal stays an explicit operator action.
type GraveyardService struct {
	profiles func() []core.Profile
	now      func() time.Time
}

func NewGraveyard(profiles func() []core.Profile, now func() time.Time) *GraveyardService {
	if now == nil {
		now = time.Now
	}
	return &GraveyardService{profiles: profiles, now: now}
}

func (s *GraveyardService) Graveyard(_ context.Context, olderThanDays float64) GraveyardOutput {
	nowMS := s.now().UnixMilli()
	cutoffMS := int64(0)
	if olderThanDays > 0 {
		cutoffMS = nowMS - int64(olderThanDays*24*3600*1000)
	}
	rows := make([]GraveyardRow, 0)
	for _, p := range s.profiles() {
		if !p.Retired {
			continue
		}
		if cutoffMS > 0 && p.RetiredMS > cutoffMS {
			continue // not yet older than the requested window
		}
		ageDays := 0.0
		if p.RetiredMS > 0 {
			ageDays = float64(nowMS-p.RetiredMS) / (24 * 3600 * 1000)
		}
		rows = append(rows, GraveyardRow{AgeDays: int(ageDays), Kind: p.Kind(), Name: p.Name, RetiredMS: p.RetiredMS, RetiredReason: p.RetiredReason, Slug: p.Slug, System: p.System})
	}
	// Oldest first — the most retention-eligible identities lead.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].RetiredMS < rows[j].RetiredMS })
	return GraveyardOutput{Graveyard: rows, Count: len(rows), OlderThanDays: int(olderThanDays)}
}

func GraveyardOperations(provider func(context.Context) *GraveyardService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster graveyard provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_graveyard", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"older_than_days":{}}}`)}, func(ctx context.Context, in GraveyardRequest) (GraveyardOutput, error) {
		return provider(ctx).Graveyard(ctx, in.days()), nil
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
