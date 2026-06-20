# AGEZT Plugin Security Model

This document describes the security model for AGEZT's plugin system: how
plugins are isolated, verified, governed, and recovered. It covers both
out-of-process plugins (the primary model) and the MCP bridge, and is explicit
about what is and is not protected.

> This is a living document. It reflects the controls present in the source at
> the time of writing. Where a control is best-effort or incomplete, this
> document says so.

## Plugin types

AGEZT has two categories of plugins with very different trust profiles:

| Category | Runs where | Isolated? | Examples |
|---|---|---|---|
| **In-process** (built-in) | Daemon address space | No | `plugins/tools/*`, `plugins/channels/*`, `plugins/providers/*`, `plugins/builtinskills/*` |
| **Out-of-process** (external) | Separate OS process | Yes (process boundary) | Plugins via `AGEZT_PLUGINS`, MCP bridge |

**In-process plugins** are compiled into the daemon binary. They are trusted
code: a bug or vulnerability in an in-process plugin can crash the daemon or
access its memory. The security model for in-process plugins is code review +
the governance layer (Edict policy applies to their tool calls regardless).

**Out-of-process plugins** are the focus of this document. They run as separate
OS processes, communicate over line-delimited JSON on stdio, and are isolated
from the daemon by the process boundary.

## Trust model

```
┌──────────────────────────────────────────────────────────────┐
│  DAEMON (trusted)                                            │
│  plugin host · pin verifier · tool allowlist · callback cap  │
│  frame-size cap · process-group kill · crash recovery        │
└────────────────┬─────────────────────────────────────────────┘
                 │  line-delimited JSON over stdio (untrusted)
┌────────────────▼─────────────────────────────────────────────┐
│  PLUGIN PROCESS (semi-trusted)                               │
│  any language · any code · crash-isolated · pinned · capped  │
└──────────────────────────────────────────────────────────────┘
```

The daemon treats the plugin process's stdout as **untrusted input**. Every
frame is size-capped, every callback is bounded, and a plugin crash never
takes down the daemon.

---

## P1: Binary integrity — BLAKE3-256 pinning

**Threat.** A plugin binary is replaced (by an attacker, a broken update, or a
filesystem race) between the time the operator verified it and the time the
daemon spawns it.

**Control.** AGEZT supports per-plugin BLAKE3-256 hash pinning via the
`AGEZT_PLUGIN_PINS` environment variable:

```bash
AGEZT_PLUGINS="search=/opt/agezt-plugins/search,translate=/opt/agezt-plugins/translate"
AGEZT_PLUGIN_PINS="search=a1b2c3...(64 hex chars),translate=d4e5f6...(64 hex chars)"
```

At spawn time, the daemon hashes the plugin binary and compares it to the pin.
If they differ, the daemon refuses to start the plugin and journals the
mismatch. See `kernel/plugin/pin.go` (`VerifyPin`, `HashFile`).

**How to pin a plugin:**

```bash
# 1. Hash the binary you've verified.
agt plugin hash /opt/agezt-plugins/search
# → a1b2c3d4...

# 2. Set the pin in the daemon's environment.
export AGEZT_PLUGIN_PINS="search=a1b2c3d4..."

# 3. Start the daemon. If the binary changes, the daemon refuses to spawn it.
```

**Why BLAKE3, not GPG signatures.** Hash pinning answers the operator's actual
question — "did this binary change since I last verified it?" — without
requiring a public-key distribution story. The operator records the hash once,
personally, and the daemon enforces it on every spawn. See the design note in
`kernel/plugin/pin.go`.

**Path resolution.** The pin verifier resolves bare plugin names through
`$PATH` so the hashed file and the executed file are the same binary. A path
with a separator is used as-is. See `resolvePluginPath` in `kernel/plugin/pin.go`.

**Limitations.**

- Pinning is opt-in. Without `AGEZT_PLUGIN_PINS`, the daemon spawns plugins
  without hash verification. An unpinned plugin is trusted on first use.
- There is no code-signing model. The pin authenticates "these are the bytes
  the operator expected," not "this came from a trusted publisher."
- A pin for a plugin not present in `AGEZT_PLUGINS` is tolerated and reported
  as an unused pin (a likely typo), not a startup failure.

