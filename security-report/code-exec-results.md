# Code-Execution & Deserialization Audit — AGEZT

Scope: Remote/arbitrary code execution, insecure deserialization, and the agent
code-execution / sandbox (warden) subsystem. Read-only review.

## TL;DR

The sandbox/warden subsystem is well-engineered and the deliberately-permissive
posture (sandbox runs arbitrary code, net-on by default) is **not** counted as a
flaw per the documented design. The non-negotiables the design rests on —
secret-scrub, workdir confinement, audit, and "agent input is never a raw shell
line" — hold in `code_exec`, `shell`, `mcp`, and `acp_agent`. No Go `plugin.Open`
/ `.so` loading, no `gob`, no `yaml.Unmarshal` of untrusted config, and no
reflection-driven exec sink was found. All `json.Unmarshal` sites decode into
concrete structs, not exec-dispatching `interface{}` type switches.

Two **real** findings, both MEDIUM and both in the *operator-configured external
agent bridges* (not the sandbox): they run with the **full unscrubbed daemon
environment**, contradicting the secret-scrub posture every other exec path
upholds. Everything else is either intended design or already correctly
defended.

The host-level RCE-capable surfaces (toolbox installer, MCP/market arbitrary
`command` spawn, AWS `credential_process`, self-update) are all gated behind the
**localhost-only, admin-token-authenticated** control plane and/or explicit
operator opt-in env gates — i.e. the operator trust boundary, not the
agent/prompt-injection boundary. Those are documented as intended; notes below.

---

## REAL FINDINGS

### F1 (MEDIUM) — `coding` tool leaks the entire daemon environment to the external agent
- File: `plugins/tools/coding/coding.go:141`
  ```go
  agentEnv := append(os.Environ(), "AGEZT_CODING_TASK="+task)
  shell, shellArg := platformShell()
  agentOut, agentErr := t.run(ctx, wt, agentEnv, shell, shellArg, t.Cmd)
  ```
- Boundary escaped: every *other* exec path in the codebase (`code_exec`,
  `shell`, `mcp.Dial`) builds a **scrubbed** env via `scrubEnv`/`scrubbedEnv`
  that drops `AGEZT_*`, `*KEY*`, `*TOKEN*`, `*SECRET*`, `AWS_*`, etc. The coding
  bridge instead forwards `os.Environ()` verbatim. The configured command
  (`AGEZT_CODING_CMD`, e.g. `claude -p "$AGEZT_CODING_TASK"`) and the third-party
  coding-agent binary it launches therefore see the daemon's provider API keys,
  vault-derived secrets, AWS creds, and the control-plane admin token if it is in
  the environment.
- Attacker path: the `task` is fully model-controlled (and thus prompt-injectable).
  While `task` is passed safely via env var (no shell-quoting of model output —
  good, see note N4), the *agent that receives it* can read every secret from its
  own environment and exfiltrate them (it has network + repo access by design).
  A prompt-injected `task` such as "print all env vars / POST them to X" turns a
  full secret dump into a one-shot. The tool is gated by Edict, but once allowed,
  the blast radius is "all daemon secrets," which the scrub posture exists to
  prevent.
- Why it matters: the project memory explicitly states secret-scrub is
  "non-negotiable." This path violates it for a class of operator-configured
  third-party binaries.
- Severity: MEDIUM (requires the operator to have configured + allowed the
  coding bridge; but then any model/injection has full secret reach).
- Fix: build a scrubbed env (reuse the `shell.scrubEnv` / `codeexec.scrubEnv`
  allowlist) and append only `AGEZT_CODING_TASK` plus whatever the coding agent
  legitimately needs (PATH, HOME, git identity). If a particular agent needs a
  provider key, require it to be named explicitly (the MCP `env` opt-in model in
  `kernel/mcp/client.go:appendEnv` is the right precedent).

### F2 (MEDIUM) — `acp_agent` tool spawns the external ACP agent with the full daemon environment
- File: `plugins/tools/acpagent/acpagent.go:238-255` (`spawnAgent`)
  ```go
  c := exec.Command(shell, arg, cmdStr) // no c.Env set → inherits os.Environ()
  ```
- Same class as F1: `spawnAgent` never sets `c.Env`, so Go inherits the parent
  (daemon) environment, handing all secrets to the spawned ACP agent (Claude
  Code / Codex / Gemini CLI / etc.).
- Attacker path: the *command* is correctly slug-restricted (see N3 — agent input
  cannot inject a raw shell line), so this is **not** an arbitrary-exec flaw. But
  the spawned trusted-command binary still runs with unscrubbed secrets; a
  prompt-injected `task` steers that agent (which has its own tools/network) and
  can have it read+exfiltrate the inherited secrets.
