// SPDX-License-Identifier: MIT

package controlplane

// World-model inspection/mutation handlers — the read/write path behind
// `agt world`. Writes go through the kernel's worldmodel.Graph so every
// node/edge mutation is journaled (worldmodel.entity.upserted /
// relation.upserted / forgotten) and auditable via `agt why`, exactly like a
// mutation the agent itself made through the `world` tool.

import (
	"net"

	"github.com/agezt/agezt/kernel/worldmodel"
)

// worldAliasesAttrs decodes the optional aliases/attrs pair shared by the
// entity add+edit handlers.
func worldAliasesAttrs(args map[string]any) ([]string, map[string]string, error) {
	aliases, _, err := argStringList(args, "aliases")
	if err != nil {
		return nil, nil, err
	}
	attrs, _, err := argStringMap(args, "attrs")
	if err != nil {
		return nil, nil, err
	}
	return aliases, attrs, nil
}

func (s *Server) handleWorldAdd(conn net.Conn, req Request) {
	name, err := requiredArgString(req.Args, "name")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	kind, _, err := argString(req.Args, "kind")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	aliases, attrs, err := worldAliasesAttrs(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	e, created, err := s.k.World().Upsert("", worldmodel.UpsertSpec{
		Kind: worldmodel.Kind(kind), Name: name, Aliases: aliases, Attrs: attrs,
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": e.ID, "created": created, "kind": string(e.Kind), "name": e.Name,
		},
	})
}

func (s *Server) handleWorldEdit(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	aliases, attrs, err := worldAliasesAttrs(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	e, ok, err := s.k.World().EditEntity("", id, aliases, attrs)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": false}})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"updated": true, "id": e.ID, "kind": string(e.Kind), "name": e.Name,
		},
	})
}

func (s *Server) handleWorldRelate(conn net.Conn, req Request) {
	sa, err := argStrings(req.Args, "from", "to", "verb")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	from, to, verb := sa["from"], sa["to"], sa["verb"]
	if from == "" || to == "" {
		s.failMsg(conn, req, "args.from and args.to required")
		return
	}
	r, err := s.k.World().Relate("", from, worldmodel.Verb(verb), to)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": r.ID, "from": r.From, "verb": string(r.Verb), "to": r.To,
		},
	})
}

func (s *Server) handleWorldResolve(conn net.Conn, req Request) {
	query, err := requiredArgString(req.Args, "query")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit, err := argLimit(req.Args, 10, 100)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	hits, err := s.k.World().ResolveQuiet(query, limit)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, map[string]any{"entity": entityView(h.Entity), "score": h.Score})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"results": out, "count": len(out)},
	})
}

func (s *Server) handleWorldNeighbors(conn net.Conn, req Request) {
	query, err := requiredArgString(req.Args, "query")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	hits, err := s.k.World().ResolveQuiet(query, 1)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if len(hits) == 0 {
		s.writeResp(conn, Response{
			ID: req.ID, Type: RespResult,
			Result: map[string]any{"found": false, "neighbors": []any{}, "count": 0},
		})
		return
	}
	center := hits[0].Entity
	ns, err := s.k.World().Neighbors(center.ID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(ns))
	for _, n := range ns {
		out = append(out, map[string]any{
			"verb":     string(n.Relation.Verb),
			"outgoing": n.Outgoing,
			"other":    entityView(n.Other),
		})
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"found": true, "entity": entityView(center), "neighbors": out, "count": len(out),
		},
	})
}

func (s *Server) handleWorldList(conn net.Conn, req Request) {
	ents, err := s.k.World().Entities()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	rels, err := s.k.World().Relations()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(ents))
	for _, e := range ents {
		out = append(out, entityView(e))
	}
	edges := make([]any, 0, len(rels))
	for _, r := range rels {
		edges = append(edges, map[string]any{
			"id": r.ID, "from": r.From, "verb": string(r.Verb), "to": r.To, "weight": r.Weight,
		})
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"entities": out, "count": len(out),
			"edges": edges, "relation_count": len(rels),
		},
	})
}

func (s *Server) handleWorldGet(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	e, found, err := s.k.World().Get(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	result := map[string]any{"found": found}
	if found {
		result["entity"] = entityView(e)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleWorldForget(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	ok, err := s.k.World().Forget("", id)
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

// entityView renders a worldmodel.Entity as a stable JSON object for the wire.
func entityView(e worldmodel.Entity) map[string]any {
	v := map[string]any{
		"id":           e.ID,
		"kind":         string(e.Kind),
		"name":         e.Name,
		"weight":       e.Weight,
		"created_ms":   e.CreatedMS,
		"last_seen_ms": e.LastSeenMS,
	}
	if len(e.Aliases) > 0 {
		v["aliases"] = e.Aliases
	}
	if len(e.Attrs) > 0 {
		v["attrs"] = e.Attrs
	}
	if e.SourceEvent != "" {
		v["source_event"] = e.SourceEvent
	}
	if e.SupersededBy != "" {
		v["superseded_by"] = e.SupersededBy
	}
	if e.Tombstoned {
		v["tombstoned"] = true
	}
	return v
}
