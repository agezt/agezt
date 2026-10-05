// SPDX-License-Identifier: MIT

// Package memory owns transport-independent memory use cases. Native adapters
// retain argument admission while the domain moves into typed operations.
package memory

import (
	"context"
	"errors"
	"time"

	store "github.com/agezt/agezt/kernel/memory"
)

type GetInput struct {
	ID string `json:"id"`
}
type SearchInput struct {
	Query string  `json:"query"`
	Limit float64 `json:"limit,omitempty"`
}
type RelatedInput struct {
	ID    string  `json:"id"`
	Limit float64 `json:"limit,omitempty"`
}

type Record struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	Subject         string            `json:"subject"`
	Content         string            `json:"content"`
	Confidence      float64           `json:"confidence"`
	CreatedMS       int64             `json:"created_ms"`
	LastSeenMS      int64             `json:"last_seen_ms"`
	Tags            map[string]string `json:"tags,omitempty"`
	SourceEvent     string            `json:"source_event,omitempty"`
	Evidence        string            `json:"evidence,omitempty"`
	HalfLifeMS      int64             `json:"half_life_ms,omitempty"`
	ExpiresMS       *int64            `json:"expires_ms,omitempty"`
	Expired         bool              `json:"expired,omitempty"`
	AddedBy         string            `json:"added_by,omitempty"`
	UpdatedBy       string            `json:"updated_by,omitempty"`
	SupersededBy    string            `json:"superseded_by,omitempty"`
	Tombstoned      bool              `json:"tombstoned,omitempty"`
	SuspendedMS     int64             `json:"suspended_ms,omitempty"`
	Suspended       bool              `json:"suspended,omitempty"`
	SuspendedReason string            `json:"suspended_reason,omitempty"`
}
type GetOutput struct {
	Found  bool    `json:"found"`
	Record *Record `json:"record,omitempty"`
}
type Hit struct {
	Record Record  `json:"record"`
	Score  float64 `json:"score"`
}
type SearchOutput struct {
	Results []Hit `json:"results"`
	Count   int   `json:"count"`
}

type Service struct{ manager *store.Manager }

func New(manager *store.Manager) *Service { return &Service{manager: manager} }

func recordView(r store.Record) Record {
	v := Record{ID: r.ID, Type: string(r.Type), Subject: r.Subject, Content: r.Content,
		Confidence: r.Confidence, CreatedMS: r.CreatedMS, LastSeenMS: r.LastSeenMS,
		Tags: r.Tags, SourceEvent: r.SourceEvent, Evidence: string(r.Evidence),
		AddedBy: r.AddedBy, UpdatedBy: r.UpdatedBy, SupersededBy: r.SupersededBy}
	v.Tombstoned = r.Tombstoned
	if r.Suspended() {
		v.SuspendedMS = r.SuspendedMS
		v.Suspended = true
		v.SuspendedReason = r.SuspendedReason
	}
	if r.HalfLifeMS > 0 {
		v.HalfLifeMS = r.HalfLifeMS
		expires := r.LastSeenMS + r.HalfLifeMS
		v.ExpiresMS = &expires
		v.Expired = r.Expired(time.Now().UnixMilli())
	}
	return v
}

func searchLimit(raw float64) int {
	limit := 10
	if raw > 0 {
		limit = int(raw)
	}
	if limit > 100 {
		limit = 100
	}
	return limit
}

func (s *Service) Get(_ context.Context, in GetInput) (GetOutput, error) {
	rec, found, err := s.manager.Get(in.ID)
	if err != nil {
		return GetOutput{}, err
	}
	out := GetOutput{Found: found}
	if found {
		v := recordView(rec)
		out.Record = &v
	}
	return out, nil
}

func (s *Service) Search(_ context.Context, in SearchInput) (SearchOutput, error) {
	hits, err := s.manager.Search(in.Query, searchLimit(in.Limit))
	if err != nil {
		return SearchOutput{}, err
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{Record: recordView(h.Record), Score: h.Score})
	}
	return SearchOutput{Results: out, Count: len(out)}, nil
}

func (s *Service) FindRelated(_ context.Context, in RelatedInput) (SearchOutput, error) {
	limit := searchLimit(in.Limit)
	seed, found, err := s.manager.Get(in.ID)
	if err != nil {
		return SearchOutput{}, err
	}
	if !found {
		return SearchOutput{}, errors.New("seed record id not found")
	}
	hits, err := s.manager.Search(seed.Content, limit+1)
	if err != nil {
		return SearchOutput{}, err
	}
	out := make([]Hit, 0, limit)
	for _, h := range hits {
		if h.Record.ID != in.ID {
			out = append(out, Hit{Record: recordView(h.Record), Score: h.Score})
		}
		if len(out) >= limit {
			break
		}
	}
	return SearchOutput{Results: out, Count: len(out)}, nil
}
