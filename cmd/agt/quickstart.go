// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
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
	lookup := catalogCredentialLookup(cat, store.Lookup)
	ready := keyedConfigured(cat, lookup)
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

	if len(p.Env) > 0 && !p.HasCredentials(lookup) {
		// Delegate to provider setup, which prompts on stdin and stores the key.
		if code := cmdProviderSetup([]string{pid}, stdout, stderr); code != 0 {
			return code
		}
	} else if len(p.Env) == 0 {
		fmt.Fprintf(stdout, "      %s is keyless (local) — nothing to add.\n", pid)
	} else {
		fmt.Fprintf(stdout, "      %s already has its key.\n", pid)
	}

	// [4/4] Persist the choice (M816) so the daemon uses it with no env vars and
	// no restart. config_set goes through the control plane, so this only works
	// when a daemon is already running — probe quietly and, if it's up, pin
	// AGEZT_PROVIDER/AGEZT_MODEL live; otherwise fall back to the env-var start
	// command (the daemon picks the same values up at boot).
	model := firstModelID(p)
	persisted := persistProviderModel(pid, model, stdout)

	if persisted {
		fmt.Fprintf(stdout, "\n[4/4] You're set — provider %q and model %q are saved (live, no restart).\n", pid, model)
		fmt.Fprintf(stdout, "Try it:\n")
		fmt.Fprintf(stdout, "  %s provider check         # live roundtrip (latency + cost)\n", brand.CLI)
		fmt.Fprintf(stdout, "  %s run \"what is this project?\"\n", brand.CLI)
		fmt.Fprintf(stdout, "\nThe daemon's Web UI URL (in its banner) opens a live monitor + the same setup screen.\n")
		return 0
	}

	// No daemon yet — print the start command. AGEZT_WORKSPACE="$PWD" scopes the
	// file tool to the launch directory (the default is a sandboxed
	// ~/.agezt/workspace); opt-in and visible here rather than changing the safe
	// default.
	fmt.Fprintf(stdout, "\n[4/4] You're set. Start the daemon from your project dir (terminal 1):\n\n")
	fmt.Fprintf(stdout, "  %sPROVIDER=%s %sMODEL=%s %sWORKSPACE=\"$PWD\" %sWEB_ADDR=127.0.0.1:8787 %s\n\n",
		brand.EnvPrefix, pid, brand.EnvPrefix, model, brand.EnvPrefix, brand.EnvPrefix, brand.Binary)
	fmt.Fprintf(stdout, "  (%sWORKSPACE=\"$PWD\" lets the file tool read the current directory;\n", brand.EnvPrefix)
	fmt.Fprintf(stdout, "   omit it to keep the file tool sandboxed to ~/.agezt/workspace.)\n\n")
	fmt.Fprintf(stdout, "Once it's running, the choice is permanent — re-run `%s quickstart`, set it in the\n", brand.CLI)
	fmt.Fprintf(stdout, "Web UI's Setup screen, or `%s config set %sPROVIDER %s`.\n\n", brand.CLI, brand.EnvPrefix, pid)
	fmt.Fprintf(stdout, "Then, in another terminal:\n")
	fmt.Fprintf(stdout, "  %s doctor                 # confirm it's healthy\n", brand.CLI)
	fmt.Fprintf(stdout, "  %s run \"what is this project?\"\n", brand.CLI)
	fmt.Fprintf(stdout, "\nThe daemon banner prints a tokenized Web UI URL — open it for a live monitor.\n")
	return 0
}

// persistProviderModel pins AGEZT_PROVIDER/AGEZT_MODEL on a RUNNING daemon via
// config_set (ApplyLive — no restart). Returns false when no daemon is
// reachable, so the caller can fall back to the env-var start command. The
// probe is silent (io.Discard) — "no daemon yet" is the normal pre-start case,
// not an error.
func persistProviderModel(pid, model string, stdout io.Writer) bool {
	base, err := paths.BaseDir()
	if err != nil {
		return false
	}
	c := dialBase(base, io.Discard)
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": brand.EnvPrefix + "PROVIDER", "value": pid}); err != nil {
		return false
	}
	if model != "" {
		if _, err := c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": brand.EnvPrefix + "MODEL", "value": model}); err != nil {
			return false
		}
	}
	fmt.Fprintf(stdout, "      saved to the running daemon (no env vars, no restart).\n")
	return true
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
