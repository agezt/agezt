// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"strings"
)

const (
	RunsDefaultLimit = 20
	RunsMaxLimit     = 100
)

// History resolves the graph before native limit admission and folds that
// canonical subject even if the graph changes before the journal read.
type History struct {
	name    string
	journal Journal
}

func (s *Reads) PrepareRuns(ref string) (History, error) {
	w, found := s.reader.Get(strings.TrimSpace(ref))
	if !found {
		return History{}, UnknownWorkflowError{Ref: ref}
	}
	return History{name: w.Name, journal: s.journal}, nil
}

type RunsInput struct{ Limit int }
type RunsOutput struct {
	Workflow string      `json:"workflow"`
	Runs     []RunRecord `json:"runs"`
	Count    int         `json:"count"`
}
type NodeEvent struct {
	Node     string `json:"node"`
	TSMS     int64  `json:"ts_ms"`
	OK       bool   `json:"ok"`
	Type     string `json:"type,omitempty"`
	Label    string `json:"label,omitempty"`
	Port     string `json:"port,omitempty"`
	Handled  *bool  `json:"handled,omitempty"`
	Error    string `json:"error,omitempty"`
	Input    string `json:"input,omitempty"`
	Output   string `json:"output,omitempty"`
	Attempts int    `json:"attempts,omitempty"`
}
type RunRecord struct {
	CorrelationID     string      `json:"correlation_id"`
	Status            string      `json:"status"`
	StartedMS         int64       `json:"started_ms"`
	NodeEvents        []NodeEvent `json:"node_events"`
	FinishedMS        int64       `json:"finished_ms,omitempty"`
	Executed          []any       `json:"executed,omitempty"`
	Error             string      `json:"error,omitempty"`
	Source            string      `json:"source,omitempty"`
	Runner            string      `json:"runner,omitempty"`
	Agent             string      `json:"agent,omitempty"`
	ScheduleID        string      `json:"schedule_id,omitempty"`
	StandingID        string      `json:"standing_id,omitempty"`
	TriggerSubject    string      `json:"trigger_subject,omitempty"`
	ParentCorrelation string      `json:"parent_correlation_id,omitempty"`
}

func (h History) Runs(_ context.Context, in RunsInput) (RunsOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = RunsDefaultLimit
	}
	if limit > RunsMaxLimit {
		limit = RunsMaxLimit
	}
	subject := "workflow." + h.name
	byCorr := map[string]*RunRecord{}
	var order []string
	get := func(corr string) *RunRecord {
		r := byCorr[corr]
		if r == nil {
			r = &RunRecord{CorrelationID: corr, Status: "running"}
			byCorr[corr] = r
			order = append(order, corr)
		}
		return r
	}
	if err := h.journal.Range(func(e *event.Event) error {
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
			r.StartedMS = e.TSUnixMS
			r.Source = p.Source
			r.Runner = p.Runner
			r.Agent = p.Agent
			r.ScheduleID = p.ScheduleID
			r.StandingID = p.StandingID
			r.TriggerSubject = p.TriggerSubject
			r.ParentCorrelation = p.ParentCorrelation
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
				return nil
			}
			node := NodeEvent{Node: p.Node, TSMS: e.TSUnixMS, OK: p.OK == nil || *p.OK, Type: p.Type, Label: p.Label, Port: p.Port, Handled: p.Handled, Error: p.Error, Input: p.Input, Output: p.Output}
			if p.Attempts > 1 {
				node.Attempts = p.Attempts
			}
			r := get(e.CorrelationID)
			r.NodeEvents = append(r.NodeEvents, node)
		case event.KindWorkflowCompleted, event.KindWorkflowFailed:
			var p struct {
				Executed []any  `json:"executed"`
				Error    string `json:"error"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			r := get(e.CorrelationID)
			r.FinishedMS = e.TSUnixMS
			r.Executed = p.Executed
			if e.Kind == event.KindWorkflowFailed {
				r.Status = "failed"
				r.Error = p.Error
			} else {
				r.Status = "completed"
			}
		}
		return nil
	}); err != nil {
		return RunsOutput{}, fmt.Errorf("journal: %w", err)
	}
	out := make([]RunRecord, 0, limit)
	for i := len(order) - 1; i >= 0 && len(out) < limit; i-- {
		row := *byCorr[order[i]]
		if row.FinishedMS <= 0 {
			row.FinishedMS = 0
		}
		out = append(out, row)
	}
	return RunsOutput{Workflow: h.name, Runs: out, Count: len(out)}, nil
}
