// SPDX-License-Identifier: MIT

package main

// `agt channel` — see communication-channel connectivity from the terminal
// (the CLI twin of the Channels wizard's status). Read-only; outbound sends
// stay on `agt send`.

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdChannel(args []string, stdout, stderr io.Writer) int {
	sub := ""
	rest := args
	if len(args) > 0 {
		sub = args[0]
		rest = args[1:]
	}
	switch sub {
	case "", "list", "ls":
		return cmdChannelList(rest, stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s channel list [--json]\n\n", brand.CLI)
		fmt.Fprintf(stdout, "  list   every registered channel with its live / configured / needs-setup status\n\n")
		fmt.Fprintf(stdout, "send a message with `%s send <channel> <to> <text>`.\n", brand.CLI)
		return 0
	default:
		fmt.Fprintf(stderr, "%s channel: unknown subcommand %q (list)\n", brand.CLI, sub)
		return 2
	}
}

func cmdChannelList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdChannelList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s channel list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	channels, _ := res["channels"].([]any)
	if len(channels) == 0 {
		fmt.Fprintf(stdout, "no channels registered\n")
		return 0
	}
	live, configured := 0, 0
	for _, raw := range channels {
		ch, _ := raw.(map[string]any)
		if ch == nil {
			continue
		}
		isLive, _ := ch["live"].(bool)
		isCfg, _ := ch["configured"].(bool)
		status := "needs setup"
		switch {
		case isLive:
			status = "● live"
			live++
		case isCfg:
			status = "configured (restart to start)"
			configured++
		}
		dir := "→ outbound"
		if b, _ := ch["duplex"].(bool); b {
			dir = "⇄ two-way"
		}
		fmt.Fprintf(stdout, "%-14s %-22s %-30s %s\n", str(ch["kind"]), str(ch["display"]), status, dir)
	}
	fmt.Fprintf(stdout, "%d channel(s) · %d live · %d configured (need a restart)\n", len(channels), live, configured)
	return 0
}
