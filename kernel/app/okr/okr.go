// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"errors"
	"fmt"
	objectives "github.com/agezt/agezt/kernel/okr"
)

type Reader interface {
	List(objectives.Filter) []objectives.Objective
	Get(string) (objectives.Objective, bool)
}
type Rollupper interface {
	Rollup(objectives.Objective) objectives.ObjectiveProgress
}
type Service struct {
	reader Reader
	rollup Rollupper
}

func New(reader Reader, rollup Rollupper) *Service { return &Service{reader: reader, rollup: rollup} }

type Record struct {
	objectives.Objective
	Progress       objectives.ObjectiveProgress `json:"progress"`
	Percent        int                          `json:"percent"`
	Achieved       bool                         `json:"achieved"`
	KeyResultCount int                          `json:"key_result_count"`
}

func (s *Service) Project(objective objectives.Objective) Record {
	progress := s.rollup.Rollup(objective)
	return Record{Objective: objective, Progress: progress, Percent: progress.Percent, Achieved: progress.Achieved, KeyResultCount: len(objective.KeyResults)}
}

type ListInput struct {
	Status          objectives.Status `json:"status,omitempty"`
	Tenant          string            `json:"tenant,omitempty"`
	IncludeArchived bool              `json:"include_archived,omitempty"`
	Limit           int               `json:"limit,omitempty"`
}
type ListOutput struct {
	Objectives []Record `json:"objectives"`
	Count      int      `json:"count"`
}
type ShowInput struct {
	ID string `json:"id"`
}
type ShowOutput struct {
	Objective Record `json:"objective"`
}

func (s *Service) List(_ context.Context, in ListInput) (ListOutput, error) {
	found := s.reader.List(objectives.Filter{Status: in.Status, Tenant: in.Tenant, IncludeArchived: in.IncludeArchived, Limit: in.Limit})
	out := make([]Record, 0, len(found))
	for _, objective := range found {
		out = append(out, s.Project(objective))
	}
	return ListOutput{Objectives: out, Count: len(out)}, nil
}
func (s *Service) Show(_ context.Context, in ShowInput) (ShowOutput, error) {
	if in.ID == "" {
		return ShowOutput{}, errors.New("okr_show requires id")
	}
	objective, found := s.reader.Get(in.ID)
	if !found {
		return ShowOutput{}, fmt.Errorf("unknown objective: %s", in.ID)
	}
	return ShowOutput{Objective: s.Project(objective)}, nil
}
