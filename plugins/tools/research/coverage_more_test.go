// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

type covRunner struct {
	gotCorr string
	gotQ    string
	gotOpts runtime.ResearchOptions
	report  runtime.ResearchReport
	err     error
}

func (f *covRunner) Research(_ context.Context, corr, q string, opts runtime.ResearchOptions) (runtime.ResearchReport, error) {
	f.gotCorr = corr
	f.gotQ = q
	f.gotOpts = opts
	return f.report, f.err
}

func TestResearchCoverageDefinitionAndHelpers(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "research" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	if !strings.Contains(def.Description, "deep-research") {
		t.Fatalf("description should mention deep-research, got %q", def.Description)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"question"`, `"max_sub_questions"`, `"max_sources"`, `"verify"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}
}

func TestResearchCoverageInvokeValidation(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	res, err := New().Invoke(context.Background(), json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("unavailable = %+v", res)
	}

	tool := New()
	tool.SetRunner(&covRunner{})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"question":""}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "question required") {
		t.Fatalf("empty question = %+v", res)
	}

	tool = New()
	tool.SetRunner(&covRunner{err: errors.New("boom")})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "boom") {
		t.Fatalf("runner error = %+v", res)
	}
}

func TestResearchCoverageInvokeHappyPath(t *testing.T) {
	fr := &covRunner{report: runtime.ResearchReport{
		Question:     "q",
		SubQuestions: []string{"q1", "q2"},
		Sources:      []runtime.ResearchSource{{ID: "S1", URL: "https://x", Title: "X", Rank: 1}},
		Markdown:     "answer",
		Confidence:   0.75,
		CitedSources: 1,
		Verified:     true,
	}}
	tool := New()
	tool.SetRunner(fr)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"q","max_sub_questions":4,"max_sources":9,"verify":false,"max_verify_claims":7}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if fr.gotQ != "q" || fr.gotOpts.MaxSubQuestions != 4 || fr.gotOpts.MaxSources != 9 || fr.gotOpts.Verify || fr.gotOpts.MaxVerifyClaims != 7 {
		t.Fatalf("runner got %+v %+v", fr.gotQ, fr.gotOpts)
	}
	if res.ObservationTrust != agent.ObservationUntrusted {
		t.Fatalf("ObservationTrust = %v, want %v", res.ObservationTrust, agent.ObservationUntrusted)
	}
	if res.ObservationSource != "research:web" {
		t.Fatalf("ObservationSource = %q", res.ObservationSource)
	}
	for _, want := range []string{"answer", `"sources"`, `"S1"`, `"cited_sources": 1`} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q: %s", want, res.Output)
		}
	}
}
