// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/netguard"
)

// cmdNetguard dispatches `agt netguard <subcommand>`. Today the only subcommand
// is `test` — the egress-policy preview (M105).
func cmdNetguard(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s netguard: subcommand required (test|log)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "test":
		return cmdNetguardTest(args[1:], stdout, stderr)
	case "log":
		return cmdNetguardLog(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s netguard: unknown subcommand %q (test|log)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdNetguardLog implements `agt netguard log [N] [--tenant <id>] [--since <dur>]
// [--json]` (M109) — the audit trail of egress connections the guard actually
// refused (a tool reaching for an internal/metadata address).
func cmdNetguardLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s netguard log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s netguard log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s netguard log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s netguard log [N] [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show egress connections the guard refused (tool tried to reach an internal/metadata address)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s netguard log: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdNetguardLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s netguard log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["blocks"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no blocked egress attempts journaled — nothing tried to reach an internal address.")
		return 0
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		when := "—"
		if ts > 0 {
			when = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		ip, _ := m["ip"].(string)
		reason, _ := m["reason"].(string)
		tool, _ := m["tool"].(string)
		fmt.Fprintf(stdout, "  %s  BLOCKED  %-18s via %-8s — %s\n", when, ip, tool, reason)
	}
	return 0
}

// ipVerdict is one resolved address and the guard's decision for it.
type ipVerdict struct {
	IP      string `json:"ip"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// guardFromEnv builds an egress guard mirroring how the daemon configures the
// http tool (M16): default-deny for internal/metadata addresses, relaxed only by
// AGEZT_HTTP_ALLOW_LOOPBACK / AGEZT_HTTP_ALLOW_PRIVATE. Link-local (cloud
// metadata) is never allowed. Run in the same environment as the daemon for a
// faithful preview.
func guardFromEnv(getenv func(string) string) *netguard.Guard {
	var opts []netguard.Option
	if getenv(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		opts = append(opts, netguard.AllowLoopback())
	}
	if getenv(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		opts = append(opts, netguard.AllowPrivate())
	}
	return netguard.New(opts...)
}

// classifyIPs runs the guard over each resolved IP, sorted for stable output.
func classifyIPs(g *netguard.Guard, ips []net.IP) []ipVerdict {
	out := make([]ipVerdict, 0, len(ips))
	for _, ip := range ips {
		ok, reason := g.Allowed(ip)
		out = append(out, ipVerdict{IP: ip.String(), Allowed: ok, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

// cmdNetguardTest implements `agt netguard test <host|ip> [--json]` (M105) — a
// daemon-free preview of the egress guard: resolve the target and report, per
// resolved IP, whether the http/browser tools would be allowed to connect. It
// catches SSRF traps (a public hostname that resolves to 169.254.169.254 or a
// private address) before a tool ever dials. Exit 0 = all allowed, 3 = at least
// one blocked, 2 = usage / resolution error.
func cmdNetguardTest(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	target := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s netguard test <host|ip> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "preview the egress guard: resolve <host> and report which IPs the http/browser tools may reach\n")
			fmt.Fprintf(stdout, "reflects %sHTTP_ALLOW_LOOPBACK / %sHTTP_ALLOW_PRIVATE in this environment; metadata is always blocked\n", brand.EnvPrefix, brand.EnvPrefix)
			fmt.Fprintf(stdout, "exit 0 = all allowed, 3 = at least one blocked\n")
			return 0
		default:
			if target != "" {
				fmt.Fprintf(stderr, "%s netguard test: one host or IP at a time (got extra %q)\n", brand.CLI, a)
				return 2
			}
			target = a
		}
	}
	if target == "" {
		fmt.Fprintf(stderr, "%s netguard test: a host or IP is required\n", brand.CLI)
		return 2
	}

	g := guardFromEnv(os.Getenv)

	var ips []net.IP
	if literal := net.ParseIP(target); literal != nil {
		ips = []net.IP{literal}
	} else {
		resolved, err := net.LookupIP(target)
		if err != nil {
			fmt.Fprintf(stderr, "%s netguard test: cannot resolve %q: %v\n", brand.CLI, target, err)
			return 2
		}
		ips = resolved
	}
	verdicts := classifyIPs(g, ips)

	if asJSON {
		anyBlocked := false
		for _, v := range verdicts {
			if !v.Allowed {
				anyBlocked = true
			}
		}
		code := encodeJSON(stdout, map[string]any{"target": target, "results": verdicts, "any_blocked": anyBlocked})
		if anyBlocked {
			return 3
		}
		return code
	}

	anyBlocked := false
	fmt.Fprintf(stdout, "egress test for %q:\n", target)
	for _, v := range verdicts {
		if v.Allowed {
			fmt.Fprintf(stdout, "  [ALLOW] %s\n", v.IP)
		} else {
			anyBlocked = true
			fmt.Fprintf(stdout, "  [BLOCK] %s — %s\n", v.IP, v.Reason)
		}
	}
	if anyBlocked {
		fmt.Fprintf(stdout, "at least one address is blocked — the http/browser tools would refuse this target.\n")
		return 3
	}
	fmt.Fprintf(stdout, "all resolved addresses are reachable by the http/browser tools.\n")
	return 0
}
