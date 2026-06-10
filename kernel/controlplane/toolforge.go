// SPDX-License-Identifier: MIT

package controlplane

// Script-tool forge handlers (M794) — the management path behind
// `agt toolforge` and the console. Lifecycle changes go through the kernel
// so every draft/edit/test/promote/quarantine/remove is journaled
// (scripttool.*) and auditable via `agt why`. Tools are addressed by
// ref = id OR name everywhere. Promotion lives HERE — an operator surface,
// not a tool op — so an agent can author and test but never self-promote.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/toolforge"
)

// scriptToolView is the stable wire shape for one script tool. The code body
// rides along only when full is set (list stays light; show carries it).
func scriptToolView(st toolforge.ScriptTool, full bool) map[string]any {
	b, _ := json.Marshal(st)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if !full {
		delete(m, "code")
	}
	if st.Status == toolforge.StatusActive {
		m["callable_as"] = "forge_" + st.Name
	}
	return m
}

func (s *Server) handleToolforgeList(conn net.Conn, req Request) {
	tools := s.k.ToolForge().List()
	out := make([]any, 0, len(tools))
	active := 0
	for _, st := range tools {
		out = append(out, scriptToolView(st, false))
		if st.Status == toolforge.StatusActive {
			active++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"tools": out, "count": len(out), "active_count": active},
	})
}

func (s *Server) handleToolforgeShow(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	st, found := s.k.ToolForge().Get(strings.TrimSpace(ref))
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown script tool: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tool": scriptToolView(st, true)}})
}

func (s *Server) handleToolforgeDraft(conn net.Conn, req Request) {
	raw, ok := req.Args["tool"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool: " + err.Error()})
		return
	}
	var st toolforge.ScriptTool
	if err := json.Unmarshal(b, &st); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool: " + err.Error()})
		return
	}
	saved, err := s.k.DraftScriptTool("", st)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tool": scriptToolView(saved, false)}})
}

// handleToolforgeEdit applies args.tool's MUTABLE fields to the tool named by
// args.ref: non-empty description/language/input_schema and code replace the
// stored values (identity/lifecycle fields are protected by the store, and a
// code/language change demotes the tool to draft with its test record
// cleared).
func (s *Server) handleToolforgeEdit(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	raw, ok := req.Args["tool"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool: " + err.Error()})
		return
	}
	var in toolforge.ScriptTool
	if err := json.Unmarshal(b, &in); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.tool: " + err.Error()})
		return
	}
	st, found, err := s.k.UpdateScriptTool("", ref, func(dst *toolforge.ScriptTool) {
		if strings.TrimSpace(in.Description) != "" {
			dst.Description = in.Description
		}
		if strings.TrimSpace(in.Language) != "" {
			dst.Language = in.Language
		}
		if in.Code != "" {
			dst.Code = in.Code
		}
		if strings.TrimSpace(in.InputSchema) != "" {
			dst.InputSchema = in.InputSchema
		}
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown script tool: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tool": scriptToolView(st, false)}})
}

func (s *Server) handleToolforgeTest(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	sample, _ := req.Args["input"].(string)
	st, out, err := s.k.TestScriptTool(context.Background(), "", ref, sample)
	if err != nil {
		if errors.Is(err, toolforge.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown script tool: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"tool": scriptToolView(st, false), "ok": st.TestedOK, "output": out},
	})
}

func (s *Server) handleToolforgePromote(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	st, err := s.k.PromoteScriptTool("", ref)
	if err != nil {
		if errors.Is(err, toolforge.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown script tool: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tool": scriptToolView(st, false)}})
}

func (s *Server) handleToolforgeQuarantine(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	reason, _ := req.Args["reason"].(string)
	st, err := s.k.QuarantineScriptTool("", ref, reason)
	if err != nil {
		if errors.Is(err, toolforge.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown script tool: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tool": scriptToolView(st, false)}})
}

func (s *Server) handleToolforgeRemove(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	ok, err := s.k.RemoveScriptTool("", ref)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": ok}})
}
