// SPDX-License-Identifier: MIT

package main

// `agt journal head [--json]` — the minimal "what's the current
// head seq?" query. Sister to `journal tail` (which also returns
// head but bundles events). The headline use case is capturing
// a checkpoint to pass back to `agt pulse --since <seq>` later:
//
//   $ checkpoint=$(agt journal head --json | jq -r .head)
//   $ ./run-some-test
//   $ agt pulse --since $checkpoint --json | jq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdJournalHead(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s journal head [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "print the current head seq + chain-tail hash\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s journal head: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdJournalHead, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal head: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	head := intOfStatus(res["head"])
	hash, _ := res["hash"].(string)
	// 64 zeros = genesis = empty journal. Surface that explicitly
	// rather than confusing operators with the all-zero hex string.
	if hash == strings.Repeat("0", 64) {
		fmt.Fprintf(stdout, "head seq=%d (empty journal — genesis)\n", head)
		return 0
	}
	fmt.Fprintf(stdout, "head seq=%d hash=%s\n", head, hash)
	return 0
}
