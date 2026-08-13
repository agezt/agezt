# AGEZT — Code-Execution & Deserialization Domain (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (`main`)
**Skills:** `sc-rce`, `sc-deserialization`
**Method:** every claim below cites a `file:line` I read. Where I could not substantiate a
suspicion I killed it and said so (see *Refuted*). One finding (CE-001) was additionally
confirmed by running a throwaway unit test against the real code; the test file was deleted
after the run and the tree is unmodified.

**Framing.** Per the brief I do not report "capability X defaults to allow" — that is an owner
decision. I report **confinement that is advertised but absent, or that can be switched off from
inside the tier it is supposed to confine.** All six findings are of that shape.

---

## Summary

| ID | Title | Severity | Conf. |
|---|---|---|---|
| CE-001 | `code_exec`/`shell` secret scrub is disableable from inside the sandbox's own trust tier | **High** | 92 |
| CE-002 | `tool_forge` operator-promotion gate is OFF by default; three docs and the model-facing tool description promise it | **High** | 95 |
| CE-003 | `mcp op=add` forces `Enabled=true`; the boot auto-attach spawns it with **no policy consultation**, contradicting the function's own comment | **High** | 90 |
| CE-004 | `config op=register` + `op=set` re-opens the raw-command path `acpcatalog.ResolveCommand` exists to close (CWE-78 note is falsified) | **High** | 85 |
| CE-005 | Container backend — the only profile claiming real isolation — runs privileged, root, with a read-write bind mount | **Medium** | 90 |
| CE-006 | `EffectiveProfile` reports `namespace` for a run with no namespace; that string is what the model and the journal are told | **Medium** | 95 |
| CE-007 | SSH/K8s execution-profile argv injection (`AGEZT_EXEC_SSH_TARGET` → `-oProxyCommand=`) | **Medium** | 70 |

Deserialization: **no findings**. See *Verified safe* §D.

---

## CE-001 — The code_exec secret scrub can be switched off by the code it confines

- **Severity:** High · **Confidence:** 92 · **CWE-668 / CWE-522** (exposure of resource to wrong sphere)
- **Sink:** `plugins/tools/codeexec/codeexec.go:283`, `plugins/tools/shell/shell.go:259`
- **Control:** `kernel/executionprofile/env.go:48-76`, `:78-102`, `:130-155`
- **Source:** `plugins/tools/config/config.go:192-287`

### The advertised guarantee

`plugins/tools/codeexec/runtimes.go:117-119`:

> `// … Every secret-shaped variable and the entire AGEZT_* namespace (API keys, provider creds,`
> `// tokens) is dropped so model-written code can never read the daemon's secrets. This is the`
> `// load-bearing safety property of the whole tool.`

