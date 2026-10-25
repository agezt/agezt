// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/okrapi"
)

// errNoObjectiveLibrary is the answer the objective app layer gives when it
// holds no library.
//
// The objective library is the DAEMON's (W3.53): the kernel holds the
// contract's port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoObjectiveLibrary = errors.New("okr: the objective library is not available on this daemon")

type Reader interface {
	List(okrapi.Filter) []okrapi.Objective
	Get(string) (okrapi.Objective, bool)
}
type Rollupper interface {
	Rollup(okrapi.Objective) okrapi.ObjectiveProgress
}
type Service struct {
	reader Reader
	rollup Rollupper
}

func New(reader Reader, rollup Rollupper) *Service { return &Service{reader: reader, rollup: rollup} }

type Record struct {
	okrapi.Objective
	Progress       okrapi.ObjectiveProgress `json:"progress"`
	Percent        int                      `json:"percent"`
	Achieved       bool                     `json:"achieved"`
	KeyResultCount int                      `json:"key_result_count"`
}

func (s *Service) Project(objective okrapi.Objective) Record {
	progress := s.rollup.Rollup(objective)
	return Record{Objective: objective, Progress: progress, Percent: progress.Percent, Achieved: progress.Achieved, KeyResultCount: len(objective.KeyResults)}
}

type ListInput struct {
	Status          okrapi.Status `json:"status,omitempty"`
	Tenant          string        `json:"tenant,omitempty"`
	IncludeArchived bool          `json:"include_archived,omitempty"`
	Limit           int           `json:"limit,omitempty"`
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
	// A host with no library has no objectives — the empty list is the honest
	// answer, not a panic (W3.53).
	if s.reader == nil {
		return ListOutput{Objectives: []Record{}}, nil
	}
	found := s.reader.List(okrapi.Filter{Status: in.Status, Tenant: in.Tenant, IncludeArchived: in.IncludeArchived, Limit: in.Limit})
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
	// A named objective cannot be answered from an empty library: "unknown
	// objective" would blame the id for a library that is not there (W3.53).
	if s.reader == nil {
		return ShowOutput{}, errNoObjectiveLibrary
	}
	objective, found := s.reader.Get(in.ID)
	if !found {
		return ShowOutput{}, fmt.Errorf("unknown objective: %s", in.ID)
	}
	return ShowOutput{Objective: s.Project(objective)}, nil
}
