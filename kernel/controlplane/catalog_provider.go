// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"

	appproviders "github.com/agezt/agezt/kernel/app/providers"
)

func (s *Server) handleProviderConnect(conn net.Conn, req Request) {
	sa, err := argStrings(req.Args, "id", "api", "model", "env", "name", "npm")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appproviders.New(s.k, s.baseDir).Connect(context.Background(), appproviders.ConnectInput{ID: sa["id"], API: sa["api"], Model: sa["model"], Env: sa["env"], Name: sa["name"], NPM: sa["npm"]})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderReload(conn net.Conn, req Request) {
	out, err := appproviders.New(s.k, s.baseDir).Reload(context.Background(), appproviders.ReloadInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
