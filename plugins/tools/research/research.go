// SPDX-License-Identifier: MIT

// Package research is the in-process `research` tool: it lets an agent run the
// deep-research harness (M1001) — decompose a question into sub-questions,
// gather independent web sources (web_search + browser.read), and synthesize a
// citation-grounded answer where every claim is attributable to a numbered
// source. The orchestration lives in kernel/runtime; this tool is the thin
// agent-facing front (mirrors plugins/tools/council and plugins/tools/conductor).
//
// Use it for open questions that need current, cross-checked evidence rather
// than one model's recollection. The returned report text is treated as
// UNTRUSTED observation, because it is derived from external web content.
package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

// Runner is the slice of *runtime.Kernel the tool needs — an interface so the
// tool is decoupled and unit-testable with a fake.
type Runner interface {
	Research(ctx context.Context, corr, question string, opts runtime.ResearchOptions) (runtime.ResearchReport, error)
}

// Tool is the `research` implementation of agent.Tool.
type Tool struct {
	runner Runner
}

// New returns an empty Tool; call SetRunner before use.
func New() *Tool { return &Tool{} }

// SetRunner injects the research orchestrator (the kernel), done by the daemon.
func (t *Tool) SetRunner(r Runner) { t.runner = r }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "research",
		Description: "Run the deep-research harness: break a question into sub-questions, gather " +
			"independent web sources (search + fetch), and return a synthesized answer where every claim " +
			"cites a numbered source [S1], [S2], .... Use it for open questions needing current, " +
			"cross-checked evidence — not one model's memory. Returns {markdown, sources, confidence, " +
			"cited_sources, sub_questions}. Costs several searches, fetches, and model calls, so reserve " +
			"it for questions that warrant real research.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["question"],
  "properties": {
    "question":          {"type":"string",  "description":"The question to research."},
    "max_sub_questions": {"type":"integer", "description":"Cap on sub-questions to explore (default 3, max 8)."},
    "max_sources":       {"type":"integer", "description":"Cap on distinct sources to gather (default 8, max 20)."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"Run several web searches, fetch pages, and make model calls to synthesize a cited report.",
			},
			AffectedResources: []string{"model-provider budget", "network (search + page fetch)", "research report"},
			RollbackNotes:     "No external state is changed beyond model/network spend and transcript records; spend cannot be recovered.",
			Confidence:        0.8,
		},
	}
}

type input struct {
	Question        string `json:"question"`
	MaxSubQuestions int    `json:"max_sub_questions,omitempty"`
	MaxSources      int    `json:"max_sources,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("research: parse input: %w", err)
	}
	if t.runner == nil {
		return errResult("research unavailable"), nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return errResult("question required"), nil
	}
	corr := agent.CorrelationFromContext(ctx)
	report, err := t.runner.Research(ctx, corr, in.Question, runtime.ResearchOptions{
		MaxSubQuestions: in.MaxSubQuestions,
		MaxSources:      in.MaxSources,
	})
	if err != nil {
		return errResult(err.Error()), nil
	}
	out, err := json.MarshalIndent(researchOutput(report), "", "  ")
	if err != nil {
		return errResult("encode report: " + err.Error()), nil
	}
	return agent.Result{
		Output: string(out),
		// The report is derived from external web content, so it must be rendered
		// as data, never as an instruction channel back into the loop.
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: "research:web",
	}, nil
}

// researchSourceOut is the per-source shape returned to the model (URL/title/
// hash/rank — not the full fetched text, which stays in the report internals).
type researchSourceOut struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Rank  int    `json:"rank"`
}

type researchOut struct {
	Question     string              `json:"question"`
	SubQuestions []string            `json:"sub_questions"`
	Sources      []researchSourceOut `json:"sources"`
	Markdown     string              `json:"markdown"`
	Confidence   float64             `json:"confidence"`
	CitedSources int                 `json:"cited_sources"`
	Notes        []string            `json:"notes,omitempty"`
}

func researchOutput(r runtime.ResearchReport) researchOut {
	srcs := make([]researchSourceOut, 0, len(r.Sources))
	for _, s := range r.Sources {
		srcs = append(srcs, researchSourceOut{ID: s.ID, URL: s.URL, Title: s.Title, Rank: s.Rank})
	}
	return researchOut{
		Question:     r.Question,
		SubQuestions: r.SubQuestions,
		Sources:      srcs,
		Markdown:     r.Markdown,
		Confidence:   r.Confidence,
		CitedSources: r.CitedSources,
		Notes:        r.Notes,
	}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "research: " + msg, IsError: true}
}
