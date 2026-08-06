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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/fetch"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	websearch "github.com/agezt/agezt/plugins/tools/websearch"
)

// RegisterAll registers the built-in tool specs. Idempotent — toolreg.Register
// replaces by name, so calling it again (daemon boot + tests) is harmless.
// NOTE: every call re-registers fresh Specs, and the spec* helpers close over
// the concrete instance their own Build produced — so a Set built from one
// RegisterAll generation always Configures its OWN instances, never a later
// generation's.
func RegisterAll() {
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
	// sites that only needed the live kernel (+ baseDir).
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
func buildHTTP(d toolreg.BuildDeps) (toolreg.Built, error) {
	ht := httptool.New()
	httpRestricted := false
	if hostsCSV := d.Get(brand.EnvPrefix + "HTTP_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		ht.AllowedHosts = splitHosts(ht.AllowedHosts, hostsCSV)
		httpRestricted = len(ht.AllowedHosts) > 0
	}
	if !httpRestricted {
		ht.AllowAll = true // default-allow: no allowlist pinned ⇒ any public host
	}
	// Master permissive switch (M611): AGEZT_ALLOW_ALL=1 implies the full open
	// posture for the network tools too — any host, including loopback and the
	// private network — so "everything allowed" really means everything. It also
	// overrides a pinned allowlist back to open.
	if d.AllowAll || d.Get(brand.EnvPrefix+"HTTP_ALLOW_ALL") == "1" {
		ht.AllowAll = true
		httpRestricted = false
	}
	// Egress guard (M16): by default the http tool refuses internal/metadata
	// addresses even for allowlisted/AllowAll hosts. Relax per range for local use.
	egress := "guarded"
	if d.AllowAll || d.Get(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		ht.AllowLoopback = true
		egress = "loopback-ok"
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		ht.AllowPrivate = true
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(d.Stderr, "WARNING: AGEZT_HTTP_ALLOW_PRIVATE=1 lets the http tool reach the private network.")
	}
	desc := fmt.Sprintf("http(any host, egress=%s)", egress)
	if httpRestricted {
		desc = fmt.Sprintf("http(hosts=%d, egress=%s)", len(ht.AllowedHosts), egress)
	}
	return toolreg.Built{Tool: ht, Desc: desc}, nil
}

// buildBrowserRead — same allowlist pattern as http (uses AGEZT_BROWSER_* env
// vars; deliberately separate from http's allowlist so operators can grant
// browser-read access to a wider domain set than POSTs). Same default-ALLOW
// posture as http (M818): any public host out of the box; a non-empty
// AGEZT_BROWSER_ALLOWED_HOSTS is the opt-OUT that restricts it.
func buildBrowserRead(d toolreg.BuildDeps) (toolreg.Built, error) {
	br := browser.New()
	browserRestricted := false
	if hostsCSV := d.Get(brand.EnvPrefix + "BROWSER_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		br.AllowedHosts = splitHosts(br.AllowedHosts, hostsCSV)
		browserRestricted = len(br.AllowedHosts) > 0
	}
	if !browserRestricted {
		br.AllowAll = true // default-allow
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ALLOW_ALL") == "1" {
		br.AllowAll = true
		browserRestricted = false
	}
	// Egress guard (M16): browser.read refuses internal/metadata addresses by
	// default, even for allowlisted/AllowAll hosts. Relax per range for local use.
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ALLOW_LOOPBACK") == "1" {
		br.AllowLoopback = true
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ALLOW_PRIVATE") == "1" {
		br.AllowPrivate = true
		fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ALLOW_PRIVATE=1 lets browser.read reach the private network.")
	}
	// Browser cookies (M1.mm) — handled out-of-band by the browser.cookies tool
	// (registered when AGEZT_BROWSER_COOKIES=1); left as a no-op here.
	desc := "browser.read(any host)"
	if browserRestricted {
		desc = fmt.Sprintf("browser.read(hosts=%d)", len(br.AllowedHosts))
	}
	return toolreg.Built{Tool: br, Desc: desc}, nil
}

