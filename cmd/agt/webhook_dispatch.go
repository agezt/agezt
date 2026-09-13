// SPDX-License-Identifier: MIT

package main

// agt webhook TEST subcommand: cmdWebhookTest. Carved out of
// webhook.go during the Day 193 god-file split so the main file
// can stay focused on the dispatcher (cmdWebhook) + the read-side
// subcommands (cmdWebhookLog + cmdWebhookStats).
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/webhook"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

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
		code := jsonout.Write(stdout, map[string]any{"results": results, "any_failed": anyFail})
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

