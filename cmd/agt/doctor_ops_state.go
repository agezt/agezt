// SPDX-License-Identifier: MIT
//
// cmd/agt doctor state checks: checkExposure + checkBudget + checkJournal +
// checkCredentials + checkChannels + checkModelReadiness + budgetWarnPct.
// Extracted from doctor_ops.go during Day 211 god-file refactor (#54).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
)

func checkExposure(status map[string]any) doctorCheck {
	const name = "network exposure"
	servers, _ := status["http_servers"].([]any)
	if len(servers) == 0 {
		return ok(name, "no HTTP servers exposed (control plane is loopback-only)")
	}
	var exposed []string
	for _, raw := range servers {
		m, _ := raw.(map[string]any)
		if lb, _ := m["loopback"].(bool); !lb {
			n, _ := m["name"].(string)
			a, _ := m["addr"].(string)
			exposed = append(exposed, fmt.Sprintf("%s (%s)", n, a))
		}
	}
	if len(exposed) > 0 {
		return warn(name,
			fmt.Sprintf("%d HTTP server(s) reachable beyond localhost: %s", len(exposed), strings.Join(exposed, ", ")),
			"the agent (shell/file/http tools) is exposed to the network, gated only by a token — bind to 127.0.0.1 and front it with a TLS reverse proxy, or restrict with a firewall")
	}
	return ok(name, fmt.Sprintf("%d HTTP server(s), all loopback-bound", len(servers)))
}
// budgetWarnPct is the daily-spend fraction (%) at which the doctor starts
// warning — close enough to the ceiling that runs will soon be blocked.
const budgetWarnPct = 90.0
func checkBudget(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdBudget, nil)
	if err != nil {
		return ok("budget", "unavailable ("+err.Error()+")")
	}
	return budgetCheckFromBudget(res)
}
func budgetCheckFromBudget(res map[string]any) doctorCheck {
	const name = "budget"
	spent := mcFromAny(res["spent_mc"])
	ceiling := mcFromAny(res["ceiling_mc"])
	if ceiling <= 0 {
		return ok(name, fmt.Sprintf("%s spent today (no daily ceiling)", fmtUSD(spent)))
	}
	used := float64(spent) / float64(ceiling) * 100
	detail := fmt.Sprintf("%s / %s today (%.0f%%)", fmtUSD(spent), fmtUSD(ceiling), used)
	switch {
	case spent >= ceiling:
		return warn(name, detail+" — daily ceiling reached",
			"new runs are blocked until the daily spend window resets at UTC midnight")
	case used >= budgetWarnPct:
		return warn(name, detail+" — near the daily ceiling",
			"runs are blocked once the ceiling is hit (resets at UTC midnight); reduce usage to avoid mid-run failures")
	default:
		return ok(name, detail)
	}
}
func checkCredentials(status map[string]any) doctorCheck {
	const name = "aws creds"
	chain, _ := status["cred_chain"].(string)
	if chain == "" {
		return ok(name, "default chain (vault → env → ~/.aws → IMDS)")
	}
	for _, layer := range []string{"web_identity", "assume_role", "sso"} {
		if strings.Contains(chain, layer+"=") {
			return ok(name, chain+"  [keyless: "+layer+"]")
		}
	}
	return ok(name, chain)
}
func checkChannels(status map[string]any) doctorCheck {
	const name = "channels"
	chans, _ := status["channels"].([]any)
	if len(chans) == 0 {
		return ok(name, "no messaging channels configured")
	}
	var halfConfigured []string
	inbound := 0
	for _, raw := range chans {
		m, _ := raw.(map[string]any)
		isIn, _ := m["inbound"].(bool)
		addr, _ := m["addr"].(string)
		if isIn {
			inbound++
		}
		// A listen addr with inbound disabled = the endpoint is up but rejects
		// everything (missing secret/key). An addr-less outbound-only channel is a
		// deliberate, fine choice — not flagged.
		if addr != "" && !isIn {
			kind, _ := m["kind"].(string)
			halfConfigured = append(halfConfigured, fmt.Sprintf("%s (%s)", kind, addr))
		}
	}
	if len(halfConfigured) > 0 {
		return warn(name,
			fmt.Sprintf("%d channel(s) listening but inbound DISABLED: %s", len(halfConfigured), strings.Join(halfConfigured, ", ")),
			"set the channel's signing secret / public key (AGEZT_SLACK_SIGNING_SECRET / AGEZT_DISCORD_PUBLIC_KEY) so inbound messages are accepted, or unset the addr to run outbound-only")
	}
	return ok(name, fmt.Sprintf("%d configured, %d can receive commands", len(chans), inbound))
}
func checkModelReadiness(status map[string]any, cat *catalog.Catalog) doctorCheck {
	const name = "model readiness"
	model, _ := status["model"].(string)
	if model == "" || model == "mock" {
		return ok(name, "offline/mock model (no catalog capabilities to assess)")
	}
	if cat == nil {
		return ok(name, fmt.Sprintf("%s (catalog not synced — capabilities unknown)", model))
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ok(name, fmt.Sprintf("%s (not in catalog — capabilities unknown)", model))
	}
	if w := m.AgentWarnings(); len(w) > 0 {
		return warn(name, fmt.Sprintf("%s — %s", model, strings.Join(w, "; ")),
			"pick a tool-capable model (AGEZT_MODEL) or set AGEZT_MODEL_STRICT=on to fail fast")
	}
	return ok(name, model+" (agent-ready: advertises tool-use)")
}
func checkJournal(ctx context.Context, client *controlplane.Client, status map[string]any) doctorCheck {
	const name = "journal"
	if _, err := client.Call(ctx, controlplane.CmdJournalVerify, nil); err != nil {
		return fail(name, "hash chain verification failed: "+err.Error(),
			"the audit log may be tampered or truncated — investigate before trusting it")
	}
	head := intOfStatus(status["journal_head"])
	return ok(name, fmt.Sprintf("BLAKE3 hash chain verified (head seq=%d)", head))
}