- Severity: MEDIUM (same gating + same blast radius as F1).
- Fix: set `c.Env` to a scrubbed allowlist before `c.Start()`, mirroring the
  shell/codeexec scrub. ACP agents that genuinely need a key should get it via an
  explicit per-agent opt-in, not ambient inheritance.

> F1/F2 share a root cause and a fix: the two "delegate to an external
> operator-configured agent" bridges were written before/around the M957 scrub
> work and never adopted it. They are the only exec paths that inherit
> `os.Environ()` wholesale.

---

## INTENDED DESIGN (verified, not flaws)

### `code_exec` sandbox (`plugins/tools/codeexec/`)
- Secret scrub: `scrubEnv` (runtimes.go:120) is allowlist-only and drops
  `isSecretName` matches + everything outside the allowlist. Correct.
- Path confinement: `sanitizeRelFile` (runtimes.go:193) rejects abs paths, `..`,
  NUL, and Windows drive-relative `C:foo` (colon check). `slug` (runtimes.go:168)
  cannot produce a separator or `..`, so project dirs can't escape
  `SandboxRoot/projects`. Verified no traversal.
- Argv built without a shell (`buildArgv`); Deno is OS-jailed to the workdir with
  `--allow-read/write=<dir>`, `--no-prompt`, and **no** `--allow-run` (can't shell
  out). Python/Node rely on the warden profile (real isolation only on
  Linux+namespace; honestly reported as downgraded elsewhere — `render`/events do
  not overstate containment).
- `validatePackages` (packages.go:27) blocks pip-flag injection (`-`-prefixed,
  whitespace/NUL). pip runs via exec, no shell. Correct.
- Running arbitrary model code IS the design; gated by `code.exec` Edict cap +
  journaled. Not a flaw.

### `shell` tool (`plugins/tools/shell/`)
- `scrubEnv` (env.go) allowlist with secret-drop; HOME/TMP redirected to workdir.
- Windows `fixupWindowsCmd` (warden/cmdline_windows.go) builds `cmd /S /C
  "<command>"` verbatim. `<command>` is model-supplied, but executing arbitrary
  shell commands is the tool's whole purpose (gated by Edict). The `/S` outer-quote
  stripping is the documented robust form; no additional injection beyond the
  intended "run a shell command" capability.

### warden engine (`kernel/warden/`)
- `Spec.Argv[0]` is the binary; engine never spawns a shell itself — callers must
  pass `{sh,-c,...}` explicitly. nil `Env` → **empty** env (not inheritance) is
  the documented anti-leak default (warden.go:291). Good.
- Linux hardening (warden_linux.go): setpgid kill-sweep + best-effort prlimits.
  The `unsafe`/Syscall6 prlimit block is confined and correct. No namespace/seccomp
  yet, honestly reported via downgrade events. Design-acknowledged.

### plugin host (`kernel/plugin/host.go`)
- Out-of-process stdio JSON-RPC children; **no** Go `plugin.Open`/`.so` dlopen.
  Binary pinning (BLAKE3 via `VerifyPin`), tool allowlist, frame-size caps,
  bounded callbacks, advertised-tool cap. `json.Unmarshal` decodes into concrete
  structs only. Plugin path/args/env come from operator wiring (`Config`), not
  agent input.

### MCP client (`kernel/mcp/client.go`)
- `Dial` spawns `exec.Command(command, args...)` with a **scrubbed** base env
  (`scrubbedEnv`) plus operator-explicit per-server `env` overlay (`appendEnv`) —
  the correct "credentialed server gets exactly what the operator typed" model.
  `command`/`args` originate from the MCP registry, populated only via the
  admin-token control plane (`controlplane/mcp.go handleMCPAdd → k.AddMCPServer`)
  or an operator-installed market pack. Not agent-reachable. `mcp.Validate`
  (store.go:122) checks shape, not command trust — acceptable given the operator
  gate.

### `acp_agent` command resolution (`kernel/acpcatalog/acpcatalog.go:266`)
- CWE-78 explicitly defended: attacker-influenced `ref` MUST be an installed
  catalog slug; the executed command comes from the trusted catalog, never the
  caller's string. Raw commands only via operator `fallback` (env). Correct.
  (Env leak in the *spawn* is F2; the *command* path is safe.)

