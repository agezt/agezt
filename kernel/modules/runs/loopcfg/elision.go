// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
)

// elidedSummaryMaxTokens bounds the abstractive summary call (M398): one short
// line, so a small cap keeps the extra spend negligible and the latency low.
const elidedSummaryMaxTokens = 64

// elidedSummaryReasoningMaxTokens is the cap when the run's model is a
// reasoning model (M926): such models spend output tokens on their chain of
// thought BEFORE the summary line, so the tight cap gets entirely consumed and
// Complete returns empty content — observed live on deepseek-v4-pro at 64
// (every abstractive summary silently degraded to the extractive head stub).
// The prompt still asks for one line; the headroom is only used by models that
// actually reason.
const elidedSummaryReasoningMaxTokens = 1024

// elidedSummaryInputCap bounds how much of a dropped output is fed to the
// summarizer — enough to summarise, while keeping the summarizer's own input
// (and therefore its cost) bounded regardless of how large the output was.
const elidedSummaryInputCap = 8 << 10

// makeElidedSummarizer builds the LoopConfig.SummarizeElided closure: a bounded,
// single-shot provider call that condenses a dropped tool output to one line
// (M398). It routes through the same provider (the Governor) as the run, so the
// extra call is billed and attributed to the run via corr. Errors propagate; the
// loop swallows them and falls back to the deterministic head snippet.
// maxTokens is caller-chosen: tight for plain models, roomy for reasoning
// models whose chain of thought eats the budget first (M926).
func makeElidedSummarizer(provider llm.Provider, model, corr string, maxTokens int) func(context.Context, string) (string, error) {
	return func(ctx context.Context, output string) (string, error) {
		in := output
		if len(in) > elidedSummaryInputCap {
			in = in[:elidedSummaryInputCap]
		}
		resp, err := provider.Complete(ctx, llm.CompletionRequest{
			Model:         model,
			CorrelationID: corr,
			TaskType:      "summarize",
			MaxTokens:     maxTokens,
			Messages: []llm.Message{{
				Role:    llm.RoleUser,
				Content: "Summarize this tool output in one short line for an agent's working memory. Output only the summary.\n\n" + in,
			}},
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}
