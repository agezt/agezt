// SPDX-License-Identifier: MIT

// Package acpcatalog discovers the Agent Client Protocol (ACP) coding agents
// installed on the host so AGEZT can drive ANY of them, not just one
// operator-configured command. It is the discovery layer over the acp_agent
// bridge (plugins/tools/acpagent): a curated catalog of the agents that speak
// ACP over stdio (Gemini CLI, Claude Code's adapter, Codex), plus read-only
// host detection (exec.LookPath + a bounded --version probe) — the same approach
// kernel/toolbox uses for CLI tools.
//
// Detection never launches the agent's ACP loop; it only checks presence and
// prints its version. Resolving a slug to a launch command lets the bridge tool
// delegate to whichever installed agent the operator (or a run) picks.
package acpcatalog

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Agent is one catalog entry: an ACP-speaking coding agent and how to launch /
// detect / install it.
type Agent struct {
	// Slug is the stable id used to select this agent (e.g. "gemini").
	Slug string `json:"slug"`
	// Name is the human label ("Gemini CLI").
	Name string `json:"name"`
	// Bin is the executable to look up on PATH to decide "installed".
	Bin string `json:"bin"`
	// Command is the full shell command that launches its ACP stdio loop, handed
	// to the acp_agent bridge (e.g. "gemini --experimental-acp").
	Command string `json:"command"`
	// VersionArgs prints the agent's version (default ["--version"]).
	VersionArgs []string `json:"version_args,omitempty"`
	// Description is a one-line summary for the UI.
	Description string `json:"description"`
	// Install is a copy-pasteable install hint (not auto-run — agent installs are
	// the operator's call; the UI shows it).
	Install string `json:"install,omitempty"`
	// Docs is a reference URL.
	Docs string `json:"docs,omitempty"`
}

func (a Agent) versionArgs() []string {
	if len(a.VersionArgs) > 0 {
		return a.VersionArgs
	}
	return []string{"--version"}
}

// Catalog is the curated set of ACP coding agents. The acp_agent bridge's own
// docs name these three as the canonical ACP agents; detection reports which are
// actually present on this host. Any other ACP binary still works via a custom
// AGEZT_ACP_AGENT_CMD — the catalog is the discoverable shortlist, not a limit.
var Catalog = []Agent{
	{
		Slug:        "gemini",
		Name:        "Gemini CLI",
		Bin:         "gemini",
		Command:     "gemini --experimental-acp",
		Description: "Google's Gemini CLI, which speaks ACP natively over stdio.",
		Install:     "npm install -g @google/gemini-cli",
		Docs:        "https://github.com/google-gemini/gemini-cli",
	},
	{
		Slug:        "claude-code",
		Name:        "Claude Code (ACP adapter)",
		Bin:         "claude-code-acp",
		Command:     "claude-code-acp",
		Description: "Anthropic's Claude Code, bridged to ACP via the Zed adapter.",
		Install:     "npm install -g @zed-industries/claude-code-acp",
		Docs:        "https://github.com/zed-industries/claude-code-acp",
	},
	{
		Slug:        "codex",
		Name:        "Codex CLI",
		Bin:         "codex",
		Command:     "codex acp",
		Description: "OpenAI's Codex CLI, driven through its ACP subcommand.",
		Install:     "npm install -g @openai/codex",
		Docs:        "https://github.com/openai/codex",
	},
}

// AgentStatus is one agent's detection result for the wire.
type AgentStatus struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Bin         string `json:"bin"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Install     string `json:"install,omitempty"`
	Docs        string `json:"docs,omitempty"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Path        string `json:"path,omitempty"`
	// Active is true when this agent's command is the configured default
	// (AGEZT_ACP_AGENT_CMD) — the one the acp_agent tool uses without an explicit
	// agent argument.
	Active bool `json:"active"`
}

// Inventory is the full discovery snapshot.
type Inventory struct {
	OS             string        `json:"os"`
	ActiveCommand  string        `json:"active_command,omitempty"`
	Agents         []AgentStatus `json:"agents"`
	InstalledCount int           `json:"installed_count"`
	MissingCount   int           `json:"missing_count"`
}

const versionTimeout = 3 * time.Second

func probeVersion(ctx context.Context, bin string, args []string) string {
	cctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return firstLine(string(out))
}

func firstLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 4096), 1<<16)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			return clip(line, 120)
		}
	}
	return clip(strings.TrimSpace(s), 120)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Detect probes every catalog agent concurrently (LookPath + bounded version
// probe). activeCmd is the configured default command (AGEZT_ACP_AGENT_CMD) so
// the matching catalog entry can be flagged Active. Read-only.
func Detect(ctx context.Context, activeCmd string) Inventory {
	inv := Inventory{OS: runtime.GOOS, ActiveCommand: strings.TrimSpace(activeCmd)}
	statuses := make([]AgentStatus, len(Catalog))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, a := range Catalog {
		st := AgentStatus{
			Slug: a.Slug, Name: a.Name, Bin: a.Bin, Command: a.Command,
			Description: a.Description, Install: a.Install, Docs: a.Docs,
			Active: commandMatchesAgent(inv.ActiveCommand, a),
		}
		if path, err := exec.LookPath(a.Bin); err == nil {
			st.Installed = true
			st.Path = path
			wg.Add(1)
			go func(idx int, bin string, va []string, base AgentStatus) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				base.Version = probeVersion(ctx, bin, va)
				statuses[idx] = base
			}(i, a.Bin, a.versionArgs(), st)
			continue
		}
		statuses[i] = st
	}
	wg.Wait()

	for _, st := range statuses {
		if st.Installed {
			inv.InstalledCount++
		} else {
			inv.MissingCount++
		}
		inv.Agents = append(inv.Agents, st)
	}
	return inv
}

// commandMatchesAgent reports whether the configured default command is this
// catalog agent (so the UI marks it Active). Matches the agent's binary as the
// command's first token, tolerating a full path / extra args.
func commandMatchesAgent(activeCmd string, a Agent) bool {
	activeCmd = strings.TrimSpace(activeCmd)
	if activeCmd == "" {
		return false
	}
	fields := strings.Fields(activeCmd)
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(fields[0])
	// Normalize a path/extension to the bare binary name.
	if i := strings.LastIndexAny(first, "/\\"); i >= 0 {
		first = first[i+1:]
	}
	first = strings.TrimSuffix(first, ".exe")
	return first == strings.ToLower(a.Bin)
}

// Find returns the catalog agent for a slug.
func Find(slug string) (Agent, bool) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	for _, a := range Catalog {
		if strings.EqualFold(a.Slug, slug) {
			return a, true
		}
	}
	return Agent{}, false
}

// Installed reports whether a catalog agent's binary is on PATH.
func Installed(a Agent) bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

// AnyInstalled reports whether at least one catalog agent is present on the host
// — used to decide whether the acp_agent bridge is worth registering even when
// no default command is configured.
func AnyInstalled() bool {
	for _, a := range Catalog {
		if Installed(a) {
			return true
		}
	}
	return false
}

// InstalledSlugs returns the slugs of catalog agents present on the host, for a
// helpful "available agents" error/message.
func InstalledSlugs() []string {
	out := make([]string, 0, len(Catalog))
	for _, a := range Catalog {
		if Installed(a) {
			out = append(out, a.Slug)
		}
	}
	return out
}

// ResolveCommand turns the acp_agent bridge's agent selector into a launch
// command. `ref` is the per-call selector and `fallback` is the operator's
// configured default (AGEZT_ACP_AGENT_CMD). ok=false means nothing usable could
// be resolved.
//
// SECURITY (CWE-78): `ref` is attacker-influenceable — it arrives in agent/LLM
// tool input (the acp_agent `agent` field), which prompt injection can steer.
// It is therefore treated STRICTLY as a catalog slug: it must name an installed
// catalog agent, and the resolved command is taken from the trusted catalog,
// never from the caller's string. A non-slug ref is rejected (ok=false) rather
// than executed as a raw shell line — that closes the arbitrary-command path the
// bridge would otherwise expose via `sh -c`/`cmd /C` in acpagent.spawnAgent.
// Raw/custom commands remain available only through `fallback`, which is
// operator-controlled (set from the environment, not from agent input) and is
// used solely when ref is empty.
func ResolveCommand(ref, fallback string) (cmd string, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		fallback = strings.TrimSpace(fallback)
		return fallback, fallback != ""
	}
	a, found := Find(ref)
	if !found {
		// Not a known catalog slug. Do NOT fall through to running it as a raw
		// command: agent input must not be able to inject an arbitrary shell
		// line. The operator's raw escape hatch is `fallback` (ref == "").
		return "", false
	}
	if !Installed(a) {
		return "", false
	}
	return a.Command, true
}
