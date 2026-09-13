// SPDX-License-Identifier: MIT
//
// Memory control-plane hygiene handlers: handleMemoryForget +
// handleMemoryPromote (the soft-delete + ownership-promotion) +
// handleMemoryPrune + handleMemoryTidy (the hard-remove + compaction) +
// defaultPruneDays (the age threshold constant).
// Extracted from memory_handlers.go during the Day-205 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"time"
)

func (s *Server) handleMemoryForget(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	ok, err := s.k.Memory().Forget("", id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"forgotten": ok},
	})
}

// handleMemoryPromote (M915) shares a private record: its scope tag is cleared
// so it joins the shared brain every agent recalls. The selective-sharing valve
// over per-agent memory; journaled as memory.promoted.
func (s *Server) handleMemoryPromote(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	rec, found, err := s.k.Memory().Promote("", id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	result := map[string]any{"promoted": found, "id": id}
	if found {
		result["subject"] = rec.Subject
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// defaultPruneDays is the age threshold below which soft-deleted records are NOT
// pruned — recently forgotten/superseded records stay recoverable for a month.
const defaultPruneDays = 30

// handleMemoryPrune hard-removes soft-deleted (tombstoned/superseded) records
// older than older_than_days, reclaiming the dead weight consolidation and
// forgets leave behind (M857 — "no memory-bomb"). dry_run (the default) reports
// the store's hygiene + how many would be pruned; dry_run=false prunes. Mirrors
// the artifact collector's confirm-first flow.
func (s *Server) handleMemoryPrune(conn net.Conn, req Request) {
	mgr := s.k.Memory()
	if mgr == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "memory unavailable"})
		return
	}
	days := dlInt(req.Args, "older_than_days")
	if days <= 0 {
		days = defaultPruneDays
	}
	dryRun, err := argDryRun(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()

	hyg, err := mgr.Hygiene(cutoff)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if dryRun {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
			"dry_run": true, "older_than_days": days, "cutoff_ms": cutoff,
			"prunable": hyg.Prunable, "stats": hyg,
		}})
		return
	}
	pruned, err := mgr.Prune("", cutoff, false)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dry_run": false, "older_than_days": days, "cutoff_ms": cutoff,
		"pruned": pruned, "stats": hyg,
	}})
}

// handleMemoryTidy collapses the near-duplicate auto-distilled notes that built
// up before the write-time subject gate (M993). dry_run (the default) reports how
// many would be collapsed; dry_run=false forgets the redundant ones, keeping the
// strongest note per subject. Curated memories are never touched. Confirm-first,
// like the prune/collect flows.
func (s *Server) handleMemoryTidy(conn net.Conn, req Request) {
	mgr := s.k.Memory()
	if mgr == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "memory unavailable"})
		return
	}
	dryRun, err := argDryRun(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	n, err := mgr.DedupeDistilled("", dryRun)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dry_run": dryRun, "collapsed": n,
	}})
}

// handleMemoryBulkForget soft-deletes multiple records in one operation.
// It is idempotent: already-tombstoned records are counted as "forgotten".
