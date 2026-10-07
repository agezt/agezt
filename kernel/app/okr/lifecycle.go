// SPDX-License-Identifier: MIT
package okr

import (
	"context"
	objectives "github.com/agezt/agezt/kernel/okr"
)

type MutationHost interface {
	CreateObjective(string, objectives.CreateSpec) (objectives.Objective, error)
	AddObjectiveKeyResult(string, string, string, int) (objectives.Objective, error)
	LinkObjectiveTask(string, string, string, string) (objectives.Objective, error)
	UnlinkObjectiveTask(string, string, string, string) (objectives.Objective, error)
	ArchiveObjective(string, string) (objectives.Objective, error)
}
type Lifecycle struct {
	host MutationHost
	view *Service
}

func NewLifecycle(host MutationHost, view *Service) *Lifecycle {
	return &Lifecycle{host: host, view: view}
}

type CreateInput struct {
	CorrelationID string                `json:"correlation_id,omitempty"`
	Spec          objectives.CreateSpec `json:"spec"`
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

func (s *Lifecycle) result(objective objectives.Objective, err error) (ObjectiveOutput, error) {
	if err != nil {
		return ObjectiveOutput{}, err
	}
	return ObjectiveOutput{Objective: s.view.Project(objective)}, nil
}
func (s *Lifecycle) Create(_ context.Context, in CreateInput) (ObjectiveOutput, error) {
	return s.result(s.host.CreateObjective(in.CorrelationID, in.Spec))
}
func (s *Lifecycle) KeyResult(_ context.Context, in KeyResultInput) (ObjectiveOutput, error) {
	return s.result(s.host.AddObjectiveKeyResult(in.CorrelationID, in.ID, in.Title, in.Target))
}
func (s *Lifecycle) Link(_ context.Context, in LinkInput) (ObjectiveOutput, error) {
	return s.result(s.host.LinkObjectiveTask(in.CorrelationID, in.ID, in.KeyResult, in.Task))
}
func (s *Lifecycle) Unlink(_ context.Context, in LinkInput) (ObjectiveOutput, error) {
	return s.result(s.host.UnlinkObjectiveTask(in.CorrelationID, in.ID, in.KeyResult, in.Task))
}
func (s *Lifecycle) Archive(_ context.Context, in ArchiveInput) (ObjectiveOutput, error) {
	return s.result(s.host.ArchiveObjective(in.CorrelationID, in.ID))
}
