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
