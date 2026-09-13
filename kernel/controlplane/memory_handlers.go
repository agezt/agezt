// SPDX-License-Identifier: MIT
//
// Memory control-plane handlers: memoryRememberSpecFromArgs (the spec helper)
// + handleMemoryAdd + handleMemorySupersede (the write path) +
// handleMemoryGet + handleMemoryList + handleMemorySearch +
// handleMemoryPromote (the read + lifecycle).
// The hygiene mutators (handleMemoryForget + handleMemoryPrune +
// handleMemoryTidy + defaultPruneDays) live in memory_handlers_tidy.go.
// Extracted from memory_handlers.go during the Day-205 god-file split.
// Public API unchanged.
package controlplane

import (
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/memory"
)

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
