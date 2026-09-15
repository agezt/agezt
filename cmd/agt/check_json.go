// SPDX-License-Identifier: MIT
//
// cmd/agt provider check JSON output + small helpers (autoPickFromCatalog +
// computeCostMicrocents + formatMicrocentsUSD + truncate). Split from
// check.go during Day 211 god-file refactor (#39). Public API unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

type jsonProbe struct {
	Provider       string     `json:"provider"`
	Family         string     `json:"family"`
	Model          string     `json:"model"`
	OK             bool       `json:"ok"`
	Reply          string     `json:"reply,omitempty"`
	StopReason     string     `json:"stop_reason,omitempty"`
	InputTokens    int        `json:"input_tokens,omitempty"`
	OutputTokens   int        `json:"output_tokens,omitempty"`
	LatencyMS      int64      `json:"latency_ms"`
	CostMicrocents int64      `json:"cost_microcents"`
	Error          string     `json:"error,omitempty"`
	Bench          *jsonBench `json:"bench,omitempty"`
}

// jsonBench holds the multi-probe stats. Embedded inside jsonProbe
// only when --bench was used.
type jsonBench struct {
	Iterations          int   `json:"iterations"`
	Successes           int   `json:"successes"`
	Failures            int   `json:"failures"`
	MinMS               int64 `json:"min_ms"`
	P50MS               int64 `json:"p50_ms"`
	P95MS               int64 `json:"p95_ms"`
	MaxMS               int64 `json:"max_ms"`
	TotalCostMicrocents int64 `json:"total_cost_microcents"`
}

// jsonSummary appears once per invocation; lets CI scripts gate on
// {failed: 0} without iterating probes.
type jsonSummary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped,omitempty"`
}

// jsonOutput is the top-level shape emitted by --json. Stable
// machine-readable contract — see the phase report for the schema.
type jsonOutput struct {
	Probes  []jsonProbe `json:"probes"`
	Summary jsonSummary `json:"summary"`
}

func probeToJSON(entry *catalog.Provider, res probeResult) jsonProbe {
	jp := jsonProbe{
		Provider:       entry.ID,
		Family:         string(entry.Family()),
		Model:          res.modelID,
		OK:             res.err == nil,
		LatencyMS:      res.latency.Milliseconds(),
		CostMicrocents: res.costMicrocents,
	}
	if res.err != nil {
		jp.Error = res.err.Error()
		return jp
	}
	jp.Reply = strings.TrimSpace(res.reply)
	jp.StopReason = res.stopReason
	jp.InputTokens = res.usage.InputTokens
	jp.OutputTokens = res.usage.OutputTokens
	return jp
}

func (b benchResult) toJSON() jsonProbe {
	jp := jsonProbe{
		Provider:       b.entry.ID,
		Family:         string(b.entry.Family()),
		Model:          b.modelID,
		OK:             b.failures == 0,
		LatencyMS:      b.stats.p50.Milliseconds(),
		CostMicrocents: b.totalCost,
		Bench: &jsonBench{
			Iterations:          b.iterations,
			Successes:           b.successes,
			Failures:            b.failures,
			MinMS:               b.stats.min.Milliseconds(),
			P50MS:               b.stats.p50.Milliseconds(),
			P95MS:               b.stats.p95.Milliseconds(),
			MaxMS:               b.stats.max.Milliseconds(),
			TotalCostMicrocents: b.totalCost,
		},
	}
	if b.lastErr != nil {
		jp.Error = b.lastErr.Error()
	}
	return jp
}

// summaryFromProbes builds the top-level summary block for the
// single-probe (or single-bench) case. The --all path constructs
// the summary itself so it can include skipped count.
func summaryFromProbes(probes ...jsonProbe) jsonSummary {
	s := jsonSummary{Total: len(probes)}
	for _, p := range probes {
		if p.OK {
			s.OK++
		} else {
			s.Failed++
		}
	}
	return s
}

// emitJSON marshals the output and prints it. Indented for human
// readability when scripts pipe through `jq` or `less`; the
// indentation costs nothing and a one-shot CI gate already invokes
// jq on the bytes anyway.
func emitJSON(probes []jsonProbe, summary jsonSummary, stdout io.Writer) int {
	out := jsonOutput{Probes: probes, Summary: summary}
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(stdout, `{"error":"marshal: %s"}`, err)
		return 1
	}
	fmt.Fprintln(stdout, string(buf))
	return 0
}

// --- helpers shared with M1.p / M1.p.x ---------------------------------

// autoPickFromCatalog mirrors the daemon's auto-pick logic: first
// supported family + has-credentials wins. Deterministic via the
// catalog's sort order.
func autoPickFromCatalog(cat *catalog.Catalog, lookup func(string) string) *catalog.Provider {
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			continue
		}
		if !entry.HasCredentials(lookup) {
			continue
		}
		return entry
	}
	return nil
}

// computeCostMicrocents returns the USD-microcents cost of a single
// call given the model's pricing and the actual token usage.
//
//	cost = (in_tokens * input_microcents_per_MTok / 1e6) + (out_tokens * output_microcents_per_MTok / 1e6)
//
// Matches the Governor's pricing arithmetic (kernel/governor/pricing.go)
// so `agt provider check` cost agrees with what the daemon would
// account for the same call.
func computeCostMicrocents(model *catalog.Model, usage agent.Usage) int64 {
	if model == nil || model.Cost == nil {
		return 0
	}
	inPrice := model.Cost.InputMicrocentsPerMTok()
	outPrice := model.Cost.OutputMicrocentsPerMTok()
	const tokensPerMTok = 1_000_000
	return (int64(usage.InputTokens)*inPrice)/tokensPerMTok +
		(int64(usage.OutputTokens)*outPrice)/tokensPerMTok
}

// formatMicrocentsUSD renders microcents as a USD string. 1 USD = 10^9
// microcents. Uses integer arithmetic throughout — float64 can't
// represent sub-cent values precisely (4500 microcents = $0.0000045
// would round to $0.000005 with %.6f).
//
//	0             → "0.00"
//	500_000       → "0.0005"     (half a cent)
//	1_000_000     → "0.001"      (one cent)
//	17_500_000    → "0.0175"     (claude small-call cost)
//	1_000_000_000 → "1.00"
//	999_999_999   → "0.999999999"
func formatMicrocentsUSD(mc int64) string {
	if mc < 0 {
		return "-" + formatMicrocentsUSD(-mc)
	}
	dollars := mc / 1_000_000_000
	sub := mc % 1_000_000_000 // 0 .. 999_999_999 sub-dollar microcents
	subStr := fmt.Sprintf("%09d", sub)
	subStr = strings.TrimRight(subStr, "0")
	if len(subStr) < 2 {
		subStr += strings.Repeat("0", 2-len(subStr))
	}
	return fmt.Sprintf("%d.%s", dollars, subStr)
}

func truncate(s string, n int) string {
	return strutil.Ellipsis(s, n, "…")
}
