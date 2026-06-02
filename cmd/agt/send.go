// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSend implements `agt send --channel KIND --to ID <text...>` — push a one-off
// outbound message through a configured channel (Telegram/Slack/Discord). The
// manual egress complement to Pulse briefs and agent replies, for scripts/CI.
func cmdSend(args []string, stdout, stderr io.Writer) int {
	channel := ""
	to := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s send --channel KIND --to ID <text...>\n", brand.CLI)
			fmt.Fprintf(stdout, "push an outbound message through a configured channel\n")
			fmt.Fprintf(stdout, "  --channel KIND   telegram | slack | discord\n")
			fmt.Fprintf(stdout, "  --to ID          chat/channel id to deliver to\n")
			return 0
		case a == "--channel":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s send: --channel needs a value\n", brand.CLI)
				return 2
			}
			i++
			channel = args[i]
		case strings.HasPrefix(a, "--channel="):
			channel = strings.TrimPrefix(a, "--channel=")
		case a == "--to":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s send: --to needs a value\n", brand.CLI)
				return 2
			}
			i++
			to = args[i]
		case strings.HasPrefix(a, "--to="):
			to = strings.TrimPrefix(a, "--to=")
		default:
			rest = append(rest, a)
		}
	}

	text := strings.TrimSpace(strings.Join(rest, " "))
	if channel == "" || to == "" || text == "" {
		fmt.Fprintf(stderr, "usage: %s send --channel KIND --to ID <text...>\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSend, map[string]any{
		"channel": channel, "to": to, "text": text,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s send: %v\n", brand.CLI, err)
		return 1
	}
	_ = res
	fmt.Fprintf(stdout, "sent to %s/%s\n", channel, to)
	return 0
}
