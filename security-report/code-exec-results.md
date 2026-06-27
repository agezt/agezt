# Security Findings — CODE EXECUTION domain (command injection / RCE / sandbox escape / deserialization)

> Scanner: code-exec hunter (sc-cmdi, sc-rce, sc-deserialization).
> Repo: `D:\Codebox\PROJECTS\AGEZT`, branch `main`.
> Scope note: `.worktrees/rebased-main/**` is a duplicate working copy of the main tree and was
> EXCLUDED from findings (every file there mirrors a canonical `kernel/…` / `plugins/…` file). All
> `*_test.go` files are context only, never findings.
>
> **Framing applied per task brief:** the agent running arbitrary code via `code_exec`, arbitrary
> shell via `shell`, spawning MCP stdio servers, installing host CLI tools via the toolbox, and
> delegating to external coding/ACP agents are all INTENDED product capabilities and are NOT reported
> as vulnerabilities. What was hunted: (1) untrusted input (channel/web/OpenAI-API/lower-trust tool
> args) reaching host exec OUTSIDE the warden/sandbox boundary; (2) sandbox/warden escape & env-scrub
> gaps; (3) Windows `cmd /S /C` verbatim-quoting binary-substitution bugs; (4) MCP stdio spawn with
> attacker-influenced command/args; (5) unsafe deserialization (gob / YAML / JSON→interface type
> confusion / JWT alg-confusion / plugin loading).

## Summary

**No exploitable vulnerabilities found in the code-execution domain.** The execution surface is
unusually disciplined: a single `kernel/warden` choke point for sandboxed exec, array-form
`exec.Command` everywhere (no `sh -c` string interpolation of model/untrusted data), consistent
env-scrubbing before every child process, slug-only resolution of agent-influenced agent selectors,
and capability/Edict gating on every spawn. The deserialization surface is effectively nil: **no
`encoding/gob`, no YAML, no `plugin.Open`, no `json.Unmarshal` into `interface{}` type-confusion**
sinks exist in the shipping tree; all JSON decoding targets concrete structs (a code-execution-safe
format), and the agentgw JWT path is alg/typ/iss/aud-pinned with constant-time HMAC.

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 0 |
| Low      | 0 |
| Informational / verified-safe (defensive notes) | 5 |

**Most serious item:** None rise to a finding. The single most security-load-bearing construct is the
Windows verbatim command-line builder `fixupWindowsCmd` (`kernel/warden/cmdline_windows.go:26`), which
sets `SysProcAttr.CmdLine = cmd /S /C "<command>"`. It was reviewed for binary-substitution / quote-
breakout and judged **safe in its current call paths** — see INFO-1. It is flagged only as the place
to re-review if a *new, lower-trust* caller of the shell tool is ever added.

---

## Verified-safe / defensive notes (no action required)

### INFO-1 — Windows verbatim `cmd /S /C "<command>"` builder (reviewed, safe)
- **File:** `kernel/warden/cmdline_windows.go:26-44`; caller `plugins/tools/shell/shell.go:167`.
- **CWE considered:** CWE-78 (argument/command injection via Windows quoting).
- **Analysis:** `fixupWindowsCmd` only rewrites the command line when `Args[0]` is `cmd`/`cmd.exe` and
  `Args[1]` is `/c`; the executed binary remains `cmd.Path` (resolved from `Args[0]`), so the raw
  `CmdLine` cannot redirect execution to a *different* program — it only changes how cmd.exe parses
  the trailing command. The trailing command is the agent-supplied `shell` tool input, which is the
  product's intended capability (agent → shell, gated by Edict). `cmd /S` strips exactly the outer
  quote pair, so an embedded quote in the model's command can alter cmd.exe tokenization, but that is
  *within* a shell the agent already fully controls — no privilege boundary is crossed. Env is scrubbed
  (`shell.go:172 scrubEnv`), output capped, timeout enforced.
- **Residual:** The construction is only safe because the trailing command originates from the agent
  (edict-gated) and `Args[2:]` for the shell tool is a single element. If a future caller passes a
  multi-element `Args[2:]` built from a *lower-trust* source (e.g. a raw channel webhook) straight into
  a `{"cmd","/c",…}` warden Spec, the `strings.Join(Args[2:], " ")` at line 39 would concatenate
  attacker fragments into one cmd.exe line. No such caller exists today. **Re-review this file whenever
  a new warden caller is added.** Confidence: 90 (safe today).

### INFO-2 — `code_exec` sandbox: env-scrub + Deno FS jail + package-flag validation (verified)
- **Files:** `plugins/tools/codeexec/codeexec.go`, `runtimes.go:120 scrubEnv`, `packages.go:27
  validatePackages`, `buildArgv` at `runtimes.go:96`.
- Model-written code runs through the warden with: a scrubbed allowlist env that drops the entire
  `AGEZT_*` namespace and anything containing `KEY/TOKEN/SECRET/PASSWORD/CRED/AWS_` (`isSecretName`,
  `runtimes.go:156`); `HOME`/`TMP` repointed into the per-run scratch dir; Deno launched with
  `--allow-read=<dir> --allow-write=<dir>` and **no `--allow-run`** (blocks shell-out escape of the FS
  jail); extra-file names validated against absolute/`..`/drive-relative traversal (`sanitizeRelFile`,
  `runtimes.go:193`); project names slugged to a single safe path segment (`slug`, `runtimes.go:168`).
