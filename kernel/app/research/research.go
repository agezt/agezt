// SPDX-License-Identifier: MIT

// Package research owns the deep-research harness surface (M1001): the
// operator, Web UI and CLI surface for the research harness in kernel/modules/reasoning.
// The operation runs plan -> gather -> synthesize -> adversarial verify and
// returns a cited report. The agent reaches the same engine through the
// `research` tool; the underlying web_search and browser.read calls stay gated
// by their own Edict capabilities inside RunTool.
package research

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Engines runs the research harness under a correlation, and mints one when
// the caller supplies none.
type Engines struct {
	Research       func(ctx context.Context, corr, question string, opts api.ResearchOptions) (api.ResearchReport, error)
	NewCorrelation func() string
}

// AskRequest is the operation input. The arguments keep their raw JSON so the
// lenient readers decide what a slightly wrong shape means.
type AskRequest struct {
	Question        json.RawMessage `json:"question"`
	Corr            json.RawMessage `json:"corr"`
	MaxSubQuestions json.RawMessage `json:"max_sub_questions"`
	MaxSources      json.RawMessage `json:"max_sources"`
	Verify          json.RawMessage `json:"verify"`
	MaxVerifyClaims json.RawMessage `json:"max_verify_claims"`
}

// Source is one gathered source. It carries no text: the report cites the
// source, the harness keeps the text.
type Source struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Rank  int    `json:"rank"`
	Hash  string `json:"hash"`
}

// Claim is one factual claim lifted from the synthesis and put through
// adversarial verification.
type Claim struct {
	Text      string   `json:"text"`
	SourceIDs []string `json:"source_ids"`
	Verdict   string   `json:"verdict"`
	Note      string   `json:"note"`
}

// AskOutput is the cited report.
type AskOutput struct {
	CorrelationID string   `json:"correlation_id"`
	Question      string   `json:"question"`
	SubQuestions  []string `json:"sub_questions"`
	Sources       []Source `json:"sources"`
	Markdown      string   `json:"markdown"`
	Claims        []Claim  `json:"claims"`
	Confidence    float64  `json:"confidence"`
	CitedSources  int      `json:"cited_sources"`
	Verified      bool     `json:"verified"`
	Notes         []string `json:"notes"`
}

// correlation keeps a plain client-supplied id, so the Web UI can subscribe to
// the run's live events before the blocking call returns, or mints one.
func (e Engines) correlation(raw json.RawMessage) string {
	if corr := app.SanitizeCorrelation(app.LenientText(raw)); corr != "" {
		return corr
	}
	return e.NewCorrelation()
}

// Ask runs the harness and answers the cited report.
func (e Engines) Ask(ctx context.Context, in AskRequest) (AskOutput, error) {
	question := app.LenientText(in.Question)
	if question == "" {
		return AskOutput{}, errors.New("args.question required")
	}
	corr := e.correlation(in.Corr)
	// Verify defaults to true; only an explicit false turns it off.
	verify := true
	if v, ok := app.LenientValue(in.Verify).(bool); ok {
		verify = v
	}
	rep, err := e.Research(ctx, corr, question, api.ResearchOptions{
		MaxSubQuestions: app.LenientCount(in.MaxSubQuestions),
		MaxSources:      app.LenientCount(in.MaxSources),
		Verify:          verify,
		MaxVerifyClaims: app.LenientCount(in.MaxVerifyClaims),
	})
	if err != nil {
		return AskOutput{}, err
	}
	sources := make([]Source, 0, len(rep.Sources))
	for _, sc := range rep.Sources {
		sources = append(sources, Source{ID: sc.ID, URL: sc.URL, Title: sc.Title, Rank: sc.Rank, Hash: sc.Hash})
	}
	claims := make([]Claim, 0, len(rep.Claims))
	for _, c := range rep.Claims {
		claims = append(claims, Claim{Text: c.Text, SourceIDs: c.SourceIDs, Verdict: c.Verdict, Note: c.Note})
	}
	return AskOutput{
		CorrelationID: corr,
		Question:      rep.Question,
		SubQuestions:  rep.SubQuestions,
		Sources:       sources,
		Markdown:      rep.Markdown,
		Claims:        claims,
		Confidence:    rep.Confidence,
		CitedSources:  rep.CitedSources,
		Verified:      rep.Verified,
		Notes:         rep.Notes,
	}, nil
}

// AskOperations declares the harness: audited, operator-only and live, so a
// disconnected client cancels the model-heavy run instead of spending it into a
// closed connection. It sends no event frames: the Web UI follows a run on the
// bus under the correlation it chose.
func AskOperations(provider func(context.Context) Engines) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("research engines provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[AskOutput](), false)
	if err != nil {
		return nil, err
	}
	frames := json.RawMessage(event.WireSchema)
	ask, err := app.NewStreamingOperation(opapi.Spec{Name: "research_ask", Stream: opapi.StreamLive, OutputSchema: out, EmissionSchema: frames, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"question":{},"corr":{},"max_sub_questions":{},"max_sources":{},"verify":{},"max_verify_claims":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/research/ask"}}, func(ctx context.Context, in AskRequest, _ func(event.Event) error) (AskOutput, error) {
		return provider(ctx).Ask(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{ask}, nil
}
