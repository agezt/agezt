// SPDX-License-Identifier: MIT
//
// cmd/agt `status` top-level command (cmdStatus).
// Extracted from status.go during Day 211 god-file refactor (#81).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/tools/peer"
)

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

	c := dialpkg.New(stderr)
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
		// Mesh peers (M208) are a client-side config (AGEZT_PEERS), so augment here.
		// Names + URLs only — tokens are never emitted.
		if mesh := meshSummary(); len(mesh) > 0 {
			res["mesh"] = mesh
		}
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

	// Provider fallbacks (M280) — surface silent primary→backup fallbacks. Quiet
	// at zero (the healthy case); when non-zero it flags that a provider has been
	// erroring and the run was served by a backup (often the mock), which used to
	// be invisible without a journal dig.
	if fb, ok := res["provider_fallbacks"].(map[string]any); ok {
		if n := intOfStatus(fb["count"]); n > 0 {
			reason, _ := fb["last_reason"].(string)
			fmt.Fprintf(stdout, "  fallbacks : %d  ⚠ a provider errored; runs served by a backup\n", n)
			if strings.TrimSpace(reason) != "" {
				fmt.Fprintf(stdout, "              last: %s\n", reason)
			}
		}
	}

	// Configured messaging channels (M141) — Telegram / Slack / Discord. Quiet
	// when none configured so single-shot operators see no noise.
	if chans, _ := res["channels"].([]any); len(chans) > 0 {
		parts := make([]string, 0, len(chans))
		for _, raw := range chans {
			c, _ := raw.(map[string]any)
			kind, _ := c["kind"].(string)
			inbound, _ := c["inbound"].(bool)
			addr, _ := c["addr"].(string)
			allow := intOfStatus(c["allowlist"])
			mode := "outbound-only"
			if inbound {
				mode = "inbound"
				if addr != "" {
					mode += " @" + addr
				}
			}
			parts = append(parts, fmt.Sprintf("%s (%s, allow %d)", kind, mode, allow))
		}
		fmt.Fprintf(stdout, "  channels  : %s\n", strings.Join(parts, ", "))
	}

	// AWS credential chain (M307) — which keyless/ambient layer engaged (IRSA,
	// SSO, assume-role, IMDS). Quiet unless AWS credentials are configured, so
	// operators not on AWS see no noise. Lets an EKS operator confirm IRSA is
	// live from `agt status` rather than the boot banner.
	if cc, _ := res["cred_chain"].(string); cc != "" {
		fmt.Fprintf(stdout, "  aws creds : %s\n", cc)
	}

	// Mesh peers (M208) — the configured federation (AGEZT_PEERS), client-side. A
	// cheap config snapshot only (no health probe — that's `agt doctor` / `agt peers`);
	// tokens are redacted. Quiet when single-node so most operators see no noise.
	if peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS")); err == nil && len(peers) > 0 {
		fmt.Fprintf(stdout, "  mesh      : %s\n", peer.Describe(peers))
	}

	// Scheduled autonomy (M130): how many typed jobs are armed, and how many enabled.
	// Quiet when there are none so single-shot operators see no noise.
	if sched, _ := res["schedules"].(map[string]any); sched != nil {
		if line := scheduleStatusLine(sched); line != "" {
			fmt.Fprintf(stdout, "  schedules : %s\n", line)
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
			total := "unbounded"
			if t := intOfStatus(deleg["max_total"]); t > 0 {
				total = fmt.Sprintf("≤%d", t)
			}
			fmt.Fprintf(stdout, "  delegation: depth≤%d, fan-out %s, total %s, spend %s\n",
				intOfStatus(deleg["max_depth"]), fanout, total, spend)
		}
	}
	return 0
}
