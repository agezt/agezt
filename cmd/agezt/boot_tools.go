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
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/builtintools"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	artifactstool "github.com/agezt/agezt/plugins/tools/artifacts"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	"github.com/agezt/agezt/plugins/tools/coding"
	conductortool "github.com/agezt/agezt/plugins/tools/conductor"
	configtool "github.com/agezt/agezt/plugins/tools/config"
	counciltool "github.com/agezt/agezt/plugins/tools/council"
	dbtool "github.com/agezt/agezt/plugins/tools/db"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	hatool "github.com/agezt/agezt/plugins/tools/homeassistant"
	"github.com/agezt/agezt/plugins/tools/peer"
	research "github.com/agezt/agezt/plugins/tools/research"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// buildTools constructs the first-party tool map for the kernel:
//
//	tools map + registry Set + plugin manifest + per-tool policy cap map +
//	human description.
//
// The *toolreg.Set (Phase 2.2) carries the registry-built specs alongside the
// map so main.go can drive their post-Open phases (Set.Configure) against the
// SAME instances the map holds.
func buildTools(baseDir string, stderr io.Writer, ward warden.Engine) (map[string]agent.Tool, *toolreg.Set, []kernelruntime.PluginInfo, map[string]string, string, error) {
	// Make the built-in tool specs available to BuildAll below. Idempotent
	// (Register replaces by name), so tests calling buildTools repeatedly and
	// the daemon calling it at boot are both fine.
	builtintools.RegisterAll()

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
		return nil, nil, nil, nil, "", fmt.Errorf("file tool: %w", err)
	}
	out["file"] = ft
	registered = append(registered, "file(root="+ft.Root()+")")

	// config — the agent's window into the Config Center: read/write/register
	// settings (same registry + vault as the `agt config` CLI and HTTP routes).
	// The kernel is bound after runtime.Open (see bindConfigTool) so live-apply
	// fields (provider/model) can rebuild the provider in place.
	out["config"] = configtool.New(baseDir)
	registered = append(registered, "config()")

	// Registry-built tools (Phase 2.2): the netguard-capable network tools —
	// http, browser.read, browser.action(+verbs), web_search, fetch — are built
	// from their plugins/builtintools specs via kernel/toolreg. Construction
	// logic (env gating, allowlists, egress-guard flags) lives in each spec's
	// Build; the returned Set is handed to main.go so Set.Configure can wire the
	// post-Open phases (netguard audit publisher, later Set*-injection).
	// AGEZT_ALLOW_ALL is the master permissive switch (M611): it implies the
	// full open posture for the network tools too — any host, including
	// loopback and the private network.
	allowAll := os.Getenv(brand.EnvPrefix+"ALLOW_ALL") == "1"
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir:       baseDir,
		WorkspaceRoot: wsRoot,
		Warden:        ward,
		Stderr:        stderr,
		Get:           os.Getenv,
		AllowAll:      allowAll,
	})
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	for name, tool := range set.Tools() {
		out[name] = tool
	}
	registered = append(registered, set.Descs()...)

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
				return nil, nil, nil, nil, "", fmt.Errorf("AGEZT_PEERS: %w", err)
			}
			// Per-tenant peer sets (M219): a tenant's runs route against its own peers,
			// falling back to the global set. Parsed/validated up front like AGEZT_PEERS.
			tenantPeers, terr := peer.ParseTenantPeers(tenantSpec)
			if terr != nil {
				return nil, nil, nil, nil, "", fmt.Errorf("AGEZT_TENANT_PEERS: %w", terr)
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
				return nil, nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_PINS: %w", err)
			}
			pins = parsed
		}
		// Tool allowlist (M1.hh) — same hard-error semantics as pins.
		var allowedTools plugin.ToolAllowlistSpec
		if allowSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_TOOLS")); allowSpec != "" {
			parsed, err := plugin.ParseToolAllowlistSpec(allowSpec)
			if err != nil {
				return nil, nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_TOOLS: %w", err)
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
			return nil, nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGINS: %w", err)
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

	return out, set, manifestEntries, toolCaps, strings.Join(registered, ", "), nil
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
