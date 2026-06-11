// SPDX-License-Identifier: MIT

package controlplane

// Memory-lite inspection/mutation handlers. Surfaces the content-addressed,
// journaled knowledge store to operators — the read/write path behind
// `agt memory`. Writes go through the kernel's memory.Manager so every
// mutation is journaled (memory.written / memory.forgotten) and auditable
// via `agt why`, exactly like a mutation the agent itself made.

import (
	"context"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/memory"
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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

func (s *Server) handleMemoryAdd(conn net.Conn, req Request) {
	content, _ := req.Args["content"].(string)
	if content == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.content required"})
		return
	}
	subject, _ := req.Args["subject"].(string)
	typ, _ := req.Args["type"].(string)
	conf, _ := req.Args["confidence"].(float64) // JSON numbers decode to float64

	tags := map[string]string{"source": "operator"}
	if raw, ok := req.Args["tags"].(map[string]any); ok {
		for k, v := range raw {
			if sv, ok := v.(string); ok {
				tags[k] = sv
			}
		}
	}

	rec, created, err := s.k.Memory().Remember("", memory.RememberSpec{
		Type:       memory.Type(typ),
		Subject:    subject,
		Content:    content,
		Tags:       tags,
		Confidence: conf,
		Actor:      "operator", // a console/CLI write (M851)
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id":      rec.ID,
			"created": created,
			"type":    string(rec.Type),
			"subject": rec.Subject,
		},
	})
}

// handleMemorySupersede revises a record (M731): stores a new one and links the
// old record's superseded_by to it (soft update — the old record is retained, recall
// uses the new one). Memory is content-addressed so an in-place edit is impossible;
// supersession is the model-correct "edit". Reviving to identical content is a no-op
// (the new id equals the old) and reported as superseded:false.
func (s *Server) handleMemorySupersede(conn net.Conn, req Request) {
	oldID, _ := req.Args["old_id"].(string)
	if oldID == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.old_id required"})
		return
	}
	content, _ := req.Args["content"].(string)
	if content == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.content required"})
		return
	}
	subject, _ := req.Args["subject"].(string)
	typ, _ := req.Args["type"].(string)
	conf, _ := req.Args["confidence"].(float64)

	tags := map[string]string{"source": "operator"}
	if raw, ok := req.Args["tags"].(map[string]any); ok {
		for k, v := range raw {
			if sv, ok := v.(string); ok {
				tags[k] = sv
			}
		}
	}

	rec, err := s.k.Memory().Supersede("", oldID, memory.RememberSpec{
		Type:       memory.Type(typ),
		Subject:    subject,
		Content:    content,
		Tags:       tags,
		Confidence: conf,
		Actor:      "operator", // a console/CLI edit (M851)
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"new_id":     rec.ID,
			"old_id":     oldID,
			"superseded": rec.ID != oldID,
			"type":       string(rec.Type),
			"subject":    rec.Subject,
		},
	})
}

func (s *Server) handleMemoryList(conn net.Conn, req Request) {
	recs, err := s.k.Memory().Active()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, recordView(r))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"records": out, "count": len(out)},
	})
}

func (s *Server) handleMemoryGet(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	rec, found, err := s.k.Memory().Get(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{"found": found}
	if found {
		result["record"] = recordView(rec)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleMemorySearch(conn net.Conn, req Request) {
	query, _ := req.Args["query"].(string)
	if query == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.query required"})
		return
	}
	limit := 10
	if l, ok := req.Args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	if limit > 100 {
		limit = 100
	}
	hits, err := s.k.Memory().Search(query, limit)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, map[string]any{"record": recordView(h.Record), "score": h.Score})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"results": out, "count": len(out)},
	})
}

func (s *Server) handleMemoryForget(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	ok, err := s.k.Memory().Forget("", id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"forgotten": ok},
	})
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
	dryRun := true
	if v, ok := req.Args["dry_run"].(bool); ok {
		dryRun = v
	} else if v, ok := req.Args["dry_run"].(string); ok {
		dryRun = !(v == "false" || v == "0")
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()

	hyg, err := mgr.Hygiene(cutoff)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dry_run": false, "older_than_days": days, "cutoff_ms": cutoff,
		"pruned": pruned, "stats": hyg,
	}})
}

// recordView renders a memory.Record as a stable JSON object for the wire.
// All fields are operator-supplied or derived; nothing here is secret (the
// store never holds credentials — that's the vault's job).
func recordView(r memory.Record) map[string]any {
	v := map[string]any{
		"id":           r.ID,
		"type":         string(r.Type),
		"subject":      r.Subject,
		"content":      r.Content,
		"confidence":   r.Confidence,
		"created_ms":   r.CreatedMS,
		"last_seen_ms": r.LastSeenMS,
	}
	if len(r.Tags) > 0 {
		v["tags"] = r.Tags
	}
	if r.SourceEvent != "" {
		v["source_event"] = r.SourceEvent
	}
	if r.AddedBy != "" {
		v["added_by"] = r.AddedBy
	}
	if r.UpdatedBy != "" {
		v["updated_by"] = r.UpdatedBy
	}
	if r.SupersededBy != "" {
		v["superseded_by"] = r.SupersededBy
	}
	if r.Tombstoned {
		v["tombstoned"] = true
	}
	return v
}
