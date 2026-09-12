// SPDX-License-Identifier: MIT

// Research: verifyResearchClaims.
// Code extracted from research.go during the Day-67 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"github.com/agezt/agezt/kernel/agent"
	"sync"
)


// parallel (like councilRound), returning claims with their verdicts filled.
func (k *Kernel) verifyResearchClaims(ctx context.Context, corr, model string, sources []ResearchSource, claims []ResearchClaim) []ResearchClaim {
	if len(claims) == 0 || k.cfg.Provider == nil {
		return nil
	}
	byID := make(map[string]ResearchSource, len(sources))
	for _, s := range sources {
		byID[s.ID] = s
	}
	out := make([]ResearchClaim, len(claims))
	var wg sync.WaitGroup
	for i, c := range claims {
		wg.Add(1)
		go func(i int, c ResearchClaim) {
			defer wg.Done()
			c.Verdict = "uncertain"
			resp, err := k.completeAux(ctx, corr, "research-verify", agent.CompletionRequest{
				Model:     model,
				MaxTokens: researchVerifyMaxTokens,
				System:    researchVerifySystem,
				Messages:  []agent.Message{{Role: agent.RoleUser, Content: buildResearchVerifyPrompt(c, byID)}},
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

// --- pure helpers (unit-tested without a Kernel) ---