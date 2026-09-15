// SPDX-License-Identifier: MIT
//
// cmd/agt provider check all + probe + stream probe sub-commands.
// Split from check.go during Day 211 god-file refactor (#39).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
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


// runCheckCapsAll renders a capability matrix: one row per supported
// catalog provider (its selected model), network-free and credential-free,
// so an operator can compare models by capability at a glance. Always exit
// 0 — it's a survey, not a gate (the single-provider --caps gates with
// exit 3 for CI). $AGEZT_MODEL, when set, selects that model for every
// provider that serves it; otherwise each provider's first model is shown.
func runCheckCapsAll(cat *catalog.Catalog, flags checkFlags, stdout io.Writer) int {
	modelOverride := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	var rows []jsonCaps
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			continue
		}
		modelID := modelOverride
		if _, ok := entry.Models[modelID]; !ok {
			modelID = compat.FirstModelID(entry)
		}
		model := entry.Models[modelID]
		if model == nil {
			continue // provider with no models — nothing to report
		}
		rows = append(rows, jsonCaps{
			Provider:       entry.ID,
			Family:         string(entry.Family()),
			Model:          modelID,
			ToolCall:       model.ToolCall,
			Reasoning:      model.Reasoning,
			Vision:         model.SupportsVision(),
			Attachment:     model.Attachment,
			JSONMode:       catalog.FamilySupportsNativeJSONMode(entry.Family()),
			StrictToolArgs: model.SupportsStrictToolArgs(),
			PromptCache:    model.SupportsPromptCache(),
			ContextLimit:   model.Limit.Context,
			Warnings:       model.AgentWarnings(),
		})
	}

	if flags.jsonOut {
		buf, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(stdout, string(buf))
		return 0
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no supported providers in the catalog")
		return 0
	}
	fmt.Fprintln(stdout, renderCapsTable(rows))
	ready := 0
	for _, r := range rows {
		if len(r.Warnings) == 0 {
			ready++
		}
	}
	fmt.Fprintf(stdout, "\n%d providers, %d agent-ready (advertise tool-use)\n", len(rows), ready)
	return 0
}

// renderCapsTable lays out the capability matrix. A leading ✓/⚠ marks
// agent-readiness (tool-use) so the eye lands on the ready ones first.
func renderCapsTable(rows []jsonCaps) string {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "-"
	}
	headers := []string{"", "PROVIDER", "MODEL", "TOOLS", "VISION", "REASON", "CONTEXT"}
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		mark := "✓"
		if len(r.Warnings) > 0 {
			mark = "⚠"
		}
		ctx := "-"
		if r.ContextLimit > 0 {
			ctx = fmt.Sprintf("%d", r.ContextLimit)
		}
		cells = append(cells, []string{
			mark, r.Provider, r.Model, yn(r.ToolCall), yn(r.Vision), yn(r.Reasoning), ctx,
		})
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, c := range cells {
		for i, v := range c {
			// Width by rune count so the ✓/⚠ column doesn't over-pad.
			if n := len([]rune(v)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var b strings.Builder
	writeRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				b.WriteString("  ")
			}
			pad := widths[i] - len([]rune(v))
			b.WriteString(v)
			if pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteByte('\n')
	}
	writeRow(headers)
	for _, c := range cells {
		writeRow(c)
	}
	return strings.TrimRight(b.String(), "\n")
}

