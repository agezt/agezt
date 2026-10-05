// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	store "github.com/agezt/agezt/kernel/memory"
)

const distillationTimeout = 5 * time.Minute

type DistillInput struct{}
type ConsolidateOutput struct {
	CorrelationID     string   `json:"correlation_id"`
	ClustersFound     int      `json:"clusters_found"`
	ClustersMerged    int      `json:"clusters_merged"`
	RecordsSuperseded int      `json:"records_superseded"`
	ConsolidatedIDs   []string `json:"consolidated_ids"`
	SkippedNonJSON    int      `json:"skipped_non_json"`
	ActiveBefore      int      `json:"active_before"`
	ActiveAfter       int      `json:"active_after"`
}
type ProfileOutput struct {
	CorrelationID string   `json:"correlation_id"`
	InputRecords  int      `json:"input_records"`
	FacetsWritten int      `json:"facets_written"`
	Facets        []string `json:"facets"`
}

type Distiller interface {
	NewCorrelation() string
	DistillBrain(context.Context, string) (store.BrainDistillReport, error)
	DistillProfile(context.Context, string) (store.ProfileReport, error)
}
type Distillation struct{ target Distiller }

func NewDistillation(target Distiller) *Distillation { return &Distillation{target: target} }

func (s *Distillation) Consolidate(ctx context.Context, _ DistillInput) (ConsolidateOutput, error) {
	if err := ctx.Err(); err != nil {
		return ConsolidateOutput{}, err
	}
	corr := opapi.CorrelationFromContext(ctx)
	if corr == "" {
		corr = s.target.NewCorrelation()
	}
	ctx, cancel := context.WithTimeout(ctx, distillationTimeout)
	defer cancel()
	report, err := s.target.DistillBrain(ctx, corr)
	if err != nil {
		return ConsolidateOutput{}, err
	}
	return ConsolidateOutput{CorrelationID: corr, ClustersFound: report.ClustersFound,
		ClustersMerged: report.ClustersMerged, RecordsSuperseded: report.RecordsSuperseded,
		ConsolidatedIDs: report.ConsolidatedIDs, SkippedNonJSON: report.SkippedNonJSON,
		ActiveBefore: report.ActiveBefore, ActiveAfter: report.ActiveAfterApprox}, nil
}

func (s *Distillation) RebuildProfile(ctx context.Context, _ DistillInput) (ProfileOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProfileOutput{}, err
	}
	corr := opapi.CorrelationFromContext(ctx)
	if corr == "" {
		corr = s.target.NewCorrelation()
	}
	ctx, cancel := context.WithTimeout(ctx, distillationTimeout)
	defer cancel()
	report, err := s.target.DistillProfile(ctx, corr)
	if err != nil {
		return ProfileOutput{}, err
	}
	return ProfileOutput{CorrelationID: corr, InputRecords: report.InputRecords,
		FacetsWritten: report.FacetsWritten, Facets: report.Facets}, nil
}
