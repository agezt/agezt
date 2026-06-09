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
	"github.com/agezt/agezt/kernel/creds"
)

// cmdProvider handles `agt provider <subcommand>`. M1.o introduced
// `creds`; M1.p added `check`; M1.r adds `reload`.
func cmdProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider: subcommand required (creds, check, reload)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "creds":
		return cmdProviderCreds(args[1:], stdout, stderr)
	case "keys":
		return cmdProviderKeys(args[1:], stdout, stderr)
	case "check":
		return cmdProviderCheck(args[1:], stdout, stderr)
	case "log":
		return cmdProviderLog(args[1:], stdout, stderr)
	case "stats":
		return cmdProviderStats(args[1:], stdout, stderr)
	case "rejections":
		return cmdProviderRejections(args[1:], stdout, stderr)
	case "reload":
		return cmdProviderReload(stdout, stderr)
	case "setup":
		return cmdProviderSetup(args[1:], stdout, stderr)
	case "import":
		return cmdProviderImport(args[1:], stdout, stderr)
	case "cost":
		return cmdProviderCost(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s provider: unknown subcommand %q (creds, keys, check, cost, log, reload, setup, import)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdProviderReload triggers the daemon to re-read catalog files and
// the credentials vault, then rebuild the primary provider in place.
// Replaces the "restart the daemon" friction that creds set/rm
// previously printed.
func cmdProviderReload(stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderReload, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider reload: %v\n", brand.CLI, err)
		return 1
	}
	pc, _ := res["provider_count"].(float64)
	pr, _ := res["providers_reloaded"].(bool)
	if pr {
		fmt.Fprintf(stdout, "reloaded: catalog (%d providers) + vault → primary provider rebuilt\n", int(pc))
	} else {
		fmt.Fprintf(stdout, "reloaded: catalog (%d providers)\n", int(pc))
		if note, _ := res["note"].(string); note != "" {
			fmt.Fprintf(stdout, "note: %s\n", note)
		}
	}
	return 0
}

// cmdProviderCreds dispatches `agt provider creds <verb>`.
func cmdProviderCreds(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider creds: subcommand required (list, set, rm)\n", brand.CLI)
		return 2
	}
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	switch args[0] {
	case "list", "ls":
		return cmdCredsList(store, stdout, stderr)
	case "set":
		return cmdCredsSet(store, args[1:], stdout, stderr)
	case "rm", "remove", "del", "delete", "unset":
		return cmdCredsRm(store, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s provider creds: unknown subcommand %q (list, set, rm)\n", brand.CLI, args[0])
		return 2
	}
}

func openCredsStore(stderr io.Writer) (*creds.Store, error) {
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return nil, err
	}
	store := creds.NewStore(base)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return nil, err
	}
	return store, nil
}

// cmdCredsList shows vault entries grouped by which catalog provider
// the env-var name belongs to, when the catalog has been synced. Names
// the catalog hasn't surfaced are listed under "(other)" so operators
// can see uncategorised vault entries.
func cmdCredsList(store *creds.Store, stdout, stderr io.Writer) int {
	names := store.Names()
	if len(names) == 0 {
		fmt.Fprintf(stdout, "vault is empty (%s)\n", store.Path)
		fmt.Fprintf(stdout, "use `%s provider creds set <NAME> <value>` to add a credential\n", brand.CLI)
		return 0
	}

	// Group by catalog provider via the synced catalog (best-effort —
	// works even if no daemon is running).
	cat, _ := loadCatalogIfAny(stderr)
	byProvider := map[string][]string{}
	uncategorised := []string{}
	nameToProvider := map[string]string{}
	if cat != nil {
		for _, p := range cat.Providers {
			for _, env := range p.Env {
				if _, ok := nameToProvider[env]; !ok {
					nameToProvider[env] = p.ID
				}
			}
		}
	}
	for _, n := range names {
		if pid, ok := nameToProvider[n]; ok {
			byProvider[pid] = append(byProvider[pid], n)
		} else {
			uncategorised = append(uncategorised, n)
		}
	}

	fmt.Fprintf(stdout, "%d vault entr%s at %s\n\n", len(names), plural(len(names), "y", "ies"), store.Path)
	pids := make([]string, 0, len(byProvider))
	for pid := range byProvider {
		pids = append(pids, pid)
	}
	sort.Strings(pids)
	for _, pid := range pids {
		fmt.Fprintf(stdout, "  %s\n", pid)
		for _, n := range byProvider[pid] {
			fmt.Fprintf(stdout, "    %-40s = %s\n", n, creds.MaskValue(store.Get(n)))
		}
	}
	if len(uncategorised) > 0 {
		fmt.Fprintf(stdout, "  (other)\n")
		for _, n := range uncategorised {
			fmt.Fprintf(stdout, "    %-40s = %s\n", n, creds.MaskValue(store.Get(n)))
		}
	}
	return 0
}

