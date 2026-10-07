// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"net"
	"strings"
)

// Resolve the currently selected server sender for each request.
func (s *Server) channelOutbound() *appchannels.Outbound {
	return appchannels.NewOutbound(appchannels.Sender(s.channelSend))
}
func (s *Server) handleSend(conn net.Conn, req Request) {
	s.channelOutbound().Send(context.Background(), appchannels.SendInput{Channel: stringArg(req.Args, "channel"), To: stringArg(req.Args, "to"), Text: stringArg(req.Args, "text")}, func(out appchannels.SendOutput, err error) {
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
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
