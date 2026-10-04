// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/system"
	"net"
)

func (s *Server) systemService() *system.Service {
	var tenants system.TenantCounter
	if s.tenants != nil {
		tenants = s.tenants
	}
	return system.New(s.k, tenants, s.httpBindings, s.channels, s.credChain)
}

func (s *Server) handleStatus(conn net.Conn, req Request) {
	result, err := s.systemService().Status(context.Background(), system.StatusInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleVersion(conn net.Conn, req Request) {
	result, err := s.systemService().Version(context.Background(), system.VersionInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
