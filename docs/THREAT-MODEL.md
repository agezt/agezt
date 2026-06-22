# AGEZT Threat Model

This document describes the security-relevant threats AGEZT is designed to
mitigate, the controls that exist today, and — importantly — where those
controls are best-effort or platform-dependent. It is written for operators
deploying AGEZT and for contributors evaluating whether a change weakens the
posture.

AGEZT runs autonomous agents that execute tools, contact the network, spawn
processes, and act on behalf of an operator. That makes it a high-trust,
high-blast-radius system. The design philosophy is **secure-by-default,
fail-closed, auditable**, but no single control is a complete defense. Defense
is layered.

> This is a living document. It reflects the controls present in the source at
> the time of writing. It is not a promise of complete security. Where a control
> is best-effort, this document says so explicitly.

## Trust boundaries

```
┌──────────────────────────────────────────────────────────────────────┐
│  UNTRUSTED                                                           │
│  inbound channels (Telegram/Slack/Discord/email/…), web content,     │
│  LLM output, model-written code, plugin outputs, peer nodes          │
└───────────────────────────┬──────────────────────────────────────────┘
                            │  every crossing is gated + journaled
┌───────────────────────────▼──────────────────────────────────────────┐
│  SEMI-TRUSTED (the governed agent loop)                              │
│  Edict policy · trust ladder · tool allow/deny · approval gates      │
│  netguard · file containment · warden · scrubbed env · redaction     │
└───────────────────────────┬──────────────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────────────┐
│  TRUSTED (operator + daemon host)                                    │
│  vault keys · control-plane token · daemon process · host filesystem │
└──────────────────────────────────────────────────────────────────────┘
```

Every tool call crosses from semi-trusted toward trusted. The job of the
governance layer is to make each crossing explicit, authorized, and recorded.

---

## T1: Prompt injection

**Threat.** Untrusted text (web pages fetched by a tool, inbound channel
messages, file contents, LLM-composed intermediate output) contains
instructions intended to make the agent perform actions the operator did not
authorize: exfiltrate secrets, run destructive commands, approve its own
escalations.

**Controls.**

- **Policy engine (Edict).** Every tool call — loop-driven and operator-driven
  (`RunTool`) — passes through a capability + trust-level + hard-deny decision.
  See `kernel/edict/`, `kernel/agent/agent.go`, `kernel/runtime/toolrun.go`.
  A denial short-circuits the invocation and journals a `policy.decision`
  event.
- **Hard-deny floor.** Catastrophic command substrings (e.g. `rm -rf /`,
  cloud-metadata addresses) are refused regardless of the capability's trust
  level. Raising a level cannot unlock a hard-denied action.
- **Trust ladder.** Capabilities carry per-capability levels (L0..L4 /
  deny/ask/allow). An "Ask"-class level gates a call behind approval mode
  (allow / deny / prompt) so high-risk actions need a live grant.
- **Loop audit.** Tool calls and their policy verdicts are journaled, so an
  injected instruction that *attempted* a denied action is visible via
  `agt edict log`, `agt why`, and the Web UI diagnostics.
- **Observation trust labels.** Tool results carry observation-trust metadata
  (e.g. HTTP tool output is marked untrusted) so the runtime can treat
  tool-derived content as lower-trust context.

**Limitations / residual risk.**

