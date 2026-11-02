// SPDX-License-Identifier: MIT
package okr

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// Lifecycle is the objective application's write side: the create, the key
// result, the link, the unlink and the archive.
//
// It holds the WORK MODULE's service (W3.76r) — the goals and the rollup rule
// are the module's, so the app asks the module for them. The daemon's own
// spellings of the same five verbs are gone: one operation, one name.
type Lifecycle struct {
	library workapi.ObjectiveService
	view    *Service
}

// NewLifecycle binds the write side to the module's service and the read side
// it projects through.
//
// A nil library is a daemon with no objective service: every write refuses
// with the application's refusal.
func NewLifecycle(library workapi.ObjectiveService, view *Service) *Lifecycle {
	return &Lifecycle{library: library, view: view}
}

type CreateInput struct {
	CorrelationID string            `json:"correlation_id,omitempty"`
	Spec          okrapi.CreateSpec `json:"spec"`
}
type KeyResultInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Target        int    `json:"target"`
}
type LinkInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
	KeyResult     string `json:"key_result"`
	Task          string `json:"task"`
}
type ArchiveInput struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	ID            string `json:"id"`
}
type ObjectiveOutput struct {
	Objective Record `json:"objective"`
}

func (s *Lifecycle) result(objective okrapi.Objective, err error) (ObjectiveOutput, error) {
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return ObjectiveOutput{Objective: s.view.Project(objective)}, nil
}

// libraryOrRefusal returns the module's service, or the refusal.
//
// The refusal is the contract's row: a daemon with no library answers no
// library, whether the store was never opened or the module was never wired.
func (s *Lifecycle) libraryOrRefusal() (workapi.ObjectiveService, error) {
	if s.library == nil {
		return nil, errNoObjectiveLibrary
	}
	return s.library, nil
}

func (s *Lifecycle) Create(_ context.Context, in CreateInput) (ObjectiveOutput, error) {
	library, err := s.libraryOrRefusal()
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return s.result(library.Create(in.CorrelationID, in.Spec))
}
func (s *Lifecycle) KeyResult(_ context.Context, in KeyResultInput) (ObjectiveOutput, error) {
	library, err := s.libraryOrRefusal()
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return s.result(library.AddKeyResult(in.CorrelationID, in.ID, in.Title, in.Target))
}
func (s *Lifecycle) Link(_ context.Context, in LinkInput) (ObjectiveOutput, error) {
	library, err := s.libraryOrRefusal()
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return s.result(library.LinkTask(in.CorrelationID, in.ID, in.KeyResult, in.Task))
}
func (s *Lifecycle) Unlink(_ context.Context, in LinkInput) (ObjectiveOutput, error) {
	library, err := s.libraryOrRefusal()
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return s.result(library.UnlinkTask(in.CorrelationID, in.ID, in.KeyResult, in.Task))
}
func (s *Lifecycle) Archive(_ context.Context, in ArchiveInput) (ObjectiveOutput, error) {
	library, err := s.libraryOrRefusal()
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return s.result(library.Archive(in.CorrelationID, in.ID))
}