// specBrowserAction wraps buildBrowserAction with the post-Open Configure hook:
// the artifact index is injected so screenshots/downloads become durable
// Files-view artifacts instead of temp-file-only paths. The hook closes over
// the concrete *browser.ActionTool its own Build produced — no string-keyed
// downcast, and nil when the spec gated itself off.
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
func buildBrowserAction(d toolreg.BuildDeps) (toolreg.Built, *browser.ActionTool, error) {
	if d.Get(brand.EnvPrefix+"BROWSER_ACTIONS") != "1" {
		return toolreg.Built{}, nil, nil
	}
	driver := strings.TrimSpace(d.Get(brand.EnvPrefix + "BROWSER_ACTION_DRIVER"))
	if driver == "" {
		driver = browser.ResolveActionDriverPath()
	}
	if driver == "" {
		fmt.Fprintf(d.Stderr, "WARNING: %sBROWSER_ACTIONS=1 but no browse.mjs driver was found; set %sBROWSER_ACTION_DRIVER.\n", brand.EnvPrefix, brand.EnvPrefix)
		return toolreg.Built{}, nil, nil
	}
	ba := browser.NewAction(d.Get(brand.EnvPrefix+"BROWSER_ACTION_NODE"), driver)
	if ba == nil {
		return toolreg.Built{}, nil, nil
	}
	actionRestricted := false
	if hostsCSV := d.Get(brand.EnvPrefix + "BROWSER_ACTION_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		ba.AllowedHosts = splitHosts(ba.AllowedHosts, hostsCSV)
		actionRestricted = len(ba.AllowedHosts) > 0
	}
	if !actionRestricted {
		ba.AllowAll = true
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_ALL") == "1" {
		ba.AllowAll = true
		actionRestricted = false
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_LOOPBACK") == "1" {
		ba.AllowLoopback = true
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_PRIVATE") == "1" {
		ba.AllowPrivate = true
		fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_PRIVATE=1 lets browser.action reach private-network pages.")
	}
	if d.Get(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_USER_PROFILE") == "1" {
		if dir := strings.TrimSpace(d.Get(brand.EnvPrefix + "BROWSER_ACTION_USER_DATA_DIR")); dir != "" {
			ba.AllowUserProfile = true
			ba.UserDataDir = dir
			fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1 lets browser.action run with an operator-configured persistent browser profile.")
		} else {
			fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1 ignored because AGEZT_BROWSER_ACTION_USER_DATA_DIR is empty.")
		}
	}
	if d.Get(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_REMOTE_CDP") == "1" {
		if cdpURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "BROWSER_ACTION_REMOTE_CDP_URL")); cdpURL != "" {
			ba.AllowRemoteCDP = true
			ba.RemoteCDPURL = cdpURL
			fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP=1 lets browser.action attach to an operator-configured remote Chrome DevTools endpoint.")
		} else {
			fmt.Fprintln(d.Stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP=1 ignored because AGEZT_BROWSER_ACTION_REMOTE_CDP_URL is empty.")
		}
	}
	if sessionDir := strings.TrimSpace(d.Get(brand.EnvPrefix + "BROWSER_ACTION_SESSION_DIR")); sessionDir != "" {
		ba.SessionRoot = sessionDir
	} else {
		ba.SessionRoot = filepath.Join(d.BaseDir, "browser-sessions")
	}
	extra := map[string]agent.Tool{}
	for _, tool := range browser.NewActionVerbTools(ba) {
		extra[tool.Definition().Name] = tool
	}
	desc := "browser.action+verbs(any public host)"
	if actionRestricted {
		desc = fmt.Sprintf("browser.action+verbs(hosts=%d)", len(ba.AllowedHosts))
	}
	return toolreg.Built{Tool: ba, Extra: extra, Desc: desc}, ba, nil
}

// buildWebSearch — keyword search against a public engine, returning result
// titles/URLs/snippets. The capability that lets the agent DISCOVER a URL
// (then fetch it with http/browser.read), not just fetch one it was handed.
// The engine host is fixed, so there's no host allowlist; the egress guard
// still refuses internal/metadata addresses (relaxed under ALLOW_ALL for
// parity with the other network tools). Always registered — no secret needed.
func buildWebSearch(d toolreg.BuildDeps) (toolreg.Built, error) {
	ws := websearch.New()
	if d.AllowAll {
		ws.AllowLoopback = true
		ws.AllowPrivate = true
	}
	return toolreg.Built{Tool: ws, Desc: "web_search(duckduckgo)"}, nil
}

// specFetch — download a URL's bytes and save them as a browsable artifact
// (M831), so the agent can keep an image/PDF/file it finds (it shows up in
// Files). Same SSRF-guarded egress as the other network tools; the artifact
// index is injected by Configure once the kernel owns it, so downloaded files
// are saved as browsable artifacts. Always registered.
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
