// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	curated "github.com/agezt/agezt/kernel/skill"
)

// LifecycleStore is the Forge's existing lifecycle boundary.
type LifecycleStore interface {
	Promote(string, string) (curated.Status, error)
	Quarantine(string, string, string) error
	Archive(string, string, string) error
	Revert(string, string) (string, error)
	RestoreStatus(string, string, curated.Status, string) (curated.Status, curated.Status, error)
}
type Lifecycle struct{ forge LifecycleStore }

func NewLifecycle(forge LifecycleStore) *Lifecycle { return &Lifecycle{forge: forge} }

type ReasonInput struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}
type RestoreInput struct {
	ID     string         `json:"id"`
	Status curated.Status `json:"status"`
	Reason string         `json:"reason,omitempty"`
}
type StatusOutput struct {
	ID     string         `json:"id"`
	Status curated.Status `json:"status"`
}
type ArchiveOutput struct {
	ID     string         `json:"id"`
	Status curated.Status `json:"status"`
	Reason string         `json:"reason"`
}
type RevertOutput struct {
	ID       string `json:"id"`
	Restored string `json:"restored"`
}
type RestoreOutput struct {
	ID     string         `json:"id"`
	From   curated.Status `json:"from"`
	Status curated.Status `json:"status"`
	Reason string         `json:"reason"`
}

// The move retains the native empty domain correlation. Operation identity is
// joined when the native family binds the common typed host.
func (s *Lifecycle) Promote(_ context.Context, in GetInput) (StatusOutput, error) {
	status, err := s.forge.Promote("", in.ID)
	if err != nil {
		return StatusOutput{}, err
	}
	return StatusOutput{ID: in.ID, Status: status}, nil
}
func (s *Lifecycle) Quarantine(_ context.Context, in ReasonInput) (StatusOutput, error) {
	if err := s.forge.Quarantine("", in.ID, in.Reason); err != nil {
		return StatusOutput{}, err
	}
	return StatusOutput{ID: in.ID, Status: curated.StatusQuarantined}, nil
}
func (s *Lifecycle) Archive(_ context.Context, in ReasonInput) (ArchiveOutput, error) {
	if err := s.forge.Archive("", in.ID, in.Reason); err != nil {
		return ArchiveOutput{}, err
	}
	return ArchiveOutput{ID: in.ID, Status: curated.StatusArchived, Reason: in.Reason}, nil
}
func (s *Lifecycle) Revert(_ context.Context, in GetInput) (RevertOutput, error) {
	restored, err := s.forge.Revert("", in.ID)
	if err != nil {
		return RevertOutput{}, err
	}
	return RevertOutput{ID: in.ID, Restored: restored}, nil
}
func (s *Lifecycle) Restore(_ context.Context, in RestoreInput) (RestoreOutput, error) {
	from, to, err := s.forge.RestoreStatus("", in.ID, in.Status, in.Reason)
	if err != nil {
		return RestoreOutput{}, err
	}
	return RestoreOutput{ID: in.ID, From: from, Status: to, Reason: in.Reason}, nil
}
