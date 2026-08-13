# AGEZT — Injection Domain Results (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (`main`)
**Skills applied:** `sc-cmdi`, `sc-ssti`, `sc-header-injection`, `sc-nosqli`, `sc-sqli`, `sc-graphql`, `sc-xxe`, `sc-ldap`

**Method:** two internal phases. (1) Discovery — grep every sink class, trace each candidate back to a
source. (2) Verification — actively attempt to refute each candidate by reading the guard, the
sanitizer and the caller chain. Findings that did not survive that pass are recorded under
**Verified safe**, not filed. Every `file:line` below was read in this session.

**Headline:** 3 findings (2 High, 1 Low). Four of the eight assigned vulnerability classes do not
exist in this codebase. The highest-value result is that the Edict *catastrophe* hard-deny rails —
the one guard the codebase says survives the default-allow posture — are inert for every execution
capability except `shell`.

---

## Findings by severity

| ID | Title | Severity | Confidence |
|---|---|---|---|
| INJ-001 | Edict hard-deny catastrophe rails are inert for every non-shell execution capability | **High** | 95 |
| INJ-002 | Argument/JSON injection into workflow tool-node arguments via raw-text interpolation | **High** | 85 |
| INJ-003 | `Content-Disposition` quoted-string breakout in the File Manager raw download | Low | 70 |

---

### INJ-001 — Edict hard-deny catastrophe rails are inert for every non-shell execution capability

- **Severity:** High · **Confidence:** 95
- **CWE:** CWE-693 (Protection Mechanism Failure), enabling CWE-78
- **File:** `kernel/edict/edict.go:645-667`, `kernel/edict/edict.go:373-378`, `kernel/edict/toolmap.go:25-32`, `:136`, `:144`

**The vulnerable code.** All sixteen built-in hard-deny rules are scoped to a single capability
(`kernel/edict/edict.go:645-667`):

```go
{Name: "fork-bomb",   Substring: ":(){:|:&};:", AppliesTo: []Capability{CapShell}},
{Name: "rm-rf-root",  Substring: "rm -rf /",    AppliesTo: []Capability{CapShell}},
{Name: "mkfs",        Substring: "mkfs",        AppliesTo: []Capability{CapShell}},
{Name: "dd-of-sd",    Substring: "of=/dev/sd",  AppliesTo: []Capability{CapShell}},
```

and the matcher short-circuits on capability before it ever looks at the string
(`kernel/edict/edict.go:373-378`):

```go
func (r HardDenyRule) matches(cap Capability, input string) bool {
	if len(r.AppliesTo) > 0 && !slices.Contains(r.AppliesTo, cap) {
		return false
	}
	return strings.Contains(strings.ToLower(input), strings.ToLower(r.Substring))
}
```

**Tainted source → sink flow.** LLM output / agent tool-call arguments (first-class untrusted input
in this architecture — a prompt-injected agent chooses the tool name *and* the arguments) →
`kernel/agent/run_tools.go:189-214` policy hook → `CapabilityForToolCall`
(`kernel/edict/toolmap.go:20`) → `Engine.DecideWithCeiling` (`kernel/edict/edict.go:754`, hard-deny
loop at `:765-779`) → `warden.Run` → `exec.CommandContext` (`kernel/warden/warden.go:319`).

The mapping is what defeats the floor. Every alternative execution route resolves to a capability
that appears in **no** rule's `AppliesTo`:

| Tool | Mapped capability | Citation | Hard-deny rules that can fire |
|---|---|---|---|
| `shell` | `CapShell` | `toolmap.go:34-35` | all 16 |
| `code_exec` | `CapCodeExec` | `toolmap.go:135-136` | **0** |
| `conductor` | `CapCodeExec` | `toolmap.go:137-144` | **0** |
| `forge_*` | `CapCodeExec` | `toolmap.go:25-27` | **0** |
| `tool_forge` op=test | `CapCodeExec` | `toolmap.go:97-100` | **0** |
| `mcp_*` | `CapMCP` | `toolmap.go:30-32` | **0** |
| `coding` | `CapCoding` | `toolmap.go:171-172` | **0** |
| `acp_agent` | `CapACPAgent` | `toolmap.go:173-174` | **0** |

**Exploitation path (concrete).** An agent is prompt-injected through any of the documented taint
sources — an inbound channel message (`plugins/channels/`), a `POST /hooks/<workflow>` body, a
fetched web page consumed by `browser.read`, or a poisoned memory record.