---

## P2: Process isolation and crash recovery

**Threat.** A plugin crashes, hangs, or enters an infinite loop, taking down
the daemon or blocking all agent work.

**Control.** Out-of-process plugins run as separate OS processes. The plugin
host manages their lifecycle:

- **Spawn.** The daemon launches the plugin via `os/exec` with stdin/stdout
  piped. The child runs in its own process group so teardown kills the whole
  tree, not just the direct child. See `kernel/plugin/host.go`, `kernel/plugin/pin.go`.
- **Crash isolation.** If a plugin exits unexpectedly, the host marks all its
  tools as unavailable. Subsequent invocations return a clear error. The daemon
  and all other plugins keep running. See the lifecycle note in
  `kernel/plugin/protocol.go`.
- **Process-group kill.** On shutdown or reload, the host sends a `shutdown`
  request, waits a short grace period, then kills the process group if the
  plugin didn't exit on its own. This prevents orphaned grandchildren.
  See `setProcessGroup` in `kernel/plugin/pin.go`.
- **Hot-reload.** A plugin can be reloaded (re-spawned) without restarting the
  daemon. The host tears down the old process and spawns a fresh one, re-running
  pin verification. See reload tests in `kernel/plugin/reload_test.go`.

**Limitations.**

- Process isolation is an OS process boundary, not a sandbox. A plugin process
  runs with the daemon's OS-level privileges. On Linux, the warden can apply
  best-effort rlimits if the plugin path goes through it (most external plugins
  do not). On Windows/macOS, there is no additional containment. See
  `docs/THREAT-MODEL.md` T3 for platform caveats.
- A plugin that hangs without exiting is bounded by the invoke timeout
  (`DefaultInvokeTimeout`), not by resource monitoring. A plugin that consumes
  excessive CPU or memory outside its tool invocations is not proactively killed.

---

## P3: Tool allowlist

**Threat.** A plugin advertises tools the operator did not intend to expose,
or a malicious plugin smuggles harmful tool definitions past configuration.

**Control.** The host applies a tool allowlist at spawn time. Only tools
declared in the allowlist are registered with the daemon's tool registry;
others are silently dropped. See `kernel/plugin/allowlist_test.go`,
`kernel/plugin/host.go`.

The allowlist is configured via `AGEZT_PLUGIN_TOOLS`:

```bash
# Only expose search and fetch from the mytools plugin.
AGEZT_PLUGIN_TOOLS="mytools=search,mytools=fetch"
```

A plugin whose tools are not in the allowlist simply has no tools visible to
the agent. This is fail-closed: without an allowlist entry, the tool does not
exist from the agent's perspective.

**Additionally:** a plugin may advertise at most `DefaultMaxAdvertisedTools`
(256) tools at initialize. A plugin exceeding this fails to spawn with
`ErrTooManyTools`. This bounds the memory cost of registration. See
`kernel/plugin/host.go`.

---

## P4: Frame-size and callback bounds

**Threat.** A plugin floods the daemon with large frames or unbounded callback
requests, causing memory exhaustion or goroutine proliferation.

**Controls.**

| Bound | Default | What it prevents | Source |
|---|---|---|---|
| `MaxFrameBytes` | 16 MiB | A single stdout frame from OOMing the daemon | `kernel/plugin/host.go` |
| `MaxConcurrentCallbacks` | 16 | Unbounded `host/invoke` goroutine spawning | `kernel/plugin/host.go` |
| `MaxAdvertisedTools` | 256 | Registration memory blow-up at spawn | `kernel/plugin/host.go` |
| `InvokeTimeout` | (configurable) | A hung tool invocation blocking forever | `kernel/plugin/host.go` |
| `InitTimeout` | (configurable) | A plugin that never initializes blocking boot | `kernel/plugin/host.go` |

When a frame exceeds `MaxFrameBytes`, the plugin is torn down (`markDead`) and
its tools become unavailable — the daemon is not killed. When a plugin exceeds
the callback cap, excess `host/invoke` requests are rejected with
`ErrTooManyCallbacks` rather than queued. See `kernel/plugin/callbacklimit_test.go`.

---

## P5: Host callbacks (host/invoke)

