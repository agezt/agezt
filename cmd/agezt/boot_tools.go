// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/kernel/redact"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	artifactstool "github.com/agezt/agezt/plugins/tools/artifacts"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	"github.com/agezt/agezt/plugins/tools/coding"
	conductortool "github.com/agezt/agezt/plugins/tools/conductor"
	configtool "github.com/agezt/agezt/plugins/tools/config"
	counciltool "github.com/agezt/agezt/plugins/tools/council"
	dbtool "github.com/agezt/agezt/plugins/tools/db"
	"github.com/agezt/agezt/plugins/tools/fetch"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	hatool "github.com/agezt/agezt/plugins/tools/homeassistant"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/peer"
	research "github.com/agezt/agezt/plugins/tools/research"
	"github.com/agezt/agezt/plugins/tools/shell"
	websearch "github.com/agezt/agezt/plugins/tools/websearch"
)

// buildTools constructs the first-party tool map for the kernel.
// Signature is preserved so callers (main.go + tests) see the same shape:
//
//	tools map + plugin manifest + per-tool policy cap map + human description.
func buildTools(baseDir string, stderr io.Writer, ward warden.Engine) (map[string]agent.Tool, []kernelruntime.PluginInfo, map[string]string, string, error) {
	out := map[string]agent.Tool{}
	var registered []string

	// Manifest of external plugins that successfully spawned.
	// Surfaced to the kernel via Config.Plugins so the control
	// plane can serve `agt plugin list`. Stays nil when no
	// AGEZT_PLUGINS entries are configured.
	var manifestEntries []kernelruntime.PluginInfo

	// Declared tool capabilities from plugin manifests (M900): prefixed tool
	// name → Edict capability the plugin claims to belong to. Validated by the
	// kernel at Open (unknown axes dropped). Stays nil without plugins.
	var toolCaps map[string]string

	// Workspace root — both the file tool (scoped to it) and the shell tool (runs
	// in it, M609) use this so `dir`/`ls` (shell) and `file read x` (file) agree
	// on what "here" is.
	wsRoot := workspaceRoot(baseDir)

	// shell — always registered, routed through Warden. Effective
	// isolation profile depends on host OS (M1.c: always ProfileNone
	// with the request journaled as a downgrade on non-Linux). Runs in the
	// shared workspace root (M609) so it sees the same files as the file tool.
	sh := shell.NewWithWarden(ward)
	sh.WorkDir = wsRoot
	sh.BaseDir = baseDir
	out["shell"] = sh
	registered = append(registered, "shell(warden=requested-namespace)")

	// file — scoped to the same workspace root.
	ft, err := filetool.NewWithCheckpoint(wsRoot, baseDir)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("file tool: %w", err)
	}
	out["file"] = ft
	registered = append(registered, "file(root="+ft.Root()+")")

	// config — the agent's window into the Config Center: read/write/register
	// settings (same registry + vault as the `agt config` CLI and HTTP routes).
	// The kernel is bound after runtime.Open (see bindConfigTool) so live-apply
	// fields (provider/model) can rebuild the provider in place.
	out["config"] = configtool.New(baseDir)
	registered = append(registered, "config()")

	// http — default-ALLOW (M818, owner law: every capability open unless you opt
	// out). Any PUBLIC host is reachable out of the box; the opt-OUT is a non-empty
	// $AGEZT_HTTP_ALLOWED_HOSTS (comma-separated), which RESTRICTS the tool to just
	// those hosts. The SSRF egress guard (loopback / private / cloud-metadata
	// refused) is the hard floor and stays on regardless — relaxed only by the
	// explicit AGEZT_HTTP_ALLOW_* flags below. So "open" means the public internet,
	// not a pivot into co-located admin surfaces.
	ht := httptool.New()
	httpRestricted := false
	if hostsCSV := os.Getenv(brand.EnvPrefix + "HTTP_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				ht.AllowedHosts = append(ht.AllowedHosts, h)
			}
		}
		httpRestricted = len(ht.AllowedHosts) > 0
	}
	if !httpRestricted {
		ht.AllowAll = true // default-allow: no allowlist pinned ⇒ any public host
	}
	// Master permissive switch (M611): AGEZT_ALLOW_ALL=1 implies the full open
	// posture for the network tools too — any host, including loopback and the
	// private network — so "everything allowed" really means everything. It also
	// overrides a pinned allowlist back to open.
	allowAll := os.Getenv(brand.EnvPrefix+"ALLOW_ALL") == "1"
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_ALL") == "1" {
		ht.AllowAll = true
		httpRestricted = false
	}
	// Egress guard (M16): by default the http tool refuses internal/metadata
	// addresses even for allowlisted/AllowAll hosts. Relax per range for local use.
	egress := "guarded"
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		ht.AllowLoopback = true
		egress = "loopback-ok"
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		ht.AllowPrivate = true
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(stderr, "WARNING: AGEZT_HTTP_ALLOW_PRIVATE=1 lets the http tool reach the private network.")
	}
	out["http"] = ht
	if httpRestricted {
		registered = append(registered, fmt.Sprintf("http(hosts=%d, egress=%s)", len(ht.AllowedHosts), egress))
	} else {
		registered = append(registered, fmt.Sprintf("http(any host, egress=%s)", egress))
	}

	// browser.read — same allowlist pattern as http (uses AGEZT_BROWSER_*
	// env vars; deliberately separate from http's allowlist so operators
	// can grant browser-read access to a wider domain set than POSTs).
	// Same default-ALLOW posture as http (M818): any public host out of the box;
	// a non-empty AGEZT_BROWSER_ALLOWED_HOSTS is the opt-OUT that restricts it.
	br := browser.New()
	browserRestricted := false
	if hostsCSV := os.Getenv(brand.EnvPrefix + "BROWSER_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				br.AllowedHosts = append(br.AllowedHosts, h)
			}
		}
		browserRestricted = len(br.AllowedHosts) > 0
	}
	if !browserRestricted {
		br.AllowAll = true // default-allow
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_ALL") == "1" {
		br.AllowAll = true
		browserRestricted = false
	}
	// Egress guard (M16): browser.read refuses internal/metadata addresses by
	// default, even for allowlisted/AllowAll hosts. Relax per range for local use.
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_LOOPBACK") == "1" {
		br.AllowLoopback = true
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_PRIVATE") == "1" {
		br.AllowPrivate = true
		fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ALLOW_PRIVATE=1 lets browser.read reach the private network.")
	}
	// Browser cookies (M1.mm) — handled out-of-band by the browser.cookies tool
	// (registered when AGEZT_BROWSER_COOKIES=1); left as a no-op here.
	out["browser.read"] = br
	if browserRestricted {
		registered = append(registered, fmt.Sprintf("browser.read(hosts=%d)", len(br.AllowedHosts)))
	} else {
		registered = append(registered, "browser.read(any host)")
	}

	// browser.action — opt-in stateless Playwright browser actions. This promotes
	// the built-in browser-use skill's driver into a first-party governed tool
	// when the operator has installed Playwright and explicitly enables it.
	if os.Getenv(brand.EnvPrefix+"BROWSER_ACTIONS") == "1" {
		driver := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "BROWSER_ACTION_DRIVER"))
		if driver == "" {
			driver = browser.ResolveActionDriverPath()
		}
		if driver == "" {
			fmt.Fprintf(stderr, "WARNING: %sBROWSER_ACTIONS=1 but no browse.mjs driver was found; set %sBROWSER_ACTION_DRIVER.\n", brand.EnvPrefix, brand.EnvPrefix)
		} else if ba := browser.NewAction(os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_NODE"), driver); ba != nil {
			actionRestricted := false
			if hostsCSV := os.Getenv(brand.EnvPrefix + "BROWSER_ACTION_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
				for h := range strings.SplitSeq(hostsCSV, ",") {
					if h = strings.TrimSpace(h); h != "" {
						ba.AllowedHosts = append(ba.AllowedHosts, h)
					}
				}
				actionRestricted = len(ba.AllowedHosts) > 0
			}
			if !actionRestricted {
				ba.AllowAll = true
			}
			if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_ALL") == "1" {
				ba.AllowAll = true
				actionRestricted = false
			}
			if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_LOOPBACK") == "1" {
				ba.AllowLoopback = true
			}
			if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_PRIVATE") == "1" {
				ba.AllowPrivate = true
				fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_PRIVATE=1 lets browser.action reach private-network pages.")
			}
			if os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_USER_PROFILE") == "1" {
				if dir := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "BROWSER_ACTION_USER_DATA_DIR")); dir != "" {
					ba.AllowUserProfile = true
					ba.UserDataDir = dir
					fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1 lets browser.action run with an operator-configured persistent browser profile.")
				} else {
					fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1 ignored because AGEZT_BROWSER_ACTION_USER_DATA_DIR is empty.")
				}
			}
			if os.Getenv(brand.EnvPrefix+"BROWSER_ACTION_ALLOW_REMOTE_CDP") == "1" {
				if cdpURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "BROWSER_ACTION_REMOTE_CDP_URL")); cdpURL != "" {
					ba.AllowRemoteCDP = true
					ba.RemoteCDPURL = cdpURL
					fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP=1 lets browser.action attach to an operator-configured remote Chrome DevTools endpoint.")
				} else {
					fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP=1 ignored because AGEZT_BROWSER_ACTION_REMOTE_CDP_URL is empty.")
				}
			}
			if sessionDir := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "BROWSER_ACTION_SESSION_DIR")); sessionDir != "" {
				ba.SessionRoot = sessionDir
			} else {
				ba.SessionRoot = filepath.Join(baseDir, "browser-sessions")
			}
			out["browser.action"] = ba
			for _, tool := range browser.NewActionVerbTools(ba) {
				out[tool.Definition().Name] = tool
			}
			if actionRestricted {
				registered = append(registered, fmt.Sprintf("browser.action+verbs(hosts=%d)", len(ba.AllowedHosts)))
			} else {
				registered = append(registered, "browser.action+verbs(any public host)")
			}
		}
	}

	// web_search — keyword search against a public engine, returning result
	// titles/URLs/snippets. The capability that lets the agent DISCOVER a URL
	// (then fetch it with http/browser.read), not just fetch one it was handed.
	// The engine host is fixed, so there's no host allowlist; the egress guard
	// still refuses internal/metadata addresses (relaxed under ALLOW_ALL for
	// parity with the other network tools). Always registered — no secret needed.
	ws := websearch.New()
	if allowAll {
		ws.AllowLoopback = true
		ws.AllowPrivate = true
	}
	out["web_search"] = ws
	registered = append(registered, "web_search(duckduckgo)")

	// fetch — download a URL's bytes and save them as a browsable artifact (M831),
	// so the agent can keep an image/PDF/file it finds (it shows up in Files). Same
	// SSRF-guarded egress as the other network tools; the artifact index is injected
	// after the kernel opens. Always registered.
	fe := fetch.New()
	if allowAll {
		fe.AllowLoopback = true
		fe.AllowPrivate = true
	}
	out["fetch"] = fe
	registered = append(registered, "fetch(url→artifact)")

	// artifacts — list/read/delete the files the agent has saved (fetch downloads,
	// offloaded tool outputs, inbound images), so a file from one run is usable in a
	// later one (M832). A read/list/delete view over the artifact index injected
	// after the kernel opens. Always registered.
	af := artifactstool.New()
	out["artifacts"] = af
	registered = append(registered, "artifacts(list/read/delete)")

	// db — the Personal Data Lake (M834): agent-built, multi-agent-shared structured
	// collections (expenses, tasks, notes, contacts, …) the human can also browse in
	// the Data view. The lake is injected after the kernel opens. Always registered.
	dbt := dbtool.New()
	out["db"] = dbt
	registered = append(registered, "db(data-lake)")

	// council — the Council of Elders (M837): convene a multi-model panel for a
	// hard decision and get a reconciled consensus. The kernel runner is injected
	// after Open. Always registered.
	ct := counciltool.New()
	out["council"] = ct
	registered = append(registered, "council(consensus panel)")

	// conductor — the Conductor (M997): an asymmetric, verify-driven panel
	// (Thinker/Worker/Verifier) that runs the worker's code and loops until it
	// passes. The kernel runner + code-exec backend are injected after Open.
	// Always registered.
	cond := conductortool.New()
	out["conductor"] = cond
	registered = append(registered, "conductor(verify-driven panel)")

	// research — the deep-research harness (M1001): decompose a question,
	// gather independent web sources via web_search + browser.read, and
	// synthesize a citation-grounded report. The kernel runner is injected
	// after Open. Always registered; the underlying searches/fetches are each
	// gated by their own capability inside RunTool.
	rt := research.New()
	out["research"] = rt
	registered = append(registered, "research(deep-research harness)")

	// coding — external coding-agent bridge (P6-CODE). Registered only when
	// AGEZT_CODING_CMD is set (the command that runs Claude Code / Codex / Aider
	// / any agent). It operates on a git worktree of the workspace and returns a
	// diff; it never merges. Off by default.
	if codingCmd := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "CODING_CMD")); codingCmd != "" {
		if ct := coding.New(codingCmd, coding.AbsRepo(wsRoot)); ct != nil {
			out["coding"] = ct
			registered = append(registered, "coding(external agent)")
		}
	}

	// code_exec — the agent writes & runs Python/JS/TS in a sandboxed workspace
	// under <baseDir>/sandbox (M683). Routed through the same warden as shell, with
	// a scrubbed env (no secrets), per-call ephemeral dirs (or named persistent
	// projects), and resource caps. Registered when at least one runtime is present,
	// UNLESS AGEZT_SANDBOX=off. Network is on by default; AGEZT_SANDBOX_NO_NET=1
	// forces it off. Bound to the kernel bus after it opens (for the code.executed
	// event), via the returned tools map in the daemon run path.
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"SANDBOX")), "off") {
		if rt := codeexec.DetectRuntimes(); len(rt) > 0 {
			netOn := os.Getenv(brand.EnvPrefix+"SANDBOX_NO_NET") != "1"
			ce := codeexec.NewWithWarden(ward, filepath.Join(baseDir, "sandbox"), rt, netOn)
			out["code_exec"] = ce
			netTag := "net=on"
			if !netOn {
				netTag = "net=off"
			}
			registered = append(registered, fmt.Sprintf("code_exec(langs=%s, %s)", strings.Join(ce.Languages(), "/"), netTag))
		}
	}

	// acp_agent — external ACP-agent bridge (SPEC-15 §3, the inverse of `agt
	// acp`). It drives an external agent speaking the Agent Client Protocol over
	// stdio (Gemini CLI, Claude Code's adapter, Codex, …) over JSON-RPC and relays
	// its answer. Registered when AGEZT_ACP_AGENT_CMD sets a default command OR any
	// catalog ACP agent is installed on the host (discovery via kernel/acpcatalog),
	// so a run can delegate to any installed agent by slug without configuration.
	acpCmd := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ACP_AGENT_CMD"))
	if at := acpagent.New(acpCmd, acpagent.AbsCwd(wsRoot)); at != nil {
		out["acp_agent"] = at
		registered = append(registered, "acp_agent(external agent)")
	}

	// homeassistant — read entity state + call services on the operator's Home
	// Assistant. Shares the channel's AGEZT_HOMEASSISTANT_URL/_TOKEN (same
	// instance), but is gated by its OWN allowlists so the outbound notify channel
	// can be configured without auto-exposing an actionable control surface:
	//   AGEZT_HOMEASSISTANT_TOOL_READ      — entity read allowlist (get_states)
	//   AGEZT_HOMEASSISTANT_TOOL_SERVICES  — service call allowlist (call_service)
	//   AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 — bypass the service allowlist (DANGEROUS)
	// Both allowlists are fail-closed; the tool registers only when URL+TOKEN are
	// set AND at least one axis is enabled (so bare channel config exposes nothing).
	{
		haURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_URL"))
		haTok := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
		read := splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOOL_READ"))
		services := splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOOL_SERVICES"))
		if os.Getenv(brand.EnvPrefix+"HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES") == "1" {
			services = append(services, "*")
			fmt.Fprintln(stderr, "WARNING: AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 lets the agent call ANY Home Assistant service.")
		}
		if haURL != "" && haTok != "" && (len(read) > 0 || len(services) > 0) {
			hat := hatool.New()
			hat.BaseURL = haURL
			hat.Token = haTok
			hat.ReadEntities = read
			hat.AllowedServices = services
			out["homeassistant"] = hat
			registered = append(registered, "homeassistant("+hat.Capabilities()+")")
		}
	}

	// remote_run — mesh delegation to a peer Agezt node over its REST API (M8).
	// Registered only when AGEZT_PEERS is set (name=url|token,…). A malformed
	// spec is a hard startup error so a misconfigured mesh is caught early.
	{
		peerSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PEERS"))
		tenantSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_PEERS"))
		if peerSpec != "" || tenantSpec != "" {
			peers, err := peer.ParsePeers(peerSpec)
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PEERS: %w", err)
			}
			// Per-tenant peer sets (M219): a tenant's runs route against its own peers,
			// falling back to the global set. Parsed/validated up front like AGEZT_PEERS.
			tenantPeers, terr := peer.ParseTenantPeers(tenantSpec)
			if terr != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_TENANT_PEERS: %w", terr)
			}
			if pt := peer.NewWithTenants(peers, tenantPeers); pt != nil {
				out["remote_run"] = pt
				if len(tenantPeers) > 0 {
					registered = append(registered, fmt.Sprintf("remote_run(%d peer(s), %d tenant override(s))", len(peers), len(tenantPeers)))
				} else {
					registered = append(registered, fmt.Sprintf("remote_run(%d peer(s))", len(peers)))
				}
			}
		}
	}

	// External plugins via AGEZT_PLUGINS env var (M1.y). Format:
	//   AGEZT_PLUGINS="<prefix>=<path> <args...>"[,...]
	// e.g. AGEZT_PLUGINS="search=/usr/local/bin/agezt-search,scrape=/opt/scraper"
	// Each plugin is spawned at daemon start; its tools register
	// under the given prefix. A plugin that fails to initialize is
	// logged to stderr and skipped; the daemon continues with
	// non-plugin tools so a broken plugin can't take down the
	// kernel.
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGINS")); spec != "" {
		// Parse pin spec first (M1.ff). A syntactically-bad pin is a
		// hard startup error — operators want fast feedback on a
		// security setting, not a silently-broken pin that lets a
		// modified binary slip through next reboot. Unknown pins
		// (for plugins not in AGEZT_PLUGINS) become warnings after
		// the plugin loop runs.
		var pins plugin.PinSpec
		if pinSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_PINS")); pinSpec != "" {
			parsed, err := plugin.ParsePinSpec(pinSpec)
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_PINS: %w", err)
			}
			pins = parsed
		}
		// Tool allowlist (M1.hh) — same hard-error semantics as pins.
		var allowedTools plugin.ToolAllowlistSpec
		if allowSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_TOOLS")); allowSpec != "" {
			parsed, err := plugin.ParseToolAllowlistSpec(allowSpec)
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_TOOLS: %w", err)
			}
			allowedTools = parsed
		}
		// Parse the spec up front (M223). A malformed entry — missing
		// '=', empty path, or a duplicate prefix — is a hard startup
		// error, matching the pin/allowlist specs parsed just above. A
		// repeated prefix used to spawn two processes whose tools then
		// collided with a misleading "in-process version" warning;
		// rejecting it surfaces the typo instead.
		entries, err := plugin.ParsePluginSpec(spec)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGINS: %w", err)
		}
		usedPrefixes := make([]string, 0, len(entries))
		// Scrub secrets of known formats from plugin stderr before it reaches the
		// daemon log (M229). Pattern-only (no literals) — a plugin leaks its own
		// secrets, not the daemon's.
		pluginRedactor := redact.New()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, e := range entries {
			prefix := e.Prefix
			usedPrefixes = append(usedPrefixes, prefix)
			cfg := plugin.Config{
				Path: e.Path,
				Args: e.Args,
				Logger: func(line string) {
					fmt.Fprintln(stderr, pluginLogLine(pluginRedactor, prefix, line))
				},
				PinnedHash:   pins[prefix],         // empty if no pin configured for this prefix
				AllowedTools: allowedTools[prefix], // nil if no allowlist for this prefix
			}
			p, err := plugin.Spawn(ctx, cfg)
			if err != nil {
				fmt.Fprintf(stderr, "WARNING: plugin %q (%s) failed to start: %v\n", prefix, e.Path, err)
				continue
			}
			pluginTools := p.Tools(prefix + ".")
			declaredCaps := p.ToolCapabilities(prefix + ".") // M900: manifest-declared policy axes
			loadedCount := 0
			for name, tool := range pluginTools {
				if _, conflict := out[name]; conflict {
					fmt.Fprintf(stderr, "WARNING: plugin %q tool %q conflicts with existing tool — keeping in-process version\n", prefix, name)
					continue
				}
				out[name] = tool
				loadedCount++
				if cap, ok := declaredCaps[name]; ok {
					if toolCaps == nil {
						toolCaps = map[string]string{}
					}
					toolCaps[name] = cap
				}
			}
			registered = append(registered, fmt.Sprintf("plugin:%s(%d tools)", prefix, len(pluginTools)))
			// Record manifest entry. tool_count is the post-conflict
			// count — what the model actually sees — not the raw
			// plugin advertisement, so the operator can spot when a
			// conflict shadowed a tool they expected.
			manifestEntries = append(manifestEntries, kernelruntime.PluginInfo{
				Prefix:       prefix,
				Path:         e.Path,
				Args:         append([]string(nil), e.Args...),
				ToolCount:    loadedCount,
				HashPinned:   pins[prefix] != "",
				AllowedTools: append([]string(nil), allowedTools[prefix]...),
			})
		}
		// Warn about pin entries that didn't match any spawned plugin
		// (typo, removed plugin, etc.) — surfaces stale config without
		// failing the daemon.
		for _, stale := range pins.UnusedPins(usedPrefixes) {
			fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGIN_PINS has entry for %q but no plugin with that prefix was loaded\n", stale)
		}
		for _, stale := range allowedTools.Unused(usedPrefixes) {
			fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGIN_TOOLS has entry for %q but no plugin with that prefix was loaded\n", stale)
		}
	}

	return out, manifestEntries, toolCaps, strings.Join(registered, ", "), nil
}

// councilSeatName labels the i-th council seat (M837): the first three get
// distinct elder names, the rest a numbered fallback.
func councilSeatName(i int) string {
	names := []string{"Elder Alpha", "Elder Beta", "Elder Gamma"}
	if i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("Elder %d", i+1)
}

// splitNonEmpty is defined in main.go (used 41+ times across this package);
// we re-use it here for the boot-time tool wiring helpers above.
