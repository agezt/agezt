// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"strings"
)

type Reader interface {
	List() []graphs.Workflow
	Get(string) (graphs.Workflow, bool)
}
type Journal interface {
	Range(func(*event.Event) error) error
}
type Reads struct {
	reader    Reader
	journal   Journal
	templates func() []graphs.Template
}

func NewReads(reader Reader, journal Journal, templates func() []graphs.Template) *Reads {
	if templates == nil {
		templates = graphs.Templates
	}
	return &Reads{reader: reader, journal: journal, templates: templates}
}

type Record struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	Description   string   `json:"description,omitempty"`
	CreatedMS     int64    `json:"created_ms"`
	UpdatedMS     int64    `json:"updated_ms"`
	NodeCount     int      `json:"node_count"`
	EdgeCount     int      `json:"edge_count"`
	TriggerKind   string   `json:"trigger_kind"`
	TriggerDetail string   `json:"trigger_detail,omitempty"`
	LastRun       *LastRun `json:"last_run,omitempty"`
}
type FullRecord struct {
	Record
	Nodes []graphs.Node `json:"nodes"`
	Edges []graphs.Edge `json:"edges,omitempty"`
}

func Project(w graphs.Workflow) Record {
	spec := w.TriggerSpec()
	out := Record{ID: w.ID, Name: w.Name, Enabled: w.Enabled, Description: w.Description, CreatedMS: w.CreatedMS, UpdatedMS: w.UpdatedMS, NodeCount: len(w.Nodes), EdgeCount: len(w.Edges), TriggerKind: spec.Kind}
	switch {
	case spec.Kind == "webhook":
		out.TriggerDetail = "POST /hooks/" + w.Name
	case spec.IntervalSec > 0:
		out.TriggerDetail = fmt.Sprintf("every %ds", spec.IntervalSec)
	case spec.DailyAt != "":
		out.TriggerDetail = "daily at " + spec.DailyAt
	case spec.Subject != "":
		out.TriggerDetail = "on " + spec.Subject
	}
	return out
}
func ProjectFull(w graphs.Workflow) FullRecord {
	return FullRecord{Record: Project(w), Nodes: w.Nodes, Edges: w.Edges}
}

// PrepareList reads the store before native flag admission; the journal stays
// opt-in and is folded once for all returned names, including an empty list.
type Listing struct {
	service *Reads
	items   []graphs.Workflow
}

func (s *Reads) PrepareList() Listing { return Listing{service: s, items: s.reader.List()} }

type ListInput struct{ WithRuns bool }
type ListOutput struct {
	Workflows    []Record `json:"workflows"`
	Count        int      `json:"count"`
	EnabledCount int      `json:"enabled_count"`
}

func (l Listing) List(_ context.Context, in ListInput) (ListOutput, error) {
	var latest map[string]LastRun
	if in.WithRuns {
		names := make([]string, 0, len(l.items))
		for _, w := range l.items {
			names = append(names, w.Name)
		}
		latest = l.service.lastRuns(names)
	}
	out := make([]Record, 0, len(l.items))
	enabled := 0
	for _, w := range l.items {
		row := Project(w)
		if run, ok := latest[w.Name]; ok {
			row.LastRun = &run
		}
		out = append(out, row)
		if w.Enabled {
			enabled++
		}
	}
	return ListOutput{Workflows: out, Count: len(out), EnabledCount: enabled}, nil
}

type ShowInput struct{ Ref string }
type ShowOutput struct {
	Workflow FullRecord `json:"workflow"`
}

func (s *Reads) Show(_ context.Context, in ShowInput) (ShowOutput, error) {
	w, found := s.reader.Get(strings.TrimSpace(in.Ref))
	if !found {
		return ShowOutput{}, fmt.Errorf("unknown workflow: %s", in.Ref)
	}
	return ShowOutput{Workflow: ProjectFull(w)}, nil
}

type TemplateRecord struct {
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	NodeCount   int        `json:"node_count"`
	Workflow    FullRecord `json:"workflow"`
}
type TemplatesInput struct{}
type TemplatesOutput struct {
	Templates []TemplateRecord `json:"templates"`
	Count     int              `json:"count"`
}

func (s *Reads) Templates(_ context.Context, _ TemplatesInput) (TemplatesOutput, error) {
	all := s.templates()
	out := make([]TemplateRecord, 0, len(all))
	for _, t := range all {
		out = append(out, TemplateRecord{Name: t.Name, Title: t.Title, Description: t.Description, Category: t.Category, NodeCount: len(t.Workflow.Nodes), Workflow: ProjectFull(t.Workflow)})
	}
	return TemplatesOutput{Templates: out, Count: len(out)}, nil
}
