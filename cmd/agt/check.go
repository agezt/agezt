// SPDX-License-Identifier: MIT
//
// cmd/agt provider check entry + flags + single check + caps check.
// Split from check.go during Day 211 god-file refactor (#39).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)


type checkFlags struct {
	all        bool
	jsonOut    bool
	stream     bool   // SSE roundtrip when the provider implements it
	caps       bool   // report model capabilities from the catalog; no network
	bench      int    // 0 = single-shot; ≥2 = run N probes and report stats
	providerID string // positional arg, empty for auto-pick
}

// parseCheckFlags accepts argv after "check". Recognised flags:
//
//	--all, -a       iterate every credentialed provider
//	--json, -j      machine-readable output (single or all)
//	--bench N       run N probes, report p50/p95 latencies
//	<provider-id>   positional, single-provider mode only
//
// Unrecognised flags return an error so typos surface immediately
// rather than getting silently treated as provider ids.
func parseCheckFlags(args []string) (checkFlags, error) {
	f := checkFlags{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--all" || a == "-a":
			f.all = true
		case a == "--json" || a == "-j":
			f.jsonOut = true
		case a == "--stream" || a == "-s":
			f.stream = true
		case a == "--caps" || a == "--capabilities":
			f.caps = true
		case a == "--bench":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--bench requires an integer count")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 2 {
				return f, fmt.Errorf("--bench requires N ≥ 2, got %q", args[i+1])
			}
			f.bench = n
			i++
		case strings.HasPrefix(a, "--bench="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil || n < 2 {
				return f, fmt.Errorf("--bench requires N ≥ 2")
			}
			f.bench = n
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("unknown flag %q", a)
		default:
			if f.providerID != "" {
				return f, fmt.Errorf("unexpected extra arg %q (provider id already set to %q)", a, f.providerID)
			}
			f.providerID = strings.ToLower(strings.TrimSpace(a))
		}
	}
	return f, nil
}

// cmdProviderCheck dispatches `agt provider check`. Modes:
//
//	agt provider check                          # single, auto-pick, human
//	agt provider check <id>                     # single, explicit, human
//	agt provider check --all                    # all-credentialed, human table
//	agt provider check --json [<id>]            # single, machine-readable
//	agt provider check --all --json             # all, machine-readable
//	agt provider check --bench 5 [<id>]         # single, 5 probes, p50/p95
//	agt provider check --all --bench 3          # all, 3 probes each
//	agt provider check --caps [<id>]            # model capabilities; no network
//	agt provider check --caps --json [<id>]     # capabilities, machine-readable
//	agt provider check --caps --all             # capability matrix, all providers
//
// Probe prompt is tiny ("Say 'pong' in one word.", MaxTokens=16) so
// even --bench 20 runs near-free on most providers.
func cmdProviderCheck(args []string, stdout, stderr io.Writer) int {
	flags, err := parseCheckFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 2
	}

	cat, err := loadCatalogIfAny(stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if cat == nil {
		fmt.Fprintf(stderr, "%s: catalog is empty — run `agt catalog sync` first\n", brand.CLI)
		return 1
	}

	// --caps reports static model capabilities from the catalog: no
	// network, no credentials, no live probe. It answers "can the model I'm
	// about to use actually call tools / see images?" before a run, and
	// warns when the agent loop's prerequisite (tool-use) is missing.
	if flags.caps {
		if flags.bench >= 2 || flags.stream {
			fmt.Fprintf(stderr, "%s: --caps cannot combine with --bench/--stream (it makes no live call)\n", brand.CLI)
			return 2
		}
		if flags.all {
			return runCheckCapsAll(cat, flags, stdout)
		}
		return runCheckCaps(cat, flags, stdout, stderr)
	}

	credStore, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	lookup := catalogCredentialLookup(cat, credStore.Lookup)

	// --stream only applies to single-provider, non-bench mode for v1.
	// Bench would re-stream the same tokens N times (visually noisy);
	// --all would interleave streams across providers. Reject these
	// combinations explicitly so the operator hears the constraint.
	if flags.stream && (flags.all || flags.bench >= 2) {
		fmt.Fprintf(stderr, "%s: --stream is not yet supported with --all or --bench\n", brand.CLI)
		return 2
	}

	if flags.all {
		return runCheckAll(cat, lookup, flags, stdout, stderr)
	}
	return runCheckSingle(cat, lookup, flags, stdout, stderr)
}
