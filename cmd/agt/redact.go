// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdRedact dispatches `agt redact <subcommand>`. Today the only subcommand is
// `test` — the redaction confidence check (M104).
func cmdRedact(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s redact: subcommand required (test)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "test":
		return cmdRedactTest(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s redact: unknown subcommand %q (test)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdRedactTest implements `agt redact test <string> [--json]` (M104) — asks the
// LIVE redactor whether a candidate would be scrubbed before it could reach the
// journal. Useful to confirm "will my API key actually be protected?" without
// running a task and inspecting the journal. Exit 0 when redaction WOULD catch
// it, 3 when it would NOT (so it's scriptable in a CI secret-hygiene check).
func cmdRedactTest(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var parts []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s redact test <string> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "check whether the live secret redactor would scrub <string> before journaling\n")
			fmt.Fprintf(stdout, "exit 0 = would be redacted, 3 = would NOT (scriptable)\n")
			return 0
		default:
			parts = append(parts, a)
		}
	}
	if len(parts) == 0 {
		fmt.Fprintf(stderr, "%s redact test: a string to test is required\n", brand.CLI)
		return 2
	}
	text := strings.Join(parts, " ")

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRedactTest, map[string]any{"text": text})
	if err != nil {
		fmt.Fprintf(stderr, "%s redact test: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	enabled, _ := res["enabled"].(bool)
	would, _ := res["would_redact"].(bool)
	redacted, _ := res["redacted"].(string)
	literalHit, _ := res["literal_hit"].(bool)
	cats := stringSliceOf(res["categories"])

	if !enabled {
		fmt.Fprintf(stdout, "redaction is DISABLED on this daemon — nothing would be scrubbed.\n")
		fmt.Fprintf(stdout, "  enable it by leaving %sREDACT unset (default on).\n", brand.EnvPrefix)
		return 3
	}
	if !would {
		fmt.Fprintf(stdout, "NOT redacted — this string would pass through to the journal unchanged.\n")
		fmt.Fprintf(stdout, "  if it is a secret, add it to the vault (`%s vault encrypt`) so it joins the literal set.\n", brand.CLI)
		return 3
	}
	fmt.Fprintf(stdout, "redacted ✓ — this string would be scrubbed before journaling.\n")
	fmt.Fprintf(stdout, "  result: %s\n", redacted)
	if len(cats) > 0 {
		fmt.Fprintf(stdout, "  matched pattern(s): %s\n", strings.Join(cats, ", "))
	}
	if literalHit {
		fmt.Fprintf(stdout, "  matched a configured secret literal\n")
	}
	return 0
}

// stringSliceOf coerces a JSON []any of strings into []string.
func stringSliceOf(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
