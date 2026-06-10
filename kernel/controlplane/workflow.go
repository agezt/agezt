// SPDX-License-Identifier: MIT

package controlplane

// Workflow engine handlers (M798) — the management + run path behind
// `agt workflow` and the console canvas. Lifecycle changes go through the
// kernel so every save/enable/remove and every run arc is journaled
// (workflow.*). Workflows are addressed by ref = id OR name everywhere.

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
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
	// Trigger summary (M799) so list rows can say HOW a workflow starts
	// without carrying the whole graph.
	spec := w.TriggerSpec()
	m["trigger_kind"] = spec.Kind
	switch {
	case spec.Kind == "webhook":
		m["trigger_detail"] = "POST /hooks/" + w.Name
	case spec.IntervalSec > 0:
		m["trigger_detail"] = fmt.Sprintf("every %ds", spec.IntervalSec)
	case spec.DailyAt != "":
		m["trigger_detail"] = "daily at " + spec.DailyAt
	case spec.Subject != "":
		m["trigger_detail"] = "on " + spec.Subject
	}
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

// handleWorkflowWebhook (M809) authenticates an external webhook POST and
// fires the workflow ASYNC. The gate is strict and entirely here, the single
// source of truth: the workflow must exist, be ENABLED, declare a webhook
// trigger, and the presented secret must match in constant time. Refusals
// are deliberately uniform ("webhook refused") so a probing caller cannot
// distinguish unknown-name from bad-secret from disabled.
func (s *Server) handleWorkflowWebhook(conn net.Conn, req Request) {
	refuse := func() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "webhook refused"})
	}
	ref, _ := req.Args["ref"].(string)
	secret, _ := req.Args["secret"].(string)
	if strings.TrimSpace(ref) == "" || secret == "" {
		refuse()
		return
	}
	w, found := s.k.Workflows().Get(strings.TrimSpace(ref))
	if !found || !w.Enabled {
		refuse()
		return
	}
	spec := w.TriggerSpec()
	if spec.Kind != "webhook" ||
		subtle.ConstantTimeCompare([]byte(spec.Secret), []byte(secret)) != 1 {
		refuse()
		return
	}
	// Fire-and-return: a webhook caller gets an immediate accept; the run
	// proceeds under its own deadline and the journal carries the arc.
	corr := s.k.NewCorrelation()
	payload := req.Args["payload"]
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), workflowRunTimeout)
		defer cancel()
		_, _ = s.k.RunWorkflow(ctx, corr, w.Name, payload) // failures land in workflow.failed
	}()
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"accepted": true, "correlation_id": corr, "workflow": w.Name},
	})
}

// handleWorkflowTemplates (M807) returns the built-in gallery — curated,
// validated starting points with their full graphs. Read-only; the caller
// instantiates by saving under a new name.
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
			r := get(e.CorrelationID)
			r.started = e.TSUnixMS
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
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.Node == "" {
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
		out = append(out, rv)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": w.Name, "runs": out, "count": len(out)},
	})
}

func (s *Server) handleWorkflowDraft(conn net.Conn, req Request) {
	desc, _ := req.Args["description"].(string)
	if strings.TrimSpace(desc) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.description required"})
		return
	}
	name, _ := req.Args["name"].(string)
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), workflowDraftTimeout)
	defer cancel()
	w, err := s.k.DraftWorkflow(ctx, corr, name, desc)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	instruction, _ := req.Args["instruction"].(string)
	if strings.TrimSpace(instruction) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.instruction required"})
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
		ref, _ := req.Args["ref"].(string)
		if strings.TrimSpace(ref) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow or args.ref required"})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"workflow": workflowView(w, true), "correlation_id": corr},
	})
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
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), workflowRunTimeout)
			defer cancel()
			_, _ = s.k.RunWorkflow(ctx, corr, w.Name, payload) // failures land in workflow.failed
		}()
		s.writeResp(conn, Response{
			ID:     req.ID,
			Type:   RespResult,
			Result: map[string]any{"accepted": true, "async": true, "correlation_id": corr, "workflow": w.Name},
		})
		return
	}
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
