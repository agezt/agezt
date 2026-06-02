// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdStatus implements `agt status` and `agt status --json`.
// One round-trip dashboard for the operator: client+daemon
// versions (with skew detection), uptime, halt state, in-flight
// work, tool count, journal head. The first thing to run when
// debugging "is my daemon healthy?".
func cmdStatus(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s status [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show daemon health overview (version, uptime, halt, runs, tools, delegation caps)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s status: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStatus, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s status: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		// Augment with client-side fields so the JSON output is
		// self-contained for downstream pipelines (CI checks the
		// skew without needing a second call).
		res["client_version"] = brand.Version
		res["client_protocol"] = brand.ProtocolVersion
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	daemonVer, _ := res["daemon"].(string)
	daemonProto := intOfStatus(res["protocol"])
	uptime := intOfStatus(res["uptime_seconds"])
	halted, _ := res["halted"].(bool)
	activeRuns := intOfStatus(res["active_runs"])
	toolCount := intOfStatus(res["tools"])
	journalHead := intOfStatus(res["journal_head"])

	state := "OK"
	if halted {
		state = "HALTED"
	}

	fmt.Fprintf(stdout, "%s: %s\n", brand.CLI, state)
	fmt.Fprintf(stdout, "  client    : %s (protocol v%d)\n", brand.Version, brand.ProtocolVersion)
	fmt.Fprintf(stdout, "  daemon    : %s (protocol v%d)\n", daemonVer, daemonProto)
	if daemonVer != brand.Version || daemonProto != brand.ProtocolVersion {
		// Skew can mean a half-upgraded install — operator probably
		// updated the binary but didn't restart, or restarted only
		// one side. Flag prominently rather than burying in a footnote.
		fmt.Fprintf(stdout, "  WARNING: client/daemon version skew — restart the daemon to align\n")
	}
	fmt.Fprintf(stdout, "  uptime    : %s\n", fmtUptime(uptime))
	fmt.Fprintf(stdout, "  runs      : %d active\n", activeRuns)
	fmt.Fprintf(stdout, "  tools     : %d registered\n", toolCount)
	fmt.Fprintf(stdout, "  journal   : head seq=%d\n", journalHead)

	// Scheduled autonomy (M130): how many intents are armed, and how many enabled.
	// Quiet when there are none so single-shot operators see no noise.
	if sched, _ := res["schedules"].(map[string]any); sched != nil {
		if total := intOfStatus(sched["total"]); total > 0 {
			fmt.Fprintf(stdout, "  schedules : %d (%d enabled)\n", total, intOfStatus(sched["enabled"]))
		}
	}
	// Tenants (M130) — only present when multi-tenancy is on.
	if _, ok := res["tenants"]; ok {
		fmt.Fprintf(stdout, "  tenants   : %d\n", intOfStatus(res["tenants"]))
	}
	// Pending HITL approvals (M130) — actionable: the operator is blocking a run.
	// Always shown so "0 waiting" is explicit, with a nudge when any are pending.
	pending := intOfStatus(res["pending_approvals"])
	if pending > 0 {
		fmt.Fprintf(stdout, "  approvals : %d PENDING — answer with `%s approvals`\n", pending, brand.CLI)
	} else {
		fmt.Fprintf(stdout, "  approvals : none pending\n")
	}

	// Delegation ceilings (M49) — make the M46–M48 governance legible: depth /
	// fan-out / spend caps in effect, or "off" when the delegate tool is
	// disabled. 0 fan-out / spend renders as "unbounded".
	if deleg, _ := res["delegation"].(map[string]any); deleg != nil {
		if enabled, _ := deleg["enabled"].(bool); !enabled {
			fmt.Fprintf(stdout, "  delegation: off\n")
		} else {
			fanout := "unbounded"
			if f := intOfStatus(deleg["max_fanout"]); f > 0 {
				fanout = fmt.Sprintf("≤%d", f)
			}
			spend := "unbounded"
			if sp := mcFromAny(deleg["max_spend_microcents"]); sp > 0 {
				spend = "≤" + fmtUSD(sp)
			}
			fmt.Fprintf(stdout, "  delegation: depth≤%d, fan-out %s, spend %s\n",
				intOfStatus(deleg["max_depth"]), fanout, spend)
		}
	}
	return 0
}

// intOfStatus mirrors mcFromAny/intOf — JSON decodes numbers as
// float64, so a direct int cast loses values >2^53. Status counts
// never reach that range (would imply quintillions of runs), so
// truncation here is harmless.
func intOfStatus(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// fmtUptime renders seconds as Hh Mm Ss with leading zero units
// suppressed: "5s", "2m 5s", "1h 2m 5s". Operators eyeball this
// at a glance — a raw integer "3725" is less useful than "1h 2m 5s".
func fmtUptime(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h == 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}
