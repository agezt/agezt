// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// estimateCostMicrocents returns the cost in microcents of inToks input + outToks
// output at the given per-Mtok microcents prices. Pure for testability (M111).
func estimateCostMicrocents(inMcPerMtok, outMcPerMtok, inToks, outToks int64) int64 {
	// price is per 1e6 tokens; cost = price * tokens / 1e6.
	return (inMcPerMtok*inToks)/1_000_000 + (outMcPerMtok*outToks)/1_000_000
}

// findModelCost locates a model by id in a CmdCatalogList response, returning its
// entry, the owning provider id, and whether it was found.
func findModelCost(res map[string]any, modelID string) (entry map[string]any, provider string, found bool) {
	provs, _ := res["providers"].([]any)
	for _, p := range provs {
		pm, _ := p.(map[string]any)
		pid, _ := pm["id"].(string)
		models, _ := pm["models"].([]any)
		for _, m := range models {
			me, _ := m.(map[string]any)
			if id, _ := me["id"].(string); id == modelID {
				return me, pid, true
			}
		}
	}
	return nil, "", false
}

// cmdProviderCost implements `agt provider cost --model <id> [--input-tokens N]
// [--output-tokens N] [--json]` (M111) — a standalone model-price lookup, so an
// operator choosing a model (or sanity-checking a bill) can see its per-Mtok
// price and the cost of a hypothetical token count without authoring a plan.
func cmdProviderCost(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	model := ""
	var inToks, outToks int64
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--model" || a == "-m":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider cost: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case a == "--input-tokens":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider cost: --input-tokens needs a number\n", brand.CLI)
				return 2
			}
			i++
			inToks = parseTokens(stderr, args[i])
			if inToks < 0 {
				return 2
			}
		case a == "--output-tokens":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider cost: --output-tokens needs a number\n", brand.CLI)
				return 2
			}
			i++
			outToks = parseTokens(stderr, args[i])
			if outToks < 0 {
				return 2
			}
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s provider cost --model <id> [--input-tokens N] [--output-tokens N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "look up a model's per-Mtok price and optionally estimate a token count's cost\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s provider cost: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if model == "" {
		fmt.Fprintf(stderr, "%s provider cost: --model required (e.g. claude-sonnet-4-6)\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCatalogList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider cost: %v\n", brand.CLI, err)
		return 1
	}
	entry, provider, found := findModelCost(res, model)
	if !found {
		fmt.Fprintf(stderr, "%s provider cost: model %q not in the catalog (try `%s catalog list`)\n", brand.CLI, model, brand.CLI)
		return 1
	}
	inMc := mcFromAny(entry["cost_input_mc_per_mtok"])
	outMc := mcFromAny(entry["cost_output_mc_per_mtok"])
	_, hasCost := entry["cost_input_mc_per_mtok"]
	if !hasCost || (inMc == 0 && outMc == 0) {
		if asJSON {
			return encodeJSON(stdout, map[string]any{"model": model, "provider": provider, "priced": false})
		}
		fmt.Fprintf(stdout, "%s (%s): no pricing in the catalog (a free/local or unpriced model)\n", model, provider)
		return 0
	}

	estimate := estimateCostMicrocents(inMc, outMc, inToks, outToks)
	if asJSON {
		out := map[string]any{
			"model":              model,
			"provider":           provider,
			"priced":             true,
			"input_mc_per_mtok":  inMc,
			"output_mc_per_mtok": outMc,
		}
		if inToks > 0 || outToks > 0 {
			out["input_tokens"] = inToks
			out["output_tokens"] = outToks
			out["estimate_mc"] = estimate
		}
		return encodeJSON(stdout, out)
	}

	fmt.Fprintf(stdout, "%s (provider %s):\n", model, provider)
	fmt.Fprintf(stdout, "  input : %s / Mtok\n", fmtUSD(inMc))
	fmt.Fprintf(stdout, "  output: %s / Mtok\n", fmtUSD(outMc))
	if inToks > 0 || outToks > 0 {
		fmt.Fprintf(stdout, "  estimate for %s in / %s out: %s\n",
			commaInt(inToks), commaInt(outToks), fmtUSD(estimate))
	}
	return 0
}

// parseTokens parses a non-negative token count, printing an error and returning
// -1 on failure (the caller treats <0 as a usage error).
func parseTokens(stderr io.Writer, s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		fmt.Fprintf(stderr, "%s provider cost: bad token count %q (want a non-negative integer)\n", brand.CLI, s)
		return -1
	}
	return n
}

// commaInt formats an integer with thousands separators for readability.
func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}
