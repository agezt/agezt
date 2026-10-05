// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"net"
)

func (s *Server) handleMemoryConsolidate(conn net.Conn, req Request) {
	out, err := appmemory.NewDistillation(s.k).Consolidate(context.Background(), appmemory.DistillInput{})
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

func (s *Server) handleProfileRebuild(conn net.Conn, req Request) {
	out, err := appmemory.NewDistillation(s.k).RebuildProfile(context.Background(), appmemory.DistillInput{})
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
