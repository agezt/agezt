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

// cmdConductor implements `agt conductor "<task>"` — run the asymmetric,
// verify-driven panel (M997): a Thinker plans, a Worker solves, and a Verifier
// checks (running the worker's code when it can), looping until accepted or the
// round cap is hit. Roles default to distinct keyed-provider models; override any
// with --thinker/--worker/--verifier (a model id or "@chain").
func cmdConductor(args []string, stdout, stderr io.Writer) int {
	var thinker, worker, verifier string
	maxRounds := 0
	plan := false
	asJSON := false
	quiet := false
	var rest []string

	needVal := func(i *int, flag string) (string, bool) {
		if *i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s conductor: %s needs a value\n", brand.CLI, flag)
			return "", false
		}
		*i++
		return args[*i], true
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			conductorUsage(stdout)
			return 0
		case a == "--thinker":
			v, ok := needVal(&i, "--thinker")
			if !ok {
				return 2
			}
			thinker = v
		case strings.HasPrefix(a, "--thinker="):
			thinker = strings.TrimPrefix(a, "--thinker=")
		case a == "--worker":
			v, ok := needVal(&i, "--worker")
			if !ok {
				return 2
			}
			worker = v
		case strings.HasPrefix(a, "--worker="):
			worker = strings.TrimPrefix(a, "--worker=")
		case a == "--verifier":
			v, ok := needVal(&i, "--verifier")
			if !ok {
				return 2
			}
			verifier = v
		case strings.HasPrefix(a, "--verifier="):
			verifier = strings.TrimPrefix(a, "--verifier=")
		case a == "--max-rounds":
			v, ok := needVal(&i, "--max-rounds")
			if !ok {
				return 2
			}
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s conductor: --max-rounds must be a non-negative integer\n", brand.CLI)
				return 2
			}
			maxRounds = n
		case strings.HasPrefix(a, "--max-rounds="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-rounds="))
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s conductor: --max-rounds must be a non-negative integer\n", brand.CLI)
				return 2
			}
			maxRounds = n
		case a == "--plan":
			plan = true
		case a == "--json":
			asJSON = true
		case a == "-q" || a == "--quiet":
			quiet = true
		default:
			rest = append(rest, a)
		}
	}

	task := strings.TrimSpace(strings.Join(rest, " "))
	if task == "" {
		conductorUsage(stderr)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConductorAsk, map[string]any{
		"task":       task,
		"thinker":    thinker,
		"worker":     worker,
		"verifier":   verifier,
		"max_rounds": maxRounds,
		"plan":       plan,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s conductor: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return conductorExit(res)
	}

	answer, _ := res["answer"].(string)
	if quiet {
		fmt.Fprintln(stdout, strings.TrimSpace(answer))
		return conductorExit(res)
	}

	passed, _ := res["passed"].(bool)
	verdict := "FAILED verification"
	if passed {
		verdict = "PASSED verification"
	}
	rounds := jsonNum(res["rounds"])
	fmt.Fprintf(stdout, "conductor: %s after %d round(s)\n", verdict, rounds)
	if roles, ok := res["roles"].(map[string]any); ok {
		fmt.Fprintf(stdout, "  roles: thinker=%v worker=%v verifier=%v\n",
			roles["thinker"], roles["worker"], roles["verifier"])
	}
	fmt.Fprintln(stdout, strings.Repeat("─", 40))
	fmt.Fprintln(stdout, strings.TrimSpace(answer))
	return conductorExit(res)
}

// conductorExit returns 0 when the verifier passed, 3 when it didn't — so a
// script can branch on whether the answer was verified.
func conductorExit(res map[string]any) int {
	if passed, _ := res["passed"].(bool); passed {
		return 0
	}
	return 3
}

// jsonNum coerces a JSON number (float64) to int for display.
func jsonNum(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func conductorUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s conductor \"<task>\" [flags]\n", brand.CLI)
	fmt.Fprintf(w, "run the Thinker/Worker/Verifier panel on a hard, verifiable task\n")
	fmt.Fprintf(w, "  --thinker <model>    model id or @chain for the planning role (default: a keyed provider)\n")
	fmt.Fprintf(w, "  --worker <model>     model id or @chain for the solving role (default: a different provider)\n")
	fmt.Fprintf(w, "  --verifier <model>   model id or @chain for the checking role (default: a different provider)\n")
	fmt.Fprintf(w, "  --max-rounds <n>     worker/verifier retry cap (default 2)\n")
	fmt.Fprintf(w, "  --plan               tailor per-role instructions with a planning call first\n")
	fmt.Fprintf(w, "  --json               print the full result (answer + transcript) as JSON\n")
	fmt.Fprintf(w, "  -q, --quiet          print only the answer\n")
	fmt.Fprintf(w, "exit: 0 = verifier passed, 3 = not verified, 1 = error\n")
}
