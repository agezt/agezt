# Security Hunt — Injection Domain (CMDI / RCE / Deserialization / Path Traversal / SSTI)

**Scanner:** sc-injection HUNTER
**Codebase:** AGEZT (Go) — D:/Codebox/PROJECTS/AGEZT
**Date:** 2026-06-13
**Domain:** OS command injection, RCE, insecure deserialization, path traversal/LFI, SSTI

## Summary of counts by severity

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 1 |
| Low      | 2 |
| Info / by-design / false-positive | 6 |

**Headline:** No critical or high-severity injection vulnerabilities found. The codebase is consistently careful: every `exec.Command*` call site uses argv-form (no `sh -c "<concat>"` with attacker data in production code), archive extraction and all path-construction sinks have explicit traversal guards, the child env is scrubbed of secrets, and there is no `text/template`/`eval`-style dynamic code surface and no unsafe deserialization (only stdlib `encoding/json` over untrusted bytes). The high-blast-radius capabilities (code_exec, shell, mcp attach) are deliberate, capability-gated, audited, and shell-free — matching documented owner policy.

---

## Findings

### INJ-001 — credential_process tokeniser drops backslash escapes, can mis-split an argv (potential argument-injection on Windows paths)
- **Severity:** Medium
- **Confidence:** 55
- **File:** kernel/creds/aws.go:148-215 (`runCredentialProcess`, `splitCommandLine`)
- **CWE:** CWE-88 (Argument Injection) / CWE-78 (adjacent)
- **Description:** `runCredentialProcess` executes a `credential_process = ...` line from `~/.aws/credentials` / `~/.aws/config` using a home-grown tokeniser (`splitCommandLine`). The tokeniser handles `"`/`'` quoting but, per its own doc comment, does **not** support backslash escapes and silently strips/merges in edge cases (e.g. a Windows path `C:\Program Files\tool.exe --flag` splits oddly; an unterminated quote swallows the rest of the line into one token). Execution is argv-form (`osexec.CommandContext(parts[0], parts[1:]...)`), so there is **no shell metacharacter injection** — but a mis-parse can produce an argv whose first element is not the intended binary, or whose flags differ from what the operator wrote.
- **Exploit path:** Requires (a) operator opt-in via `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED=1`, and (b) an attacker who can write the `credential_process` line (i.e. already has write access to `~/.aws/*`). With write access to that file an attacker can simply name any binary directly, so the tokeniser quirk does not materially expand the attack surface — the realistic impact is a **reliability/footgun** (wrong binary runs) more than a new injection primitive. Hence Medium-leaning-Low.
- **Remediation:** Either adopt a vetted shell-words splitter (e.g. mvdan.cc/sh syntax or google/shlex) with documented escape semantics, or — better — keep the existing "wrap complex commands in a script" guidance but reject ambiguous input (unterminated quote → error rather than silent merge). Add a unit case for a Windows absolute path with spaces.
- **False-positive note:** This is gated behind an explicit env opt-in and requires pre-existing config-file write access; report it as defense-in-depth, not an exploitable remote vuln.

### INJ-002 — Sandbox file-read handler does not resolve symlinks before containment check
- **Severity:** Low
- **Confidence:** 40
- **File:** kernel/controlplane/sandbox.go:104-147 (`handleSandboxFile`), :180-191 (`confineUnder`)
- **CWE:** CWE-22 (Path Traversal via symlink)
- **Description:** `confineUnder` blocks `..`, absolute paths, and NUL, and verifies the cleaned join stays under the root by string prefix. It does **not** call `filepath.EvalSymlinks`, so a symlink *already present inside* the sandbox project dir that points outside (e.g. created by previously-run sandboxed code) would let an authenticated operator read the link target through this control-plane endpoint.
- **Exploit path:** code_exec/agent writes a symlink `proj/leak -> /etc/passwd` inside its sandbox dir; later a control-plane `sandbox_file` request for `proj/leak` follows it. Requires control-plane auth (operator token / password) and the sandboxed code to have planted the link. Low because the endpoint is operator-authenticated and the operator already has full host access.
- **Remediation:** After `confineUnder`, `EvalSymlinks(full)` and re-verify the resolved path is still under `cleanRoot`; or open with `O_NOFOLLOW` on the final component.

