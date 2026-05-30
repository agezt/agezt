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

func (s *Server) handleWorldAdd(conn net.Conn, req Request) {
	name, _ := req.Args["name"].(string)
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	kind, _ := req.Args["kind"].(string)

	var aliases []string
	if raw, ok := req.Args["aliases"].([]any); ok {
		for _, a := range raw {
			if sv, ok := a.(string); ok {
				aliases = append(aliases, sv)
			}
		}
	}
	var attrs map[string]string
	if raw, ok := req.Args["attrs"].(map[string]any); ok {
		attrs = make(map[string]string, len(raw))
		for k, v := range raw {
			if sv, ok := v.(string); ok {
				attrs[k] = sv
			}
		}
	}

	e, created, err := s.k.World().Upsert("", worldmodel.UpsertSpec{
		Kind: worldmodel.Kind(kind), Name: name, Aliases: aliases, Attrs: attrs,
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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

func (s *Server) handleWorldRelate(conn net.Conn, req Request) {
	from, _ := req.Args["from"].(string)
	to, _ := req.Args["to"].(string)
	if from == "" || to == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.from and args.to required"})
		return
	}
	verb, _ := req.Args["verb"].(string)
	r, err := s.k.World().Relate("", from, worldmodel.Verb(verb), to)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	hits, err := s.k.World().ResolveQuiet(query, limit)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	query, _ := req.Args["query"].(string)
	if query == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.query required"})
		return
	}
	hits, err := s.k.World().ResolveQuiet(query, 1)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	rels, err := s.k.World().Relations()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(ents))
	for _, e := range ents {
		out = append(out, entityView(e))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entities": out, "count": len(out), "relation_count": len(rels)},
	})
}

func (s *Server) handleWorldGet(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	e, found, err := s.k.World().Get(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{"found": found}
	if found {
		result["entity"] = entityView(e)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleWorldForget(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	ok, err := s.k.World().Forget("", id)
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