### Toolforge (script-tool forge) — no HITL bypass
- `controlplane/toolforge.go`: agents can draft/edit/test forged tools, but
  **Promote** (`handleToolforgePromote → k.PromoteScriptTool`) is an
  operator-only control-plane op. An agent cannot self-promote a script tool to
  the auto-offered `forge_<name>` active state. Forged tools run in the same
  code-exec sandbox and every `forge_*` call is classified `CapCodeExec`
  (`kernel/edict/toolmap.go:25`). Gate intact.

### Market install (`kernel/market/`)
- Install materializes skills (sandbox-run) + registers MCP servers; it does NOT
  host-install tools (`manager.go:280` reports tool requirements only — host exec
  needs explicit Toolbox consent). Remote sync (`sync.go`) is netguard-screened
  (SSRF), same-host pack refs, content-hash + optional Ed25519 signature
  (`VerifyPack`). An MCP server registered by an **unsigned** pack from a
  remote source can carry an arbitrary `command` — but adding the source
  (`agt market add`) and installing (`agt market install`) are both
  admin-token control-plane actions, and attach is a further operator step.
  Operator-trust-boundary, consistent with intended design. (Hardening idea: warn
  loudly when an unsigned remote pack contributes a stdio MCP `command`.)

### Self-update (`kernel/update/update.go`)
- SHA256 + Ed25519 verification before atomic rename; netguard-screened fetch;
  fail-safe (no auto-restart on validation failure). Correct.

### AWS `credential_process` (`kernel/creds/aws.go:149`)
- Execs an arbitrary binary from `~/.aws/{config,credentials}` — classic RCE
  sink — but **double-gated**: requires `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED=1`
  (opt-in, documented footgun) AND the command comes from the operator's own
  `~/.aws` files, not agent input. `splitCommandLine` (aws.go:195) refuses to run
  a mis-split argv (unterminated quote → error, no half-parsed exec). Reads at
  daemon boot/reload only. Operator-trust-boundary; intended.

### Toolbox installer (`kernel/toolbox/toolbox.go`)
- Host-level `exec.Command` of package managers (winget/brew/apt/...). Argv comes
  from the **static in-Go `Catalog`** (toolbox/catalog.go), selected by a tool
  `name` that must match a catalog entry (`byName`); the install argv is never
  built from caller free-text. Reached only via the admin-token control plane.
  Per memory `[[cli-toolbox]]` this is intentionally un-sandboxed (an installer
  must change the host). Operator-trust-boundary; intended.

---

## NOTES / LOWER-PRIORITY OBSERVATIONS

- N1 — Control-plane auth boundary confirmed: `controlplane/server.go` binds
  `127.0.0.1:0` only, mints a random per-process token to a `0600` file, and uses
  constant-time comparison (`tokenIsPrimary`). All host-RCE-capable management
  ops (MCP add, toolbox install, market add/install, update) sit behind this
  admin token. The agent/prompt-injection boundary therefore does NOT reach those
  ops — good architectural separation. (If a tunnel/agentgw ever exposes the
  control plane beyond loopback, re-audit; out of scope here.)

- N2 — `mcp.Server.Command` / `Args` have no path/traversal validation
  (store.go:122 checks name/transport/arg-emptiness only). Acceptable today
  because the field is operator-only, but if a future feature ever lets a less
  privileged principal (tenant token, agent op) register MCP servers, this becomes
  a direct host-exec primitive. Flag for defense-in-depth.

- N3 — Deserialization sweep: every `json.Unmarshal` reviewed decodes into a
  concrete typed struct (`input`, `shellInput`, `ScriptTool`, `mcp.Server`,
  `Pack`, IMDS/cred docs, plugin frames). No `interface{}` type-switch-then-exec,
  no `gob`, no `yaml` of untrusted bytes. Insecure-deserialization risk: none
  found.

- N4 — Good pattern worth preserving: both external-agent bridges pass the
  model-controlled task via an **environment variable** (`AGEZT_CODING_TASK`) /
  ACP protocol field rather than interpolating it into the shell line — this
  correctly avoids shell-injection of model output. F1/F2 are about the *env
  contents leaking secrets*, not about task injection.

---

## Suggested fix priority
1. F1 + F2 (shared fix): scrub the env for the coding/acp_agent external-agent
   spawns; allow secrets only via explicit per-agent opt-in.
2. N2: add command/path validation (or an allowlist) to `mcp.Validate` as
   defense-in-depth before any non-admin principal can reach MCP registration.
3. Market: surface a clear warning when an unsigned remote pack registers a
   stdio MCP `command`.

Report written to: D:/Codebox/PROJECTS/AGEZT/security-report/code-exec-results.md
