// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// errNoObjectiveLibrary is the answer the objective app layer gives when it
// holds no library.
//
// The objective library is the DAEMON's FILE (W3.53): the kernel holds the
// contract's port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoObjectiveLibrary = errors.New("okr: the objective library is not available on this daemon")

// Reader reads the objective library.
//
// It is the CONTRACT's store port: the daemon opens the file and hands the
// port over. It is NIL when the daemon holds no objective library — the
// application refuses rather than reading into nothing.
type Reader interface {
	List(okrapi.Filter) []okrapi.Objective
	Get(string) (okrapi.Objective, bool)
}

// Service is the objective application's read side: the list and the show.
//
// It holds the library's READ port and the work MODULE's service (W3.76r) —
// the rollup is the module's rule, so the app asks the module for it instead
// of asking the daemon to spell it out.
type Service struct {
	reader  Reader
	library workapi.ObjectiveService
}

// New binds the read side: the store to read from and the module's service
// for the rollup.
//
// A nil library is a daemon with no objective service: the rollup answers
// "nothing counts", and the writes refuse.
func New(reader Reader, library workapi.ObjectiveService) *Service {
	return &Service{reader: reader, library: library}
}

type Record struct {
	okrapi.Objective
	Progress       okrapi.ObjectiveProgress `json:"progress"`
	Percent        int                      `json:"percent"`
	Achieved       bool                     `json:"achieved"`
	KeyResultCount int                      `json:"key_result_count"`
}

func (s *Service) Project(objective okrapi.Objective) Record {
	var progress okrapi.ObjectiveProgress
	if s.library != nil {
		progress = s.library.Progress(objective)
	}
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
