// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/taste"
)

func tasteExemplarView(e taste.Exemplar) map[string]any {
	b, _ := json.Marshal(e)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (s *Server) handleTasteList(conn net.Conn, req Request) {
	f := taste.Filter{
		Scope: stringArg(req.Args, "scope"),
		Tag:   stringArg(req.Args, "tag"),
		Limit: intArg(req.Args["limit"], 200),
	}
	exemplars := s.k.Taste().List(f)
	out := make([]any, 0, len(exemplars))
	for _, e := range exemplars {
		out = append(out, tasteExemplarView(e))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"exemplars": out, "count": len(out)}})
}

func (s *Server) handleTasteCreate(conn net.Conn, req Request) {
	title := stringArg(req.Args, "title")
	body := stringArg(req.Args, "body")
	if title == "" || body == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "taste_create requires title and body"})
		return
	}
	e, err := s.k.Taste().Create(taste.CreateSpec{
		Title: title,
		Body:  body,
		Scope: stringArg(req.Args, "scope"),
		Tags:  workboardStringSliceArg(req.Args["tags"]),
	}, time.Now())
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"exemplar": tasteExemplarView(e)}})
}

func (s *Server) handleTasteDelete(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "taste_delete requires id"})
		return
	}
	if err := s.k.Taste().Delete(id); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"deleted": id}})
}
