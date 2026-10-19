// SPDX-License-Identifier: MIT

package research

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
)

// researchEngines is a scripted harness.
type researchEngines struct {
	calls  []runtime.ResearchOptions
	asked  []string
	report runtime.ResearchReport
	err    error
}

func (e *researchEngines) engines() Engines {
	return Engines{
		Research: func(_ context.Context, corr, question string, opts runtime.ResearchOptions) (runtime.ResearchReport, error) {
			e.asked = append(e.asked, corr+"|"+question)
			e.calls = append(e.calls, opts)
			return e.report, e.err
		},
		NewCorrelation: func() string { return "minted-correlation" },
	}
}

// TestAskRejectsAMissingQuestion: the empty question fails before the harness
// runs, with the legacy message.
func TestAskRejectsAMissingQuestion(t *testing.T) {
	e := &researchEngines{}
	for _, raw := range []string{``, `null`, `""`, `"   "`, `7`, `{}`} {
		out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(raw)})
		if err == nil || err.Error() != "args.question required" {
			t.Fatalf("%s: %v, want args.question required", raw, err)
		}
		if !reflect.DeepEqual(out, AskOutput{}) {
			t.Fatalf("%s: a refused question answers nothing: %+v", raw, out)
		}
	}
	if len(e.asked) != 0 {
		t.Fatal("a refused question never reaches the harness", e.asked)
	}
}

// TestAskMapsTheReport: the report becomes the typed answer, the sources carry
// no text, and the empty lists stay empty lists.
func TestAskMapsTheReport(t *testing.T) {
	e := &researchEngines{report: runtime.ResearchReport{
		Question:     "why?",
		SubQuestions: []string{"why?", "how?"},
		Sources: []runtime.ResearchSource{
			{ID: "S1", URL: "https://example.com", Title: "Example", Text: "the body", Rank: 1, Hash: "h1"},
		},
		Markdown:     "# Report",
		Claims:       []runtime.ResearchClaim{{Text: "a claim", SourceIDs: []string{"S1"}, Verdict: "supported", Note: ""}},
		Confidence:   0.75,
		CitedSources: 1,
		Verified:     true,
		Notes:        []string{"a note"},
	}}
	out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`), Corr: json.RawMessage(`"client-corr"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "client-corr" || out.Question != "why?" || out.Markdown != "# Report" || out.Confidence != 0.75 || out.CitedSources != 1 || !out.Verified {
		t.Fatalf("output lost a field: %+v", out)
	}
	if len(out.SubQuestions) != 2 || len(out.Notes) != 1 {
		t.Fatalf("sub questions and notes: %+v", out)
	}
	if len(out.Sources) != 1 || out.Sources[0] != (Source{ID: "S1", URL: "https://example.com", Title: "Example", Rank: 1, Hash: "h1"}) {
		t.Fatalf("sources: %+v", out.Sources)
	}
	if len(out.Claims) != 1 || out.Claims[0].Text != "a claim" || out.Claims[0].Verdict != "supported" || len(out.Claims[0].SourceIDs) != 1 {
		t.Fatalf("claims: %+v", out.Claims)
	}
	// The claim's note is carried even when it is empty, and the source's text
	// is not: the report cites the source, the harness keeps the body.
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"note":""`) {
		t.Fatalf("an empty note is still a note: %s", raw)
	}
	if strings.Contains(string(raw), `"text":"the body"`) {
		t.Fatalf("the source body stays in the harness: %s", raw)
	}
}

// TestAskKeepsEmptyListsEmpty: an empty report answers with empty lists, not
// nulls, so the Web UI can iterate them.
func TestAskKeepsEmptyListsEmpty(t *testing.T) {
	e := &researchEngines{report: runtime.ResearchReport{Question: "why?"}}
	out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"why?"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Sources == nil || out.Claims == nil {
		t.Fatalf("empty lists stay lists: %+v", out)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"sources":[]`) || !strings.Contains(string(raw), `"claims":[]`) {
		t.Fatalf("empty lists: %s", raw)
	}
}

