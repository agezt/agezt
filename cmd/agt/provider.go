package main

// Provenance: SPDX-License-Identifier: MIT cmd/agt `provider` top-level dispatcher +
//             subcommand dispatchers (cmdProvider, cmdProviderReload,
//             cmdProviderCreds). Extracted from provider.go during Day 211 god-file
//             refactor (#74). Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func cmdProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider: subcommand required %s\n", brand.CLI, cliProvider.listNames())
		return 2
	}
	if sub, ok := cliProvider.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s provider: unknown subcommand %q %s\n", brand.CLI, args[0], cliProvider.listNames())
	return 2
}
func cmdProviderReload(stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, opapi.ProviderReload, nil)
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
func cmdProviderCreds(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider creds: subcommand required %s\n", brand.CLI, cliProviderCreds.listNames())
		return 2
	}
	if sub, ok := cliProviderCreds.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s provider creds: unknown subcommand %q %s\n", brand.CLI, args[0], cliProviderCreds.listNames())
	return 2
}

// credsList lists the stored credentials. The store is opened here: a table
// entry cannot carry the resource the dispatcher used to open.
func credsList(args []string, stdout, stderr io.Writer) int {
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	return cmdCredsList(store, stdout, stderr)
}

// credsSet stores a credential.
func credsSet(args []string, stdout, stderr io.Writer) int {
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	return cmdCredsSet(store, args, stdout, stderr)
}

// credsRm removes a credential.
func credsRm(args []string, stdout, stderr io.Writer) int {
	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	return cmdCredsRm(store, args, stdout, stderr)
}
