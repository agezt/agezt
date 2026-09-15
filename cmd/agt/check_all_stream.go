// SPDX-License-Identifier: MIT
//
// cmd/agt provider check --stream sub-command: probes a provider via its
// streaming path (agent.StreamingProvider), rendering incoming text chunks
// inline. Extracted from check_all.go during Day 211 god-file refactor (#50).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

// streamingUnsupportedMessage explains that a resolved provider's adapter does
// not implement streaming, so the operator can fall back to the plain check.
// Every first-party provider family (anthropic, openai, google, bedrock,
// vertex, cohere, ollama, and openai-compatible vendors) now streams, so this
// is reached only by an adapter that genuinely lacks a streaming path — not the
// stale "only anthropic is wired" state it once described.
func streamingUnsupportedMessage(family string) string {
	return fmt.Sprintf("%s: provider family %q does not implement streaming in this build — re-run `%s provider check` without --stream",
		brand.CLI, family, brand.CLI)
}
// runStreamProbe issues the probe via the provider's streaming path
// (agent.StreamingProvider) and renders incoming text chunks inline.
// Errors out cleanly if the resolved provider doesn't implement
// streaming — that's not a failure of the provider, it's a
// not-yet-wired adapter, and the operator should know so they can
// fall back to the regular check.
func runStreamProbe(entry *catalog.Provider, lookup func(string) string, stdout, stderr io.Writer) int {
	modelID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	if modelID == "" {
		fmt.Fprintf(stderr, "%s: provider %q has no models\n", brand.CLI, entry.ID)
		return 1
	}

	prov, _, err := compat.Build(entry, modelID, lookup)
	if err != nil {
		fmt.Fprintf(stderr, "%s: build %s: %v\n", brand.CLI, entry.ID, err)
		return 1
	}

	sp, ok := prov.(agent.StreamingProvider)
	if !ok {
		fmt.Fprintf(stderr, "%s\n", streamingUnsupportedMessage(string(entry.Family())))
		return 2
	}

	fmt.Fprintf(stdout, "streaming provider=%s model=%s family=%s …\n\n",
		entry.ID, modelID, entry.Family())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := sp.CompleteStream(ctx, agent.CompletionRequest{
		Model:    modelID,
		System:   "Be terse.",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
		// Bump MaxTokens above the regular probe so streaming has
		// room to be visibly progressive on chatty models. Still
		// trivially cheap.
		MaxTokens: 64,
	}, func(c agent.Chunk) error {
		if c.TextDelta != "" {
			fmt.Fprint(stdout, c.TextDelta)
		}
		if c.ToolUseStart != nil {
			fmt.Fprintf(stdout, "\n→ tool_use_start: %s (id=%s)\n", c.ToolUseStart.Name, c.ToolUseStart.ID)
		}
		if c.ToolInputJSONDelta != "" {
			// Tool input JSON is rendered raw so operators can see
			// how Anthropic streams it (one of the more surprising
			// parts of the API).
			fmt.Fprint(stdout, c.ToolInputJSONDelta)
		}
		if c.ToolUseStop != "" {
			fmt.Fprintf(stdout, "\n← tool_use_stop: %s\n", c.ToolUseStop)
		}
		return nil
	})
	latency := time.Since(start)
	fmt.Fprintln(stdout)

	if err != nil {
		fmt.Fprintf(stderr, "\nFAIL: %v (after %s)\n", err, latency.Truncate(time.Millisecond))
		return 1
	}

	model := entry.Models[modelID]
	cost := computeCostMicrocents(model, resp.Usage)
	fmt.Fprintf(stdout, "\nOK\n")
	fmt.Fprintf(stdout, "  total latency   : %s (wall-clock for the full stream)\n", latency.Truncate(time.Millisecond))
	fmt.Fprintf(stdout, "  stop_reason     : %s\n", resp.StopReason)
	fmt.Fprintf(stdout, "  tokens in / out : %d / %d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	if cost > 0 {
		fmt.Fprintf(stdout, "  this call cost  : $%s (%d microcents)\n",
			formatMicrocentsUSD(cost), cost)
	}
	return 0
}
