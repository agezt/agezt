// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdResearch implements `agt research "<question>"` — run the deep-research
// harness (M1001): decompose the question, gather independent web sources
// (web_search + browser.read), synthesize a citation-grounded answer, and
// adversarially verify each cited claim against its source. Prints the answer
// with a confidence line; --json emits the full report (sources + claims).
func cmdResearch(args []string, stdout, stderr io.Writer) int {
	maxSources := 0
	maxSubQ := 0
	maxVerify := 0
	noVerify := false
	asJSON := false
	quiet := false
	var rest []string

	needVal := func(i *int, flag string) (string, bool) {
		if *i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s research: %s needs a value\n", brand.CLI, flag)
			return "", false
		}
		*i++
		return args[*i], true
	}
	atoiFlag := func(v, flag string) (int, bool) {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			fmt.Fprintf(stderr, "%s research: %s must be a non-negative integer\n", brand.CLI, flag)
			return 0, false
		}
		return n, true
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			researchUsage(stdout)
			return 0
		case a == "--max-sources":
			v, ok := needVal(&i, "--max-sources")
			if !ok {
				return 2
			}
			if maxSources, ok = atoiFlag(v, "--max-sources"); !ok {
				return 2
			}
		case strings.HasPrefix(a, "--max-sources="):
			n, ok := atoiFlag(strings.TrimPrefix(a, "--max-sources="), "--max-sources")
			if !ok {
				return 2
			}
			maxSources = n
		case a == "--max-sub-questions":
			v, ok := needVal(&i, "--max-sub-questions")
			if !ok {
				return 2
			}
			if maxSubQ, ok = atoiFlag(v, "--max-sub-questions"); !ok {
				return 2
			}
		case strings.HasPrefix(a, "--max-sub-questions="):
			n, ok := atoiFlag(strings.TrimPrefix(a, "--max-sub-questions="), "--max-sub-questions")
			if !ok {
				return 2
			}
			maxSubQ = n
		case a == "--max-verify-claims":
			v, ok := needVal(&i, "--max-verify-claims")
			if !ok {
				return 2
			}
			if maxVerify, ok = atoiFlag(v, "--max-verify-claims"); !ok {
				return 2
			}
		case strings.HasPrefix(a, "--max-verify-claims="):
			n, ok := atoiFlag(strings.TrimPrefix(a, "--max-verify-claims="), "--max-verify-claims")
			if !ok {
				return 2
			}
			maxVerify = n
		case a == "--no-verify":
			noVerify = true
		case a == "--json":
			asJSON = true
		case a == "-q" || a == "--quiet":
			quiet = true
		default:
			rest = append(rest, a)
		}
	}

	question := strings.TrimSpace(strings.Join(rest, " "))
	if question == "" {
		researchUsage(stderr)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdResearchAsk, map[string]any{
		"question":          question,
		"max_sources":       maxSources,
		"max_sub_questions": maxSubQ,
		"max_verify_claims": maxVerify,
		"verify":            !noVerify,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s research: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	markdown, _ := res["markdown"].(string)
	if quiet {
		fmt.Fprintln(stdout, strings.TrimSpace(markdown))
		return 0
	}

	conf := jsonNum100(res["confidence"])
	verified, _ := res["verified"].(bool)
	badge := "unverified"
	if verified {
		badge = "verified"
	}
	nSources := 0
	if ss, ok := res["sources"].([]any); ok {
		nSources = len(ss)
	}
	fmt.Fprintf(stdout, "research: %d%% confidence · %s · %d source(s)\n", conf, badge, nSources)
	fmt.Fprintln(stdout, strings.Repeat("─", 40))
	fmt.Fprintln(stdout, strings.TrimSpace(markdown))

	// Surface any refuted claims — the point of the adversarial pass.
	if claims, ok := res["claims"].([]any); ok {
		var refuted []string
		for _, cv := range claims {
			m, ok := cv.(map[string]any)
			if !ok {
				continue
			}
			if v, _ := m["verdict"].(string); v == "refuted" {
				t, _ := m["text"].(string)
				refuted = append(refuted, t)
			}
		}
		if len(refuted) > 0 {
			fmt.Fprintln(stdout, strings.Repeat("─", 40))
			fmt.Fprintf(stdout, "REFUTED under verification (%d):\n", len(refuted))
			for _, t := range refuted {
				fmt.Fprintf(stdout, "  ✗ %s\n", t)
			}
		}
	}
	return 0
}

// jsonNum100 coerces a 0..1 JSON float to an integer percentage.
func jsonNum100(v any) int {
	if f, ok := v.(float64); ok {
		return int(f*100 + 0.5)
	}
	return 0
}

func researchUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s research \"<question>\" [flags]\n", brand.CLI)
	fmt.Fprintf(w, "deep-research: decompose, gather web sources, synthesize a cited answer, verify claims\n")
	fmt.Fprintf(w, "  --max-sources <n>        cap distinct sources gathered (default 8, max 20)\n")
	fmt.Fprintf(w, "  --max-sub-questions <n>  cap sub-questions explored (default 3, max 8)\n")
	fmt.Fprintf(w, "  --max-verify-claims <n>  cap claims adversarially verified (default 6, max 12)\n")
	fmt.Fprintf(w, "  --no-verify              skip the adversarial claim-verification pass\n")
	fmt.Fprintf(w, "  --json                   print the full report (sources + claims) as JSON\n")
	fmt.Fprintf(w, "  -q, --quiet              print only the synthesized answer\n")
}
