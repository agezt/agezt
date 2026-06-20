# Demo: Plugin and Tool Governance

This is the fourth runnable positioning demo for AGEZT. It proves the claim from `docs/COMPARISON.md`:

> In-process and out-of-process tools are governed. High-risk tools have explicit effect metadata, bounds, audit hooks, and containment controls.

And from `docs/PLUGIN-SECURITY.md`:

> The daemon treats the plugin process's stdout as untrusted input. Every frame is size-capped, every callback is bounded, and a plugin crash never takes down the daemon.

## What this demo shows

1. **Tool registry visibility.** `agt tool list` shows the in-process tools the daemon advertises to the model — the first place to look when a tool isn't being called.
2. **Tool invocation audit.** `agt tool log` shows what the agent actually ran: tool name, input, output, ok/error, and latency.
3. **Policy gating of tools.** `agt edict test` dry-runs a policy decision against a tool capability, proving governance is runtime, not just UI.
4. **Plugin pin hashing.** `agt plugin hash` computes a BLAKE3-256 digest suitable for `AGEZT_PLUGIN_PINS`, proving the binary-integrity control is operational.
5. **Plugin list surface.** `agt plugin list` shows whether external plugins are loaded, their pin status, and their tool allowlists.
6. **Governance chain.** The full path from tool → policy decision → journal audit is demonstrable with `agt why`.

## What this demo does NOT show (honest limitation)

The echo daemon (`AGEZT_DEMO_ECHO=1`) has no external plugins loaded by default. To see a live external plugin spawn, pin verification, and crash recovery, the operator must:

1. Build a plugin (e.g., from `kernel/plugin/testdata/echoplugin/`)
2. Configure `AGEZT_PLUGINS` and `AGEZT_PLUGIN_PINS`
3. Start the daemon with those env vars

This demo proves the **governance surfaces** (tool list, tool log, edict test, plugin hash, plugin list) that plugin trust builds on. See `docs/PLUGIN-SECURITY.md` for the full plugin trust model.

## Prerequisites

- Go 1.26.4+ (see `go.mod`)
- Bash (Linux/macOS/git-bash). On Windows use Git Bash or WSL.
- No provider key. No network. No external LLM.

## Run it

From the repository root:

```bash
make build
bash examples/autonomous/plugin-governance/run.sh
```

Or with prebuilt binaries:

```bash
bash examples/autonomous/plugin-governance/run.sh /path/to/agezt /path/to/agt
```

## Positioning claims this proves

| Claim | How |
|---|---|
| Tools are governed, not just advertised | `agt edict test shell "rm -rf /"` → denied |
| Tool invocations are auditable | `agt tool log` shows what ran and how it ended |
| Plugin binary integrity is operational | `agt plugin hash` produces a BLAKE3-256 pin |
| Plugin trust surface is visible | `agt plugin list` shows pin status + allowlist |
| Governance is a chain, not a single gate | tool → edict → journal → `agt why` |
