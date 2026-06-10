// SPDX-License-Identifier: MIT

package controlplane

// Workflow engine handlers (M798) — the management + run path behind
// `agt workflow` and the console canvas. Lifecycle changes go through the
// kernel so every save/enable/remove and every run arc is journaled
// (workflow.*). Workflows are addressed by ref = id OR name everywhere.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/workflow"
)

// workflowRunTimeout bounds one synchronous run over the wire — generous
// (delay nodes alone may sleep minutes), but never unbounded.
const workflowRunTimeout = 15 * time.Minute

// workflowView is the stable wire shape. The full graph rides only when
// full is set (list stays light; show/save carry it for the canvas).
func workflowView(w workflow.Workflow, full bool) map[string]any {
	b, _ := json.Marshal(w)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["node_count"] = len(w.Nodes)
	m["edge_count"] = len(w.Edges)
	if !full {
		delete(m, "nodes")
		delete(m, "edges")
	}
	return m
}

func (s *Server) handleWorkflowList(conn net.Conn, req Request) {
	items := s.k.Workflows().List()
	out := make([]any, 0, len(items))
	enabled := 0
	for _, w := range items {
		out = append(out, workflowView(w, false))
		if w.Enabled {
			enabled++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflows": out, "count": len(out), "enabled_count": enabled},
	})
}

func (s *Server) handleWorkflowShow(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	w, found := s.k.Workflows().Get(strings.TrimSpace(ref))
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"workflow": workflowView(w, true)}})
}

func (s *Server) handleWorkflowSave(conn net.Conn, req Request) {
	raw, ok := req.Args["workflow"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow: " + err.Error()})
		return
	}
	var w workflow.Workflow
	if err := json.Unmarshal(b, &w); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow: " + err.Error()})
		return
	}
	saved, created, err := s.k.SaveWorkflow("", w)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(saved, true), "created": created},
	})
}

func (s *Server) handleWorkflowRemove(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	ok, err := s.k.RemoveWorkflow("", ref)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": ok}})
}

func (s *Server) handleWorkflowSetEnabled(conn net.Conn, req Request) {
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
	w, err := s.k.SetWorkflowEnabled("", ref, enabled)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"workflow": workflowView(w, false)}})
}

func (s *Server) handleWorkflowRun(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	// payload may be any JSON value (object on the canvas, a string from the
	// CLI's --payload) — passed verbatim into {{trigger.payload}}.
	payload := req.Args["payload"]
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), workflowRunTimeout)
	defer cancel()
	res, err := s.k.RunWorkflow(ctx, corr, ref, payload)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + ref})
			return
		}
		s.writeResp(conn, Response{
			ID: req.ID, Type: RespError,
			Error: err.Error() + " (correlation " + corr + ")",
		})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"correlation_id": corr,
			"executed":       res.Executed,
			"outputs":        res.Outputs,
		},
	})
}
