// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"

	"github.com/agezt/agezt/kernel/app/providers"
)

func (s *Server) providerOAuth() *providers.OAuth {
	s.providerOAuthOnce.Do(func() {
		s.providerOAuthState = providers.NewOAuth(s.k, s.baseDir, func() ([]string, string) {
			if s.chatgptSync == nil {
				return nil, ""
			}
			return s.chatgptSync()
		})
	})
	return s.providerOAuthState
}
func (s *Server) handleProviderOAuthStart(conn net.Conn, req Request) {
	out, err := s.providerOAuth().Start(context.Background(), providers.OAuthStartInput{Provider: stringArg(req.Args, "provider")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderOAuthStatus(conn net.Conn, req Request) {
	out, err := s.providerOAuth().Status(context.Background(), providers.OAuthStatusInput{State: stringArg(req.Args, "state")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderOAuthImport(conn net.Conn, req Request) {
	out, err := s.providerOAuth().Import(context.Background(), providers.OAuthImportInput{Path: stringArg(req.Args, "path")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderOAuthLogout(conn net.Conn, req Request) {
	out, err := s.providerOAuth().Logout(context.Background(), providers.OAuthLogoutInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
