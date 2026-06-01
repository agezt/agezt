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

// cmdWhoami implements `agt whoami [--tenant <id>] [--json]` — reports the
// authenticated principal (M62): the primary (admin) token, or a tenant's own
// token (with AGEZT_TOKEN set) and which tenant. Useful when juggling multiple
// tokens to confirm "who am I right now?".
func cmdWhoami(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	tenant := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s whoami: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s whoami [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "report the authenticated principal (primary admin token vs a tenant's own token)\n")
			fmt.Fprintf(stdout, "  set AGEZT_TOKEN=<tenant-token> + --tenant <id> to authenticate as a tenant\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s whoami: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if tenant != "" {
		callArgs["tenant"] = tenant
	}
	res, err := c.Call(ctx, controlplane.CmdWhoami, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s whoami: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	if primary, _ := res["primary"].(bool); primary {
		fmt.Fprintf(stdout, "%s: primary (admin token — full access)\n", brand.CLI)
	} else {
		t, _ := res["tenant"].(string)
		fmt.Fprintf(stdout, "%s: tenant %q (own token — tenant-scoped access)\n", brand.CLI, t)
	}
	return 0
}