**Threat.** A plugin uses `host/invoke` to call daemon-side tools, potentially
amplifying its reach beyond its own tool set.

**Control.** The `host/invoke` mechanism lets a plugin call back into the
daemon to use a host-provided tool (e.g., fetching a URL through the governed
HTTP tool rather than opening its own socket). These callbacks are:

- **Bounded.** At most `DefaultMaxConcurrentCallbacks` (16) simultaneous
  callbacks per plugin. Excess requests are rejected. See
  `kernel/plugin/host.go`.
- **Governed.** A host callback runs a host-registered tool, which itself
  passes through the same Edict policy and trust ladder as any agent-initiated
  tool call. The plugin does not bypass governance.
- **Timeout-bounded.** Each callback is bounded by `InvokeTimeout`.

**Limitations.**

- By default, with no host tools registered, `host/invoke` returns an error.
  The daemon operator must explicitly register host tools for callbacks.
- A plugin that legitimately needs many concurrent callbacks (e.g., parallel
  fetches) is limited to 16 at once. This is intentional: the daemon's
  resources are not infinitely fungible.

---

## P6: Registry install verification

**Threat.** A plugin or skill installed from a remote registry is tampered in
transit or the registry itself serves a different binary than the index
promises.

**Control.** The `agt plugin registry <url> --install <name>` and
`agt skill registry <url> --install <name>` commands verify the downloaded
content against a BLAKE3 hash from the registry index **before** writing it to
the operator's plugin directory. See `kernel/market/verify.go`,
`kernel/plugin/pin.go` (`HashBytes`).

The flow:

1. Fetch the registry index (JSON).
2. Look up the requested plugin/skill name.
3. Download the binary/content.
4. Hash it with BLAKE3-256.
5. Compare to the hash in the index.
6. If they match, write to the plugin directory and print the env to enable.
7. If they differ, refuse to install.

**Limitations.**

- The registry index hash is trusted as-is. There is no signature on the index
  itself. An operator who trusts a malicious registry gets malicious plugins.
- The operator should verify the registry URL out-of-band (HTTPS, known
  publisher) before installing.

---

## P7: MCP bridge plugin

**Threat.** The MCP bridge exposes external MCP servers (stdio + SSE transports)
to the agent, broadening the tool surface beyond AGEZT's own governance.

**Control.** The MCP bridge (`plugins/external/mcpbridge/`) is itself an
out-of-process plugin. MCP tools it exposes are subject to:

- the same tool allowlist as any plugin tool
- the same Edict policy/trust decisions as any tool call
- the same frame-size and callback bounds

The bridge translates MCP wire protocol into AGEZT's internal plugin protocol.
From the daemon's perspective, MCP tools are plugin tools.

**Limitations.**

- An MCP server that opens its own network connections outside the bridge is
  not covered by AGEZT's netguard/SSRF controls. MCP server authors must
  implement their own egress safety.
- The MCP bridge supports stdio and SSE transports; transport-level security
  (TLS for SSE) is the MCP server's responsibility.

---

## P8: Environment and secret isolation

**Threat.** A plugin process inherits the daemon's environment, gaining access
to provider API keys, vault passphrases, or the control-plane token.

**Control.** The plugin host constructs the child's environment explicitly via
`Config.Env`. When `Env` is nil, the child inherits the parent's environment;
when set, only the specified variables are passed. The daemon's own boot code
sets plugin environments to include only what the plugin needs.

For the warden (shell, code_exec), a nil `Env` is treated as an **empty**
environment (the safe default). Plugin spawning uses the `Config.Env` field
directly, so the operator/daemon controls exactly what leaks.

**Limitations.**

- If the daemon sets `Config.Env = nil` for a plugin, the plugin inherits the
  full daemon environment, including secrets. The daemon should always pass a
  minimal env. Plugin authors should not assume secrets are available.
- A plugin running with the daemon's OS user can read the daemon's files
  (`creds.json`, `control.token`) directly from the filesystem, regardless of
  environment. Process isolation does not protect against filesystem access by
  the same user.

---

## P9: Protocol version compatibility check

