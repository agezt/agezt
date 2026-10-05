// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
	"net"
)

func (s *Server) handleTasteList(conn net.Conn, req Request) {
	out, err := apptaste.New(s.k.Taste()).List(context.Background(), apptaste.ListInput{Scope: stringArg(req.Args, "scope"), Tag: stringArg(req.Args, "tag"), Limit: intArg(req.Args["limit"], 200)})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeTasteResult(s, conn, req, out)
}
func (s *Server) handleTasteCreate(conn net.Conn, req Request) {
	// Keep lenient native string/collection accessors during the business move.
	title, body := stringArg(req.Args, "title"), stringArg(req.Args, "body")
	if title == "" || body == "" {
		s.failMsg(conn, req, "taste_create requires title and body")
		return
	}
	out, err := apptaste.New(s.k.Taste()).Create(context.Background(), apptaste.CreateInput{Title: title, Body: body, Scope: stringArg(req.Args, "scope"), Tags: workboardStringSliceArg(req.Args["tags"])})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeTasteResult(s, conn, req, out)
}
func (s *Server) handleTasteDelete(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.failMsg(conn, req, "taste_delete requires id")
		return
	}
	out, err := apptaste.New(s.k.Taste()).Delete(context.Background(), apptaste.DeleteInput{ID: id})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeTasteResult(s, conn, req, out)
}
func writeTasteResult(s *Server, conn net.Conn, req Request, out any) {
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
