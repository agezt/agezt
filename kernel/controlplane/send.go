// SPDX-License-Identifier: MIT

package controlplane

// Operator-initiated outbound (M142). The manual egress complement to Pulse
// briefs and agent replies: a script/CI/operator pushes a one-off message out a
// configured channel ("deploy finished → notify Slack"). Authenticated by the
// control plane (primary token), so no per-channel allowlist gate — the caller
// already holds daemon authority. The channel's own Send journals the
// channel.outbound event, so `agt inbox` / `agt why` still see it.

import (
	"context"
	"net"
	"strings"
	"time"
)

func (s *Server) handleSend(conn net.Conn, req Request) {
	kind := strings.ToLower(stringArg(req.Args, "channel"))
	to := stringArg(req.Args, "to")
	text := stringArg(req.Args, "text")
	if kind == "" || to == "" || text == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "send requires channel, to, and text"})
		return
	}
	if s.channelSend == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no channels configured (set a channel token to enable send)"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.channelSend(ctx, kind, to, text); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"sent": true, "channel": kind, "to": to},
	})
}

// stringArg reads a string argument from a request arg map, "" when absent or not
// a string.
func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
