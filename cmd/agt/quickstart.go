// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
)

// cmdQuickstart implements `agt quickstart` — a one-command, client-side
// onboarding wizard that chains the pieces a new operator needs: sync the
// catalog (offline), pick a provider and add its key, then print the exact
// command to start the daemon. No daemon required to run it.
//
// It is deliberately thin glue over the existing surfaces (catalog sync
// --local, provider setup, the creds vault) — the wizard's value is sequencing
// and guidance, not new behaviour.
func cmdQuickstart(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(stdout, "usage: %s quickstart\n", brand.CLI)
		fmt.Fprintf(stdout, "interactive first-run: sync catalog, add a provider key, print the start command\n")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintf(stderr, "%s quickstart: takes no arguments (got %q)\n", brand.CLI, args[0])
		return 2
	}

	fmt.Fprintf(stdout, "%s quickstart — let's get you running.\n\n", brand.CLI)

	// [1/4] Catalog.
	cat, _ := loadCatalogIfAny(stderr)
	if cat == nil || len(cat.Providers) == 0 {
		fmt.Fprintf(stdout, "[1/4] No catalog yet — syncing models.dev (offline)…\n")
		if code := cmdCatalogSync([]string{"--local"}, stdout, stderr); code != 0 {
			return code
		}
		cat, _ = loadCatalogIfAny(stderr)
		if cat == nil || len(cat.Providers) == 0 {
			fmt.Fprintf(stderr, "%s quickstart: catalog still empty after sync\n", brand.CLI)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "[1/4] Catalog ready (%d providers).\n", len(cat.Providers))
	}

	// [2/4] Existing credentials.
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	ready := keyedConfigured(cat, store.Lookup)
	if len(ready) > 0 {
		fmt.Fprintf(stdout, "[2/4] Already configured: %s\n", strings.Join(ready, ", "))
	} else {
		fmt.Fprintf(stdout, "[2/4] No provider keys yet.\n")
	}

	// [3/4] Choose a provider and (if needed) add its key.
	fmt.Fprintf(stdout, "[3/4] Which provider do you want to use?\n")
	fmt.Fprintf(stdout, "      A models.dev id — e.g. minimax-coding-plan, anthropic, openai,\n")
	fmt.Fprintf(stdout, "      or ollama-local for a local no-key model.\n")
	def := ""
	if len(ready) > 0 {
		def = ready[0]
	}
	if def != "" {
		fmt.Fprintf(stdout, "      provider id [%s]: ", def)
	} else {
		fmt.Fprintf(stdout, "      provider id: ")
	}
	pid := strings.TrimSpace(readLine(stdin()))
	if pid == "" {
		pid = def
	}
	if pid == "" {
		fmt.Fprintf(stderr, "%s quickstart: no provider chosen\n", brand.CLI)
		return 2
	}
	p, ok := cat.Providers[pid]
	if !ok {
		fmt.Fprintf(stderr, "%s quickstart: %q not in catalog\n", brand.CLI, pid)
		if sugg := suggestProviders(cat, pid); len(sugg) > 0 {
			fmt.Fprintf(stderr, "did you mean: %s\n", strings.Join(sugg, ", "))
		}
		return 2
	}

	if len(p.Env) > 0 && !p.HasCredentials(store.Lookup) {
		// Delegate to provider setup, which prompts on stdin and stores the key.
		if code := cmdProviderSetup([]string{pid}, stdout, stderr); code != 0 {
			return code
		}
	} else if len(p.Env) == 0 {
		fmt.Fprintf(stdout, "      %s is keyless (local) — nothing to add.\n", pid)
	} else {
		fmt.Fprintf(stdout, "      %s already has its key.\n", pid)
	}

	// [4/4] Start command + next steps.
	model := firstModelID(p)
	fmt.Fprintf(stdout, "\n[4/4] You're set. Start the daemon (terminal 1):\n\n")
	fmt.Fprintf(stdout, "  %sPROVIDER=%s %sMODEL=%s %sWEB_ADDR=127.0.0.1:8787 %s\n\n",
		brand.EnvPrefix, pid, brand.EnvPrefix, model, brand.EnvPrefix, brand.Binary)
	fmt.Fprintf(stdout, "Then, in another terminal:\n")
	fmt.Fprintf(stdout, "  %s doctor                 # confirm it's healthy\n", brand.CLI)
	fmt.Fprintf(stdout, "  %s provider check         # live roundtrip (latency + cost)\n", brand.CLI)
	fmt.Fprintf(stdout, "  %s run \"what is this project?\"\n", brand.CLI)
	fmt.Fprintf(stdout, "\nThe daemon banner prints a tokenized Web UI URL — open it for a live monitor.\n")
	return 0
}

// keyedConfigured returns the ids of catalog providers that require a key and
// have one set (via the given lookup). Keyless providers are excluded — the
// point is "what can you already authenticate as?".
func keyedConfigured(cat *catalog.Catalog, lookup func(string) string) []string {
	var out []string
	for _, p := range cat.ProviderList() {
		if len(p.Env) > 0 && p.HasCredentials(lookup) {
			out = append(out, p.ID)
		}
	}
	sort.Strings(out)
	return out
}

// stdin is indirected so tests can stub it.
var stdin = func() io.Reader { return os.Stdin }

func readLine(r io.Reader) string {
	line, _ := bufio.NewReader(r).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}