1. The injected instruction first tries the obvious route:
   `shell {"command":"rm -rf / --no-preserve-root"}` → `cap = CapShell` → rule `rm-rf-root` fires at
   `edict.go:766-778` → **Deny**. The guard works.
2. It then reissues the identical action through the sandbox tool:
   `code_exec {"language":"python","code":"import os; os.system('rm -rf / --no-preserve-root')"}`
   → `cap = CapCodeExec` (`toolmap.go:135-136`) → the loop at `edict.go:766` calls `r.matches` sixteen
   times and every call returns `false` at `edict.go:374` because `CapCodeExec ∉ [CapShell]` → no
   hard-deny → level lookup `e.levels[CapCodeExec]` = `LevelAllow` (`edict.go:634-640`) → **Allow**.
3. `plugins/tools/codeexec/codeexec.go:322-337` writes the model's code to `entry` and runs
   `buildArgv(interp, lang, entry, dir, allowNet)` under warden. There is no content denylist
   anywhere on this path — the only pre-execution validation is `validatePackages` for `pip`
   package *names*. The command executes.

On Windows and macOS the warden resolves every profile to `ProfileNone`
(`kernel/warden/warden_other.go:20`), so nothing else stands between step 3 and the host.

**Why this is not a false positive — what I checked.**

- I did *not* rely on the recon note; I read `DefaultHardDeny()` and confirmed all sixteen entries
  carry `AppliesTo: []Capability{CapShell}` with no exception.
- I confirmed the capability passed to the hard-deny loop is the *mapped* one, not the tool name:
  `DecideWithCeiling(cap, ...)` at `edict.go:754` receives `cap` and passes it straight into
  `r.matches(cap, c)` at `edict.go:768`.
- I checked whether a second, unscoped floor exists elsewhere. It does not: the only other
  `HardDeny` producers are `main.go:278-283` (operator rules appended to the same slice) and
  `Engine.AddHardDeny`. Operator-supplied rules *can* be global (an empty `AppliesTo` matches every
  capability — proven by `edict_test.go:458-468`, `TestCustomHardDenyList`), but no built-in rule
  uses that form.
- I checked whether `code_exec` has its own denylist. `plugins/tools/codeexec/codeexec.go:186-344`
  has none; `validatePackages` only rejects package names with a leading `-`.
- I confirmed the evasion-hardening at `edict.go:388-399` (`denyCandidates`, JSON-decode +
  whitespace-collapse) operates on the *input string* only and does nothing about the capability
  short-circuit that happens first.
- Scope check: `edict_test.go:399-407` (`TestDecide_HardDenyScopedToCapability`) shows the scoping is
  *deliberate* — it exists so `rm -rf /` inside an HTTP request body is not denied. That rationale
  is sound for `CapHTTPPost`. It is not sound for `CapCodeExec`, `CapCoding` or `CapACPAgent`, which
  are execution capabilities where the string is a command, not a payload. The test locks in the
  behaviour but never asserts anything about the execution capabilities.

**The advertised-guarantee gap (why this is reportable under the default-allow carve-out).** This is
not a "defaults to allow" complaint — it is a guard that is documented as surviving the permissive
posture and provably does not. Two places state it:

- `kernel/edict/edict.go:622-624` — the `DefaultLevels` doc comment lists what the MAX-AUTONOMY
  posture "deliberately does NOT relax", first item: *"the F4 hard-deny strings (fork bombs,
  rm -rf /, raw-device writes)"*. Stated unconditionally, with no mention that it holds for one tool.
- `cmd/agezt/main.go:287-289` — *"The built-in catastrophe hard-deny rails (fork-bomb,
  dd-to-raw-device) deliberately stay, since they guard against self-destruction rather than gate
  normal tools"*. Under `AGEZT_ALLOW_ALL=1` they stay for `shell` and for nothing else.

It is also the "opt-out that fails to restrict when actually configured" case: an operator who sets
`shell` to L0 through the Policy view still has a fully-functional, hard-deny-free `os.system()` via
`code_exec` at L4.

**Remediation.**
1. Split the rail set by intent. Catastrophe rails (fork-bomb, `rm -rf /`, `mkfs`, `wipefs`,
   `dd of=/dev/*`, `shutdown`/`poweroff`/`reboot`, `format-volume`) describe *host destruction* and
   should carry `AppliesTo: nil` (all capabilities), or at minimum the execution set
   `{CapShell, CapCodeExec, CapCoding, CapACPAgent, CapMCP, CapToolForge}`.
