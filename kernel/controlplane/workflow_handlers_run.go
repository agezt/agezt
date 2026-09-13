// SPDX-License-Identifier: MIT

package controlplane

// Workflow templates + runs + draft + refine + run handlers
// (handleWorkflowTemplates + handleWorkflowRuns + handleWorkflowDraft +
// handleWorkflowRefine + handleWorkflowRun) + workflowDraftTimeout const.
// Carved out of workflow_handlers.go during the Day 166 god-file split
// so the main file can stay focused on CRUD + test/webhook dispatch.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
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
func (s *Server) handleWorkflowRuns(conn net.Conn, req Request) {
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
	limit := workflowRunsDefaultLimit
	switch v := req.Args["limit"].(type) {
	case float64:
		limit = int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = workflowRunsDefaultLimit
	}
	if limit > workflowRunsMaxLimit {
		limit = workflowRunsMaxLimit
	}

	subject := "workflow." + w.Name
	type runFold struct {
		corr     string
		started  int64
		finished int64
		status   string // running|completed|failed
		errText  string
		source   string
		runner   string
		agent    string
		schedule string
		standing string
		trigger  string
		parent   string
		executed []any
		nodes    []map[string]any
	}
	byCorr := map[string]*runFold{}
	var order []string // first-seen order == chronological (journal is append-only)
	get := func(corr string) *runFold {
		r := byCorr[corr]
		if r == nil {
			r = &runFold{corr: corr, status: "running"}
			byCorr[corr] = r
			order = append(order, corr)
		}
		return r
	}
	if err := s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != subject || e.CorrelationID == "" {
			return nil
		}
		switch e.Kind {
		case event.KindWorkflowStarted:
			var p struct {
				Source            string `json:"source"`
				Runner            string `json:"runner"`
				Agent             string `json:"agent"`
				ScheduleID        string `json:"schedule_id"`
				StandingID        string `json:"standing_id"`
				TriggerSubject    string `json:"trigger_subject"`
				ParentCorrelation string `json:"parent_correlation_id"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			r := get(e.CorrelationID)
			r.started = e.TSUnixMS
			r.source = p.Source
			r.runner = p.Runner
			r.agent = p.Agent
			r.schedule = p.ScheduleID
			r.standing = p.StandingID
			r.trigger = p.TriggerSubject
			r.parent = p.ParentCorrelation
		case event.KindWorkflowNode:
			var p struct {
				Node     string `json:"node"`
				Type     string `json:"type"`
				Label    string `json:"label"`
				OK       *bool  `json:"ok"`
				Port     string `json:"port"`
				Handled  *bool  `json:"handled"`
				Error    string `json:"error"`
				Input    string `json:"input"`
				Output   string `json:"output"`
				Attempts int    `json:"attempts"`
				Test     bool   `json:"test"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.Node == "" || p.Test {
				// Single-node tests (M811) are probes, not arcs — they never
				// belong in run history.
				return nil
			}
			nv := map[string]any{"node": p.Node, "ts_ms": e.TSUnixMS}
			nv["ok"] = p.OK == nil || *p.OK
			if p.Type != "" {
				nv["type"] = p.Type
			}
			if p.Label != "" {
				nv["label"] = p.Label
			}
			if p.Port != "" {
				nv["port"] = p.Port
			}
			if p.Handled != nil {
				nv["handled"] = *p.Handled
			}
			if p.Error != "" {
				nv["error"] = p.Error
			}
			// Per-node data snippets (M808): what the node consumed/produced.
			if p.Input != "" {
				nv["input"] = p.Input
			}
			if p.Output != "" {
				nv["output"] = p.Output
			}
			if p.Attempts > 1 {
				nv["attempts"] = p.Attempts
			}
			get(e.CorrelationID).nodes = append(get(e.CorrelationID).nodes, nv)
		case event.KindWorkflowCompleted, event.KindWorkflowFailed:
			var p struct {
				Executed []any  `json:"executed"`
				Error    string `json:"error"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			r := get(e.CorrelationID)
			r.finished = e.TSUnixMS
			r.executed = p.Executed
			if e.Kind == event.KindWorkflowFailed {
				r.status = "failed"
				r.errText = p.Error
			} else {
				r.status = "completed"
			}
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "journal: " + err.Error()})
		return
	}

	// Newest first, capped.
	out := make([]any, 0, limit)
	for i := len(order) - 1; i >= 0 && len(out) < limit; i-- {
		r := byCorr[order[i]]
		rv := map[string]any{
			"correlation_id": r.corr,
			"status":         r.status,
			"started_ms":     r.started,
			"node_events":    r.nodes,
		}
		if r.finished > 0 {
			rv["finished_ms"] = r.finished
		}
		if len(r.executed) > 0 {
			rv["executed"] = r.executed
		}
		if r.errText != "" {
			rv["error"] = r.errText
		}
		if r.source != "" {
			rv["source"] = r.source
		}
		if r.runner != "" {
			rv["runner"] = r.runner
		}
		if r.agent != "" {
			rv["agent"] = r.agent
		}
		if r.schedule != "" {
			rv["schedule_id"] = r.schedule
		}
		if r.standing != "" {
			rv["standing_id"] = r.standing
		}
		if r.trigger != "" {
			rv["trigger_subject"] = r.trigger
		}
		if r.parent != "" {
			rv["parent_correlation_id"] = r.parent
		}
		out = append(out, rv)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": w.Name, "runs": out, "count": len(out)},
	})
}

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
	switch v := req.Args["async"].(type) {
	case bool:
		async = v
	case string:
		async = strings.EqualFold(v, "true") || v == "1"
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