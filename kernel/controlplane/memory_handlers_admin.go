// SPDX-License-Identifier: MIT

// Memory bulk + audit + cleanup handlers (BulkForget/FindRelated/Audit/Clean).
// Code extracted from memory_handlers.go during the Day-81 god-file split.
// Public API unchanged.
package controlplane



import (
	"net"
)

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