2. Keep the `edict_test.go:399` intent by scoping the exemption to the *data* capabilities
   (`CapHTTPGet`, `CapHTTPPost`, `CapMemory`, `CapNotify`, `CapBoard`, …) via a `NotAppliesTo`
   field, rather than by enumerating a single allowed capability.
3. Add a regression test asserting `Decide(CapCodeExec, "…rm -rf /…").HardDenied == true`.
4. Correct `edict.go:622-624` and `main.go:287-289` to state the real scope until (1) ships.

---

### INJ-002 — Argument/JSON injection into workflow tool-node arguments via raw-text interpolation

- **Severity:** High · **Confidence:** 85
- **CWE:** CWE-88 (Argument Injection), CWE-94
- **File:** `kernel/runtime/workflowrun.go:454`, `kernel/workflow/template.go:36`, `:69-82`, `kernel/workflow/workflow.go:315`

**The vulnerable line** (`kernel/runtime/workflowrun.go:449-460`):

```go
case workflow.NodeTool:
	var c workflow.ToolConfig
	if err := json.Unmarshal(n.Config, &c); err != nil { return nil, "", err }
	args := strings.TrimSpace(workflow.Interpolate(string(c.Args), data))
	if args == "" { args = "{}" }
	return k.invokeWorkflowTool(ctx, c.Tool, "wf-"+n.ID, json.RawMessage(args))
```

`c.Args` is `json.RawMessage` — its own declaration calls it *"templated JSON"*
(`kernel/workflow/workflow.go:315`). `Interpolate` is applied to the **serialized JSON text**, and
the substituted value is written with no escaping at all (`kernel/workflow/template.go:36`,
`renderValue` at `:69-82`: a `string` is returned verbatim, anything else as compact JSON — which
itself contains `"`). The result is re-cast to `json.RawMessage` and becomes the tool's argument
object. Attacker-controlled text is therefore interpolated into a JSON *string literal* it can
close.

**Tainted source → sink flow.**
`POST /hooks/<workflow>` body (`kernel/webui/webui.go:1012-1022` — the body is JSON-decoded into
`any`, or rides **verbatim as a string** at `:1020`) → `payload := map[string]any{"kind":"webhook",
"body": body}` (`:1030-1033`) → `CmdWorkflowWebhook` → run context `data` → `Interpolate` at
`workflowrun.go:454` → `json.RawMessage(args)` → `invokeWorkflowTool` → `tool.Invoke`
(`workflowrun.go:753`).

**Exploitation path (concrete), using a shipped template.**
`kernel/workflow/templates.go:141` ships this node:

```go
{ID: "save", Type: NodeTool, Label: "Remember digest",
 Config: raw(`{"tool":"memory","args":{"action":"remember","subject":"list-pipeline","content":"digest: {{shape.output}}"}}`)}
```

`{{shape.output}}` derives from `{{trigger.payload.items}}` via the map node at `templates.go:140`.
An attacker who can reach the trigger (the workflow's `/hooks/` secret, or any channel/schedule
wired to it) posts an item whose `name` is:

```
x", "action":"forget", "junk":"
```

After interpolation the raw args text becomes a **structurally valid** JSON document carrying a
second `action` key. Go's `encoding/json` takes the last occurrence of a duplicate key, so the node
executes `action=forget` instead of the `action=remember` the workflow author wrote. The attacker
has overridden the node's declared operation.

The same mechanism is materially worse for a tool whose arguments are themselves a command string.
A tool node with `{"tool":"shell","args":{"command":"echo {{trigger.payload.body}}"}}` — the
natural way to write "log the webhook" and a shape the node library invites — needs no JSON
breakout at all: `;`, `|`, `$( )` and backticks require no JSON escaping, so the webhook body *is*
the shell command.

**Why this is not a false positive — what I checked.**

- I read `Interpolate` (`template.go:21-39`) and confirmed it is pure text substitution into `s`,
  with no JSON awareness and no escaping hook.
- I confirmed `renderValue` (`template.go:69-82`) returns `string` values **verbatim** (`case
  string: return t`) — there is no quoting layer anywhere between payload and JSON text.
- I confirmed the sibling HTTP node does it *correctly*, which rules out "this is just how the
  engine works". At `workflowrun.go:531-541` `NodeHTTP` interpolates each value and then
  `json.Marshal`s a `map[string]any` — structural construction, values escaped by the encoder. The
  tool node is the outlier, not the pattern.
