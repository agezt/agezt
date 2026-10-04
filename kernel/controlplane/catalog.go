// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
)

func (s *Server) handleCatalogSync(ctx context.Context, conn net.Conn, req Request) {
	url, _, err := argString(req.Args, "url")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	timeout, _, err := argFloat64(req.Args, "timeout_s")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	output, err := appcatalog.New(s.k, s.baseDir).Sync(ctx, appcatalog.SyncInput{URL: url, TimeoutSeconds: timeout})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: output})
}

func (s *Server) handleCatalogList(conn net.Conn, req Request) {
	output, err := appcatalog.New(s.k, s.baseDir).List(context.Background(), appcatalog.ListInput{})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: output})
}

func (s *Server) handleCatalogDiscover(ctx context.Context, conn net.Conn, req Request) {
	endpoint, _, err := argString(req.Args, "endpoint")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	output, err := appcatalog.New(s.k, s.baseDir).Discover(ctx, appcatalog.DiscoverInput{Endpoint: endpoint})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: output})
}

// registerCatalogCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerCatalogCommands() {
	register(
		commandSpec{Cmd: CmdCatalogSync, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogSync(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCatalogList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCatalogDiscover, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogDiscover(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderReload, Handler: func(dc *DispatchCtx) { dc.S.handleProviderReload(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderConnect, Handler: func(dc *DispatchCtx) { dc.S.handleProviderConnect(dc.Conn, dc.Req) }},
	)
}
