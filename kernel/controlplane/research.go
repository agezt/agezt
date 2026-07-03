// SPDX-License-Identifier: MIT

package controlplane

// Deep-research control plane (M1001): the operator/Web UI/CLI surface for the
// research harness (kernel/runtime). `research_ask` runs plan -> gather ->
// synthesize -> adversarial verify and returns a cited report. The agent reaches
// the same engine through the `research` tool; the underlying web_search and
// browser.read calls stay gated by their own Edict capabilities inside RunTool.

import (
	"context"
	"net"

	"github.com/agezt/agezt/kernel/runtime"
)

// handleResearchAsk runs the deep-research harness for a question and returns
// the cited report (markdown + sources + verified claims + confidence).
func (s *Server) handleResearchAsk(ctx context.Context, conn net.Conn, req Request) {
	question := stringArg(req.Args, "question")
	if question == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.question required"})
		return
	}
	corr := sanitizeCorr(stringArg(req.Args, "corr"))
	if corr == "" {
		corr = s.k.NewCorrelation()
	}
	// verify defaults to true; only an explicit false turns it off.
	verify := true
	if v, ok := req.Args["verify"]; ok {
		if b, ok2 := v.(bool); ok2 {
			verify = b
		}
	}

	// A disconnected client can't receive the report — cancel the (model-heavy)
	// harness instead of spending it into a closed connection.
	ctx, cancel := cancelOnConnClose(ctx, conn)
	defer cancel()

	rep, err := s.k.Research(ctx, corr, question, runtime.ResearchOptions{
		MaxSubQuestions: dlInt(req.Args, "max_sub_questions"),
		MaxSources:      dlInt(req.Args, "max_sources"),
		Verify:          verify,
		MaxVerifyClaims: dlInt(req.Args, "max_verify_claims"),
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sources := make([]map[string]any, 0, len(rep.Sources))
	for _, sc := range rep.Sources {
		sources = append(sources, map[string]any{
			"id": sc.ID, "url": sc.URL, "title": sc.Title, "rank": sc.Rank, "hash": sc.Hash,
		})
	}
	claims := make([]map[string]any, 0, len(rep.Claims))
	for _, c := range rep.Claims {
		claims = append(claims, map[string]any{
			"text": c.Text, "source_ids": c.SourceIDs, "verdict": c.Verdict, "note": c.Note,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation_id": corr,
		"question":       rep.Question,
		"sub_questions":  rep.SubQuestions,
		"sources":        sources,
		"markdown":       rep.Markdown,
		"claims":         claims,
		"confidence":     rep.Confidence,
		"cited_sources":  rep.CitedSources,
		"verified":       rep.Verified,
		"notes":          rep.Notes,
	}})
}
