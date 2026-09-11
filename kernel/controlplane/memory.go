// SPDX-License-Identifier: MIT

// Memory top-level: handleMemoryConsolidate + handleProfileRebuild.
// Code extracted from memory.go during the Day-61 god-file split. Public API unchanged.
package controlplane


import (
	"context"
	"net"
	"time"
)



// memoryConsolidateTimeout bounds one brain-distillation pass — at most
// maxClustersPerPass provider calls.
const memoryConsolidateTimeout = 5 * time.Minute

// handleMemoryConsolidate (M804) runs one synchronous brain-distillation
// pass and returns its report. The pass journals memory.consolidated +
// memory.superseded under a fresh correlation, so `agt why` explains every
// merge.
func (s *Server) handleMemoryConsolidate(conn net.Conn, req Request) {
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), memoryConsolidateTimeout)
	defer cancel()
	report, err := s.k.DistillBrain(ctx, corr)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"correlation_id":     corr,
			"clusters_found":     report.ClustersFound,
			"clusters_merged":    report.ClustersMerged,
			"records_superseded": report.RecordsSuperseded,
			"consolidated_ids":   report.ConsolidatedIDs,
			"skipped_non_json":   report.SkippedNonJSON,
			"active_before":      report.ActiveBefore,
			"active_after":       report.ActiveAfterApprox,
		},
	})
}

// handleProfileRebuild (M1000) runs one synchronous operator-profile
// distillation pass and returns its report. The pass journals memory.profiled +
// memory.written under a fresh correlation.
func (s *Server) handleProfileRebuild(conn net.Conn, req Request) {
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), memoryConsolidateTimeout)
	defer cancel()
	report, err := s.k.DistillProfile(ctx, corr)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"correlation_id": corr,
			"input_records":  report.InputRecords,
			"facets_written": report.FacetsWritten,
			"facets":         report.Facets,
		},
	})
}

// memoryRememberSpecFromArgs decodes the RememberSpec fields shared by the
// memory add and supersede commands with the typed accessors (Phase 1.2b), so
// a mistyped field errors instead of silently zeroing. Content is validated
// required; Actor/Force are the operator-write constants both callers use.