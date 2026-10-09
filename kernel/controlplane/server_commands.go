package controlplane

// Provenance: SPDX-License-Identifier: MIT Control-plane command handlers:
//             handleWhoami. Extracted from server_handlers.go during the Day-206
//             god-file split; the other handlers have since moved to typed app
//             operations. whoami stays native: it echoes the transport principal
//             and must not route the tenant it names.

import (
	"net"
)

// ----- command handlers -----

// handleWhoami reports the authenticated principal (M62). By the time a request
// reaches a handler, handleConn has already verified the token: the primary
// token equals s.Token(); any other token that got here authenticated as the
// tenant named in (and pinned to) req.Args["tenant"]. So identity is a pure
// read of req.Token vs the primary token — no new auth state.
func (s *Server) handleWhoami(conn net.Conn, req Request) {
	if s.tokenIsPrimary(req.Token) {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"identity": "primary",
				"primary":  true,
				"tenant":   "",
			},
		})
		return
	}
	tenant, _, _ := argString(req.Args, "tenant")
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"identity": "tenant",
			"primary":  false,
			"tenant":   tenant,
		},
	})
}
