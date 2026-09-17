// SPDX-License-Identifier: MIT

// Package builtintools registers the first-party tool specs into the
// kernel/toolreg registry (Phase 2.2) — the tool mirror of builtinchannels.
// Each spec's Build carries the construction logic that used to live inline in
// cmd/agezt/boot_tools.go, with env reads going through BuildDeps.Get so tests
// can drive registration with a map-backed environment.
//
// This first slice holds the five netguard-capable network tools: http,
// browser.read, browser.action (+ its ten verb tools via Built.Extra),
// web_search, and fetch. All five are Netguard:true, so Set.Configure wires
// the daemon's netguard.blocked audit publisher into each instance generically
// — including fetch, whose OnBlock field existed since M831 but was never
// wired by the old hand-listed wireNetguardAudit (LD-2 residue, fixed here).
package builtintools

import (
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/fetch"
)


// RegisterAll registers the built-in tool specs. Idempotent — toolreg.Register
// replaces by name, so calling it again (daemon boot + tests) is harmless.
// NOTE: every call re-registers fresh Specs, and the spec* helpers close over
// the concrete instance their own Build produced — so a Set built from one
// RegisterAll generation always Configures its OWN instances, never a later
// generation's.
//
// ORDER MATTERS twice over: Set.Descs() follows registration order and feeds
// the daemon's "tools :" banner line (kept identical to the old hand-assembled
// order — shell, file, network tools, injection batch, then the env-gated
// externals and plugins), and the "plugins" spec must stay LAST so its
// YieldOnConflict drop always loses to every in-process name. The ratchet test
// (ratchet_test.go) pins the exact ordered list.
func RegisterAll() {
	// Workspace pair — first in the boot banner, as always.
	toolreg.Register(specShell())
	toolreg.Register(specFile())
	// Netguard slice (Phase 2.2 PR 2): the egress-guarded network tools.
	toolreg.Register(toolreg.Spec{Name: "http", Netguard: true, Build: buildHTTP})
	toolreg.Register(toolreg.Spec{Name: "browser.read", Netguard: true, Build: buildBrowserRead})
	toolreg.Register(specBrowserAction())
	toolreg.Register(toolreg.Spec{Name: "web_search", Netguard: true, Build: buildWebSearch})
	toolreg.Register(specFetch())
	// Set*-injection batch (Phase 2.2 PR 3): tools whose post-Open deps used to
	// be injected via string-keyed downcasts in cmd/agezt/main.go.
	toolreg.Register(specConfig())
	toolreg.Register(specArtifacts())
	toolreg.Register(specDB())
	toolreg.Register(specCouncil())
	toolreg.Register(specConductor())
	toolreg.Register(specResearch())
	toolreg.Register(specCodeExec())
	// Kernel-bound zero-arg tools (Phase 2.2 PR 4): the captured-local Bind
	// sites that only needed the live kernel (+ baseDir). No banner descs.
	toolreg.Register(specSchedule())
	toolreg.Register(specRuns())
	toolreg.Register(specStanding())
	toolreg.Register(specSkill())
	toolreg.Register(specIntrospect())
	toolreg.Register(specOverseer())
	toolreg.Register(specToolForge())
	toolreg.Register(specMCP())
	toolreg.Register(specWorkflow())
	toolreg.Register(specWorkboard())
	// Late-bound trio (Phase 2.2 PR 5): wired by Set.ConfigureLate once the
	// live channels / board store exist. No banner descs — they keep their own
	// dedicated boot lines in main.go.
	toolreg.Register(specNotify())
	toolreg.Register(specSendMedia())
	toolreg.Register(specBoard())
	// Env-gated externals (Phase 2.2 PR 6) — banner order preserved.
	toolreg.Register(specCoding())
	toolreg.Register(specACPAgent())
	toolreg.Register(specHomeAssistant())
	toolreg.Register(specRemoteRun())
	// External plugin host — LAST, so in-process names always win conflicts.
	toolreg.Register(specPlugins())
}

// splitHosts appends the non-empty comma-separated entries of csv to dst.
func splitHosts(dst []string, csv string) []string {
	for h := range strings.SplitSeq(csv, ",") {
		if h = strings.TrimSpace(h); h != "" {
			dst = append(dst, h)
		}
	}
	return dst
}

// buildHTTP — default-ALLOW (M818, owner law: every capability open unless you
// opt out). Any PUBLIC host is reachable out of the box; the opt-OUT is a
// non-empty $AGEZT_HTTP_ALLOWED_HOSTS (comma-separated), which RESTRICTS the
// tool to just those hosts. The SSRF egress guard (loopback / private /
// cloud-metadata refused) is the hard floor and stays on regardless — relaxed
// only by the explicit AGEZT_HTTP_ALLOW_* flags below. So "open" means the
// public internet, not a pivot into co-located admin surfaces.
func specBrowserAction() toolreg.Spec {
	var ba *browser.ActionTool
	return toolreg.Spec{
		Name:     "browser.action",
		Netguard: true,
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			built, tool, err := buildBrowserAction(d)
			ba = tool
			return built, err
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if ba != nil && d.Artifacts != nil {
				ba.SetIndex(d.Artifacts)
			}
			return nil
		},
	}
}

// buildBrowserAction — opt-in stateless Playwright browser actions. This
// promotes the built-in browser-use skill's driver into a first-party governed
// tool when the operator has installed Playwright and explicitly enables it.
// The ten per-verb tools ride along as Built.Extra, keyed by their names.
// Also returns the concrete instance (nil when gated off) for the spec's
// Configure closure.
func specFetch() toolreg.Spec {
	var fe *fetch.Tool
	return toolreg.Spec{
		Name:     "fetch",
		Netguard: true,
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			fe = fetch.New()
			if d.AllowAll {
				fe.AllowLoopback = true
				fe.AllowPrivate = true
			}
			return toolreg.Built{Tool: fe, Desc: "fetch(url→artifact)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.Artifacts != nil {
				fe.SetIndex(d.Artifacts)
			}
			return nil
		},
	}
}