### INJ-003 — `mcp attach` / agent-driven self-install spawns an arbitrary executable with arbitrary argv (by design, flagged for awareness)
- **Severity:** Low (Info-leaning; by-design per owner policy)
- **Confidence:** 90 (behaviour) / N/A (vuln)
- **File:** plugins/tools/mcptool/tool.go:108-133 (`add`/`attach`), kernel/runtime/mcptool.go:110-123 (`dialMCP`), kernel/mcp/client.go:112-138 (`Dial` → `exec.Command(command, args...)`)
- **CWE:** CWE-78 (adjacent — but no shell)
- **Description:** The agent-facing `mcp` tool lets the model register (`op=add`) a server with model-supplied `command` + `args`, then `op=attach` spawns it. The spawn is **argv-form, no shell** (`exec.Command(command, args...)`), with a **scrubbed environment** (`scrubbedEnv()` drops AGEZT_*/KEY/TOKEN/SECRET/PASSWORD/CRED/AWS_ vars), frame-size caps, and journaling. `mcp.Store.Validate` constrains the server *name* (regex), env-var key names, and header names, but intentionally does **not** allowlist the `command`.
- **Why not a vuln:** This is the documented "governed self-install" capability gated by the `mcp.install` Edict capability (Ask by default), consistent with the owner's default-allow / max-capability sandbox-tool posture. There is no shell-metachar injection because no shell is invoked, and the daemon's secrets are scrubbed from the child. Effectively equivalent in blast radius to the deliberately-capable `code_exec`/`shell` tools.
- **Remediation / hardening (optional, do not tighten without owner sign-off per policy):** Confirm `mcp.install` defaults to Ask in shipped Edict policy; consider surfacing the resolved absolute command path in the approval prompt so an operator sees exactly what will spawn.

---

## Areas reviewed and found SAFE (false-positive candidates / by-design)

1. **kernel/warden/warden.go (`engine.Run`, exec.CommandContext line 280)** — argv-form only; package contract explicitly states it does NOT spawn a shell and that callers wanting shell expansion must pass `{"sh","-c",cmd}` themselves. `Argv[0]==""` rejected. nil `Env` correctly translated to an EMPTY environment (M186) so the daemon's secrets never leak into a child by default. This is the central, correctly-built isolation primitive.

2. **plugins/tools/codeexec/codeexec.go + runtimes.go (`sanitizeRelFile`)** — the `files` map keys are validated: absolute path, NUL, `..` traversal, `.`, and Windows drive-relative (`C:foo`, colon) all rejected before `filepath.Join`. Entry/stdin/deps all land under the per-call work dir. Env is scrubbed. Running arbitrary code is the tool's *purpose* (owner policy: deliberately max-capability) — not reported.

3. **cmd/agt/backup.go (`restoreBackup`, `isAllowedBackupPath`)** — tar extraction is **zip-slip-safe**: rejects `..`, absolute, and backslash-prefixed names; allowlists only `journal/` and `catalog/` subtrees; re-verifies the joined target is under `cleanDest` with a separator-anchored prefix check; uses `O_EXCL` so it never overwrites; refuses to restore into a non-empty home. Inspect path is read-only. Exemplary handling.

4. **kernel/skill/bundle.go (`cleanRel`)** — bundle writes/reads validate the relative path against `..`-escape and absolute paths before any `filepath.Join`; per-file and per-bundle size caps; temp-dir-swap commit. No traversal.

5. **kernel/mcp/client.go (`scrubbedEnv`, `isSecretName`)** — child MCP servers get an allowlisted, secret-scrubbed environment; the AGEZT_* namespace and any KEY/TOKEN/SECRET/PASSWORD/CRED/AWS_-shaped name is stripped. Operator opt-in env (M898) is the only way secrets reach a child, by explicit design.

6. **Deserialization / RCE / SSTI surface** — No `text/template`/`html/template` `.Parse`/`.Execute` over user-controlled template *strings* anywhere (grep returned only `time.Parse`, `url.Parse`, `flag.Parse`). No `pickle`/`Marshal`/`gob` of untrusted data; untrusted bytes are decoded only with stdlib `encoding/json` (safe, no arbitrary-type instantiation), under size caps (`maxFrameBytes`, `LimitReader`). No `plugin.Open`, `yaegi`, or eval-like dynamic-code construct on a request path. SSTI/RCE/insecure-deser: none found.

7. **Other exec call sites** — kernel/tunnel/tunnel.go:227 (cloudflared/ngrok, operator-configured binary, argv-form), cmd/agt/listen.go:123 (`AGEZT_VOICE_RECORD_CMD` operator env template, argv-form via `strings.Fields`), kernel/update/update.go (SHA256-validated binary, atomic rename), kernel/plugin/host.go+pin.go (BLAKE3 pin verification, `$PATH` resolution made consistent between hash and exec in M422), kernel/creds/machineid_darwin.go (constant argv). All argv-form; sources are operator/local config, not remote attacker input.

---

## Methodology notes
- Searched all `exec.Command`/`exec.CommandContext` sites (30 files) and traced argv provenance; confirmed none concatenate untrusted data into a `sh -c`/`cmd /C` string in production code (the only `sh -c` + concatenation patterns are in `*_test.go`).
- Verified every `filepath.Join` with externally-influenced input passes through a containment/sanitiser (`confineUnder`, `sanitizeRelFile`, `cleanRel`, `isAllowedBackupPath` + prefix re-check).
- Confirmed archive extraction (only `cmd/agt/backup.go`) is traversal-guarded.
- Confirmed no unsafe deserialization formats and no dynamic-code-eval surface.
