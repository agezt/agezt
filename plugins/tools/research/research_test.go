// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

type fakeRunner struct {
	report      runtime.ResearchReport
	err         error
	gotQuestion string
	gotOpts     runtime.ResearchOptions
	called      bool
}

func (f *fakeRunner) Research(_ context.Context, _, question string, opts runtime.ResearchOptions) (runtime.ResearchReport, error) {
	f.called = true
	f.gotQuestion = question
	f.gotOpts = opts
	return f.report, f.err
}

func TestResearchTool_Definition(t *testing.T) {
	d := New().Definition()
	if d.Name != "research" {
		t.Fatalf("name = %q", d.Name)
	}
	if !json.Valid(d.InputSchema) {
		t.Fatalf("input schema is not valid JSON")
	}
}

func TestResearchTool_HappyPath(t *testing.T) {
	fr := &fakeRunner{report: runtime.ResearchReport{
		Question:     "why is the sky blue?",
		SubQuestions: []string{"why is the sky blue?"},
		Sources: []runtime.ResearchSource{
			{ID: "S1", URL: "https://a.example", Title: "Rayleigh", Text: "big body text stays internal", Rank: 1},
		},
		Markdown:     "The sky is blue due to Rayleigh scattering [S1]. Confidence: high.",
		Confidence:   1,
		CitedSources: 1,
	}}
	tool := New()
	tool.SetRunner(fr)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"why is the sky blue?","max_sources":5}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %q", res.Output)
	}
	if res.ObservationTrust != "untrusted" {
		t.Fatalf("report must be untrusted, got %q", res.ObservationTrust)
	}
	if !fr.called || fr.gotQuestion != "why is the sky blue?" {
		t.Fatalf("runner not called correctly: %+v", fr)
	}
	if fr.gotOpts.MaxSources != 5 {
		t.Fatalf("max_sources not forwarded: %+v", fr.gotOpts)
	}
	// Output is JSON carrying the markdown + confidence, but NOT the full source text.
	if !strings.Contains(res.Output, "Rayleigh scattering [S1]") {
		t.Fatalf("markdown missing from output: %s", res.Output)
	}
	if strings.Contains(res.Output, "big body text stays internal") {
		t.Fatalf("full source text must not leak into tool output")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if out["confidence"].(float64) != 1 {
		t.Fatalf("confidence not surfaced: %v", out["confidence"])
	}
}

func TestResearchTool_EmptyQuestion(t *testing.T) {
	tool := New()
	tool.SetRunner(&fakeRunner{})
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"  "}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "question required") {
		t.Fatalf("expected question-required error, got %q", res.Output)
	}
}

func TestResearchTool_RunnerUnavailable(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"question":"x"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("expected unavailable, got %q", res.Output)
	}
}

func TestResearchTool_RunnerError(t *testing.T) {
	tool := New()
	tool.SetRunner(&fakeRunner{err: errors.New("provider down")})
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"x"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "provider down") {
		t.Fatalf("expected runner error surfaced, got %q", res.Output)
	}
}

func TestResearchTool_BadInput(t *testing.T) {
	tool := New()
	tool.SetRunner(&fakeRunner{})
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":`)); err == nil {
		t.Fatalf("expected parse error for malformed JSON")
	}
}
