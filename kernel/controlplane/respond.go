// SPDX-License-Identifier: MIT

package controlplane

import "net"

// fail writes the standard error envelope for req (Phase 1.2). One place to
// later grow error redaction and structured error codes; before this existed
// the literal `Response{ID: req.ID, Type: RespError, Error: err.Error()}`
// appeared ~270 times across the package.
func (s *Server) fail(conn net.Conn, req Request, err error) {
	s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
}

// failMsg is fail for a plain message (validation errors and the like).
func (s *Server) failMsg(conn net.Conn, req Request, msg string) {
	s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: msg})
}

// ok writes the standard result envelope for req.
func (s *Server) ok(conn net.Conn, req Request, result map[string]any) {
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
