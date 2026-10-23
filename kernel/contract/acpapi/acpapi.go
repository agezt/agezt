// SPDX-License-Identifier: MIT

// Package acpapi is the contract the ACP-agent bridge and the catalogue of
// installed ACP agents agree on.
//
// The **acp_agent** tool delegates a task to an EXTERNAL agent that speaks the
// Agent Client Protocol (Gemini CLI, Claude Code's adapter, Codex, …). It is a
// PLUGIN, and a plugin may reach the contract and the platform layers and
// nothing else — so the two things it used to reach for must be reachable:
//
//   - The PROTOCOL CLIENT, which speaks ACP over the agent process's stdio.
//     That one is infrastructure with no domain knowledge, so it lives in the
//     platform layer (kernel/platform/acpclient) — the plugin may reach it
//     directly, and no port is needed.
//
//   - The CATALOGUE of installed ACP agents — which agents this host has, and
//     which launch command a chosen slug maps to. That one is a MODULE's
//     business (kernel/modules/extensions/acpcatalog), and a plugin may not
//     name a module. So the catalogue answers through this contract's PORT,
//     and the daemon hands the port over.
//
// The port is deliberately narrow: three methods, and every one of them is
// something the bridge tool must ask before it may spawn a process.
//
//   - The rules of the road:
//   - Ports and the rules that belong to the port only.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package acpapi

// Catalog answers the bridge tool's questions about the ACP agents installed
// on this host.
//
// The implementation lives with the catalogue module (the curated list of
// agents, the host detection and the command resolution); the port is what a
// plugin may hold.
type Catalog interface {
	// AnyInstalled reports whether at least one catalogued ACP agent is
	// present on this host. The bridge tool registers itself only when this
	// is true (or when the operator configured a default command).
	AnyInstalled() bool
	// InstalledSlugs returns the slugs of the catalogued agents present on
	// this host, for a helpful "available agents" message.
	InstalledSlugs() []string
	// ResolveCommand turns the bridge tool's agent selector into a launch
	// command: ref is the per-call selector (a catalog slug) and fallback is
	// the operator's configured default command. ok=false means nothing
	// usable could be resolved.
	//
	// SECURITY (CWE-78): ref is attacker-influenceable — it arrives in
	// agent/LLM tool input, which prompt injection can steer. The
	// implementation treats it STRICTLY as a catalog slug: it must name an
	// installed catalogued agent, and the resolved command is taken from the
	// trusted catalogue, never from the caller's string. A non-slug ref is
	// refused rather than executed as a raw shell line. Raw/custom commands
	// remain available only through fallback, which is operator-controlled.
	ResolveCommand(ref, fallback string) (cmd string, ok bool)
}
