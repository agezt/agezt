// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/workflow"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func (s *Server) handleWorkflowTemplates(conn net.Conn, req Request) {
	all := workflow.Templates()
	out := make([]any, 0, len(all))
	for _, t := range all {
		out = append(out, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"category":    t.Category,
			"node_count":  len(t.Workflow.Nodes),
			"workflow":    workflowView(t.Workflow, true),
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"templates": out, "count": len(out)},
	})
}

// workflowDraftTimeout bounds one copilot draft — up to two provider
// round-trips (the draft and one repair).
const workflowDraftTimeout = 3 * time.Minute

// workflowRunsDefaultLimit / Max bound the run-history fold (M806).
const (
	workflowRunsDefaultLimit = 20
	workflowRunsMaxLimit     = 100
)

// handleWorkflowRuns (M806) folds the journal into a workflow's run
// history: every started→node…→completed|failed arc under subject
// workflow.<name>, grouped by correlation, newest first. This is what the
// console's Runs drawer replays on the canvas — the journal is the truth,
// nothing new is stored.
func (s *Server) handleWorkflowDraft(conn net.Conn, req Request) {
	desc, err := requiredArgString(req.Args, "description")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	name, _, err := argString(req.Args, "name")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), workflowDraftTimeout)
	defer cancel()
	w, err := s.k.DraftWorkflow(ctx, corr, name, desc)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// The draft is NOT saved — the caller (canvas, CLI) reviews and saves.
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(w, true), "correlation_id": corr},
	})
}

// handleWorkflowRefine (M805): revise an existing graph from a plain-language
// instruction. The base is the POSTED graph when args.workflow is present
// (the canvas's truth, unsaved edits included), else the STORED one at
// args.ref (the CLI's path). The revision returns UNSAVED, like a draft.
func (s *Server) handleWorkflowRefine(conn net.Conn, req Request) {
	instruction, err := requiredArgString(req.Args, "instruction")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var base workflow.Workflow
	if raw, ok := req.Args["workflow"]; ok && raw != nil {
		b, err := json.Marshal(raw)
		if err == nil {
			err = json.Unmarshal(b, &base)
		}
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow: " + err.Error()})
			return
		}
	} else {
		ref, _, err := argString(req.Args, "ref")
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		if strings.TrimSpace(ref) == "" {
			s.failMsg(conn, req, "args.workflow or args.ref required")
			return
		}
		w, found := s.k.Workflows().Get(strings.TrimSpace(ref))
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + ref})
			return
		}
		base = w
	}
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), workflowDraftTimeout)
	defer cancel()
	w, err := s.k.RefineWorkflow(ctx, corr, base, instruction)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(w, true), "correlation_id": corr},
	})
}

func (s *Server) handleWorkflowRun(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// payload may be any JSON value (object on the canvas, a string from the
	// CLI's --payload) — passed verbatim into {{trigger.payload}}.
	payload := req.Args["payload"]
	corr := s.k.NewCorrelation()

	// Async mode (M810): start the run and return immediately — the canvas
	// follows it live on the SSE arc, long runs stop being hostage to wire
	// timeouts (the webui's JSON proxy caps a held connection at 120s while
	// the engine legitimately allows 15m). The ref is resolved BEFORE
	// detaching so a typo is still an honest, synchronous error.
	async := false
	if b, ok, _ := argBool(req.Args, "async"); ok {
		async = b
	} else if s, ok, _ := argString(req.Args, "async"); ok {
		async = strings.EqualFold(s, "true") || s == "1"
	}
	if async {
		w, found := s.k.Workflows().Get(strings.TrimSpace(ref))
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workflow: " + ref})
			return
		}
		go s.runWorkflowDetached(kernelruntime.WakeContext{Source: "manual"}, corr, w.Name, payload)
		s.writeResp(conn, Response{
			ID:     req.ID,
			Type:   RespResult,
			Result: map[string]any{"accepted": true, "async": true, "correlation_id": corr, "workflow": w.Name},
		})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), workflowRunTimeout)
	defer cancel()
	ctx = kernelruntime.WithWakeContext(ctx, kernelruntime.WakeContext{Source: "manual"})
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

// registerWorkflowCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
