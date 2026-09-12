// SPDX-License-Identifier: MIT

// Research: Research() (main entry point).
// Code extracted from research.go during the Day-67 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
	"strings"
)


// error only when there is no provider or the synthesis model call fails.
func (k *Kernel) Research(ctx context.Context, corr, question string, opts ResearchOptions) (ResearchReport, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return ResearchReport{}, fmt.Errorf("research: question required")
	}
	if k.cfg.Provider == nil {
		return ResearchReport{}, fmt.Errorf("research: no provider configured")
	}
	opts = opts.withDefaults()
	report := ResearchReport{Question: question}

	// 1. PLAN — decompose into sub-questions (falls back to the question itself).
	subqs := []string{question}
	if planResp, err := k.completeAux(ctx, corr, "research", agent.CompletionRequest{
		Model:     opts.Model,
		MaxTokens: researchPlanMaxTokens,
		System:    "You are a research planner. Break the user's question into distinct, specific sub-questions that together cover it. Reply with ONLY a JSON array of strings.",
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: buildResearchPlanPrompt(question, opts.MaxSubQuestions)}},
	}); err == nil {
		subqs = parseSubQuestions(planResp.Message.Content, question, opts.MaxSubQuestions)
	} else {
		report.Notes = append(report.Notes, "planning failed, using the original question only: "+err.Error())
	}
	report.SubQuestions = subqs

	// 2. GATHER — discover URLs via web_search, deduped across sub-questions.
	seenURL := map[string]bool{}
	var hits []researchHit
	call := 0
	for _, sq := range subqs {
		args, _ := json.Marshal(map[string]any{"query": sq, "limit": opts.ResultsPerQuery})
		call++
		res, err := k.RunTool(ctx, corr, fmt.Sprintf("research-search-%d", call), "web_search", args)
		if err != nil || res.IsError {
			continue
		}
		for _, h := range parseSearchHits(res.Output) {
			if seenURL[h.URL] {
				continue
			}
			seenURL[h.URL] = true
			hits = append(hits, h)
		}
	}
	if len(hits) > opts.MaxSources {
		hits = hits[:opts.MaxSources]
	}

	// 2b. FETCH — read each page; fall back to the search snippet if the fetch
	// returns nothing (blocked page, JS-only shell), so a source still counts.
	for i, h := range hits {
		args, _ := json.Marshal(map[string]any{"url": h.URL, "max_chars": researchSourceTextMax})
		call++
		text := ""
		if res, err := k.RunTool(ctx, corr, fmt.Sprintf("research-fetch-%d", call), "browser.read", args); err == nil && !res.IsError {
			text = strings.TrimSpace(res.Output)
		}
		if text == "" {
			text = strings.TrimSpace(h.Snippet)
		}
		sum := sha256.Sum256([]byte(text))
		report.Sources = append(report.Sources, ResearchSource{
			ID:    fmt.Sprintf("S%d", i+1),
			URL:   h.URL,
			Title: strings.TrimSpace(h.Title),
			Text:  clip(text, researchSourceTextMax),
			Hash:  hex.EncodeToString(sum[:])[:16],
			Rank:  i + 1,
		})
	}

	if len(report.Sources) == 0 {
		report.Notes = append(report.Notes, "no sources gathered — web_search/browser.read returned nothing or were denied")
		report.Markdown = "No sources could be gathered for this question."
		return report, nil
	}

	// 3. SYNTHESIZE — cited answer grounded only on the numbered sources.
	synthResp, err := k.completeAux(ctx, corr, "research", agent.CompletionRequest{
		Model:     opts.Model,
		MaxTokens: researchSynthMaxTokens,
		System:    researchSynthSystem,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: buildResearchSynthPrompt(question, report.Sources)}},
	})
	if err != nil {
		return report, fmt.Errorf("research: synthesis failed: %w", err)
	}
	report.Markdown = strings.TrimSpace(synthResp.Message.Content)

	// 4. CITATION ENFORCEMENT — confidence tracks distinct sources actually cited.
	cited := extractCitedSources(report.Markdown, len(report.Sources))
	report.CitedSources = len(cited)
	report.Confidence = researchConfidence(len(cited), len(report.Sources))
	if len(cited) == 0 {
		report.Notes = append(report.Notes, "synthesis contained no [S#] citations; treat with low confidence")
	}

	// 5. ADVERSARIAL VERIFICATION (Faz 2) — put each cited claim through a
	// skeptical refute-first check against its own source text. This is the
	// Conductor's Verifier role applied to research: a claim survives only when
	// the source plainly supports it. When verification runs, confidence is the
	// supported fraction of verified claims (a stronger signal than mere citation
	// coverage), and any refuted claim is surfaced as a note.
	if opts.Verify {
		claims := extractClaims(report.Markdown, len(report.Sources))
		if len(claims) > opts.MaxVerifyClaims {
			claims = claims[:opts.MaxVerifyClaims]
		}
		report.Claims = k.verifyResearchClaims(ctx, corr, opts.Model, report.Sources, claims)
		report.Verified = len(report.Claims) > 0
		if report.Verified {
			supported := 0
			var refuted []ResearchClaim
			for _, c := range report.Claims {
				switch c.Verdict {
				case "supported":
					supported++
				case "refuted":
					refuted = append(refuted, c)
				}
			}
			report.Confidence = researchConfidence(supported, len(report.Claims))
			if len(refuted) > 0 {
				report.Notes = append(report.Notes, fmt.Sprintf("%d of %d verified claim(s) were REFUTED under adversarial check", len(refuted), len(report.Claims)))
				// Surface the refutation INSIDE the answer itself — a reader of
				// the synthesis block must see that a stated claim did not hold
				// up, not only a reader of the separate claims list.
				report.Markdown += buildRefutedWarning(refuted)
			}
		}
	}
	return report, nil
}

// verifyResearchClaims runs the adversarial verifier over each claim in