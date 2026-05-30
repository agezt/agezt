// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

// cmdProviderSetup implements `agt provider setup [provider-id]` — the guided
// key-adding flow for catalog providers. Fully client-side (reads the synced
// catalog + writes the vault), so it works with the daemon off, pairing with
// `agt catalog sync --local`.
//
//   - No arg: list every catalog provider that needs an API key, showing which
//     have credentials and which are missing — a "what do I still need to set
//     up?" overview.
//   - With a provider id: walk that provider's required env vars and prompt for
//     each missing one (the value is read from stdin, never the command line,
//     so it doesn't leak to shell history).
func cmdProviderSetup(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprintf(stdout, "usage: %s provider setup [provider-id]\n", brand.CLI)
			fmt.Fprintf(stdout, "  no arg      list providers that need an API key + their status\n")
			fmt.Fprintf(stdout, "  <id>        prompt to add the missing keys for that provider\n")
			return 0
		}
	}

	cat, _ := loadCatalogIfAny(stderr)
	if cat == nil || len(cat.Providers) == 0 {
		fmt.Fprintf(stderr, "%s provider setup: no catalog — run `%s catalog sync` first\n", brand.CLI, brand.CLI)
		return 1
	}
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}

	// Overview mode.
	if len(args) == 0 {
		return listProvidersNeedingKeys(cat, store, stdout)
	}

	// Setup mode for one provider.
	pid := args[0]
	p, ok := cat.Providers[pid]
	if !ok {
		fmt.Fprintf(stderr, "%s provider setup: %q not in catalog\n", brand.CLI, pid)
		if sugg := suggestProviders(cat, pid); len(sugg) > 0 {
			fmt.Fprintf(stderr, "did you mean: %s\n", strings.Join(sugg, ", "))
		}
		fmt.Fprintf(stderr, "run `%s provider setup` to list providers that need keys\n", brand.CLI)
		return 2
	}
	if len(p.Env) == 0 {
		fmt.Fprintf(stdout, "%s (%s) needs no API key — it's a local/keyless provider.\n", p.ID, p.Family())
		return 0
	}

	fmt.Fprintf(stdout, "setting up %s (family=%s)\n", p.ID, p.Family())
	if api := strings.TrimSpace(p.API); api != "" {
		fmt.Fprintf(stdout, "  endpoint: %s\n", api)
	}
	added := 0
	for _, env := range p.Env {
		if store.Has(env) {
			fmt.Fprintf(stdout, "  %s — already set (%s); skipping. Use `%s provider creds set %s` to change.\n",
				env, creds.MaskValue(store.Get(env)), brand.CLI, env)
			continue
		}
		// Reuse the exact prompt+persist+mask path of `creds set`: passing
		// just the name makes it read the value from stdin.
		if code := cmdCredsSet(store, []string{env}, stdout, stderr); code != 0 {
			return code
		}
		added++
	}

	if added == 0 {
		fmt.Fprintf(stdout, "%s already has all its credentials.\n", p.ID)
	} else {
		fmt.Fprintf(stdout, "\n%s is ready. Start the daemon with:\n", p.ID)
		fmt.Fprintf(stdout, "  %sPROVIDER=%s %sMODEL=%s %s\n",
			brand.EnvPrefix, p.ID, brand.EnvPrefix, firstModelID(p), brand.Binary)
		fmt.Fprintf(stdout, "(or `%s provider reload` if the daemon is already running)\n", brand.CLI)
	}
	return 0
}

// listProvidersNeedingKeys prints every keyed provider with its credential
// status. Keyless (local) providers are omitted — there's nothing to set up.
func listProvidersNeedingKeys(cat *catalog.Catalog, store *creds.Store, stdout io.Writer) int {
	type row struct {
		id, family string
		ready      bool
		missing    []string
	}
	var ready, pending []row
	for _, p := range cat.ProviderList() {
		if len(p.Env) == 0 {
			continue // keyless
		}
		r := row{id: p.ID, family: string(p.Family())}
		for _, env := range p.Env {
			if !store.Has(env) {
				r.missing = append(r.missing, env)
			}
		}
		r.ready = len(r.missing) == 0
		if r.ready {
			ready = append(ready, r)
		} else {
			pending = append(pending, r)
		}
	}

	fmt.Fprintf(stdout, "providers needing an API key: %d ready, %d unconfigured\n",
		len(ready), len(pending))

	if len(pending) > 0 {
		fmt.Fprintf(stdout, "\nunconfigured (run `%s provider setup <id>`):\n", brand.CLI)
		for _, r := range pending {
			fmt.Fprintf(stdout, "  %-28s family=%-18s needs: %s\n", r.id, r.family, strings.Join(r.missing, ", "))
		}
	}
	if len(ready) > 0 {
		fmt.Fprintf(stdout, "\nconfigured:\n")
		for _, r := range ready {
			fmt.Fprintf(stdout, "  %-28s family=%-18s [creds OK]\n", r.id, r.family)
		}
	}
	if len(pending) == 0 && len(ready) == 0 {
		fmt.Fprintf(stdout, "  (no keyed providers in the catalog — try `%s catalog sync`)\n", brand.CLI)
	}
	return 0
}

// suggestProviders returns up to 5 catalog provider ids that contain the query
// as a substring — a cheap "did you mean?" for typos.
func suggestProviders(cat *catalog.Catalog, query string) []string {
	q := strings.ToLower(query)
	var out []string
	for _, p := range cat.ProviderList() {
		if strings.Contains(strings.ToLower(p.ID), q) {
			out = append(out, p.ID)
			if len(out) == 5 {
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// firstModelID returns a deterministic first model id for the start-command
// hint (alphabetically smallest), mirroring compat.FirstModelID without
// importing the wire layer into the CLI.
func firstModelID(p *catalog.Provider) string {
	best := ""
	for id := range p.Models {
		if best == "" || id < best {
			best = id
		}
	}
	if best == "" {
		return "<model-id>"
	}
	return best
}
