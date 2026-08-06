// SPDX-License-Identifier: MIT

package controlplane

// Memory-lite inspection/mutation handlers. Surfaces the content-addressed,
// journaled knowledge store to operators — the read/write path behind
// `agt memory`. Writes go through the kernel's memory.Manager so every
// mutation is journaled (memory.written / memory.forgotten) and auditable
// via `agt why`, exactly like a mutation the agent itself made.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
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
func memoryRememberSpecFromArgs(args map[string]any) (memory.RememberSpec, error) {
	content, _, err := argString(args, "content")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	if content == "" {
		return memory.RememberSpec{}, errors.New("args.content required")
	}
	subject, _, err := argString(args, "subject")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	typ, _, err := argString(args, "type")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	conf, _, err := argFloat64(args, "confidence")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	evidence, _, err := argString(args, "evidence")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	halfLifeMS := int64(0)
	if raw, _, err := argFloat64(args, "half_life_ms"); err != nil {
		return memory.RememberSpec{}, err
	} else if raw > 0 {
		halfLifeMS = int64(raw)
	}
	tags := map[string]string{"source": "operator"}
	rawTags, _, err := argStringMap(args, "tags")
	if err != nil {
		return memory.RememberSpec{}, err
	}
	for k, v := range rawTags {
		tags[k] = v
	}
	return memory.RememberSpec{
		Type:       memory.Type(typ),
		Subject:    subject,
		Content:    content,
		Tags:       tags,
		Confidence: conf,
		Evidence:   memory.Evidence(evidence),
		HalfLifeMS: halfLifeMS,
		Actor:      "operator", // a console/CLI write (M851)
		Force:      true,
	}, nil
}

func (s *Server) handleMemoryAdd(conn net.Conn, req Request) {
	spec, err := memoryRememberSpecFromArgs(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	rec, created, err := s.k.Memory().Remember("", spec)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id":       rec.ID,
			"created":  created,
			"type":     string(rec.Type),
			"subject":  rec.Subject,
			"evidence": string(rec.Evidence),
		},
	})
}

