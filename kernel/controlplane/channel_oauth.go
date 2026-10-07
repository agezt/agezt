// SPDX-License-Identifier: MIT

package controlplane

// Channel OAuth connect flow (Phase 4). For channels whose ConnectMethod is
// "oauth", an operator registers an OAuth app with the provider, pastes the
// client id + secret into the Connect page, and clicks "Connect with X" instead
// of hunting for a token. The daemon builds the provider's authorize URL, the
// browser authorizes and is redirected to the daemon's public /oauth/callback,
// and the daemon exchanges the code for an access token which it writes into the
// account's "#label" vault slot — the same storage every other channel field
// uses. Token paste stays available as a fallback.
//
// Scope: providers whose returned access_token is DIRECTLY usable as the
// channel's token — Slack (v2 returns the bot token at top-level access_token)
// and Mastodon (per-instance user token). Discord (static bot token from the dev
// portal) and Google Chat (service account) don't benefit from user-OAuth and
// stay on token entry.

import (
	"context"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"net"
)

// Native codecs preserve lenient string reads; selected app service owns use cases.
func (s *Server) handleChannelOAuthStart(conn net.Conn, req Request) {
	out, err := s.channelOAuth().Start(context.Background(), appchannels.OAuthStartInput{Kind: stringArg(req.Args, "kind"), Label: stringArg(req.Args, "label"), ClientID: stringArg(req.Args, "client_id"), ClientSecret: stringArg(req.Args, "client_secret"), RedirectURI: stringArg(req.Args, "redirect_uri"), InstanceURL: stringArg(req.Args, "instance_url")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleChannelOAuthCallback(ctx context.Context, conn net.Conn, req Request) {
	out, err := s.channelOAuth().Callback(ctx, appchannels.OAuthCallbackInput{Code: stringArg(req.Args, "code"), State: stringArg(req.Args, "state")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleChannelOAuthStatus(conn net.Conn, req Request) {
	out, err := s.channelOAuth().Status(context.Background(), appchannels.OAuthStatusInput{State: stringArg(req.Args, "state")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
