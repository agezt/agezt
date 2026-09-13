// SPDX-License-Identifier: MIT

// Workflow runs list handler: handleWorkflowRuns. Carved out of
// workflow_handlers_run.go during the Day 190 god-file split so the
// main file can stay focused on the small lifecycle handlers
// (Templates/Draft/Refine/Run).
// Public API unchanged.
package controlplane


import (
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

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

