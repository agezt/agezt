// SPDX-License-Identifier: MIT
//
// cmd/agt `provider creds` operations (openCredsStore, cmdCredsList,
// cmdCredsSet, cmdCredsRm).
// Extracted from provider.go during Day 211 god-file refactor (#74).
// Public API unchanged.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

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
				scoped := catalog.ProviderCredentialName(p.ID, env)
				if _, ok := nameToProvider[scoped]; !ok {
					nameToProvider[scoped] = p.ID
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