// runCheckAll iterates credentialed providers. Each provider runs
// either a single probe or a benchmark, depending on flags.bench.
func runCheckAll(cat *catalog.Catalog, lookup func(string) string, flags checkFlags, stdout, stderr io.Writer) int {
	var (
		rows    []checkRow
		benches []benchResult
		jsonOut []jsonProbe
		skipped int
	)
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			skipped++
			continue
		}
		if !entry.HasCredentials(lookup) {
			skipped++
			continue
		}
		if !flags.jsonOut {
			fmt.Fprintf(stdout, "checking %-20s …\n", entry.ID)
		}
		if flags.bench >= 2 {
			b := runBench(entry, lookup, flags.bench, stdout, false)
			benches = append(benches, b)
			rows = append(rows, b.toCheckRow())
			if flags.jsonOut {
				jsonOut = append(jsonOut, b.toJSON())
			}
		} else {
			res := runProbe(entry, lookup)
			r := checkRow{
				id:      entry.ID,
				family:  string(entry.Family()),
				model:   res.modelID,
				latency: res.latency,
				cost:    res.costMicrocents,
				ok:      res.err == nil,
			}
			if res.err != nil {
				r.err = truncate(res.err.Error(), 60)
			}
			rows = append(rows, r)
			if flags.jsonOut {
				jsonOut = append(jsonOut, probeToJSON(entry, res))
			}
		}
	}

	if len(rows) == 0 {
		if flags.jsonOut {
			return emitJSON([]jsonProbe{}, jsonSummary{Skipped: skipped}, stdout)
		}
		fmt.Fprintf(stderr, "%s: no catalog provider has credentials + a wired family — set creds via `agt provider creds set`\n", brand.CLI)
		return 1
	}

	pass, fail := 0, 0
	for _, r := range rows {
		if r.ok {
			pass++
		} else {
			fail++
		}
	}

	if flags.jsonOut {
		sum := jsonSummary{Total: len(rows), OK: pass, Failed: fail, Skipped: skipped}
		exit := 0
		if fail > 0 {
			exit = 1
		}
		if rc := emitJSON(jsonOut, sum, stdout); rc != 0 {
			return rc
		}
		return exit
	}

	fmt.Fprintln(stdout)
	if flags.bench >= 2 {
		fmt.Fprintln(stdout, renderBenchAllTable(benches))
	} else {
		fmt.Fprintln(stdout, renderCheckAllTable(rows))
	}
	fmt.Fprintf(stdout, "\n%d checked: %d ok, %d failed (skipped %d uncredentialed/unsupported)\n",
		len(rows), pass, fail, skipped)
	if fail > 0 {
		return 1
	}
	return 0
}

// probeResult is the structured outcome of one runProbe call. Used by
// both the single-provider and --all paths so their reported numbers
// can't drift.
type probeResult struct {
	modelID        string
	model          *catalog.Model
	reply          string
	latency        time.Duration
	stopReason     string
	usage          agent.Usage
	costMicrocents int64
	err            error
}

// runProbe resolves the model, builds the provider, and issues one
// "say pong" Complete call. Returns the structured result rather than
// printing — the caller decides between single-provider detail and
// the --all summary row.
func runProbe(entry *catalog.Provider, lookup func(string) string) probeResult {
	modelID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	if modelID == "" {
		return probeResult{err: fmt.Errorf("provider %q has no models; set %sMODEL or sync the catalog",
			entry.ID, brand.EnvPrefix)}
	}

	prov, _, err := compat.Build(entry, modelID, lookup)
	if err != nil {
		return probeResult{modelID: modelID, err: fmt.Errorf("build %s: %w", entry.ID, err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := prov.Complete(ctx, agent.CompletionRequest{
		Model:     modelID,
		System:    "Be terse.",
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
		MaxTokens: 16,
	})
	latency := time.Since(start)
	if err != nil {
		return probeResult{modelID: modelID, latency: latency, err: err}
	}
	if resp == nil {
		// A provider returning (nil, nil) violates the contract; this CLI probe
		// runs against a raw provider (not the daemon governor that normalizes
		// this), so guard the deref here rather than panicking the check.
		return probeResult{modelID: modelID, latency: latency, err: fmt.Errorf("provider returned a nil response without an error")}
	}

	model := entry.Models[modelID]
	return probeResult{
		modelID:        modelID,
		model:          model,
		reply:          resp.Message.Content,
		latency:        latency,
		stopReason:     string(resp.StopReason),
		usage:          resp.Usage,
		costMicrocents: computeCostMicrocents(model, resp.Usage),
	}
}

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

// benchResult aggregates N runProbe calls for one provider.
