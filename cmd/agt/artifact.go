// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdArtifact implements `agt artifact <subcommand>`. Today the one subcommand is
// `get`, which fetches a content-addressed tool output the agent loop offloaded
// out of the journal (the tool.result event carries the raw_ref). SPEC-04 §3.6.
func cmdArtifact(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s artifact: subcommand required (get)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "get":
		return cmdArtifactGet(args[1:], stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s artifact get <ref> [--out <file>]\n", brand.CLI)
		fmt.Fprintf(stdout, "fetch a content-addressed tool output offloaded from the journal (its raw_ref)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s artifact: unknown subcommand %q (want: get)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdArtifactGet implements `agt artifact get <ref> [--out <file>]`. Without
// --out it writes the raw bytes to stdout; with --out it writes them to a file.
func cmdArtifactGet(args []string, stdout, stderr io.Writer) int {
	ref := ""
	outPath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s artifact get: --out needs a file path\n", brand.CLI)
				return 2
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "--out="):
			outPath = strings.TrimPrefix(a, "--out=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s artifact get <ref> [--out <file>]\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s artifact get: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if ref != "" {
				fmt.Fprintf(stderr, "%s artifact get: unexpected arg %q (one ref)\n", brand.CLI, a)
				return 2
			}
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "%s artifact get: a ref is required (the raw_ref from a tool.result)\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdArtifactGet, map[string]any{"ref": ref})
	if err != nil {
		fmt.Fprintf(stderr, "%s artifact get: %v\n", brand.CLI, err)
		return 1
	}
	enc, _ := res["data"].(string)
	data, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		fmt.Fprintf(stderr, "%s artifact get: decode response: %v\n", brand.CLI, derr)
		return 1
	}

	if outPath == "" {
		_, _ = stdout.Write(data)
		return 0
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "%s artifact get: write %s: %v\n", brand.CLI, outPath, err)
		return 1
	}
	fmt.Fprintf(stderr, "wrote %d byte(s) to %s\n", len(data), outPath)
	return 0
}