// TestAskReadsTheLenientArguments: the caps accept a number or a digit
// string, and the verification flag defaults to on.
func TestAskReadsTheLenientArguments(t *testing.T) {
	e := &researchEngines{report: runtime.ResearchReport{Question: "q"}}
	if _, err := e.engines().Ask(context.Background(), AskRequest{
		Question:        json.RawMessage(`"q"`),
		MaxSubQuestions: json.RawMessage(`"4"`),
		MaxSources:      json.RawMessage(`12`),
		MaxVerifyClaims: json.RawMessage(`7`),
	}); err != nil {
		t.Fatal(err)
	}
	opts := e.calls[0]
	if opts.MaxSubQuestions != 4 || opts.MaxSources != 12 || opts.MaxVerifyClaims != 7 {
		t.Fatalf("the caps: %+v", opts)
	}
	if !opts.Verify {
		t.Fatal("the verification pass is on unless it is explicitly off")
	}
	// An argument that is not a number and not a digit string reads as zero,
	// and the harness applies its own default.
	e = &researchEngines{report: runtime.ResearchReport{Question: "q"}}
	if _, err := e.engines().Ask(context.Background(), AskRequest{
		Question:        json.RawMessage(`"q"`),
		MaxSubQuestions: json.RawMessage(`"nope"`),
	}); err != nil {
		t.Fatal(err)
	}
	if e.calls[0].MaxSubQuestions != 0 {
		t.Fatalf("an odd cap reads as zero: %+v", e.calls[0])
	}
}

// TestAskReadsTheVerifyFlag: only an explicit boolean turns the verification
// pass off, as before.
func TestAskReadsTheVerifyFlag(t *testing.T) {
	for raw, want := range map[string]bool{
		`false`:   false,
		`true`:    true,
		`"false"`: true,
		`"no"`:    true,
		`7`:       true,
		`null`:    true,
	} {
		e := &researchEngines{report: runtime.ResearchReport{Question: "q"}}
		if _, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`), Verify: json.RawMessage(raw)}); err != nil {
			t.Fatal(err)
		}
		if e.calls[0].Verify != want {
			t.Fatalf("verify %s: %v, want %v", raw, e.calls[0].Verify, want)
		}
	}
}

// TestAskKeepsTheClientCorrelation: a plain id is kept, an odd one is minted.
func TestAskKeepsTheClientCorrelation(t *testing.T) {
	e := &researchEngines{report: runtime.ResearchReport{Question: "q"}}
	out, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`), Corr: json.RawMessage(`"client-corr"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "client-corr" || e.asked[0] != "client-corr|q" {
		t.Fatalf("the client correlation is kept: %+v", out)
	}
	e = &researchEngines{report: runtime.ResearchReport{Question: "q"}}
	out, err = e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`), Corr: json.RawMessage(`"not a plain id"`)})
	if err != nil {
		t.Fatal(err)
	}
	if out.CorrelationID != "minted-correlation" {
		t.Fatalf("an odd correlation id is minted: %q", out.CorrelationID)
	}
}

// TestAskReportsTheHarnessFailure: a harness failure is the operation's
// failure.
func TestAskReportsTheHarnessFailure(t *testing.T) {
	e := &researchEngines{err: errors.New("research: no provider configured")}
	if _, err := e.engines().Ask(context.Background(), AskRequest{Question: json.RawMessage(`"q"`)}); err == nil || err.Error() != "research: no provider configured" {
		t.Fatal(err)
	}
}

// TestAskOperations: the harness is a primary-only, live and audited
// operation on its own route.
func TestAskOperations(t *testing.T) {
	if _, err := AskOperations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := AskOperations(func(context.Context) Engines { return Engines{} })
	if err != nil || len(ops) != 1 {
		t.Fatal(len(ops), err)
	}
	s := ops[0].Spec()
	out, err := schema.FromType(reflect.TypeFor[AskOutput](), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "research_ask" || s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamLive || !s.AllowUnknownInput || s.HTTP.Method != "POST" || s.HTTP.Path != "/api/research/ask" || s.Output != reflect.TypeFor[AskOutput]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("research_ask spec: %+v", s)
	}
	if len(s.EmissionSchema) == 0 {
		t.Fatal("research_ask declares no event frames")
	}
	for _, in := range []string{`{}`, `{"question":"q","max_sources":3,"verify":false}`} {
		if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
			t.Fatal(in, err)
		}
	}
}

// TestTheSharedReadersLiveInTheAppLayer: the panel, the Conductor and the
// harness share one set of lenient readers.
func TestTheSharedReadersLiveInTheAppLayer(t *testing.T) {
	if app.LenientText(json.RawMessage(`"  padded  "`)) != "padded" {
		t.Fatal("the text reader trims")
	}
	if !app.LenientBool(json.RawMessage(`"1"`)) || app.LenientBool(json.RawMessage(`"no"`)) {
		t.Fatal("the flag reader")
	}
	if app.LenientCount(json.RawMessage(`"12"`)) != 12 || app.LenientCount(json.RawMessage(`7`)) != 7 || app.LenientCount(json.RawMessage(`"x"`)) != 0 {
		t.Fatal("the count reader")
	}
}
