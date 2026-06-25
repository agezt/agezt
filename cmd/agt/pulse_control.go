// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdPulseAsks implements `agt pulse asks [--json]` (list) and
// `agt pulse asks {approve|reject} <issue_key>` (M1001) — the CLI half of the
// operator-approval bridge the Jarvis presence pillar drives. Approval re-emits the
// signal onto pulse.initiative.act (the act path); reject drops it.
func cmdPulseAsks(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var verb, key string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s pulse asks [--json]\n       %s pulse asks {approve|reject} <issue_key>\n", brand.CLI, brand.CLI)
			return 0
		case "approve", "reject":
			verb = a
		default:
			if verb != "" && key == "" {
				key = a
			} else {
				fmt.Fprintf(stderr, "%s pulse asks: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	if verb != "" && key == "" {
		fmt.Fprintf(stderr, "%s pulse asks %s: an <issue_key> is required\n", brand.CLI, verb)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Resolve verb: approve/reject one ask.
	if verb != "" {
		res, err := c.Call(ctx, controlplane.CmdPulseAskResolve, map[string]any{"issue_key": key, "approve": verb == "approve"})
		if err != nil {
			fmt.Fprintf(stderr, "%s pulse asks %s: %v\n", brand.CLI, verb, err)
			return 1
		}
		if asJSON {
			return encodeJSON(stdout, res)
		}
		if verb == "reject" {
			fmt.Fprintf(stdout, "dismissed %s\n", key)
			return 0
		}
		if acted, _ := res["acted"].(bool); acted {
			fmt.Fprintf(stdout, "approved %s — handed to the initiative responder\n", key)
		} else {
			fmt.Fprintf(stdout, "approved %s — enable the Initiative responder (agt standing) to act on it\n", key)
		}
		return 0
	}

	// List verb: show pending asks.
	res, err := c.Call(ctx, controlplane.CmdPulseAsks, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s pulse asks: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	asks, _ := res["asks"].([]any)
	if len(asks) == 0 {
		fmt.Fprintln(stdout, "no pending asks — the heartbeat hasn't raised anything for your verdict")
		return 0
	}
	fmt.Fprintf(stdout, "%d pending ask(s) — approve/reject with `%s pulse asks {approve|reject} <issue_key>`:\n", len(asks), brand.CLI)
	for _, a := range asks {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["issue_key"].(string)
		summary, _ := m["summary"].(string)
		source, _ := m["source"].(string)
		if summary == "" {
			summary = source
		}
		fmt.Fprintf(stdout, "  %-28s %s\n", key, summary)
	}
	return 0
}

// cmdPulseControl implements `agt pulse {status|pause|resume} [--json]`,
// driving the resident proactive engine (SPEC-03 §2).
func cmdPulseControl(sub string, args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s pulse %s [--json]\n", brand.CLI, sub)
			return 0
		default:
			fmt.Fprintf(stderr, "%s pulse %s: unexpected arg %q\n", brand.CLI, sub, a)
			return 2
		}
	}

	cmd := map[string]string{
		"status": controlplane.CmdPulseStatus,
		"pause":  controlplane.CmdPulsePause,
		"resume": controlplane.CmdPulseResume,
	}[sub]

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s pulse %s: %v\n", brand.CLI, sub, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	switch sub {
	case "pause":
		fmt.Fprintln(stdout, "pulse paused")
	case "resume":
		fmt.Fprintln(stdout, "pulse resumed")
	default: // status
		if enabled, _ := res["enabled"].(bool); !enabled {
			fmt.Fprintln(stdout, "pulse is disabled (AGEZT_PULSE=off)")
			return 0
		}
		running, _ := res["running"].(bool)
		state := "running"
		if !running {
			state = "paused"
		}
		beats, _ := res["beats"].(float64)
		dial, _ := res["dial"].(string)
		pending, _ := res["digest_pending"].(float64)
		fmt.Fprintf(stdout, "pulse %s · %d beats · dial=%s · digest pending=%d\n",
			state, int64(beats), dial, int64(pending))
		if obs, ok := res["observers"].([]any); ok && len(obs) > 0 {
			fmt.Fprintf(stdout, "observers:\n")
			for _, o := range obs {
				if name, ok := o.(string); ok {
					fmt.Fprintf(stdout, "  %s\n", name)
				}
			}
		}
	}
	return 0
}
