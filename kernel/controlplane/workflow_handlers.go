// SPDX-License-Identifier: MIT

// Package controlplane: Workflow CRUD handlers (handleWorkflowList + Show +
// Save + Restore + Remove + SetEnabled). Test/invoke handlers
// (handleWorkflowTestNode + handleWorkflowWebhook + runWorkflowDetached) moved
// to workflow_handlers_invoke.go. Day-211 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/workflow"
)

func (s *Server) handleWorkflowList(conn net.Conn, req Request) {
	items := s.k.Workflows().List()

	// The journal fold is opt-in: `agt workflow list` wants the cheap answer,
	// the console's list wants to show whether each workflow last succeeded.
	var runs map[string]lastRunSummary
	// argFlag, not argBool: the web console reaches this through the HTTP proxy,
	// where every query arg arrives as a string.
	withRuns, _, err := argFlag(req.Args, "with_runs")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if withRuns {
		names := make([]string, 0, len(items))
		for _, w := range items {
			names = append(names, w.Name)
		}
		runs = s.workflowLastRuns(names)
	}

	out := make([]any, 0, len(items))
	enabled := 0
	for _, w := range items {
		view := workflowView(w, false)
		if r, ok := runs[w.Name]; ok {
			last := map[string]any{"status": r.status, "at_ms": r.atMS}
			if r.durationMS > 0 {
				last["duration_ms"] = r.durationMS
			}
			view["last_run"] = last
		}
		out = append(out, view)
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
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
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
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(saved, true), "created": created},
	})
}

func (s *Server) handleWorkflowRestore(conn net.Conn, req Request) {
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
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	restored, created, err := s.k.RestoreWorkflow("", w, reason)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(restored, true), "created": created},
	})
}

func (s *Server) handleWorkflowRemove(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	ok, err := s.k.RemoveWorkflow("", ref)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": ok}})
}

func (s *Server) handleWorkflowSetEnabled(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
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
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"workflow": workflowView(w, false)}})
}
