// SPDX-License-Identifier: MIT
//
// cmd/agt `provider` top-level dispatcher + subcommand dispatchers
// (cmdProvider, cmdProviderReload, cmdProviderCreds).
// Extracted from provider.go during Day 211 god-file refactor (#74).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/keys"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider: subcommand required (connect, chatgpt, creds, keys, check, reload)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "creds":
		return cmdProviderCreds(args[1:], stdout, stderr)
	case "keys":
		return keys.Run(args[1:], stdout, stderr)
	case "connect":
		return cmdProviderConnect(args[1:], stdout, stderr)
	case "chatgpt":
		return cmdProviderChatGPT(args[1:], stdout, stderr)
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
		fmt.Fprintf(stderr, "%s provider: unknown subcommand %q (connect, chatgpt, creds, keys, check, cost, log, reload, setup, import)\n", brand.CLI, args[0])
		return 2
	}
}
func cmdProviderReload(stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
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