- pip packages are array-appended to `python -m pip install` (no shell) and pre-validated to reject
  pip-flag injection (`-`-prefixed) and whitespace/NUL (`packages.go:27`). **Safe.** Confidence: 90.

### INFO-3 — MCP stdio spawn uses array-form exec + scrubbed env; registration is cap-gated
- **Files:** `kernel/mcp/client.go:112 Dial` (`exec.Command(command, args...)`), `store.go:122
  Validate` (name/env-key/header-name regexes, arg/env/header count caps).
- The server `Command`/`Args` are spawned array-form (never through a shell), the child gets the
  `scrubbedEnv()` allowlist (`client.go:320`) plus only operator-typed per-server `Env` entries, and
  registering/attaching a server is gated by the `mcp.install` Edict capability (Ask by default). An
  agent (prompt-injection) cannot register a server silently, and even a registered server's command
  cannot inject shell metacharacters because there is no shell. **Safe.** Confidence: 88.

### INFO-4 — Agent-influenced ACP `agent` selector is slug-only (CWE-78 closed by design)
- **Files:** `plugins/tools/acpagent/acpagent.go:132`, `kernel/acpcatalog/acpcatalog.go:266
  ResolveCommand`.
- The `acp_agent` tool's `agent` field is LLM/prompt-injection-influenceable and the spawn does run
  through a platform shell (`spawnAgent`, `acpagent.go:241` `sh -c`/`cmd /C`). But `ResolveCommand`
  treats `ref` strictly as a catalog slug: a non-slug `ref` is rejected (`ok=false`), and the executed
  command string is taken from the trusted `acpcatalog.Catalog`, never from the caller's string. A raw
  arbitrary command is only reachable via the operator-set `AGEZT_ACP_AGENT_CMD` fallback (used only
  when `ref` is empty). The `coding` tool likewise passes the model's task via the `AGEZT_CODING_TASK`
  env var, never interpolated into the command line (`coding.go:143-145`). **Safe.** Confidence: 88.

### INFO-5 — Host-level exec (toolbox install, watchdog, update, tunnel, AWS credential_process): trust boundary correct
- **Files:** `kernel/toolbox/toolbox.go:264` (install), `cmd/agezt/watchdog.go:223`,
  `kernel/update/update.go:691 SpawnRestart`, `kernel/tunnel/tunnel.go:227`, `kernel/creds/aws.go:149
  runCredentialProcess`, `cmd/agt/listen.go:123`.
- These deliberately run host-level (un-sandboxed) processes, but every one is fed from a *trusted*
  source and uses array-form exec:
  - toolbox `Install` runs only catalog-defined recipes resolved by exact `byName` lookup; the
    user-supplied `name` selects a recipe, it is never interpolated into argv.
  - watchdog/update re-spawn the daemon's own `os.Executable()` with fixed args.
  - tunnel command is operator-configured (`AGEZT_TUNNEL`/`AGEZT_TUNNEL_CMD`).
  - AWS `credential_process` is **opt-in** behind `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED=1`, reads the
    command from the operator's local `~/.aws/{config,credentials}`, and tokenizes it with a custom
    splitter into array-form `osexec.CommandContext` — **not** a shell (`aws.go:161,195`).
  - `agt listen` splits the operator-set `AGEZT_VOICE_RECORD_CMD` with `strings.Fields` (no shell) and
    substitutes only a generated temp path + integer seconds.
  None of these are reachable from channel/web/OpenAI untrusted ingress. **Safe.** Confidence: 85.

---

## Deserialization sweep result

- `encoding/gob`: **0 occurrences** in the shipping tree (no Go-gadget surface).
- YAML: no YAML parser dependency; nothing to unsafe-load.
- `plugin.Open` (Go native plugins): **not used**; third-party plugins are separate subprocesses
  hash-pinned with BLAKE3 (`kernel/plugin/pin.go`), not loaded into the daemon address space.
- `json.Unmarshal`/`json.NewDecoder`: all sinks decode into concrete structs (channels into
  `getUpdatesResp` etc. behind `io.LimitReader` caps; agentgw into `TokenClaims`/`ConfigSetRequest`;
  update into `Manifest`). JSON is a code-execution-safe format (no arbitrary object instantiation),
  and no `interface{}`/`any` type-confusion dispatch was found.
- JWT (agentgw, `kernel/agentgw/token.go`): alg pinned to `HS256`, `typ` pinned to `JWT`, `iss`/`aud`
  pinned, signature compared with `hmac.Equal` (constant-time) — classic alg-confusion / `none`-alg
  hole is explicitly closed (`token.go:99-115`). **No deserialization finding.**

## Conclusion

The code-execution attack surface is the product's core and is correspondingly well-guarded. Across
warden, `code_exec`, `shell`, MCP stdio, the ACP/coding bridges, the toolbox installer, and all CLI
host-exec, every command is built array-form from trusted or intentionally-agent-controlled inputs
with scrubbed environments, and the one shell-string spawn reachable from agent input (ACP) is
slug-constrained. No command injection, no RCE-via-eval, no sandbox-escape, and no insecure
deserialization were identified.