- Policy decides whether a call *runs*; it does not sandproof the content of
  what the call returns. A fetched web page can still contain instructions that
  influence later reasoning. Defense is layered (policy + trust labels + the
  operator's review of approvals), not total.
- Prompt injection is not solvable by a single mechanism. AGEZT's posture is
  "contain the blast radius of a successful injection," not "prevent injection
  outright."

---

## T2: Tool misuse and destructive action

**Threat.** An agent (via error, confusion, or injection) issues a destructive
tool call: deleting files, overwriting config, posting to a channel, running a
harmful shell command.

**Controls.**

- **Capability gating + approval.** As above: each tool call is decided by
  capability and trust level; high-risk classes (`shell`, `file.write`,
  `http.post`, `code.exec`) can require approval before running.
- **Effect metadata.** Tools declare an effect class (reversible /
  compensable / irreversible), predicted effects, affected resources, and
  rollback notes. The runtime uses these to route decisions and to surface
  consequences to the operator. See `kernel/agent/schema.go` and tool
  definitions under `plugins/tools/`.
- **File containment.** The file tool resolves the workspace root with
  `filepath.EvalSymlinks`, rejects `..` and out-of-root absolute paths,
  resolves new-file ancestors to defeat symlinked-parent escapes, uses
  `O_NOFOLLOW` on writes to close the TOCTOU window, and refuses to delete the
  workspace root. See `plugins/tools/file/file.go`.
- **HTTP egress guard.** The HTTP tool is default-deny by host, blocks
  loopback / RFC1918 / link-local (cloud metadata) by default, re-checks the
  host allowlist on every redirect hop, and caps request/response sizes. See
  `plugins/tools/http/http.go`, `kernel/netguard/netguard.go`.
- **Output and time bounds.** Shell, HTTP, and code-exec tools cap output
  bytes and wall time so a runaway or hostile command cannot exhaust the
  journal/context budget or hang the daemon.

**Limitations.**

- Tools with `EffectIrreversible` (notably `shell` and `code_exec`) have no
  generic rollback. Defense is gating + audit, not undo.
- The operator is responsible for reviewing approval requests. Auto-approve
  modes weaken this control; they are an explicit operator choice.

---

## T3: Process and code-execution isolation

**Threat.** Model-written code or a shell command escapes its intended scope:
reads secrets from the environment, pivots to local services, fills the disk,
fork-bombs.

**Controls.**

- **Warden engine.** External execution (shell, code-exec, and plugin-spawned
  tools) goes through a single isolation engine that applies: a timeout with
  `WaitDelay` for orphaned children, output truncation, a scrubbed environment
  (the daemon's secrets are never inherited), a working-directory scope, and —
  on Linux — best-effort `prlimit64` CPU/memory/FD/file-size caps and
  process-group SIGKILL. See `kernel/warden/warden.go`.
- **Scrubbed environment.** A nil `Env` in a warden `Spec` is treated as an
  *empty* environment, not inheritance, so daemon secrets (`*_API_KEY`,
  tokens, `AGEZT_*`) are not leaked into untrusted children.
- **Code-exec sandbox.** Each `code_exec` run lands in an ephemeral scratch
  directory (or a namespaced project dir), validates extra file names against
  traversal, validates pip package names against flag-injection, and exposes
  only the sandbox directory to the program. See
  `plugins/tools/codeexec/codeexec.go`.
- **Honest effective-profile reporting.** When a stronger isolation profile is
  requested but unavailable, the warden downgrades transparently and journals a
  `warden.profile_downgraded` event. The effective profile is never overstated.

**Platform caveats (important).**

- On **Linux**, `ProfileNamespace` can engage `Setpgid` + best-effort rlimits.
  This hardens against accidental runaway. It is **not** a hardened sandbox
  against a determined escape: without unprivileged user namespaces (which need
  root or a sysctl change most operators don't set), rlimits are applied
  *after* the child starts, leaving a small allocation window. Treat it as
  accident-hardening, not containment-of-malice.
- On **Windows and macOS**, the cross-platform warden downgrades to
  `ProfileNone`: timeout, output cap, scrubbed env, and workdir apply, but
  there is **no process-level isolation**. Shell and code-exec run with the
  daemon's privileges. On these platforms, Edict policy and approval gates are
  the primary defense; do not assume the child is contained.
- **Deno** code-exec gets an OS-level filesystem jail (real on every platform,
  Windows included) when Deno is the runtime; Python and Node do not.

**Residual risk.** High-blast-radius tools (`shell`, `code_exec`) remain
high-blast-radius. Operators on Windows/macOS in particular should keep these
capabilities behind `ask`/`prompt` or deny them for untrusted agents.

---

## T4: Secret and credential exposure

**Threat.** Provider API keys, channel tokens, vault passphrases, or the
daemon's control-plane token are leaked via logs, the environment, journal
events, tool output, or filesystem access.

**Controls.**

- **Encrypted vault at rest.** Credentials are stored in an AES-256-GCM
  envelope, never plaintext on disk. Key derivation uses a high-iteration
  PBKDF2-style KDF with a stored-iteration floor (the daemon rejects a lowered
  iteration count). Passphrase rotation is supported. See
  `kernel/creds/encrypt.go`, `kernel/creds/machine.go`.
- **Scrubbed child environments.** As above: warden children never inherit the
  daemon's secrets by default.
- **Redaction.** A redaction layer (`kernel/redact/`) is applied before
  content is journaled or returned, to suppress secret-shaped values.
- **Constant-time token comparison.** The control-plane admin token and tenant
  tokens are compared in constant time to avoid timing side channels. See
  `kernel/controlplane/server.go`, `kernel/restapi/restapi.go`.
- **Provider credentials via stdin.** `agt provider setup` prompts on stdin,
  never argv, so secrets don't land in shell history.

**Limitations.**

- The vault's confidentiality rests on the passphrase and the host filesystem
  permissions. If the host is compromised, an attacker with the encrypted
  blob can mount an offline brute-force attack against the passphrase; the KDF
  iteration count raises the cost but does not make it impossible.
- Secrets that an operator places into tool inputs (e.g. pasting a token into
  an `http` tool body) are subject to that tool's own handling. Operators
  should keep secrets in the vault and reference them, not inline them.

---

## T5: Control-plane and API token exposure

**Threat.** The daemon's admin token authorizes every command on every tenant.
Anyone who can read it (from the runtime file, from a shared screen, from a
copied URL) can fully control the daemon.

**Controls.**

- **Loopback-only control plane.** The control plane binds to `127.0.0.1` on
  an ephemeral port. The token file is written with `0600` permissions.
- **Empty token fails closed.** An empty admin token never authorizes; a blank
  presented token never matches.
- **Tenant token scoping.** With multi-tenancy on, a per-tenant token
  authorizes only its own tenant (via `X-Agezt-Tenant`); the admin token still
  authorizes any.
- **Request body caps.** All network-facing surfaces cap request bodies
  (16 MiB) to bound pre-auth memory exhaustion.

**Limitations / operator responsibilities.**

- **Query-string tokens.** The Web UI and EventSource/SSE path carry the token
  as `?token=` because EventSource cannot set headers. Query tokens leak more
  easily: browser history, referrers, screenshots, reverse-proxy logs, copied
  URLs. Prefer `Authorization: Bearer` for fetch/XHR. Treat the query form as
  a compatibility path for the browser, not a general auth mechanism.
- **Tunnel exposure.** `AGEZT_TUNNEL` (cloudflared/ngrok/custom) can expose the
  Web UI / REST API to the internet. This is opt-in and operator-supervised.
  If enabled, the daemon's token becomes an internet-reachable credential;
  operators must treat it accordingly.
- **Local multi-user hosts.** The token file is `0600`, but on a shared host
  any process running as the same user can read it. AGEZT assumes a
  single-user trusted host by default.

---

## T6: Inbound channel abuse

**Threat.** An attacker sends messages to a configured channel (Telegram bot,
Slack, Discord, email inbox) to drive the agent, spam it, or trigger costly
runs.

**Controls.**

- **Fail-closed allowlists.** Every inbound channel uses an allowlist of
  sender/chat/channel ids. An empty allowlist receives no replies and drives
  no agent action. Non-allowlisted senders are ignored. See
  `plugins/channels/*/` and `kernel/channel/`.
- **Self-message skip.** The daemon skips its own outbound messages on inbound
  to prevent reply loops.
- **Size and rate bounds.** Channels apply chunking, payload-size caps, and
  slowloris protection to bound abuse.

**Limitations.**

- An allowlisted account is fully trusted to drive the agent. If an
  allowlisted Slack workspace or email mailbox is compromised, the attacker
  can issue agent commands subject to the same Edict policy as the operator.
- Inbound messages are still subject to prompt-injection risk (T1); the
  allowlist controls *who* can talk to the agent, not *what effect* their
  words have.

---

## T7: Plugin and marketplace compromise

**Threat.** A malicious or buggy plugin gains code execution, floods the host
with callbacks, or smuggles harmful tool definitions past the allowlist.

**Controls.**

- **Out-of-process isolation.** External plugins run as separate processes over
  a JSON protocol; a plugin crash does not take down the daemon. The host
  reaps dead children and supports hot-reload. See `kernel/plugin/host.go`.
- **Tool allowlists.** The host applies a tool allowlist at spawn time: only
  declared tools are exposed to the model. See `kernel/plugin/allowlist*`.
- **Bounded callbacks.** Host callbacks (`host/invoke`) are dispatched through
  a bounded slot pool so a plugin cannot spawn unbounded host-side goroutines.
  See `kernel/plugin/callback*`.
- **Content-address verification.** Plugin and skill installs from a registry
  are verified by BLAKE3 hash against a pin. See `kernel/plugin/pin.go`,
  `kernel/market/`.

**Limitations.**

- In-process plugins (built-in tools/channels/providers/guardians compiled into
  the daemon) are **not** process-isolated; they run in the daemon's address
  space. The out-of-process model applies to external plugins only.
- There is no code-signing model yet; verification is hash-based pinning, which
  authenticates "this is the bytes the pin expected," not "this came from a
  trusted publisher." Operators must trust the registry/pin source.
- A plugin that the operator permits and that the allowlist exposes still runs
  with the daemon's effective privileges on Windows/macOS (see T3 caveats).

---

## T8: Tenant boundary

**Threat.** One tenant's requests reach another tenant's kernel, journal, or
roster; a tenant token is reused against the primary engine.

**Controls.**

- **Per-tenant engines.** With multi-tenancy on, the `X-Agezt-Tenant` header
  routes a request to a per-tenant Engine + bus + journal. See
  `kernel/restapi/restapi.go`, `kernel/tenant/`.
- **Tenant token scoping.** A tenant token authorizes only its own tenant; the
  admin token authorizes any. Tokens are compared in constant time.
- **Isolated journals.** Tenant runs land in that tenant's journal; `agt why`
  traces stay within the tenant when invoked with the tenant token.

**Limitations.**

- Tenancy is a logical separation on a shared daemon and host. It is not a
  hard isolation boundary against a malicious tenant who can exploit a kernel
  bug; for strong isolation, run separate daemon instances.
- Mis-routing a request without a tenant header falls through to the primary
  engine. Operators must ensure tenant-aware clients always send the header.

---

## T9: Network egress and SSRF

**Threat.** An agent is steered to contact an internal service, the cloud
metadata endpoint, or a localhost admin surface to exfiltrate credentials or
pivot.

**Controls.**

- **Netguard dial-time guard.** The HTTP tool's default client validates the
  *resolved IP* on every dial (initial and each redirect hop), not just the
  URL string, defeating DNS rebinding. It blocks loopback, RFC1918 + ULA,
  link-local (incl. 169.254.169.254), the 0.0.0.0/8 block, CGNAT, and
  broadcast by default. NAT64-embedded IPv4 is collapsed and classified. See
  `kernel/netguard/netguard.go`.
- **Host allowlist + redirect re-check.** Beyond the IP guard, the HTTP tool
  re-checks the configured host allowlist on every redirect hop so an
  allowlisted host can't redirect to an arbitrary external host carrying the
  request's headers.
- **Default-deny host policy.** The HTTP tool refuses every host unless
  explicitly allowlisted (or `AllowAll` is set for trusted/dev contexts).

**Limitations.**

- `AllowLoopback` / `AllowPrivate` opt back in for legitimate local-service
  use. These are operator choices and weaken the default.
- Egress controls apply to governed tools. A plugin that opens its own sockets
  outside the netguard client is not covered; plugin authors must use the
  provided primitives.

---

## T10: Workspace and filesystem escape

**Threat.** An agent reads or writes outside its workspace root via `..`,
absolute paths, or symlink planting.

**Controls.**

- **Root canonicalization.** The file tool resolves the workspace root with
  `Abs` + `EvalSymlinks` at construction.
- **Per-request symlink resolution.** Every requested path is resolved the same
  way; if its absolute, symlink-resolved form is not under the root, it is
  rejected. See `plugins/tools/file/file.go` (`resolve`, `resolveNewWithinRoot`).
- **New-file ancestor resolution.** For not-yet-existing paths, the deepest
  existing ancestor is resolved and verified, so a symlinked parent directory
  cannot place a new file outside root.
- **`O_NOFOLLOW` writes.** Writes open with no-follow to close the TOCTOU
  window between resolution and open.
- **No recursive delete.** The file tool refuses recursive directory deletion;
  only individual files may be deleted, and never the workspace root itself.

**Limitations.**

- Containment is path-based and symlink-aware, but it is not a kernel-level
  filesystem jail on non-Linux. A process with the daemon's privileges (e.g.
  via `shell` on Windows/macOS) can still read the broader filesystem unless
  the capability is denied.

---

## Cross-cutting: what "auditable" buys you

AGEZT's strongest cross-cutting control is not a single gate; it is that every
significant action is an event in a tamper-evident, BLAKE3-hash-chained
journal. This does not *prevent* any of the threats above on its own. What it
does:

- makes a successful attack **visible** (`agt why`, `agt edict log`,
  `agt journal verify`)
- makes the **governing decision** inspectable (capability, level, reason,
  hard-deny rule)
- makes **recovery and forensics** possible (the event chain shows what
  happened, in what order, under what authority)

Where a preventive control is best-effort (notably process isolation on
Windows/macOS and prompt-injection resistance generally), auditability is the
compensating control. Operators should treat `agt why` and the Web UI
diagnostics as part of the security surface, not just observability.

---

## Operator deployment checklist

Minimum posture for a production deployment:

1. **Keep the daemon loopback-only.** Do not set `AGEZT_API_ADDR` /
   `AGEZT_REST_ADDR` / `AGEZT_WEB_ADDR` to a non-loopback address unless you
   have a reverse proxy with auth in front.
2. **Treat the admin token as a root credential.** It authorizes every tenant.
   Don't paste it into shared channels or screenshots.
3. **Encrypt the vault.** Run `agt vault encrypt` and set a strong passphrase;
   rotate it periodically.
4. **Default `shell` / `code_exec` to `ask` or `deny`** for any agent that
   touches untrusted input (web fetch, inbound channels, peer nodes).
5. **Confirm the isolation profile.** On Linux, prefer running under an
   unprivileged user namespace setup. On Windows/macOS, do not assume
   process isolation; rely on policy + approval.
6. **Keep channel allowlists tight.** Every allowlisted sender can drive the
   agent. Review them when onboarding/offboarding.
7. **Verify plugins before enabling.** Confirm the BLAKE3 pin against a trusted
   source; prefer out-of-process plugins over in-process code for untrusted
   functionality.
8. **Monitor denials and approvals.** `agt edict stats`, the Web UI diagnostics,
   and `agt why` are your intrusion-detection surface. A spike in denials is a
   signal worth investigating.
9. **Run `agt doctor --strict` in CI/monitoring** so advisory-level signals
   (failing schedule, egress block, throttling) alert before they become
   incidents.

---

## Claims guardrails

Use precise language when describing AGEZT's security posture:

- Say **contains blast radius** for prompt injection; do not say prompt injection is solved.
- Say **governed and audited irreversible tools**; do not promise generic rollback for shell/code execution.
- Say **requested vs effective isolation is visible**; do not claim equal sandboxing across Linux, Windows, and macOS.
- Say **hash-pinned plugins**; do not imply publisher signing or provenance verification.

---

## What is explicitly out of scope

- AGEZT does not provide a hardened sandbox against a malicious local user on
  the daemon host. It assumes the host and the daemon process are trusted.
- AGEZT does not sign plugins cryptographically; verification is hash-based
  pinning.
- AGEZT does not guarantee prevention of prompt injection; it contains the
  blast radius of a successful injection through layered gating + audit.
- AGEZT's process isolation is best-effort and platform-dependent; on
  Windows/macOS there is no process-level containment for shell/code-exec.

When in doubt, the safe default is **deny**. Edict is designed to fail closed.
