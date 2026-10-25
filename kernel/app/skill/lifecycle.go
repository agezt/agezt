// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// LifecycleStore is the Forge's existing lifecycle boundary.
type LifecycleStore interface {
	Promote(string, string) (skillapi.Status, error)
	Quarantine(string, string, string) error
	Archive(string, string, string) error
	Revert(string, string) (string, error)
	RestoreStatus(string, string, skillapi.Status, string) (skillapi.Status, skillapi.Status, error)
}
type Lifecycle struct{ forge LifecycleStore }

func NewLifecycle(forge LifecycleStore) *Lifecycle { return &Lifecycle{forge: forge} }

type ReasonInput struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}
type RestoreInput struct {
	ID     string          `json:"id"`
	Status skillapi.Status `json:"status"`
	Reason string          `json:"reason,omitempty"`
}
type StatusOutput struct {
	ID     string          `json:"id"`
	Status skillapi.Status `json:"status"`
}
type ArchiveOutput struct {
	ID     string          `json:"id"`
	Status skillapi.Status `json:"status"`
	Reason string          `json:"reason"`
}
type RevertOutput struct {
	ID       string `json:"id"`
	Restored string `json:"restored"`
}
type RestoreOutput struct {
	ID     string          `json:"id"`
	From   skillapi.Status `json:"from"`
	Status skillapi.Status `json:"status"`
	Reason string          `json:"reason"`
}

// Forward the trusted operation identity; context-free callers retain the
// legacy empty domain correlation.
func (s *Lifecycle) Promote(ctx context.Context, in GetInput) (StatusOutput, error) {
	if s.forge == nil {
		return StatusOutput{}, errNoSkillLibrary
	}
	status, err := s.forge.Promote(opapi.CorrelationFromContext(ctx), in.ID)
	if err != nil {
		return StatusOutput{}, err
	}
	return StatusOutput{ID: in.ID, Status: status}, nil
}
func (s *Lifecycle) Quarantine(ctx context.Context, in ReasonInput) (StatusOutput, error) {
	if s.forge == nil {
		return StatusOutput{}, errNoSkillLibrary
	}
	if err := s.forge.Quarantine(opapi.CorrelationFromContext(ctx), in.ID, in.Reason); err != nil {
		return StatusOutput{}, err
	}
	return StatusOutput{ID: in.ID, Status: skillapi.StatusQuarantined}, nil
}
func (s *Lifecycle) Archive(ctx context.Context, in ReasonInput) (ArchiveOutput, error) {
	if s.forge == nil {
		return ArchiveOutput{}, errNoSkillLibrary
	}
	if err := s.forge.Archive(opapi.CorrelationFromContext(ctx), in.ID, in.Reason); err != nil {
		return ArchiveOutput{}, err
	}
	return ArchiveOutput{ID: in.ID, Status: skillapi.StatusArchived, Reason: in.Reason}, nil
}
func (s *Lifecycle) Revert(ctx context.Context, in GetInput) (RevertOutput, error) {
	if s.forge == nil {
		return RevertOutput{}, errNoSkillLibrary
	}
	restored, err := s.forge.Revert(opapi.CorrelationFromContext(ctx), in.ID)
	if err != nil {
		return RevertOutput{}, err
	}
	return RevertOutput{ID: in.ID, Restored: restored}, nil
}
func (s *Lifecycle) Restore(ctx context.Context, in RestoreInput) (RestoreOutput, error) {
	if s.forge == nil {
		return RestoreOutput{}, errNoSkillLibrary
	}
	from, to, err := s.forge.RestoreStatus(opapi.CorrelationFromContext(ctx), in.ID, in.Status, in.Reason)
	if err != nil {
		return RestoreOutput{}, err
	}
	return RestoreOutput{ID: in.ID, From: from, Status: to, Reason: in.Reason}, nil
}
