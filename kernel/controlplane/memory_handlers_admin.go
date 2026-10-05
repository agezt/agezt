// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Memory bulk + audit + cleanup handlers
//             (BulkForget/FindRelated/Audit/Clean). Code extracted from
//             memory_handlers.go during the Day-81 god-file split. Public API
//             unchanged.

import (
	"context"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
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
	limit, _, err := argFloat64(req.Args, "limit")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appmemory.New(s.k.Memory()).FindRelated(context.Background(), appmemory.RelatedInput{ID: id, Limit: limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	body, err := jsonMap(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
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
