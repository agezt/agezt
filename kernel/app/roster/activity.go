// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
)

// rawValue reads a raw request field as it came: the value it carries, and
// whether the field was sent at all.
func rawValue(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v, true
}

type ActivityItem struct {
	Seq           int64  `json:"seq"`
	Kind          string `json:"kind"`
	TSUnixMS      int64  `json:"ts_unix_ms"`
	CorrelationID string `json:"correlation_id"`
	Summary       string `json:"summary"`
}

type ActivityOutput struct {
	Slug       string         `json:"slug"`
	Activity   []ActivityItem `json:"activity"`
	Count      int            `json:"count"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// ActivityService folds the selected journal into one agent's timeline: the
// runs it executed and every event attributable to it, newest first.
type ActivityService struct {
	get     func(string) (agentapi.Profile, bool)
	journal func(func(*event.Event) error) error
}

func NewActivity(get func(string) (agentapi.Profile, bool), journal func(func(*event.Event) error) error) *ActivityService {
	return &ActivityService{get: get, journal: journal}
}

func (s *ActivityService) Activity(_ context.Context, in opapi.AgentPageRequest) (ActivityOutput, error) {
	ref, err := refOf(in.Ref)
	if err != nil {
		return ActivityOutput{}, err
	}
	p, ok := s.get(ref)
	if !ok {
		return ActivityOutput{}, errors.New("unknown agent: " + ref)
	}
	slug := p.Slug
	limit := pageLimit(in.Limit, 50, 500)

	// One pass: task.received carries the agent slug, so its correlation ids
	// scope the council consults and delegations that happened during the runs.
	runCorr := map[string]bool{}
	var items []ActivityItem
	_ = s.journal(func(e *event.Event) error {
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil && plString(pl, "agent") == slug && e.CorrelationID != "" {
				runCorr[e.CorrelationID] = true
			}
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		summary, ok := ActivitySummary(e, pl, slug, runCorr)
		if !ok {
			return nil
		}
		items = append(items, ActivityItem{Seq: e.Seq, Kind: string(e.Kind), TSUnixMS: e.TSUnixMS, CorrelationID: e.CorrelationID, Summary: summary})
		return nil
	})

	// Newest first, capped.
	sort.SliceStable(items, func(i, j int) bool { return items[i].Seq > items[j].Seq })
	total := len(items)

	// The cursor is the "<seq>" boundary of the previous page; the list is
	// sorted DESC, so strictly older means strictly smaller seq.
	if cursorSeq, cursorOK := pageCursor(in.Cursor); cursorOK {
		filtered := items[:0]
		for _, it := range items {
			if it.Seq >= cursorSeq {
				continue
			}
			filtered = append(filtered, it)
		}
		items = filtered
	}
	var next string
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		next = strconv.FormatInt(items[limit-1].Seq, 10)
	}
	return ActivityOutput{Slug: slug, Activity: items, Count: len(items), Total: total, NextCursor: next}, nil
}

func ActivityOperations(provider func(context.Context) *ActivityService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster activity provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: opapi.AgentActivity, ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.AgentPageRequest) (ActivityOutput, error) {
		return provider(ctx).Activity(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
