// SPDX-License-Identifier: MIT

package main

// Provenance: Package main: `agt market publish` + `agt market keygen` — the
//             publish-flow ops (publish a workflow as a marketplace entry / generate
//             the operator's marketplace signing keypair). Extracted from
//             market_ops.go during the Day-211 god-file split. Public API unchanged.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/market"
)

func cmdMarketPublish(args []string, stdout, stderr io.Writer) int {
	var dir, out, keyFile, name string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market publish <dir> --out <dir> [--key <file>] [--name N]\n", brand.CLI)
			return 0
		case a == "--out" && i+1 < len(args):
			i++
			out = args[i]
		case a == "--key" && i+1 < len(args):
			i++
			keyFile = args[i]
		case a == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case dir == "":
			dir = a
		default:
			fmt.Fprintf(stderr, "%s market publish: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if dir == "" || out == "" {
		fmt.Fprintf(stderr, "%s market publish: <dir> and --out <dir> are required\n", brand.CLI)
		return 2
	}
	p, err := market.BuildPackFromDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, err)
		return 1
	}
	if name == "" {
		name = "local"
	}
	var priv []byte
	if keyFile != "" {
		kb, rerr := os.ReadFile(keyFile)
		if rerr != nil {
			fmt.Fprintf(stderr, "%s market publish: read key: %v\n", brand.CLI, rerr)
			return 1
		}
		pk, perr := market.PrivateKeyFromHex(string(kb))
		if perr != nil {
			fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, perr)
			return 1
		}
		priv = pk
	}
	if err := market.Publish(p, out, name, priv, time.Now().UnixMilli()); err != nil {
		fmt.Fprintf(stderr, "%s market publish: %v\n", brand.CLI, err)
		return 1
	}
	signed := "unsigned"
	if priv != nil {
		signed = "signed"
	}
	fmt.Fprintf(stdout, "published %s@%s (%s) → %s\n", p.Name, p.Version, signed, filepath.Join(out, "marketplace.json"))
	return 0
}
func cmdMarketKeygen(args []string, stdout, stderr io.Writer) int {
	prefix := "agezt-market"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s market keygen [--out <prefix>]\n", brand.CLI)
			return 0
		case a == "--out" && i+1 < len(args):
			i++
			prefix = args[i]
		default:
			fmt.Fprintf(stderr, "%s market keygen: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	pub, priv, err := market.GenerateKeypair()
	if err != nil {
		fmt.Fprintf(stderr, "%s market keygen: %v\n", brand.CLI, err)
		return 1
	}
	// The signing key is written through the platform's atomic writer: a crash
	// mid-write would leave a TRUNCATED key file, and a truncated signing key
	// is an unrecoverable loss — the operator's whole publishing identity is
	// gone, with no error to show for it. The atomic writer writes to a
	// unique temp, fsyncs it and renames it into place, so the file on disk
	// is either the whole key or not there.
	//
	// 0600 is the store framework's posture for a file that holds material
	// the journal records. On Windows the mode bits do not apply (the file is
	// as private as the directory holding it), and the repo's own answer to
	// that is the ENCRYPTED vault — not a mode bit.
	if err := atomicfile.WriteFile(prefix+".key", []byte(priv+"\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "%s market keygen: write key: %v\n", brand.CLI, err)
		return 1
	}
	if err := atomicfile.WriteFile(prefix+".pub", []byte(pub+"\n"), 0o644); err != nil {
		fmt.Fprintf(stderr, "%s market keygen: write pub: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s.key (keep secret) and %s.pub\n", prefix, prefix)
	fmt.Fprintf(stdout, "  pubkey: %s\n", pub)
	fmt.Fprintf(stdout, "publish with: %s market publish <dir> --out <dir> --key %s.key\n", brand.CLI, prefix)
	fmt.Fprintf(stdout, "consumers pin it: %s market add <url> --pubkey %s\n", brand.CLI, pub)
	return 0
}