// handleMemorySupersede revises a record (M731): stores a new one and links the
// old record's superseded_by to it (soft update — the old record is retained, recall
// uses the new one). Memory is content-addressed so an in-place edit is impossible;
// supersession is the model-correct "edit". Reviving to identical content is a no-op
// (the new id equals the old) and reported as superseded:false.
func (s *Server) handleMemorySupersede(conn net.Conn, req Request) {
	oldID, _, err := argString(req.Args, "old_id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if oldID == "" {
		s.failMsg(conn, req, "args.old_id required")
		return
	}
	spec, err := memoryRememberSpecFromArgs(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	rec, err := s.k.Memory().Supersede("", oldID, spec)
	if err != nil {
		s.fail(conn, req, err)
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
		s.fail(conn, req, err)
		return
	}
	// Cursor pagination (M-pending follow-up): the SPA's Memory view polls
	// this on every render; for a busy memory store the response is large
	// enough to slow the panel. Newest first; cursor = (CreatedMS, ID).
	limit := 100
	if raw, _, lerr := argFloat64(req.Args, "limit"); lerr != nil {
		s.fail(conn, req, lerr)
		return
	} else if raw > 0 {
		limit = int(raw)
	}
	if limit > 1000 {
		limit = 1000
	}
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].CreatedMS != recs[j].CreatedMS {
			return recs[i].CreatedMS > recs[j].CreatedMS
		}
		return recs[i].ID > recs[j].ID
	})
	total := len(recs)
	var cursorMS int64
	var cursorID string
	cursorOK := false
	if raw, _, cerr := argString(req.Args, "cursor"); cerr != nil {
		s.fail(conn, req, cerr)
		return
	} else if raw != "" {
		msStr, id, _ := strings.Cut(raw, ":")
		if ms, err := strconv.ParseInt(msStr, 10, 64); err == nil {
			cursorMS, cursorID, cursorOK = ms, id, true
		}
	}
	if cursorOK {
		filtered := recs[:0]
		for _, r := range recs {
			if r.CreatedMS > cursorMS {
				continue
			}
			if r.CreatedMS == cursorMS && r.ID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		recs = filtered
	}
	var nextCursor string
	if limit > 0 && len(recs) > limit {
		recs = recs[:limit]
		nextCursor = strconv.FormatInt(recs[limit-1].CreatedMS, 10) + ":" + recs[limit-1].ID
	}
	out := make([]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, recordView(r))
	}
	result := map[string]any{"records": out, "count": len(out), "total": total}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleMemoryGet(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	rec, found, err := s.k.Memory().Get(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	result := map[string]any{"found": found}
	if found {
		result["record"] = recordView(rec)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleMemorySearch(conn net.Conn, req Request) {
	query, err := requiredArgString(req.Args, "query")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit := 10
	if l, _, lerr := argFloat64(req.Args, "limit"); lerr != nil {
		s.fail(conn, req, lerr)
		return
	} else if l > 0 {
		limit = int(l)
	}
	if limit > 100 {
		limit = 100
	}
	hits, err := s.k.Memory().Search(query, limit)
	if err != nil {
		s.fail(conn, req, err)
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
// Args: ids (required, array of string). Returns: { forgotten: N, not_found: M }.
func (s *Server) handleMemoryBulkForget(conn net.Conn, req Request) {
	strIDs, present, err := argStringList(req.Args, "ids")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !present {
		s.failMsg(conn, req, "args.ids required")
		return
	}
	if len(strIDs) == 0 {
		s.ok(conn, req, map[string]any{"forgotten": 0, "not_found": 0})
		return
	}
	if len(strIDs) > 500 {
		s.failMsg(conn, req, "args.ids exceeds 500 — use smaller batches")
		return
	}

	var forgotten, notFound int
	for _, id := range strIDs {
		ok, err := s.k.Memory().Forget("", id)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		if ok {
			forgotten++
		} else {
			notFound++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"forgotten": forgotten, "not_found": notFound},
	})
}

// handleMemoryFindRelated uses embedding-based similarity to find active records
// related to a given seed record. The seed's content is embedded and compared
// against the active corpus. The seed record itself is excluded from results.
// Args: id (required), limit (optional; default 10, max 100).
// Returns: { results: [{record, score}, ...], count }.
func (s *Server) handleMemoryFindRelated(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit := 10
	if l, _, lerr := argFloat64(req.Args, "limit"); lerr != nil {
		s.fail(conn, req, lerr)
		return
	} else if l > 0 {
		limit = int(l)
	}
	if limit > 100 {
		limit = 100
	}

	seed, found, err := s.k.Memory().Get(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "seed record id not found"})
		return
	}

	// Search with the seed's content as the query — uses hybrid (keyword +
	// embedding) search so it works even when no embedder is configured.
	hits, err := s.k.Memory().Search(seed.Content, limit+1) // +1 because seed itself may appear
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Exclude the seed record from results.
	out := make([]any, 0, limit)
	for _, h := range hits {
		if h.Record.ID != id {
			out = append(out, map[string]any{"record": recordView(h.Record), "score": h.Score})
		}
		if len(out) >= limit {
			break
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"results": out, "count": len(out)},
	})
}

func (s *Server) handleMemoryAudit(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	report, err := k.Memory().Audit()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	body, _ := jsonMap(report)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}

func (s *Server) handleMemoryClean(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	dryRun, err := argDryRun(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	report, err := k.Memory().CleanLowValue("", dryRun)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	body, _ := jsonMap(report)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
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
	if r.Evidence != "" {
		v["evidence"] = string(r.Evidence)
	}
	if r.HalfLifeMS > 0 {
		v["half_life_ms"] = r.HalfLifeMS
		v["expires_ms"] = r.LastSeenMS + r.HalfLifeMS
		if r.Expired(time.Now().UnixMilli()) {
			v["expired"] = true
		}
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
	if r.Suspended() {
		v["suspended_ms"] = r.SuspendedMS
		v["suspended"] = true
		if r.SuspendedReason != "" {
			v["suspended_reason"] = r.SuspendedReason
		}
	}
	return v
}

func jsonMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}

// registerMemoryCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerMemoryCommands() {
	register(
		commandSpec{Cmd: CmdMemoryAdd, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySupersede, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySupersede(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryList, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryGet, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySearch, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySearch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryConsolidate, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryConsolidate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProfileRebuild, Handler: func(dc *DispatchCtx) { dc.S.handleProfileRebuild(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPromote, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPromote(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPrune, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPrune(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryTidy, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryTidy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryBulkForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryBulkForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryFindRelated, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryFindRelated(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryAudit, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAudit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryClean, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryClean(dc.Conn, dc.Req) }},
	)
}
