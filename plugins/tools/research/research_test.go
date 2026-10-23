// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
)

type fakeRunner struct {
	report      reasoningapi.ResearchReport
	err         error
	gotQuestion string
	gotOpts     reasoningapi.ResearchOptions
	called      bool
}

func (f *fakeRunner) Research(_ context.Context, _, question string, opts reasoningapi.ResearchOptions) (reasoningapi.ResearchReport, error) {
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
	fr := &fakeRunner{report: reasoningapi.ResearchReport{
		Question:     "why is the sky blue?",
		SubQuestions: []string{"why is the sky blue?"},
		Sources: []reasoningapi.ResearchSource{
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
	if !fr.gotOpts.Verify {
		t.Fatalf("verify must default on when unset: %+v", fr.gotOpts)
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

// TestResearchTool_SurfacesTheWholeReport pins the output mapping: every field
// of the report the run produced must reach the agent — the claims and their
// verdicts, the verification flag, the cited-source count, the sub-questions,
// the notes and the source rows with their citation ids and ranks. A mapping
// that quietly drops a field leaves the agent with a thinner report than the
// run paid for.
func TestResearchTool_SurfacesTheWholeReport(t *testing.T) {
	fr := &fakeRunner{report: reasoningapi.ResearchReport{
		Question:     "q",
		SubQuestions: []string{"q1"},
		Sources: []reasoningapi.ResearchSource{
			{ID: "S1", URL: "https://a.example", Title: "A", Rank: 2},
		},
		Markdown:     "m",
		Claims:       []reasoningapi.ResearchClaim{{Text: "c1", SourceIDs: []string{"S1"}, Verdict: "supported"}},
		Confidence:   0.75,
		CitedSources: 3,
		Verified:     true,
		Notes:        []string{"n1"},
	}}
	tool := New()
	tool.SetRunner(fr)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %q", res.Output)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	for _, key := range []string{
		"question", "sub_questions", "sources", "markdown", "claims",
		"confidence", "cited_sources", "verified", "notes",
	} {
		if _, ok := out[key]; !ok {
			t.Errorf("output is missing %q", key)
		}
	}
	if out["cited_sources"].(float64) != 3 {
		t.Errorf("cited_sources = %v, want 3", out["cited_sources"])
	}
	if out["verified"] != true {
		t.Errorf("verified = %v, want true", out["verified"])
	}
	if out["confidence"].(float64) != 0.75 {
		t.Errorf("confidence = %v, want 0.75", out["confidence"])
	}
	claims, _ := out["claims"].([]any)
	if len(claims) != 1 {
		t.Fatalf("claims = %v, want the one verified claim", out["claims"])
	}
	claim, _ := claims[0].(map[string]any)
	if claim["text"] != "c1" || claim["verdict"] != "supported" {
		t.Errorf("claim = %v, want the text and its verdict", claim)
	}
	sources, _ := out["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources = %v, want the one gathered source", out["sources"])
	}
	src, _ := sources[0].(map[string]any)
	if src["id"] != "S1" || src["url"] != "https://a.example" || src["rank"].(float64) != 2 {
		t.Errorf("source row = %v, want the citation id, url and rank", src)
	}
}

func TestResearchTool_VerifyCanBeDisabled(t *testing.T) {
	fr := &fakeRunner{}
	tool := New()
	tool.SetRunner(fr)
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"q","verify":false}`)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fr.gotOpts.Verify {
		t.Fatalf("verify:false must be forwarded as off: %+v", fr.gotOpts)
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