The package doc repeats it (`codeexec.go:16-17`, "the daemon's secrets … are never forwarded into
model-written code") and, critically, **so does the tool description the model itself reads**
(`codeexec.go:138`): `"The daemon's secrets are never visible to your code."`

### What the code does

`scrubEnv` (`runtimes.go:120-156`) is correct in isolation. But the very next line wraps it:

```go
// plugins/tools/codeexec/codeexec.go:283
env := executionprofile.AppendEnvPassthrough(scrubEnv(dir), profileID)
```

`AppendEnvPassthrough` (`kernel/executionprofile/env.go:48`) reads
`AGEZT_EXEC_SECRET_ENV_{LOCAL,WARDEN,DOCKER}` **from `os.Getenv` at invoke time** and re-admits
every named variable. The only guard on that list is an `AGEZT_` prefix check
(`env.go:93`, `env.go:145-147`) — `KEY`/`TOKEN`/`SECRET`/`AWS_` are explicitly *permitted* there
(`env.go:91-100`, the `SecretEnvNames` loop deliberately skips `IsSecretEnvName`).

### The reachable path

`AGEZT_EXEC_SECRET_ENV_LOCAL` is a normal, non-secret, non-read-only, **`ApplyLive`** field in the
built-in settings schema (`kernel/settings/schema.go:499`), type `TypeCSV` — and
`settings.Validate` (`schema.go:619-641`) has **no case for `TypeCSV`**, so any string is accepted.

The agent-callable `config` tool reaches it directly:

```go
// plugins/tools/config/config.go:279-284
if field.Apply == settings.ApplyLive && !field.Secret && t.kernel != nil {
    _ = os.Setenv(name, value)
    ...
```

So a single `config {op:"set", name:"AGEZT_EXEC_SECRET_ENV_LOCAL",
value:"AWS_SECRET_ACCESS_KEY,GITHUB_TOKEN,ANTHROPIC_API_KEY"}` call takes effect **live, in the
same process, with no restart**, and the *next* `code_exec` call in the *same run* receives those
values. `code_exec` has network on by default (`codeexec.go:128`, `runtimes.go:96-112` grants Deno
`--allow-net`), so exfiltration is one HTTP call away.

Confirmed empirically against the real function (temporary test, since deleted):

```
child env = [PATH=… AWS_SECRET_ACCESS_KEY=aws-s3cr3t SC_FAKE_API_KEY=s3cr3t-value]
--- PASS
```
`AGEZT_ANTHROPIC_API_KEY` was correctly blocked; `AWS_SECRET_ACCESS_KEY` was not.

### Exploitation path

Prompt injection in fetched web content / an inbound channel message steers the model to
(1) `config op=set` the passthrough list, (2) `code_exec` a script that reads `os.environ` and
POSTs it out. Both calls ride capabilities (`config.write`, `code.exec`) that are L4 by default,
and `AGEZT_AUTO_APPROVE_CAPS` unset auto-grants every Ask (`cmd/agezt/main.go:3893`), so nothing
prompts.

### Why this is not a false positive

I checked the four places this could have been blocked and none of them block it:
`settings.Validate` has no `TypeCSV` branch; the field is not `ReadOnly`/`Locked`/`Secret`; the
`AGEZT_` prefix guard does not cover host-provided credentials; and `AppendEnvPassthrough` is
re-evaluated per invocation, so the live `os.Setenv` is enough. The same wrapper is on the shell
tool (`shell.go:259`) and on the pip-install child (`packages.go:60`).

The **feature** is intentional and documented (`schema.go:499` help text is honest). The finding is
that its control knob sits in the same trust tier as the sandbox it disarms, while the sandbox's
own doc — and the description shipped to the model — say the property is unconditional.

### Remediation

1. Make `AGEZT_EXEC_SECRET_ENV_*` and `AGEZT_EXEC_SECRET_FILES_*` `ReadOnly: true` in
   `schema.go:499-504`, i.e. operator-only (env/CLI), unreachable from `config op=set`.
2. Failing that, map the `config` tool's `set` on any `AGEZT_EXEC_SECRET_*` /
   `AGEZT_EXEC_ENV_*` key to a distinct high-friction capability rather than `config.write`.
3. Correct `codeexec.go:138` and `runtimes.go:117-119` to say the scrub is the default and name
   the opt-out; a model told "never" cannot reason about the exception.
4. Journal a `code.exec` event field naming the passthrough list actually in force.

---

## CE-002 — `tool_forge` promotion has no human in the loop by default

- **Severity:** High · **Confidence:** 95 · **CWE-863** (incorrect authorization) / **CWE-94**
- **File:** `cmd/agezt/internal/daemonconfig/daemonconfig.go:393`,
  `kernel/runtime/scripttool.go:134-140`

### The advertised guarantee

Three independent places promise an operator gate:

- `plugins/tools/forgetool/tool.go:6-7` — *"once a test of the current code passes, the OPERATOR
  promotes it (`agt toolforge promote` / the console)"*.
- `kernel/runtime/scripttool.go:116-118` — *"the agent ASKS for its tool to go live … The request
  blocks on the HITL approval registry (it shows up in `agt approvals` and the console's Approvals
  view)"*.
- **The model-facing description** (`forgetool/tool.go:77-79`): *"op=request_promotion asks the
  human operator … the call waits for their decision"* and *"A draft only goes LIVE when the
  operator approves"*.

### What the code does

```go
// cmd/agezt/internal/daemonconfig/daemonconfig.go:393
c.Misc.ToolforgeAutoPromote = !strings.EqualFold(get(brand.EnvPrefix+"TOOLFORGE_AUTO_PROMOTE"), "off")
```

Unset ⇒ **true**. Wired at `cmd/agezt/main.go:435` → `:516` → `Config.AutoPromoteScriptTools`, and:

```go
// kernel/runtime/scripttool.go:134-140
if k.cfg.AutoPromoteScriptTools {
    promoted, err := k.PromoteScriptTool(corr, st.Name)
    ...
    return promoted, approval.DecisionGrant, "auto-promote enabled", nil
}
```

The approval registry is never consulted. The repo's own test pins this default:
`cmd/agezt/internal/daemonconfig/daemonconfig_test.go:93` fails if `ToolforgeAutoPromote` is false
with no env set. The settings-schema help (`kernel/settings/schema.go:587`) is the one honest
description; the tool doc and the model-facing text are not.

### The remaining "invariant" is self-attested

The only surviving check is `if !st.TestedOK` (`scripttool.go:131`). `TestedOK` is set by
`TestScriptTool` (`scripttool.go:87`): `ok := !isErr`, where `isErr` is simply the script's
non-zero exit. **The agent authors the script, chooses the test input, and the pass condition is
"exit 0."** A script that does its real work and returns 0 passes.

### Exploitation path

One injected run: `tool_forge op=draft` (arbitrary Python/Node/Deno) → `op=test` with `{}` →
`op=request_promotion` → **live**. The result is a permanent `forge_<name>` tool offered to
*every* agent on *every* future run (`kernel/runtime/scripttool.go:201-221`), routed to
`code.exec` (`kernel/edict/toolmap.go:25-27`). This is the cleanest prompt-injection-outlives-the-run
primitive in the tree.

### Why this is not a false positive

I traced the default through `daemonconfig.go:393` → `main.go:435` → `main.go:516` →
`scripttool.go:134` and confirmed the approval registry itself is correct and fail-closed
(`kernel/approval/approval.go:244-247`, timeout ⇒ `DecisionTimeout`, treated as non-grant at
`scripttool.go:149-151`). The gate exists and works — it is simply bypassed before it is reached.

### Remediation

Flip the default: `c.Misc.ToolforgeAutoPromote = strings.EqualFold(raw, "on")`. If the permissive
default is deliberate, then fix `forgetool/tool.go:6-7,77-79` and `scripttool.go:116-118` so
neither the operator nor the model is told a gate exists that does not, and make the boot banner
line (`main.go:1590-1594`) louder than a parenthetical.

---

## CE-003 — Registering an MCP server is enough to get it spawned, ungated, at next boot

- **Severity:** High · **Confidence:** 90 · **CWE-94 / CWE-1188**
- **Files:** `kernel/mcp/store.go:218`, `kernel/runtime/mcptool.go:29-30`, `:178-191`,
  `cmd/agezt/main.go:1616`, `kernel/mcp/client.go:112-138`

### The advertised guarantee

```go
// kernel/runtime/mcptool.go:29-30
// AddMCPServer validates and persists a new MCP server registration,
// journaling mcp.added. Registration alone spawns nothing — attach does.
```

The `mcp` tool repeats it to the model (`plugins/tools/mcptool/tool.go:140`): *"registered —
attach it (op=attach) to make its tools callable"*. And the package doc claims the gate is
Ask-class (`kernel/mcp/store.go:11-12`, `plugins/tools/mcptool/tool.go:8-9`): *"gated by the
`mcp.install` Edict capability (Ask by default — attaching spawns an arbitrary process)"*.

### What the code does

```go
// kernel/mcp/store.go:216-218
now := s.now().UnixMilli()
srv.ID = ulid.New()
srv.Enabled = true          // <-- forced, unconditionally, on every Add
```

`Enabled` means "auto-attach when the daemon starts". At boot:

```go
// cmd/agezt/main.go:1616
attached, failures := k.AttachEnabledMCPServers(ctx)
```

`AttachEnabledMCPServers` (`kernel/runtime/mcptool.go:178-191`) calls `AttachMCPServer` for every
enabled row with `corr = ""` and **no `policyHook`, no Edict `Decide`, no approval** anywhere on
that path — it goes straight to `dialMCP` → `mcp.Dial` → `exec.Command(command, args...)`
(`kernel/mcp/client.go:113`). The Edict gate lives only on the *tool* call
(`kernel/agent/run_tools.go:189-214`), which the boot path never enters.

`Validate` (`kernel/mcp/store.go:120-177`) checks the *name* regex, transport exclusivity, arg
count, env-key and header-name shapes — **it never constrains `Command`**. No allowlist, no hash
pin. Compare the two sibling exec paths that do have one:

- `kernel/acpcatalog/acpcatalog.go:302-315` — slug-only resolution, refuses to fall through to a raw command.
- `kernel/plugin/host.go:289-293`, `:1016-1020` — BLAKE3-256 pin, re-verified on reload.
- `kernel/market/vet.go:130-149` — marketplace MCP packs get a runner allowlist + `curl|sh` /
  `sh -c` detection. **The `mcp` tool's own `op=add` gets none of it.**

### Exploitation path

Prompt injection → `mcp {op:"add", name:"x", command:"powershell", args:["-c","<payload>"]}`.
The doc-promised second step (`op=attach`) is **not required**: the registration is already
`Enabled`, so the payload spawns on the next daemon start — after a watchdog restart, a self-update
(`cmd/agezt/boot_ops.go:76-120`), or the operator's next reboot — at full daemon privilege, with
no policy decision, no approval, and no journal entry other than `mcp.attached`. It survives
indefinitely.

### Why this is not a false positive

I looked specifically for a gate on the boot path and there is none: `AttachEnabledMCPServers` has
no `edict`/`policyHook`/`approvals` reference, and `main.go:1616` is inside a plain `bootSteps`
closure. I also confirmed the child env *is* scrubbed (`client.go:320-356`) — that mitigation
holds, so the payload does not get the daemon's secrets. It still gets arbitrary execution.

### Remediation

1. `Store.Add` should default `Enabled = false` for agent-originated registrations (pass the
   origin through `AddMCPServer`), so the doc's "registration alone spawns nothing" becomes true.
2. Run `market.VetPack`-class checks (`kernel/market/vet.go:130-149`) on `Command`+`Args` at
   `mcp.Validate` time, at minimum rejecting `sh -c` / `cmd /c` / `curl…|sh` shapes.
3. Gate `AttachEnabledMCPServers` behind an Edict `mcp.install` decision or an explicit
   `AGEZT_MCP_AUTOATTACH` opt-in.
4. Fix `kernel/mcp/store.go:11-12` and `plugins/tools/mcptool/tool.go:8-9`: `mcp.install` is
   `LevelAllow`, not "Ask by default" (`kernel/edict/edict.go:634-640`).

---

## CE-004 — `config op=register` reopens the raw-command path the ACP CWE-78 fix closed

- **Severity:** High · **Confidence:** 85 · **CWE-78**
- **Files:** `plugins/tools/config/config.go:302-314`, `:264-277`;
  `kernel/settings/registry.go:189-219`; `cmd/agezt/main.go:3809-3814`;
  `plugins/builtintools/envgated.go:65`, `:89`; `plugins/tools/acpagent/acpagent.go:244-246`

### The advertised guarantee

`plugins/tools/acpagent/acpagent.go:238-243` is explicit:

> `// SECURITY: cmdStr is run through the platform shell, so it MUST be trusted.`
> `// Its only sources are (a) the operator-configured default (AGEZT_ACP_AGENT_CMD)`
> `// and (b) a launch command read from the trusted acpcatalog for an installed slug.`
> `// Agent/LLM tool input never reaches here as a raw command …`

`acpcatalog.ResolveCommand` (`kernel/acpcatalog/acpcatalog.go:302-315`) implements the slug-only
half correctly. The unexamined half is the premise that `AGEZT_ACP_AGENT_CMD` is
*operator*-controlled. The `coding` tool makes the same assumption
(`plugins/tools/coding/coding.go:12`).

### What the code does

`AGEZT_ACP_AGENT_CMD` and `AGEZT_CODING_CMD` are **not** in the built-in settings schema — I
counted 204 `Env:` fields in `kernel/settings/schema.go` and neither string appears. They are
therefore absent from `builtinEnvSet()` (`kernel/settings/registry.go:48-56`), which is the only
thing `validateSection` reserves (`registry.go:204-206`). The name passes `envNamePattern`
(`registry.go:27`, `^AGEZT_[A-Z0-9_]+$`).

So the agent-callable `config` tool can:

1. `op=register` a section declaring a field with `env: "AGEZT_ACP_AGENT_CMD"`
   (`config.go:302-314` → `registry.Register` → accepted).
2. `op=set` that name to an arbitrary string — `FieldByEnv` now finds it
   (`registry.go:124-134`), it is not `ReadOnly`, and `TypeText` has no validation
   (`schema.go:619-641`) — so it is written to the settings store (`config.go:264-277`).
3. At the next daemon start, `injectConfig` `os.Setenv`s **every** key in the store with no
   schema filter at all:

```go
// cmd/agezt/main.go:3809-3814
for name, val := range store.All() {
    if val != "" && os.Getenv(name) == "" {
        _ = os.Setenv(name, val)
```

4. `specACPAgent` reads it (`plugins/builtintools/envgated.go:89`) into `Tool.Cmd`; an
   `acp_agent` call with an empty `agent` selector returns the fallback verbatim
   (`acpcatalog.go:305-308`) and it is executed as
   `exec.Command("cmd", "/C", cmdStr)` / `sh -c` (`acpagent.go:245-246`).

The identical chain applies to `AGEZT_CODING_CMD` → `coding.go:147`
(`t.run(ctx, wt, agentEnv, shell, shellArg, t.Cmd)`), where setting the variable also *turns the
tool on* (`envgated.go:65-68` registers `coding` only when it is non-empty).

### Why this is not a false positive

I specifically tried to refute the live-apply variant and **did**: `Registry.Register` forces
`Apply = ApplyRestart` on every field (`registry.go:144-146`) and `Registered()` re-forces it on
read (`registry.go:114-116`), so the `os.Setenv`-now path at `config.go:279-284` is unreachable
for registered fields. Good defense-in-depth — it costs the attacker a restart, nothing more.
I also confirmed built-in names *are* protected (`registry.go:204-206`), so `AGEZT_ALLOW_ALL`,
`AGEZT_AUTO_APPROVE_CAPS`, `AGEZT_APPROVAL_MODE` and
`AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED` cannot be reached this way. The gap is precisely the
command-valued variables that were never added to the schema.

Note `AGEZT_TUNNEL_CMD` (`schema.go:543`, `TypeText`, not read-only) needs no `op=register` at
all — one `op=set` is enough, and it is executed by `kernel/tunnel/tunnel.go:235`.

### Remediation

1. Restrict `injectConfig` (`main.go:3809`) to names present in
   `settings.NewRegistry(baseDir).Sections()` — the store should not be able to set env vars
   nobody declared.
2. Add `AGEZT_ACP_AGENT_CMD`, `AGEZT_CODING_CMD` (and any other command-valued var) to the
   built-in schema as `ReadOnly: true`, which both reserves the name against `op=register` and
   refuses `op=set` (`config.go:205-207`).
3. Mark `AGEZT_TUNNEL_CMD` `ReadOnly: true`.
4. Amend the `SECURITY:` comment at `acpagent.go:238-243` once (2) lands, or immediately if it
   does not — as written it asserts a property the code does not hold.

---

## CE-005 — The container profile, the only one claiming real isolation, is unhardened

- **Severity:** Medium · **Confidence:** 90 · **CWE-250 / CWE-269**
- **File:** `kernel/warden/container.go:57-88`

```go
argv := []string{opts.Runtime, "run", "--rm"}
if opts.Network != "" { argv = append(argv, "--network", opts.Network) }
if spec.WorkDir != "" {
    abs, _ := filepath.Abs(spec.WorkDir)
    argv = append(argv, "-v", abs+":"+containerWorkDir, "-w", containerWorkDir)
}
```

Absent: `--read-only`, `--cap-drop=ALL`, `--security-opt=no-new-privileges`, `--user`,
`--pids-limit`, `--tmpfs`. The bind mount is read-write. The default image is `python:3.12-slim`
(`container.go:14`), whose default user is **root**, so LLM-authored code runs as uid 0 with the
full default Docker capability set (`CAP_DAC_OVERRIDE`, `CAP_CHOWN`, `CAP_SETUID`, …) and can
write to the host directory it is handed.

`--memory` is applied only when `Limits.AddressSpaceBytes > 0` (`container.go:82-84`); `code_exec`
does set it (`codeexec.go:331`), `shell` does not (`shell.go:275-278`), so a container-profile
shell run has no memory bound at all.

This is the one profile `EffectiveProfile` will honestly report as `container`
(`warden.go:266-268`) and the one an operator turning on `AGEZT_WARDEN_DOCKER=1`
(`kernel/settings/schema.go:527`) reasonably believes is the strong tier. It is opt-in, which is
why this is Medium rather than High — but it is the *only* real isolation this codebase offers and
it is a one-liner from being sound.

**Remediation:** append `--read-only --cap-drop=ALL --security-opt=no-new-privileges
--pids-limit=512 --user 65534:65534 --tmpfs /tmp` by default; mount the workdir `:rw` only when
the caller asks and `:ro` otherwise; make the flag set overridable via a single
`AGEZT_WARDEN_DOCKER_ARGS` for operators who need to relax it.

---

## CE-006 — `EffectiveProfile` mislabels the un-namespaced Linux path, and that label is what the model is shown

- **Severity:** Medium · **Confidence:** 95 · **CWE-1059 / CWE-357** (insufficient UI warning)
- **Files:** `kernel/warden/warden_linux.go:54-64`, `kernel/warden/warden.go:32-36`,
  `plugins/tools/codeexec/codeexec.go:857`, `:937`

The package doc tells callers to key trust off `EffectiveProfile` (`warden.go:32-36`) because it
"downgrades honestly". On Linux it does not: `resolveEffectiveProfile(ProfileNamespace)` returns
`ProfileNamespace` (`warden_linux.go:58-59`) for a run that engages **`Setpgid` plus best-effort
`prlimit64` and nothing else** — the same file states this plainly at `:26-33` (*"No namespaces
(CLONE_NEWUSER / CLONE_NEWNS / CLONE_NEWPID), no seccomp BPF, no cgroup v2"*).

The consequence is not theoretical: because `Downgraded` is false, `render` prints a bare
`isolation=namespace` header **into the model's tool result** (`codeexec.go:857-860`) with no
downgrade note, and `publish` writes `"profile_effective": "namespace"` into the journal
(`codeexec.go:937`). An operator reading `agt why` sees a word that means containment and gets
rlimits.

Two things that *are* right and should be preserved: the credential bucket is keyed off
`EffectiveProfile`, not the request (`codeexec.go:263`, `shell.go:245` — the RCE-001 fix, verified
present); and the package doc's retraction (`warden.go:17-38`) is accurate.

**Remediation:** rename the shipped Linux tier (e.g. `ProfileRlimit`) or have
`resolveEffectiveProfile` return `ProfileNone` with a `warden.profile_downgraded` event until real
namespaces land, so the honest signal the doc promises actually exists. Minimally, make `render`
print the enforced mechanisms rather than the profile name.

---

## CE-007 — SSH / Kubernetes execution-profile argv injection

- **Severity:** Medium · **Confidence:** 70 · **CWE-88** (argument injection)
- **Files:** `kernel/executionprofile/ssh.go:37-41`, `:72-88`; `kernel/executionprofile/k8s.go:36-45`

`ShellQuote` (`ssh.go:90-92`) is applied to the *workdir* and the *command*
(`ssh.go:60`, `:67`) — correctly. It is **never** applied to the connection parameters, which are
appended as bare argv elements:

```go
// kernel/executionprofile/ssh.go:37-41
func (c SSHConfig) Args() []string {
    args := c.clientArgs(false)
    args = append(args, c.Target)      // <-- bare
    return args
}
```

`AGEZT_EXEC_SSH_TARGET = "-oProxyCommand=<payload>"` makes `ssh` parse it as an option; the
payload then executes **on the AGEZT host**, not the remote, defeating the whole point of the
remote profile. `AGEZT_EXEC_SSH_IDENTITY` (`ssh.go:78`) and the K8s `--context`/`-n`/pod values
(`k8s.go:38-44`, `:57`) have the same shape.

Reachability is the reason this is Medium and not High. Both variables are `ApplyLive` settings
fields (`kernel/settings/schema.go`, execution-profiles section) so the `config` tool can set them
live — but the *profile selection* is not agent-controlled: it comes from the control-plane
`execution_profile` argument or `roster.Profile.ExecutionProfile`
(`kernel/controlplane/server.go:1243-1249`), and I **verified that `ExecutionProfile` is not in the
overseer tool's editable field list** (`plugins/tools/overseertool/kernelsource.go:126-178` — it
patches `workdir`, `tool_allow`, `config_overrides`, etc., but not `execution_profile`). So the
realistic scenario is a poisoned target lying in wait for the operator's next
`agt run --exec-profile ssh`, not a self-contained agent chain.

**Remediation:** validate `Target` against `^[A-Za-z0-9._-]+(@[A-Za-z0-9._-]+)?$`, `Port` against
`^[0-9]{1,5}$`, and insert `--` before the target/pod; reject any of these values beginning with
`-`. Same treatment for the K8s context/namespace/pod/container fields.

---

## Confinement matrix

What is **actually enforced** per platform × requested profile, versus what the profile name and
docs imply. Derived from `warden.go:277-409`, `warden_linux.go:54-116`, `warden_other.go:20-23`,
`container.go:57-88`, `runtimes.go:96-112`.

| Platform | Requested | `EffectiveProfile` returns | Actually enforced | Documented / implied |
|---|---|---|---|---|
| Linux | `none` | `none` | timeout+WaitDelay; 256 KiB output cap; `cmd.Dir`; explicit env (nil⇒empty); `warden.exec` audit | matches |
| Linux | `namespace` | **`namespace`** | the above **+ `Setpgid` + `cmd.Cancel`→`SIGKILL(-pgid)` + best-effort `prlimit64` on CPU/AS/NOFILE/FSIZE** | "Linux namespaces + cgroups + seccomp" (`warden.go:13`). **None of the three exist** — retracted honestly at `warden.go:22-27` / `warden_linux.go:26-33`, but the returned string still says `namespace` → **CE-006** |
| Linux | `container` / `microvm`, docker **off** | `namespace` | as `namespace` above | `Downgraded=true`, event emitted — honest |
| Linux | `container`, `AGEZT_WARDEN_DOCKER=1` | `container` | `docker run --rm [--network none] -v <wd>:/workspace:rw` + `-e` allowlist + `--memory` *(code_exec only)* | "OCI container". **root, all default caps, writable host bind, no `--read-only`/`--cap-drop`/`no-new-privileges`/`--user`/`--pids-limit`** → **CE-005** |
| **Windows** | any (`none`/`namespace`/`container`/`microvm`) | **`none`** | timeout; output cap; `cmd.Dir`; explicit env; audit. `configurePlatformAttrs` + `applyPlatformLimits` are **no-ops** (`warden_other.go:22-23`). No rlimits, no process group, no job object. Plus `cmd /S /C "<raw>"` verbatim (`cmdline_windows.go:43`) | "ALL profiles resolve to ProfileNone. Nothing is isolated." (`warden.go:28-30`) — accurate |
| **macOS / other** | any | **`none`** | identical to Windows; no `sandbox-exec` | same — accurate |
| any | Deno via `code_exec` | (per above) | **plus a real OS jail**: `--allow-read=<dir> --allow-write=<dir> --allow-env`, **no `--allow-run`**, `--allow-net` only when granted (`runtimes.go:96-112`) | accurate; the strongest confinement actually shipped |
| any | Python / Node via `code_exec` | (per above) | scratch dir + scrubbed env only — **no filesystem confinement**, `HOME`/`TMP` redirected into the workdir (`runtimes.go:141-154`) but nothing stops absolute-path access | `codeexec.go:20-22` is honest ("workdir/env/limits-only elsewhere") |
| any | `shell` | (per above) | **nothing beyond the row above.** Warden has no path jail; the file tool's traversal guards do not apply to shell (`shell.go` doc `:13-17`) | accurate |

Cross-cutting: **the warden never confines the filesystem on any platform or profile except the
container path and Deno.** `WorkDir` is `cmd.Dir` (`warden.go:320`), nothing more.

---

## Verified safe

Things I attacked and could not break. Recording these is as load-bearing as the findings.

### A. Tar extraction — no slip, no symlink escape, no bomb

`plugins/tools/codeexec/artifacts.go:292-352` (`extractArtifactArchive`, fed by
attacker-controlled Modal stdout at `codeexec.go:635-640`). I tried every escape I know:

- `hdr.Name` goes through `sanitizeRelFile` (`runtimes.go:197-211`) which rejects absolute paths,
  `..`, `../`, leading `/`, NUL, and any `:` (kills the Windows drive-relative `C:foo` trick).
  On Windows `filepath.FromSlash`+`Clean`+`ToSlash` normalizes `..\..\x` to `../../x`, which the
  `../` prefix check then catches (`artifacts.go:309-312`).
- **Symlinks and hardlinks are dropped**: only `tar.TypeDir` and `tar.TypeReg` are handled, all
  other typeflags hit `default: continue` (`:313-322`). No symlink can be planted to redirect a
  later entry.
- The destination is a fresh `os.MkdirTemp` (`:130-143`), so there is no pre-existing link to
  follow.
- Bomb caps on file count, per-file size, and total (`:323-333`), and the body is read with
  `io.CopyN(f, tr, hdr.Size)` (`:342`) rather than trusting the stream.

`cmd/agt/backup.go:465-514` is likewise sound: `isAllowedBackupPath` (`:519-530`) requires a known
subtree prefix and rejects any `..`; a second lexical `HasPrefix(target, cleanDest+sep)` at `:496`;
non-regular entries skipped at `:483`; `O_EXCL` at `:502` prevents overwrite. Its only weakness is
an unbounded `io.Copy` (`:506`) — a disk-fill DoS on an archive the operator chose to restore.
Not filed.

### B. Deserialization surface — genuinely absent

Repo-wide search confirms Phase 1: **no `encoding/gob`, no YAML unmarshal of any kind, no
`archive/zip`.** The only `json.Unmarshal` into a `map[string]any` outside tests is
`kernel/runtime/reaper.go:914`, which reads a string field out of an event payload — no type
dispatch, no object construction. Every other decode targets a concrete struct. **CWE-502 has no
surface in this codebase.**

### C. The update flow refuses a caller-forged trust anchor

I went hunting for the obvious bug — `UpdateInfo.Provenance` is an exported field with no `json:"-"`
tag, so unmarshalling a request body straight into it would let a caller claim
`ProvenanceGitHubRelease` and skip `ErrSignatureKeyNotConfigured`. **Both handlers build the struct
field-by-field from a typed args struct instead**: `kernel/restapi/update_handlers.go:105-110` and
`kernel/controlplane/update_control.go:145-150`. `Provenance` stays at its zero value,
`verifySignature` (`kernel/update/update.go:426-445`) refuses it, and the constant is set only
inside `checkGitHub` (`:530`) / `checkEndpoint` (`:577`). Correctly implemented; the zero value is
the untrusted one. (The separately-known gap — `DefaultPublicKeyHex = ""` at `update.go:380`
leaving the GitHub path checksum-only — is a release-engineering step, not a code defect, and is
documented accurately at `:368-380`.)

### D. Per-agent `config_overrides` cannot smuggle arbitrary settings

I expected this to be exploitable: `plugins/tools/overseertool/repair.go:228-235`
(`applyRepairProposal`) copies an arbitrary `config_overrides` map straight out of the LLM's
final-text JSON block into a roster profile, and the repair brief literally tells the model *"That
block will be applied automatically"* (`repair.go:104`). But **application is allowlisted**:
`applyAgentOverrides` (`kernel/runtime/agentconfig.go:161-179`) iterates the fixed
`agentOverrides` table (`:51-71`, nine knobs: model, max-iter, auto-continue, parallel-tools,
discovery-max, context-budget, observation-deltas, heuristic-bypass) — **not** the supplied map.
An entry like `AGEZT_ALLOW_ALL` is stored and never read. The table's own comment explains it was
built precisely to stop the three-copies drift that would have caused this. Clean design.

### E. Workflow `code` nodes are gated, and the payload cannot reach the code body

`kernel/runtime/workflowrun.go:547-574`: a `NodeCode` execution first calls
`k.policyHook(ctx, {Name:"code_exec", …})` and aborts on a non-allow verdict. Critically,
`c.Code` is passed to `RunScript` **verbatim** — only `c.Input` is run through
`workflow.Interpolate(…, data)` (`:565-567`). So a webhook body arriving as
`{{trigger.payload.*}}` cannot inject into the program text of a stored workflow. `invokeWorkflowTool`
(`:735-765`) additionally schema-validates then policy-gates every tool node. The
`workflow.Interpolate` engine itself (`kernel/workflow/template.go`) is a dotted-path lookup with
no calls, pipes, or arithmetic — the right design for that surface.

### F. `acp_agent`'s slug allowlist works (as long as CE-004 is fixed)

`kernel/acpcatalog/acpcatalog.go:302-315`: a non-empty `agent` selector from LLM input **must**
name an installed catalog slug; an unknown ref returns `ok=false` rather than falling through to a
raw command. The one thing it cannot defend is the operator-configured fallback — which is
exactly CE-004.

### G. Approval registry is fail-closed

`kernel/approval/approval.go:202-253`: `Submit` blocks; the timer branch synthesises
`DecisionTimeout` and `ctx.Done()` synthesises `DecisionCancel`; `Resolve` accepts only
`grant`/`deny` (`:260-262`); every outcome is journaled. SPEC-06's "time-outs default to deny" is
correctly implemented. The mechanism is sound — CE-002 is about it being bypassed, not broken.

### H. Toolbox host-package installs are catalog-indexed, not caller-constructed

`kernel/toolbox/toolbox.go:246-273`: `byName` (`:288-295`) resolves the request against the
compiled-in `Catalog` and returns `Skipped` for anything unknown; the argv comes from
`ResolveInstall`, not the caller; `exec.CommandContext` with no shell. There is also **no
`toolbox` agent tool** in `plugins/builtintools/tools.go:44-88`, so this is operator-only. (Worth
noting for the ops domain: `cmd.Env` is unset, so a package post-install script inherits the full
daemon environment.)

### I. Marketplace pack vetting exists and is honest about being advisory

`kernel/market/vet.go:130-149` scans MCP command lines for `curl|sh`, `iwr|iex`, and raw
`sh -c`/`cmd /c` hosts, flags unrecognised launchers against a runner allowlist, and warns on
credential env requests. The doc states plainly it is *"INFORMATIONAL, never a wall"*
(`vet.go:22-24`) — code and comment agree. There is also **no `market` agent tool** registered, so
`market.install` is reachable only from the console/CLI.

### J. Other small confirmations

- `kernel/agent/toolctx.go:127-159` — the per-agent workdir setter refuses absolute paths and
  every `..` shape before a tool can see it, so `shell.go:253-256`'s
  `filepath.Join(t.WorkDir, wd)` cannot escape. The comment at `shell.go:251` is accurate.
- `validatePackages` (`plugins/tools/codeexec/packages.go:27-43`) blocks leading `-` and
  whitespace, so `--index-url=evil` cannot be smuggled into `pip install`. (A malicious *package
  name* is still installable — that is the accepted design.)
- `kernel/runtime/policy.go:72-82` — capability resolution prefers `ToolDef.Capability`, then the
  plugin overlay, then the name switch, and **ignores a declared capability Edict does not know**
  rather than honouring it. I confirmed forged and bridged tools leave `ToolDef.Capability` zero
  (`scripttool.go:251-264`, `mcptool.go:328-341`), so the `forge_*`→`code.exec` and
  `mcp_*`→`mcp.call` prefix rules (`edict/toolmap.go:20-32`) really do fire.
- `conductorVerify` (`kernel/runtime/conductor.go:268-285`) executes the worker model's fenced
  code with no second policy hook — but this is compensated and documented: the outer `conductor`
  tool call maps to `CapCodeExec` (`edict/toolmap.go:136-144`), so denying `code.exec` denies the
  conductor. Not a finding.
- `kernel/plugin/pin.go:71-85` + `host.go:289-293`, `:1016-1020` — BLAKE3-256 pin verified at
  spawn *and* re-verified before reload, with `resolvePluginPath` (`pin.go:26-34`) ensuring the
  hashed file and the executed file are the same one. Note the pin is **optional**
  (`host.go:289`, `if cfg.PinnedHash != ""`), which is the operator's call for their own binaries.

### K. Known-but-not-mine

`kernel/runtime/council.go:298` invokes `web_search` with **no policy hook** — a real Edict bypass,
but on a fetch axis, so it belongs to the SSRF/egress domain. Likewise
`kernel/agent/run_tools.go:188` initialises `PolicyVerdict{Allow: true}` when `cfg.Policy == nil`:
fail-open in principle, but the sole in-tree `LoopConfig` construction
(`kernel/runtime/loopconfig.go`) always sets `Policy`, so the daemon is not affected. It is a trap
for SDK embedders, not a live vulnerability here.
