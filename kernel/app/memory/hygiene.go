// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	store "github.com/agezt/agezt/kernel/memory"
)

type PruneInput struct {
	OlderThanDays int  `json:"older_than_days,omitempty"`
	DryRun        bool `json:"dry_run"`
}
type HygieneInput struct {
	DryRun bool `json:"dry_run"`
}
type PruneOutput struct {
	DryRun        bool               `json:"dry_run"`
	OlderThanDays int                `json:"older_than_days"`
	CutoffMS      int64              `json:"cutoff_ms"`
	Prunable      *int               `json:"prunable,omitempty"`
	Pruned        *int               `json:"pruned,omitempty"`
	Stats         store.HygieneStats `json:"stats"`
}
type TidyOutput struct {
	DryRun    bool `json:"dry_run"`
	Collapsed int  `json:"collapsed"`
}

func (s *Service) Prune(ctx context.Context, in PruneInput) (PruneOutput, error) {
	if s.manager == nil {
		return PruneOutput{}, errors.New("memory unavailable")
	}
	days := in.OlderThanDays
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	hyg, err := s.manager.Hygiene(cutoff)
	if err != nil {
		return PruneOutput{}, err
	}
	out := PruneOutput{DryRun: in.DryRun, OlderThanDays: days, CutoffMS: cutoff, Stats: hyg}
	if in.DryRun {
		out.Prunable = &hyg.Prunable
		return out, nil
	}
	pruned, err := s.manager.Prune(opapi.CorrelationFromContext(ctx), cutoff, false)
	if err != nil {
		return PruneOutput{}, err
	}
	out.Pruned = &pruned
	return out, nil
}

func (s *Service) Tidy(ctx context.Context, in HygieneInput) (TidyOutput, error) {
	if s.manager == nil {
		return TidyOutput{}, errors.New("memory unavailable")
	}
	n, err := s.manager.DedupeDistilled(opapi.CorrelationFromContext(ctx), in.DryRun)
	if err != nil {
		return TidyOutput{}, err
	}
	return TidyOutput{DryRun: in.DryRun, Collapsed: n}, nil
}

func (s *Service) Audit(_ context.Context, _ struct{}) (store.AuditReport, error) {
	return s.manager.Audit()
}

func (s *Service) Clean(ctx context.Context, in HygieneInput) (store.CleanReport, error) {
	return s.manager.CleanLowValue(opapi.CorrelationFromContext(ctx), in.DryRun)
}
