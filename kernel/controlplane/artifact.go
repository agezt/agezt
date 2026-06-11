// SPDX-License-Identifier: MIT

package controlplane

// Artifact retrieval (M391, SPEC-04 §3.6). The agent loop offloads a large tool
// output to the content-addressed artifact store and the journaled tool.result
// carries a raw_ref. This handler fetches the full bytes back by that ref, so an
// operator (via `agt artifact get`) or a UI can recover an output that never sat
// inline in the journal. The store re-verifies the bytes against the ref, so a
// corrupted blob is reported rather than returned.

import (
	"encoding/base64"
	"errors"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/artifact"
)

// defaultCollectDays is the age past which an artifact is a collection candidate
// when the caller doesn't specify (M845).
const defaultCollectDays = 30

// handleArtifactCollect reaps stale artifacts older than older_than_days (M845).
// dry_run (the default) only reports candidates; dry_run=false deletes them. The
// operator-facing flow runs a dry-run first, then confirms.
func (s *Server) handleArtifactCollect(conn net.Conn, req Request) {
	idx := s.k.ArtifactIndex()
	if idx == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact index unavailable"})
		return
	}
	days := dlInt(req.Args, "older_than_days")
	if days <= 0 {
		days = defaultCollectDays
	}
	// dry_run defaults to TRUE — collection only deletes when explicitly asked.
	dryRun := true
	if v, ok := req.Args["dry_run"].(bool); ok {
		dryRun = v
	} else if v, ok := req.Args["dry_run"].(string); ok {
		dryRun = !(v == "false" || v == "0")
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()

	if dryRun {
		cands := idx.StaleEntries(cutoff)
		out := make([]map[string]any, 0, len(cands))
		var bytes int64
		for _, e := range cands {
			bytes += e.Size
			out = append(out, map[string]any{
				"id": e.ID, "name": e.Name, "kind": e.Kind, "source": e.Source,
				"size": e.Size, "created_ms": e.CreatedMs,
			})
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
			"dry_run": true, "older_than_days": days, "cutoff_ms": cutoff,
			"count": len(out), "bytes": bytes, "candidates": out,
		}})
		return
	}
	collected, bytes := idx.Collect(cutoff)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dry_run": false, "older_than_days": days, "cutoff_ms": cutoff,
		"count": collected, "bytes": bytes,
	}})
}

func (s *Server) handleArtifactGet(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if ref == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	store := s.k.Artifacts()
	if store == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact store unavailable"})
		return
	}
	data, err := store.Get(ref)
	if err != nil {
		// Map the store sentinels to clear operator-facing messages.
		switch {
		case errors.Is(err, artifact.ErrBadRef):
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "malformed ref (want a 64-hex content address)"})
		case errors.Is(err, artifact.ErrNotFound):
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact not found: " + ref})
		case errors.Is(err, artifact.ErrCorrupt):
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact CORRUPT (bytes do not match ref): " + ref})
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		}
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"ref":  ref,
			"size": len(data),
			"data": base64.StdEncoding.EncodeToString(data),
		},
	})
}

// handleArtifactList returns the artifact INDEX entries (M822), newest first,
// optionally filtered by kind/source/corr — metadata only, no bytes. The file
// manager and Inbox use this to enumerate stored artifacts.
func (s *Server) handleArtifactList(conn net.Conn, req Request) {
	idx := s.k.ArtifactIndex()
	if idx == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact index unavailable"})
		return
	}
	kind, _ := req.Args["kind"].(string)
	source, _ := req.Args["source"].(string)
	corr, _ := req.Args["corr"].(string)
	ents := idx.List(artifact.Filter{Kind: kind, Source: source, Corr: corr})
	out := make([]map[string]any, 0, len(ents))
	for _, e := range ents {
		out = append(out, map[string]any{
			"id": e.ID, "ref": e.Ref, "name": e.Name, "mime": e.Mime,
			"kind": e.Kind, "source": e.Source, "sender": e.Sender,
			"corr": e.Corr, "size": e.Size, "created_ms": e.CreatedMs,
			"caption": e.Caption,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"count":   len(out),
		"entries": out,
	}})
}

// handleArtifactDelete removes an index entry by id (M822); the blob is GC'd when
// no other entry references it.
func (s *Server) handleArtifactDelete(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	idx := s.k.ArtifactIndex()
	if idx == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact index unavailable"})
		return
	}
	if err := idx.Delete(id); err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "artifact not found: " + id})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"deleted": true, "id": id}})
}
