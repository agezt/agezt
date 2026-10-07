// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"net"
)

// Native decoder preserves float64-only limit and lenient channel input, with
// cursor errors returned after the selected journal scan as in the legacy handler.
func inboxInput(req Request) appchannels.InboxInput {
	in := appchannels.InboxInput{}
	if value, present, err := argFloat64(req.Args, "limit"); present && err == nil {
		limit := int(value)
		in.Limit = &limit
	}
	in.Channel, _, _ = argString(req.Args, "channel")
	in.Cursor, _, in.CursorError = argString(req.Args, "cursor")
	return in
}
func (s *Server) channelInbox() *appchannels.Inbox { return appchannels.NewInbox(s.k.Journal()) }
func (s *Server) handleInbox(conn net.Conn, req Request) {
	out, err := s.channelInbox().List(context.Background(), inboxInput(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
