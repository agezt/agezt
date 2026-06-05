// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/webhook"
)

// cmdWebhook dispatches `agt webhook <subcommand>` (M112) — visibility into
// outbound webhook delivery (webhook.delivered / webhook.failed).
func cmdWebhook(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s webhook: subcommand required (test|log|stats)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "test":
		return cmdWebhookTest(args[1:], stdout, stderr)
	case "log":
		return cmdWebhookLog(args[1:], stdout, stderr)
	case "stats":
		return cmdWebhookStats(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s webhook: unknown subcommand %q (test|log|stats)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdWebhookTest implements `agt webhook test [<url>] [--subject <pat>]
// [--secret <key>] [--json]` (M122) — a daemon-free probe that POSTs one
// synthetic webhook.test event to a sink, using the byte-identical body,
// headers, and HMAC signature a real delivery sends, so an operator can confirm
// a sink is reachable and accepts the format before relying on it (the natural
// companion to the `agt doctor` webhook check and `agt webhook stats`). With no
// <url> it probes every sink in AGEZT_WEBHOOKS — the same spec the daemon reads.
// Exit 0 = all sinks returned 2xx, 3 = at least one failed, 2 = usage error.
//
// This is an operator command POSTing to an operator-chosen URL (like curl), so
// it is intentionally not subject to the agent egress guard (netguard).
func cmdWebhookTest(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	url, subject, secret := "", "", ""
	subjectSet, secretSet := false, false
	takeVal := func(i *int, flag string) (string, bool) {
		if *i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s webhook test: %s needs a value\n", brand.CLI, flag)
			return "", false
		}
		*i++
		return args[*i], true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--subject":
			v, ok := takeVal(&i, "--subject")
			if !ok {
				return 2
			}
			subject, subjectSet = v, true
		case strings.HasPrefix(a, "--subject="):
			subject, subjectSet = strings.TrimPrefix(a, "--subject="), true
		case a == "--secret":
			v, ok := takeVal(&i, "--secret")
			if !ok {
				return 2
			}
			secret, secretSet = v, true
		case strings.HasPrefix(a, "--secret="):
			secret, secretSet = strings.TrimPrefix(a, "--secret="), true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s webhook test [<url>] [--subject <pat>] [--secret <key>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "POST one synthetic webhook.test event to a sink (exact body/headers/signature of a real delivery)\n")
			fmt.Fprintf(stdout, "with no <url>, probes every sink in %sWEBHOOKS; exit 0 = all 2xx, 3 = at least one failed\n", brand.EnvPrefix)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s webhook test: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if url != "" {
				fmt.Fprintf(stderr, "%s webhook test: one url at a time (got extra %q)\n", brand.CLI, a)
				return 2
			}
			url = a
		}
	}

	var sinks []webhook.Sink
	if url != "" {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			fmt.Fprintf(stderr, "%s webhook test: url must be http(s):// (got %q)\n", brand.CLI, url)
			return 2
		}
		s := webhook.Sink{URL: url, Subject: ">"}
		if subjectSet && subject != "" {
			s.Subject = subject
		}
		if secretSet {
			s.Secret = secret
		}
		sinks = []webhook.Sink{s}
	} else {
		if subjectSet || secretSet {
			fmt.Fprintf(stderr, "%s webhook test: --subject/--secret only apply with an explicit <url>\n", brand.CLI)
			return 2
		}
		spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOKS"))
		if spec == "" {
			fmt.Fprintf(stderr, "%s webhook test: no <url> given and %sWEBHOOKS is unset\n", brand.CLI, brand.EnvPrefix)
			return 2
		}
		parsed, err := webhook.ParseSinks(spec)
		if err != nil {
			fmt.Fprintf(stderr, "%s webhook test: %v\n", brand.CLI, err)
			return 2
		}
		if len(parsed) == 0 {
			fmt.Fprintf(stderr, "%s webhook test: %sWEBHOOKS has no sinks\n", brand.CLI, brand.EnvPrefix)
			return 2
		}
		sinks = parsed
	}

	// Probe under the SAME egress guard the daemon's dispatcher uses (M416), so a
	// test reflects what a real delivery will do: a sink pointing at loopback /
	// RFC1918 / metadata is refused unless the operator opted that range in.
	var guardOpts []netguard.Option
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_LOOPBACK") == "1" {
		guardOpts = append(guardOpts, netguard.AllowLoopback())
	}
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_PRIVATE") == "1" {
		guardOpts = append(guardOpts, netguard.AllowPrivate())
	}
	probeClient := netguard.New(guardOpts...).HTTPClient(webhook.DefaultTimeout)

	results := make([]webhook.ProbeResult, 0, len(sinks))
	anyFail := false
	for _, s := range sinks {
		ctx, cancel := context.WithTimeout(context.Background(), webhook.DefaultTimeout+2*time.Second)
		r := webhook.Probe(ctx, s, time.Now(), probeClient)
		cancel()
		results = append(results, r)
		if !r.OK() {
			anyFail = true
		}
	}

	if asJSON {
		code := encodeJSON(stdout, map[string]any{"results": results, "any_failed": anyFail})
		if anyFail {
			return 3
		}
		return code
	}

	okN := 0
	fmt.Fprintf(stdout, "webhook test — POST one synthetic webhook.test event:\n\n")
	for _, r := range results {
		sig := ""
		if r.Signed {
			sig = "  (signed)"
		}
		if r.OK() {
			okN++
			fmt.Fprintf(stdout, "  [OK]   %s  %d  in %s%s\n", r.URL, r.Status, r.Latency.Round(time.Millisecond), sig)
		} else if r.Err != "" {
			fmt.Fprintf(stdout, "  [FAIL] %s  — %s%s\n", r.URL, r.Err, sig)
		} else {
			fmt.Fprintf(stdout, "  [FAIL] %s  status %d (not 2xx)%s\n", r.URL, r.Status, sig)
		}
	}
	fmt.Fprintf(stdout, "\n%d ok, %d failed.\n", okN, len(results)-okN)
	if anyFail {
		return 3
	}
	return 0
}

func cmdWebhookLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	failedOnly := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--failed":
			failedOnly = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s webhook log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s webhook log [N] [--failed] [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show recent outbound webhook deliveries (2xx) and failures (exhausted retries)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s webhook log: unexpected arg %q\n", brand.CLI, a)
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
	if failedOnly {
		callArgs["failed"] = true
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWebhookLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s webhook log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["deliveries"].([]any)
	if len(rows) == 0 {
		if failedOnly {
			fmt.Fprintln(stdout, "no webhook failures journaled.")
		} else {
			fmt.Fprintln(stdout, "no webhook deliveries journaled yet.")
		}
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
		url, _ := m["url"].(string)
		kind, _ := m["event_kind"].(string)
		attempts := intOfStatus(m["attempts"])
		if ok, _ := m["ok"].(bool); ok {
			fmt.Fprintf(stdout, "  %s  OK      %-22s → %s  [%d, %d attempt(s)]\n",
				when, kind, url, intOfStatus(m["status"]), attempts)
		} else {
			errMsg, _ := m["error"].(string)
			fmt.Fprintf(stdout, "  %s  FAILED  %-22s → %s  after %d attempt(s): %s\n",
				when, kind, url, attempts, errMsg)
		}
	}
	return 0
}

func cmdWebhookStats(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s webhook stats: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook stats: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s webhook stats: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s webhook stats [--tenant <id>] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "aggregate webhook delivery: total, delivered, failed, failure rate, by URL\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s webhook stats: unexpected arg %q\n", brand.CLI, a)
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
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWebhookStats, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s webhook stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	total := intOfStatus(res["total"])
	suffix := ""
	if sinceLabel != "" {
		suffix = " in the last " + sinceLabel
	}
	if total == 0 {
		fmt.Fprintf(stdout, "no webhook deliveries%s.\n", suffix)
		return 0
	}
	rate, _ := res["failure_rate"].(float64)
	fmt.Fprintf(stdout, "webhook deliveries (over %d%s):\n\n", total, suffix)
	fmt.Fprintf(stdout, "  delivered    : %d\n", intOfStatus(res["delivered"]))
	fmt.Fprintf(stdout, "  failed       : %d\n", intOfStatus(res["failed"]))
	fmt.Fprintf(stdout, "  failure rate : %.1f%%\n", rate*100)
	if byURL, _ := res["by_url"].(map[string]any); len(byURL) > 0 {
		fmt.Fprintf(stdout, "\n  by URL:\n")
		urls := make([]string, 0, len(byURL))
		for u := range byURL {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		for _, u := range urls {
			c, _ := byURL[u].(map[string]any)
			fmt.Fprintf(stdout, "    %-40s delivered=%d failed=%d\n", u, intOfStatus(c["delivered"]), intOfStatus(c["failed"]))
		}
	}
	return 0
}
