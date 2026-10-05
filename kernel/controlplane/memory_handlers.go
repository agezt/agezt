package controlplane

// Provenance: SPDX-License-Identifier: MIT Memory control-plane handlers:
//             memoryRememberSpecFromArgs (the spec helper) + handleMemoryAdd +
//             handleMemorySupersede (the write path) + handleMemoryGet +
//             handleMemoryList + handleMemorySearch + handleMemoryPromote (the read
//             + lifecycle). The hygiene mutators (handleMemoryForget +
//             handleMemoryPrune + handleMemoryTidy + defaultPruneDays) live in
//             memory_handlers_tidy.go. Extracted from memory_handlers.go during the
//             Day-205 god-file split. Public API unchanged.

import (
	"errors"
	"net"

	"context"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
)

func memoryRememberSpecFromArgs(args map[string]any) (appmemory.RememberInput, error) {
	content, _, err := argString(args, "content")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	if content == "" {
		return appmemory.RememberInput{}, errors.New("args.content required")
	}
	subject, _, err := argString(args, "subject")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	typ, _, err := argString(args, "type")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	conf, _, err := argFloat64(args, "confidence")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	evidence, _, err := argString(args, "evidence")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	halfLifeMS, _, err := argFloat64(args, "half_life_ms")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	tags, _, err := argStringMap(args, "tags")
	if err != nil {
		return appmemory.RememberInput{}, err
	}
	return appmemory.RememberInput{
		Type:       typ,
		Subject:    subject,
		Content:    content,
		Tags:       tags,
		Confidence: conf,
		Evidence:   evidence,
		HalfLifeMS: halfLifeMS,
	}, nil
}

func (s *Server) handleMemoryAdd(conn net.Conn, req Request) {
	spec, err := memoryRememberSpecFromArgs(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appmemory.New(s.k.Memory()).Remember(context.Background(), spec)
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
	out, err := appmemory.New(s.k.Memory()).Supersede(context.Background(), appmemory.SupersedeInput{RememberInput: spec, OldID: oldID})
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

func (s *Server) handleMemoryList(conn net.Conn, req Request) {
	prepared, err := appmemory.New(s.k.Memory()).PrepareList(context.Background())
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit, _, err := argFloat64(req.Args, "limit")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cursor, _, err := argString(req.Args, "cursor")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := prepared.Page(context.Background(), appmemory.ListInput{Limit: limit, Cursor: cursor})
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

func (s *Server) handleMemoryGet(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appmemory.New(s.k.Memory()).Get(context.Background(), appmemory.GetInput{ID: id})
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

func (s *Server) handleMemorySearch(conn net.Conn, req Request) {
	query, err := requiredArgString(req.Args, "query")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit, _, err := argFloat64(req.Args, "limit")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appmemory.New(s.k.Memory()).Search(context.Background(), appmemory.SearchInput{Query: query, Limit: limit})
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