**Threat.** A plugin compiled against a newer or older wire protocol than the host's produces
silent corruption or cryptic mid-run failures — a tool call arrives with a field the host doesn't
understand, or the host sends a method the plugin doesn't handle, and the agent gets a confusing
error instead of a clear "this plugin is incompatible."

**Control.** The plugin protocol now carries a `ProtocolVersion` constant in
`kernel/plugin/protocol.go` (currently `1`). Plugins echo it back in their `initialize` response
via the `protocol_version` field. The host's spawn path calls `checkProtocolVersion` immediately
after parsing the initialize response:

- If the plugin's version matches the host's, it loads normally.
- If the plugin omits the field entirely (zero value), it is treated as `1` for backward
  compatibility with plugins written before this field existed.
- If the versions differ, the host returns `ErrProtocolVersionMismatch` and tears down the
  partially-started plugin — the operator gets a clear error at spawn, not a mystery at runtime.

**Versioning policy.**

- A **major version bump** means a breaking wire change (new required fields, removed methods,
  changed semantics). The host rejects plugins with a different major version.
- A **minor change** (new optional field, new method) does **not** bump the version. Both host
  and plugin must tolerate unknown optional fields — the JSON wire shape is inherently
  forward-compatible for additive changes.

**Limitations.**

- The check is a spawn-time gate, not a continuous enforcement. A plugin that changes behavior
  mid-run after passing the version check is not caught by this mechanism (it is caught by the
  crash-isolation and frame-size bounds).
- Plugins written before this field was introduced pass by default (v1 back-compat). This means
  the version check cannot distinguish "v1 plugin" from "plugin that doesn't know about the field."
  This is intentional — it avoids breaking the existing plugin ecosystem.

See: `kernel/plugin/protocol.go` (`ProtocolVersion`, `InitializeResult`), `kernel/plugin/host.go`
(`checkProtocolVersion`, `ErrProtocolVersionMismatch`), `kernel/plugin/protocol_version_test.go`.

---

## Operator deployment checklist

Minimum posture for plugins in a production deployment:

1. **Pin every external plugin.** Run `agt plugin hash <path>` once, set
   `AGEZT_PLUGIN_PINS`, and never start the daemon without pins for
   security-sensitive plugins.
2. **Use the tool allowlist.** Set `AGEZT_PLUGIN_TOOLS` to expose only the
   tools you intend the agent to use.
3. **Verify registry sources.** Only install from registries you trust over
   HTTPS. The BLAKE3 verify protects against in-transit tampering, not against
   a malicious registry.
4. **Prefer out-of-process plugins.** For untrusted or experimental code, use
   the external plugin protocol, not an in-process plugin compiled into the
   daemon.
5. **Monitor plugin crashes.** A plugin that repeatedly crashes is visible in
   the daemon log and through `agt plugin list`. Investigate recurring crashes.
6. **Register host tools deliberately.** Only register host tools for
   `host/invoke` callbacks when the plugin genuinely needs them.
7. **Run the daemon as a dedicated user.** A plugin process shares the
  daemon's OS user. A dedicated user limits filesystem exposure.
8. **Check protocol version compatibility.** Plugins that advertise a
  `protocol_version` different from the host's are rejected at spawn. If a
  plugin fails to load with `ErrProtocolVersionMismatch`, update the plugin
  or the daemon so both speak the same protocol version.

---

## What is explicitly out of scope

- AGEZT does not cryptographically sign plugins. Verification is hash-based
  pinning, which authenticates bytes, not provenance.
- AGEZT does not sandbox plugin processes at the OS level (no seccomp, no
  namespaces for external plugins). Process isolation is the OS process
  boundary only.
- AGEZT does not protect against a plugin reading the daemon's files when both
  run as the same OS user.
- AGEZT does not rate-limit plugin tool invocations beyond the callback cap
  and invoke timeout. A plugin whose tool runs expensive work on every call is
  bounded by the governor/budget, not by plugin-specific rate limiting.
- The MCP bridge does not impose AGEZT's network egress controls on MCP
  servers. Egress safety is the MCP server's responsibility.

When in doubt, **pin and allowlist**. The plugin host is designed to fail
closed: without a pin, without an allowlist entry, and without a host tool
registration, the plugin cannot do harm beyond its own process.