- I checked whether the policy gate blocks it. It does not: `invokeWorkflowTool`
  (`workflowrun.go:736-752`) runs `ValidateToolInput` at `:742` and `k.policyHook` at `:745` — both
  against the **post-injection** `args`. A well-formed injection passes schema validation, and the
  capability Edict then computes is the *injected* one. Combined with `DefaultLevels()` = L4
  (`edict.go:634-640`) nothing stops it. A *mal*-formed injection fails at `:742`, so the engine
  fails safe only against a clumsy attacker.
- I confirmed there is no re-interpolation loop: `Interpolate` advances `s = s[start+end+2:]` after
  writing the substituted value, so injected `{{…}}` in the payload is not re-expanded. Second-order
  template injection is genuinely absent — the flaw is JSON-structural, not template-recursive.
- I confirmed reachability of `trigger.payload` from an unauthenticated-token path: `/hooks/` is
  `TierPublic` and gated only by the per-workflow secret, accepted from `?secret=`
  (`webui.go:1008-1011`).

**Remediation.** Stop templating serialized JSON. Decode `c.Args` into `map[string]any` first, walk
the tree, apply `Interpolate` to each **string leaf**, then `json.Marshal` the result — exactly the
shape `NodeHTTP` already uses at `workflowrun.go:535-541`. As a defence in depth, evaluate the
policy hook against the node's *declared* tool + argument shape as well as the resolved one, so a
payload cannot change which capability is being exercised.

---

### INJ-003 — `Content-Disposition` quoted-string breakout in the File Manager raw download

- **Severity:** Low · **Confidence:** 70
- **CWE:** CWE-113 (improper neutralization in an HTTP header) — header *value* corruption, not response splitting
- **File:** `kernel/webui/files_route.go:377-379`

```go
name := filepath.Base(targetAbs)
if download {
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
}
```

**Flow.** An agent holding `file` write capability (`CapFileWrite`, L4 by default) creates a file
inside the File Manager root whose name contains a double quote — legal on Linux and macOS — e.g.
`report".txt; filename="payroll.xlsx`. The operator later clicks download in the console;
`filepath.Base` returns the name unchanged and it is concatenated into the header value, closing the
quoted string early and appending attacker-chosen parameters. The browser saves the file under the
spoofed name.

**Why this is not a false positive, and why it is only Low.**

- I confirmed the value is genuinely unsanitized here, and that the codebase *knows* the right
  answer 90 lines away: `kernel/webui/artifact_route.go:53` calls `sanitizeFilename`, and
  `sanitizeFilename` (`artifact_route.go:79-90`) strips `\`, `/`, `"`, `\n` and `\r`. The File
  Manager route simply does not call it. That inconsistency is the finding.
- I refuted the *response-splitting* escalation: Go's `net/http` response writer runs header values
  through a replacer that converts `\r` and `\n` to spaces before emitting them, so CRLF cannot
  split the response. Impact is confined to filename spoofing, hence Low.
- On Windows — the owner's platform — `"` is not a legal filename character, so the finding does not
  reproduce there. It reproduces on a Linux or macOS deployment.

**Remediation.** Call the existing `sanitizeFilename` helper at `files_route.go:379`, or use
`mime.FormatMediaType("attachment", map[string]string{"filename": name})`.

---

## Verified safe

Checks that were actively attacked and held. Recorded because a negative result here is load-bearing
for the next scan.

