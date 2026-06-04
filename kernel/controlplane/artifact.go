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

	"github.com/agezt/agezt/kernel/artifact"
)

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
