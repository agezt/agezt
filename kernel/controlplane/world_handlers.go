package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane world-model HTTP
//             handlers (handleWorldAdd/Edit/Relate/
//             Resolve/Neighbors/List/Get/Forget). Extracted from world.go during Day
//             211 god-file refactor (#79). Public API unchanged.

import (
	"net"

	"context"
	"encoding/json"
	appworld "github.com/agezt/agezt/kernel/app/world"
)

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

	out, err := appworld.New(s.k.World()).Add(context.Background(), appworld.AddInput{Name: name, Kind: kind, Aliases: aliases, Attrs: attrs})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
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
	out, err := appworld.New(s.k.World()).Edit(context.Background(), appworld.EditInput{ID: id, Aliases: aliases, Attrs: attrs})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
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
	out, err := appworld.New(s.k.World()).Relate(context.Background(), appworld.RelateInput{From: from, To: to, Verb: verb})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
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
	out, err := appworld.New(s.k.World()).Resolve(context.Background(), appworld.ResolveInput{Query: query, Limit: limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
func (s *Server) handleWorldNeighbors(conn net.Conn, req Request) {
	query, err := requiredArgString(req.Args, "query")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appworld.New(s.k.World()).Neighbors(context.Background(), appworld.QueryInput{Query: query})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
func (s *Server) handleWorldList(conn net.Conn, req Request) {
	out, err := appworld.New(s.k.World()).List(context.Background(), appworld.ListInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
func (s *Server) handleWorldGet(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appworld.New(s.k.World()).Get(context.Background(), appworld.GetInput{ID: id})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
func (s *Server) handleWorldForget(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appworld.New(s.k.World()).Forget(context.Background(), appworld.GetInput{ID: id})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
