// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/platform/netout"
	"net"
)

// Native codecs retain lenient string arguments and manual primary read-only binding.
func (s *Server) channelGateway() *appchannels.Gateway {
	return appchannels.NewGateway(netout.GatewayGET)
}
func gatewayInput(req Request) appchannels.GatewayInput {
	return appchannels.GatewayInput{URL: wgArg(req, "url"), Backend: wgArg(req, "backend"), Session: wgArg(req, "session"), Key: wgArg(req, "key")}
}
func (s *Server) handleWhatsAppGatewayStatus(conn net.Conn, req Request) {
	out, err := s.channelGateway().Status(context.Background(), gatewayInput(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleWhatsAppGatewayQR(conn net.Conn, req Request) {
	out, err := s.channelGateway().QR(context.Background(), gatewayInput(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}

// wgArg preserves missing/non-string/empty leniency through the typed accessor.
func wgArg(req Request, key string) string { v, _, _ := argString(req.Args, key); return v }
