// SPDX-License-Identifier: MIT

package reasoning

// research_verify.go owns the adversarial verification pass (Faz 2): each cited
// claim is put through a skeptical, refute-first check against the text of the
// source(s) it cites.

import (
	"context"
	"sync"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// verifyResearchClaims runs the adversarial verifier over each claim in
// parallel (like councilRound), returning claims with their verdicts filled.
func (s *Service) verifyResearchClaims(ctx context.Context, corr, model string, sources []ResearchSource, claims []ResearchClaim) []ResearchClaim {
	if len(claims) == 0 || s.deps.HasProvider == nil || !s.deps.HasProvider() {
		return nil
	}
	byID := make(map[string]ResearchSource, len(sources))
	for _, src := range sources {
		byID[src.ID] = src
	}
	out := make([]ResearchClaim, len(claims))
	var wg sync.WaitGroup
	for i, c := range claims {
		wg.Add(1)
		go func(i int, c ResearchClaim) {
			defer wg.Done()
			c.Verdict = "uncertain"
			resp, err := s.complete(ctx, corr, "research-verify", llm.CompletionRequest{
				Model:     model,
				MaxTokens: researchVerifyMaxTokens,
				System:    researchVerifySystem,
				Messages:  []llm.Message{{Role: llm.RoleUser, Content: buildResearchVerifyPrompt(c, byID)}},
			})
			if err != nil {
				c.Note = "verifier error: " + err.Error()
			} else {
				verdict, note := parseResearchVerdict(resp.Message.Content)
				c.Verdict = verdict
				c.Note = note
			}
			out[i] = c
		}(i, c)
	}
	wg.Wait()
	return out
}
