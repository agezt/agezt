// SPDX-License-Identifier: MIT

package controlplane

// MCP self-install handlers (M796) — the management path behind `agt mcp`
// and the console. Lifecycle changes go through the kernel so every
// add/attach/detach/enable/remove is journaled (mcp.*) and auditable via
// `agt why`. Servers are addressed by ref = id OR name everywhere.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/mcp"
)

// mcpServerView is the stable wire shape for one registration, joined with
// its live attachment status.
func (s *Server) mcpServerView(srv mcp.Server) map[string]any {
	b, _ := json.Marshal(srv)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	attached := s.k.MCPAttached()
	if n, live := attached[srv.Name]; live {
		m["attached"] = true
		m["tool_count"] = n
	} else {
		m["attached"] = false
	}
	return m
}

func (s *Server) handleMCPList(conn net.Conn, req Request) {
	servers := s.k.MCPStore().List()
	attached := s.k.MCPAttached()
	out := make([]any, 0, len(servers))
	for _, srv := range servers {
		out = append(out, s.mcpServerView(srv))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"servers": out, "count": len(out), "attached_count": len(attached)},
	})
}

func (s *Server) handleMCPAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["server"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.server required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.server: " + err.Error()})
		return
	}
	var srv mcp.Server
	if err := json.Unmarshal(b, &srv); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.server: " + err.Error()})
		return
	}
	saved, err := s.k.AddMCPServer("", srv)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"server": s.mcpServerView(saved)}})
}

func (s *Server) handleMCPAttach(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	srv, tools, err := s.k.AttachMCPServer(context.Background(), "", ref)
	if err != nil {
		if errors.Is(err, mcp.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown mcp server: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"server": s.mcpServerView(srv), "tools": tools},
	})
}

func (s *Server) handleMCPDetach(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	if err := s.k.DetachMCPServer("", ref); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"detached": true}})
}

func (s *Server) handleMCPSetEnabled(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	srv, err := s.k.SetMCPServerEnabled("", ref, enabled)
	if err != nil {
		if errors.Is(err, mcp.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown mcp server: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"server": s.mcpServerView(srv)}})
}

func (s *Server) handleMCPRemove(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	ok, err := s.k.RemoveMCPServer("", ref)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": ok}})
}