// cmdCredsSet stores a credential. Usage:
//
//	agt provider creds set ANTHROPIC_API_KEY sk-...
//	agt provider creds set ANTHROPIC_API_KEY=sk-...   (= form also accepted)
//	agt provider creds set ANTHROPIC_API_KEY          (prompts via stdin)
//
// Reading the value from stdin avoids leaking it to shell history when
// operators paste interactively.
func cmdCredsSet(store *creds.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s provider creds set <NAME> [<value>]\n", brand.CLI)
		return 2
	}
	name := args[0]
	var value string

	// Support NAME=VALUE in one arg.
	if eq := strings.IndexByte(name, '='); eq >= 0 {
		value = name[eq+1:]
		name = name[:eq]
	} else if len(args) >= 2 {
		// Re-join in case the value had spaces and the shell split it.
		value = strings.Join(args[1:], " ")
	} else {
		// Prompt on stdin (line read; trims trailing newline).
		fmt.Fprintf(stdout, "value for %s: ", name)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(stderr, "%s: read stdin: %v\n", brand.CLI, err)
			return 1
		}
		value = strings.TrimRight(line, "\r\n")
	}

	if strings.TrimSpace(value) == "" {
		fmt.Fprintf(stderr, "%s: value is empty (use `provider creds rm %s` to remove)\n", brand.CLI, name)
		return 2
	}

	existed := store.Has(name)
	if err := store.Set(name, value); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s: save vault: %v\n", brand.CLI, err)
		return 1
	}
	verb := "stored"
	if existed {
		verb = "updated"
	}
	fmt.Fprintf(stdout, "%s %s = %s in %s\n", verb, name, creds.MaskValue(value), store.Path)
	fmt.Fprintf(stdout, "run `%s provider reload` to apply (or restart the daemon)\n", brand.CLI)
	return 0
}

// cmdCredsRm removes a credential from the vault.
func cmdCredsRm(store *creds.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s provider creds rm <NAME> [<NAME>...]\n", brand.CLI)
		return 2
	}
	removed := 0
	for _, name := range args {
		if store.Remove(name) {
			removed++
			fmt.Fprintf(stdout, "removed %s\n", name)
		} else {
			fmt.Fprintf(stderr, "%s: no vault entry %q\n", brand.CLI, name)
		}
	}
	if removed == 0 {
		return 1
	}
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s: save vault: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "run `%s provider reload` to apply (or restart the daemon)\n", brand.CLI)
	return 0
}

// loadCatalogIfAny reads <baseDir>/catalog from disk. Returns (nil, nil)
// when the catalog directory is empty or missing — cmdCredsList falls
// back to listing all vault entries under "(other)".
func loadCatalogIfAny(stderr io.Writer) (*catalog.Catalog, error) {
	base, err := paths.BaseDir()
	if err != nil {
		return nil, err
	}
	store := catalog.NewStore(base + "/catalog")
	cat, err := store.Load()
	if err != nil {
		// Non-fatal — the operator just won't see provider grouping.
		fmt.Fprintf(stderr, "warning: catalog load failed (continuing without grouping): %v\n", err)
		return nil, err
	}
	if cat == nil || len(cat.Providers) == 0 {
		return nil, nil
	}
	return cat, nil
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