| # | What I attacked | Verdict | Evidence |
|---|---|---|---|
| 1 | **`ShellQuote`** — the POSIX quoting primitive | **Sound.** I could not break it. | `kernel/executionprofile/ssh.go:90-92`. It is the canonical `'` → `'"'"'` form. Tracing `a'; id; '` yields `'a'"'"'; id; '"'"''`, which the shell reassembles as the single literal word `a'; id; '`. Backslashes, newlines and `$()` are inert inside single quotes. A NUL byte is rejected by Go's `os/exec` before reaching a shell, so it errors rather than injects. |
| 2 | Remote-exec **command** strings (ssh / k8s / modal / daytona) | Safe | The recon note that "`ShellQuote` is applied to the workdir only, never to `command`" is accurate but is **not** a vulnerability. SSH quotes it explicitly (`ssh.go:60`, `"sh -lc "+ShellQuote(command)`). K8s (`k8s.go:61`), Daytona (`daytona.go:47`) and Modal (`modal.go:56`) pass `command` as a **discrete argv element**, never concatenated into a larger shell string, so there is no surrounding syntax to escape. |
| 3 | Remote-exec **config** as an injection source | Safe | `WithSSHOverride`/`WithK8sOverride`/`WithModalOverride`/`WithDaytonaOverride` are called from exactly one non-test site, `kernel/controlplane/server.go:1259-1315`, and every one builds its config from `…ConfigFromEnv()`. The request body chooses only the profile *name* (`execution_profile`, `server.go:1238`) from a fixed set. No request-controlled value reaches `ssh`/`kubectl` argv, so the classic `-oProxyCommand=` argument injection against `SSHConfig.Target` is not reachable from any HTTP surface. |
| 4 | `fixupWindowsCmd` — the verbatim `cmd /S /C` path | Safe *as used* | `kernel/warden/cmdline_windows.go:26-44` joins `cmd.Args[2:]` with spaces and no quoting, which would be argument→command injection for any caller passing untrusted argv tails. I enumerated every `cmd /C` caller that reaches warden: `plugins/tools/shell/shell.go:268` (`{shellBin, shellArg, in.Command}` — exactly 3 args, and the command is the tool's declared purpose), `plugins/tools/coding/coding.go:147` and `plugins/tools/acpagent/acpagent.go:246`. All three pass a single command element. No caller today appends an untrusted argument after a fixed command, so the join is not currently exploitable — but it is a latent trap for the next caller. |
| 5 | `coding` tool — LLM task text into a shell | Safe, and well designed | `plugins/tools/coding/coding.go:145-147` puts the LLM-authored task into an **environment variable** (`AGEZT_CODING_TASK=`+task) and executes only the operator-configured `t.Cmd`. The task never enters the command string. |
| 6 | `acp_agent` — LLM `agent` selector into a shell | Safe | `plugins/tools/acpagent/acpagent.go:238-243` documents the invariant and it holds: the selector is resolved slug-only through `acpcatalog.ResolveCommand`, which refuses to fall through to a raw command. |
| 7 | `mcp` tool → child process spawn | Not command injection | `kernel/mcp/client.go:113` is `exec.Command(command, args...)` — argv form, **no shell**, so no metacharacter interpretation. It is arbitrary *program* execution (the tool's declared purpose, lacking an allowlist or hash pin), which belongs to the authorization domain, not the injection domain. Noted here so it is not double-counted. |
| 8 | **SSTI in the workflow expression engine** | Safe by construction | `kernel/workflow/template.go:21-82`. `{{dotted.path}}` lookups against `map[string]any`/`[]any` only: no pipes, no function calls, no arithmetic, no attribute/reflection access, no object graph. Misses return `""` (`Lookup` `:43-67`). Substituted values are **not re-scanned** (`Interpolate` advances past the substitution at `:37`), so second-order template injection is impossible. This is the safest reasonable design for the surface — the INJ-002 flaw is JSON-structural and downstream of it, not a template-evaluation flaw. |
| 9 | Outbound **HTTP header injection** (CRLF) | Safe — framework-enforced | Two LLM/attacker-controlled header maps exist: `plugins/tools/http/http.go:231-232` (`for k, v := range in.Headers { req.Header.Set(k, v) }`, keys *and* values chosen by the model) and `kernel/mcp/http.go:255`. Go's `net/http` validates header names and values with `httpguts.ValidHeaderFieldName`/`ValidHeaderFieldValue` when writing the request and returns an error, so a `\r\n` payload aborts the request instead of smuggling a header. Matches `sc-header-injection` false-positive class 1. |
| 10 | Response-header injection / response splitting | Safe — framework-enforced | Go's response writer rewrites `\r` and `\n` in header values to spaces before emitting. I surveyed every `w.Header().Set` in the tree; the overwhelming majority take constants. The only attacker-influenced values are the two `Content-Disposition` sinks (one sanitized at `artifact_route.go:53`, one not — INJ-003) and `Content-Type` at `artifact_route.go:41-42`, which is allowlisted by `safeContentType` (`:65-76`) with `application/octet-stream` as the default and an added CSP sandbox for SVG (`:50`). That route is correctly built. |
| 11 | Host-header injection | Not present | No password-reset or email-link flow exists (no password hashing or recovery anywhere). `r.Host` is used only for the same-origin comparison in `sameOriginMutation` and the `hostAllowed` allowlist, never to construct a URL that is emailed or persisted. |
| 12 | Workflow **HTTP node** argument construction | Safe | `kernel/runtime/workflowrun.go:531-541` interpolates values and then `json.Marshal`s a `map[string]any`. Structural construction, encoder-escaped. This is the correct pattern and the direct contrast that makes INJ-002 a bug rather than a design property. |

---

## Not applicable

Vulnerability classes with no surface in this codebase, with the evidence for absence.

| Class | Status | Evidence |
|---|---|---|
| **SQL injection** (`sc-sqli`) | **Absent — independently confirmed** | I did not take the recon's word for it. A tree-wide grep for `database/sql`, `gorm.io`, `jmoiron/sqlx` and `sql.Open` across all `*.go` returns **zero** matches. `go.mod:5-13` declares no database driver (the only dependencies are btcec, coder/websocket, go-imap, `golang.org/x/net`, blake3). Persistence is entirely file-based — `kernel/journal`, `kernel/jsonstore`, `kernel/state`, `kernel/datalake`, `kernel/creds`. There is no query string, no query builder, and no DSN anywhere. `sqlite3` appears only as an installable entry in the toolbox catalog. **No SQL injection surface exists.** Budget released to cmdi/SSTI/header injection as instructed. |
| **NoSQL injection** (`sc-nosqli`) | Absent | No MongoDB, Redis, CouchDB, DynamoDB or Elasticsearch client is present — consistent with the empty database dependency set in `go.mod:5-13`. The nearest analogue is `kernel/datalake`, a local JSON document store whose collection/record names are constrained by `validName`. There is no query language to inject operators into. |
| **GraphQL** (`sc-graphql`) | Absent | A case-insensitive tree-wide grep for `graphql`/`GraphQL` across **all** file types (not just Go) returns **zero occurrences in zero files**. No schema, no resolver, no introspection endpoint, no client library. |
| **LDAP injection** (`sc-ldap`) | Absent | Same grep, `ldap`/`LDAP`: **zero occurrences in zero files**. No directory integration exists. Authentication is bearer-token, console-password session, or per-tenant token (`kernel/auth`, `kernel/webui/session.go`, `kernel/tenant`) — none of which builds a distinguished name or filter. |
| **XXE** (`sc-xxe`) | Absent — not exploitable in Go | XML parsing exists at exactly three sites: `kernel/creds/sts.go:208`, `kernel/creds/web_identity.go:160`, and `plugins/channels/wecom/wecom.go:187`, `:486`. All four calls are `xml.Unmarshal`, which uses a default `xml.Decoder`. Go's `encoding/xml` **never** resolves external entities or fetches DTDs — it recognizes only the five predefined entities, and an unknown entity is a parse error. Because these are `xml.Unmarshal` calls and not a hand-constructed `xml.NewDecoder`, there is no `Decoder.Entity` map or `Strict` field to misconfigure (a tree-wide grep confirms **zero** `xml.NewDecoder` call sites). Neither classic XXE nor billion-laughs applies. Worth noting the wecom site is internet-facing (a channel webhook) — it is safe by virtue of the standard library, not by virtue of a guard, so this holds only as long as the code stays on `xml.Unmarshal`. |
| **SSTI** (`sc-ssti`) | Absent as a *template-engine* class | There is no template engine in the Go tree: zero matches for `text/template` or `html/template`. The only `{{…}}` syntax is the workflow interpolator, which is not an expression language (see Verified safe #8). The two HTML-emitting handlers (`kernel/webui/webui.go:904-921`, `kernel/controlplane/provider_oauth.go:237-255`) are `fmt.Fprintf` with a hand-rolled 5-character escaper — an XSS surface for the frontend/XSS domain to judge, not template injection, since no user input becomes template *code*. |

---

## Notes for the orchestrator

- **INJ-001 is the one to act on.** It is cheap to fix (one field on sixteen struct literals plus a
  regression test) and it is the difference between "the catastrophe rails hold under the
  default-allow posture" — which two comments in the tree assert — and the actual behaviour.
- **INJ-002 has a shipped-template proof** (`kernel/workflow/templates.go:141`), so it is not a
  theoretical pattern the operator would have to opt into.
- The `mcp` → `exec.Command` path (Verified safe #7) is real risk but is an **authorization** finding
  (no binary allowlist, no hash pin, `CapMCPInstall` at L4) rather than an injection one. Whoever
  owns the authz domain should pick it up so it is not lost between domains.
- `kernel/warden/cmdline_windows.go:39` (`strings.Join(cmd.Args[2:], " ")`, unquoted) is safe with
  today's three callers but is a latent argument-injection trap. Worth a comment or an assertion
  that `len(Args) == 3` on that path.
