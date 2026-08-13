# AGEZT — Secrets / Crypto / Data-Exposure Results (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (branch `main`)
**Skills:** `sc-secrets`, `sc-crypto`, `sc-data-exposure`
**Method:** discovery, then adversarial verification of every candidate. Findings that could not be
substantiated in source were dropped, and three claims inherited from the Phase 1 recon were
actively **refuted** (see §Refuted). Four findings were proven by executing code, not by reading it.

Threat model applied: localhost-first, single-operator, token-gated daemon; console **ON by default**
at `127.0.0.1:8787`; 15 channel webhook listeners internet-facing by nature; operators may
reverse-proxy. Default-allow capability posture is a recorded owner decision and is **not** reported
as a vulnerability on its own — only where a documented guarantee is inert or an opt-out fails.

No real secret value appears anywhere in this document.

---

## Summary

| ID | Title | Severity | Confidence |
|---|---|---|---|
| SECRET-001 | Plugin children inherit the daemon's whole environment; the documented isolation control is inert | **High** | 95 |
| EXPOSE-001 | MCP server registry stores operator credentials in plaintext, world-readable | **High** | 96 |
| SECRET-002 | Hardcoded default console password `"agezt"` on the default-on control plane | **High** | 95 |
| EXPOSE-002 | Config Center audit log writes cleartext secret prefixes, world-readable, around the redactor | **Medium** | 94 |
| EXPOSE-003 | Tool output reaches the LLM provider unredacted while the local record is scrubbed | **Medium** | 90 |
| CRYPTO-001 | Unsalted SHA-256 used as the "safe" audit representation of config values | **Medium** | 88 |
| EXPOSE-004 | Config Center entry files store values in plaintext, world-readable | **Low** | 92 |
| EXPOSE-005 | Webhook secret accepted in a URL query string | **Low** | 85 |
| EXPOSE-006 | Second-tier redactors carry no literals; env-only credentials with no built-in pattern are never scrubbed | **Low** | 80 |
| EXPOSE-007 | File Manager returns raw OS error strings containing absolute host paths | **Low** | 82 |
| EXPOSE-008 | Agent-gateway audit writes around the bus, bypassing redaction (latent) | **Low** | 90 |
| CRYPTO-002 | NIP-04 unauthenticated AES-CBC (protocol-mandated) | **Informational** | 85 |
| SECRET-003 | Vault-backed secret file mounts land inside the agent's own workspace | **Low** | 60 (partly unverified) |

---

## SECRET-001 — Plugin children inherit the daemon's whole environment; the documented isolation control is inert

- **Severity:** High · **Confidence:** 95 · **CWE-214** (Invocation of Process Using Visible Sensitive Information), **CWE-522**
- **File:** `plugins/builtintools/plugins.go:95-103`; `kernel/plugin/host.go:84-85`, `:295-296`, `:1055-1056`; doc claim at `docs/PLUGIN-SECURITY.md:277-280`

`kernel/plugin/host.go:84-85` defines the contract:

```go
// Env is the child's environment. Nil inherits the parent's.
Env []string
```

and the spawn path honours it conditionally (`host.go:295-296`, and identically on the reload path at
`:1055-1056`):

```go
if cfg.Env != nil {
    cmd.Env = cfg.Env
}
```

`docs/PLUGIN-SECURITY.md:277-280` advertises the control as implemented:

> "The plugin host constructs the child's environment explicitly via `Config.Env`. … **The daemon's
> own boot code sets plugin environments to include only what the plugin needs.**"

It does not. The **only** `plugin.Config` literal in the tree is `plugins/builtintools/plugins.go:95-103`:

```go
cfg := plugin.Config{
    Path: e.Path,
    Args: e.Args,
    Logger: func(line string) { ... },
    PinnedHash:   pins[prefix],
    AllowedTools: allowedTools[prefix],
}
```

`Env` is never set, so it is nil, so every external plugin inherits the daemon's full environment.

**Exposure path.** `cmd/agezt/main.go:3809-3825` (`injectConfig`) copies the Config Center store **and
every `AGEZT_*` vault secret** into the daemon's own process environment at boot
(`os.Setenv(name, val)`), on top of whatever provider keys the operator exported. A plugin child
therefore receives `AGEZT_VAULT_PASSPHRASE`, `AGEZT_WEB_PASSWORD`, channel tokens, and provider API
keys. The effect is transitive: the shipped `mcpbridge` plugin spawns its own MCP server child at
`plugins/external/mcpbridge/stdio_transport.go:38` with `exec.Command(path, args...)` and **no
`cmd.Env`**, passing the inherited environment down another level to a third-party npm/pip package.

**Why not a false positive.** This is the exact shape the repo already fixed once, for the AWS
credential helper, and documented as wrong at `kernel/creds/aws.go:107-112`:

> "Previously `cmd.Env` was never set, so the helper inherited the daemon's ENTIRE environment:
> `AGEZT_VAULT_PASSPHRASE`, every provider API key, the console password. … it was handed every other
> secret we own."

Every other subprocess sink in the tree scrubs (`kernel/mcp/client.go:114`,
`plugins/tools/acpagent/acpagent.go:247`, `plugins/tools/browser/action.go:843`,
`plugins/tools/coding/coding.go:145`, `plugins/tools/shell/shell.go:273`,
`kernel/creds/aws.go:222`). The plugin host is the one that does not, while its own security document
says it does. The `docs/PLUGIN-SECURITY.md:288-290` "Limitations" paragraph describes this nil-`Env`
case as a hypothetical residual risk; it is the actual shipped configuration.

**Remediation.** Set `Env: envscrub.Scrubbed()` in `plugins/builtintools/plugins.go:95`, with an
explicit per-plugin opt-in list mirroring `kernel/mcp.Server.Env`. Set `cmd.Env` in
`plugins/external/mcpbridge/stdio_transport.go:38` too. Add a guard test asserting no
`plugin.Config` literal leaves `Env` nil, and correct `docs/PLUGIN-SECURITY.md:279-280`.

---

## EXPOSE-001 — MCP server registry stores operator credentials in plaintext, world-readable

- **Severity:** High · **Confidence:** 96 · **CWE-732** (Incorrect Permission Assignment for Critical Resource), **CWE-522**
- **File:** `kernel/mcp/store.go:61-74`, `:191`, `:312` → `kernel/jsonstore/jsonstore.go:73` → `internal/atomicfile/atomicfile.go:44`

`kernel/mcp/store.go:61-74` documents that both credential fields are stored in the clear:

```go
// Env is an OPT-IN set of environment variables injected into THIS server's
// process on attach (M898) — e.g. {"GITHUB_PERSONAL_ACCESS_TOKEN": "..."}.
// ... Stored like Args (plaintext in the registry) — use a dedicated low-scope
// token. Redacted out of read APIs.
Env map[string]string `json:"env,omitempty"`
// Headers is an OPT-IN set of HTTP request headers for a remote (URL)
// server (M904) — e.g. {"Authorization": "Bearer ..."} ... stored plaintext
Headers map[string]string `json:"headers,omitempty"`
```

Persistence: `store.go:191` `jsonstore.LoadFrom(dir, "servers.json", ...)` (dir created `0o755` at
`jsonstore.go:55`), `store.go:312` `jsonstore.Save(s.path, s.servers)` →
`jsonstore.go:73` `atomicfile.WriteFile(path, b, 0o644)`. `atomicfile.go:44` applies the mode
explicitly before the rename (`os.Chmod(tmpName, mode)`), so the 0644 is real on POSIX, not
incidental.

**Proven, not inferred.** I added a temporary test against the real package (since removed) that
called `OpenStore` + `Add` with `Env: {"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_PROOFVALUE"}` and stat'd
the result:

```
PROOF mode=0666 path=…\mcp\servers.json      (Windows reports 0666; the code path writes 0o644)
PROOF plaintext-token-on-disk=true
```

**Why not a false positive.** This is the identical defect the repo fixed for the journal three weeks
earlier, and the reasoning is recorded verbatim at `kernel/journal/journal.go:69-79`:

> "…it shipped world-readable (0644 segments in a 0755 directory) **while the vault, artifacts, auth
> tokens and datalake all used 0600/0700. Any other local user could read the entire history with no
> credential.**"

`journal.go:81-83` then sets `journalDirPerm = 0o700` / `journalSegmentPerm = 0o600`. `servers.json`
was not swept in that pass and holds live third-party credentials — a GitHub PAT, a remote MCP
`Authorization: Bearer` — next to a vault that is 0600 *and* AES-256-GCM encrypted.

I verified the exposure is **at-rest only**: `kernel/controlplane/mcp.go:22-49` correctly deletes
`env` and `headers` from the wire shape and returns only sorted key names, so the read API does not
leak. That narrows the finding rather than removing it.

Same class, same file mode, lower value: `kernel/state/state.go:222`, `kernel/seat/store.go:219`,
`kernel/edict/snapshot.go:112`, `kernel/market/store.go:123` and every other `jsonstore.Save`
consumer (`board`, `cadence`, `memory`, `okr`, `roster`, `skill`, `standing`, `taste`, `toolforge`,
`workboard`, `workflow`, `worldmodel`). Agent memory and standing orders can quote a secret the agent
saw; those files are 0644 too.

**Remediation.** Change `kernel/jsonstore/jsonstore.go:73` to `0o600` and `LoadFrom`'s `MkdirAll`
(`:55`) to `0o700`, tightening in place on existing directories exactly as `journal.Open` does. If a
blanket change is too broad, at minimum give `kernel/mcp` a dedicated 0600 writer. Consider moving
`Server.Env` / `Server.Headers` values into the existing `kernel/creds` vault and storing only key
references in the registry.

---

## SECRET-002 — Hardcoded default console password `"agezt"` on the default-on control plane

- **Severity:** High · **Confidence:** 95 · **CWE-798** (Hardcoded Credentials), **CWE-1392** (Use of Default Credentials)
- **File:** `cmd/agezt/httpsurfaces.go:230`, `:232-244`; console default-on at `:82`; gate semantics at `kernel/webui/webui.go:1443-1452`

```go
// cmd/agezt/httpsurfaces.go:230
const defaultLoopbackWebPassword = "agezt"

func effectiveWebPassword(addr string) string {
    if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")); v != "" { return v }
    switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD_DEFAULT"))) {
    case "off", "disabled", "none", "no", "0", "false": return ""
    }
    if isLoopback(addr) { return defaultLoopbackWebPassword }
    return ""
}
```

On a stock install the console is on at `127.0.0.1:8787`, `AGEZT_WEB_PASSWORD` is unset, and the bind
is loopback — so the password is the publicly-known string `"agezt"`. `kernel/webui/webui.go:1443-1452`
shows that in the default (non-strict) mode the password is a **sufficient** credential, not a second
factor:

```go
if s.passwordStrictOn() { return s.dataTokenPresented(r) && s.sessionValid(r) }
return s.dataTokenPresented(r) || s.sessionValid(r)
```

A session opened with `"agezt"` therefore reaches the full control plane: `POST /api/run`
(arbitrary agent execution), `POST /api/config/set` (vault writes), `POST /api/files/delete`,
`POST /api/toolbox/install`, `POST /api/mcp/add`.

**Verification — what actually mitigates it, and what does not.** I checked each guard rather than
assuming the worst:

- Non-loopback binds return `""` (`:243`), so this never applies to a LAN-exposed console. ✅
- Wildcard binds additionally force strict mode (`httpsurfaces.go:129-133`). ✅
- Browser-driven attacks are blocked: `sameOriginMutation` (`webui.go:1345-1362`) rejects
  `Sec-Fetch-Site: cross-site` and mismatched `Origin`, which browsers always send on a cross-origin
  POST; `hostAllowed` (`:1328-1343`) rejects unregistered DNS names, so DNS rebinding fails. ✅
- Login is rate-limited: 8 failures → 5-minute lockout (`kernel/webui/session.go:38-39`). Irrelevant
  when the password is known. ❌
- **Not mitigated:** any non-browser local client. `hostAllowed` accepts any IP literal
  unconditionally (`:1337-1339`) and a missing `Origin` header returns `true` (`:1355-1357`), so a
  local process or a second OS user on a shared host authenticates with one known string.

That residual path is real on a machine whose whole purpose is running LLM-directed code. It is also
the same trust boundary the repo already treats as adversarial in `kernel/journal/journal.go:74-76`
("Any other local user could read…").

**Remediation.** Generate a random default password at first boot, print it once in the boot banner,
and persist it 0600 — same shape as `kernel/auth/tokenfile.go:20-38`. Failing that, force strict mode
whenever the built-in default is in effect, so the token remains mandatory.

---

## EXPOSE-002 — Config Center audit log writes cleartext secret prefixes, world-readable, around the redactor

- **Severity:** Medium · **Confidence:** 94 · **CWE-532** (Insertion of Sensitive Information into Log File), **CWE-312**
- **File:** `kernel/configcenter/audit.go:59-74`, `:81-91`, `:113`; enabling condition at `kernel/controlplane/configcenter_handler.go:36` + `kernel/configcenter/center.go:107-110`

```go
// kernel/configcenter/audit.go:81-91
func (a *AuditLogger) previewValue(value string, rating Rating) string {
    if rating == RatingPublic || rating == "" { return value }
    if len(value) <= 8 { return value }
    return value[:8] + "..." + HashValue(value)[:8]
}
```

reached from `:59-67` whenever an allowed access has any rating other than `RatingSecret` and
`RatingPublic`. The file is opened world-readable:

```go
// kernel/configcenter/audit.go:113
f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
```

**The enabling condition.** `kernel/controlplane/configcenter_handler.go:36` sets
`rating := configcenter.RatingInternal` as the default when the caller omits `rating` (identically at
`cmd/agt/configcenter.go:132`), and assigns it to the entry explicitly. `center.go:107-110` only
auto-classifies when `entry.Rating == ""`:

```go
if entry.Rating == "" { entry.Rating = c.classifier.Classify(entry.Key, entry.Value) }
```

So a secret stored without an explicit `--rating secret` is **never classified**, lands as
`RatingInternal`, is `PolicyAuto` (readable by any agent, `config.go:48`), and its first 8 characters
are logged in cleartext. The classifier also misses secret-bearing key names it does not enumerate —
`AGEZT_VAULT_PASSPHRASE` matches none of the key patterns at `classifier.go:51-63` ("passphrase" is
not "password"/"passwd"/"pwd").

**Proven, not inferred.** A temporary test against the real package (since removed) stored
`AGEZT_VAULT_PASSPHRASE` with the daemon's own default rating and read it back through
`Center.Get`:

```
PROOF auditfile=audit_2026-08-13.jsonl mode=0666    (Windows; the code path writes 0o644)
PROOF line={"id":"audit_…","event":"config.access","key":"AGEZT_VAULT_PASSPHRASE","rating":"",
       "decision":"allowed","policy":"auto","value_log":"SuperSec...c12e9b5c"}
```

The first 8 characters of the passphrase are on disk in the clear, in a world-readable file.

**Why not a false positive.** `writeToFile` is a direct `os.OpenFile` — it never passes through
`bus.Publish`, so `kernel/bus/bus.go:198`'s `redactSpecLocked` never runs and `AGEZT_REDACT` has no
effect on it. The Config Center is opened unconditionally at kernel boot
(`kernel/runtime/runtime.go:909`) with `DefaultConfig`, so `a.config` is non-nil and
`LogPublicValues` is `false` (`config.go:64-68`) — I confirmed the `a.config == nil` branch at
`audit.go:62`, which would log **full** public values, is unreachable in production. That branch and
the dead `rating == ""` case at `:82` are latent bugs, not the live one; the live one is the 8-char
prefix.

**Remediation.** Never write raw value bytes to the audit log — drop `previewValue`'s plaintext
prefix and log only a keyed digest (see CRYPTO-001). Open the file `0o600` and the directory `0o700`.
Make the handler default `rating` to `""` so `Center.Set`'s auto-classifier actually runs.

---

## EXPOSE-003 — Tool output reaches the LLM provider unredacted while the local record is scrubbed

- **Severity:** Medium · **Confidence:** 90 · **CWE-201** (Insertion of Sensitive Information Into Sent Data), **CWE-200**
- **File:** `kernel/agent/run_tools.go:429`, `:436-440`; contrast `kernel/bus/bus.go:198`, `:247`; doc claim at `kernel/redact/redact.go:3-9`

`finalizeToolJobs` publishes the tool result to the bus — where it *is* redacted — and then appends
the **raw** output to the conversation:

```go
// kernel/agent/run_tools.go:429  (redacted: bus.Publish → redactSpecLocked → journal + SSE)
if _, err := s.publish(event.KindToolResult, "tool", resultPayload); err != nil { ... }

// kernel/agent/run_tools.go:436-440  (NOT redacted: this is what goes to the provider)
messages = append(messages, Message{
    Role:       RoleTool,
    Content:    modelOutput,
    ToolCallID: job.tc.ID,
})
```

`modelOutput` derives from `job.result.Output` (`:378`) and passes through no redactor on any branch.

**Exposure path.** An agent (under prompt injection or otherwise) runs `shell`/`file`/`code_exec`
against a credential the daemon does not hold as a literal — `~/.aws/credentials`, `~/.ssh/id_*`,
`~/.config/gh/hosts.yml`, or, per EXPOSE-001, the world-readable
`<baseDir>/mcp/servers.json` and, per EXPOSE-002, `<baseDir>/configcenter/audit_*.jsonl`. The bytes
go over the wire to Anthropic/OpenAI/DeepSeek/whichever provider is configured. Meanwhile the
journal, the SSE feed, and the outbound webhook dispatcher all show `[REDACTED]` — so the operator's
permanent audit record **understates** what left the machine. That asymmetry is the finding: not that
the model sees tool output (it must), but that the only durable evidence says otherwise.

**Documentation divergence.** `kernel/redact/redact.go:3-9` calls the package

> "the chokepoint that prevents that (SPEC-06 …)"

with no mention that it covers only the local record, nor that it is disableable
(`cmd/agezt/internal/daemonconfig/daemonconfig.go:512`: `c.Misc.Redact = !EqualFold(get("AGEZT_REDACT"), "off")`).

**Remediation.** Either redact `modelOutput` on the same literal set before it enters `messages`
(literals only — pattern redaction would corrupt legitimate tool output the model needs), or, if the
raw output must reach the model, emit a distinct `tool.result.unredacted_to_model` marker on the bus
so the audit trail records that a literal secret was forwarded. Correct `redact.go:3-9` to scope the
guarantee.

---

## CRYPTO-001 — Unsalted SHA-256 used as the "safe" audit representation of config values

- **Severity:** Medium · **Confidence:** 88 · **CWE-759** (Use of a One-Way Hash without a Salt), **CWE-916**
- **File:** `kernel/configcenter/access.go:332-334`, consumed at `kernel/configcenter/audit.go:66`, `:72`, `:90`

```go
func HashValue(value string) string {
    hash := sha256.Sum256([]byte(value))
    return hex.EncodeToString(hash[:])
}
```

This is the fallback the audit logger uses precisely *because* it is meant to be the non-leaking
representation (`audit.go:66`, `:72`), and it is concatenated onto the 8-char plaintext prefix at
`audit.go:90`. Unsalted, uniterated SHA-256 over a config value is reversible for anything
low-entropy — a short password, a PIN, an enum, a hostname — by dictionary or rainbow table. Combined
with EXPOSE-002 the attacker gets the first 8 characters *plus* an unsalted digest of the whole
value, which collapses the remaining search space dramatically.

**Why not a false positive.** This is not content-addressing or a cache key (the sc-crypto
false-positive list); the function's only callers are the audit logger's secrecy branches, where its
stated job is to represent a value without disclosing it. Note the repo gets this right elsewhere:
`kernel/creds/encrypt.go:305-307` deliberately hashes the passphrase *only* to build a cache key and
says so ("so the passphrase doesn't sit in a map key string"), while the actual key derivation uses
200 000-round salted PBKDF2.

**Remediation.** Replace with an HMAC keyed by a per-install random value, or reuse the vault's
salted KDF. If the digest exists only for correlation, a truncated HMAC is sufficient.

---

## EXPOSE-004 — Config Center entry files store values in plaintext, world-readable

- **Severity:** Low · **Confidence:** 92 · **CWE-732**, **CWE-312**
- **File:** `kernel/configcenter/center.go:370-386`

```go
os.MkdirAll(c.config.Dir, 0755)
...
return os.WriteFile(filename, data, 0644)
```

Proven in the same run as EXPOSE-002:

```
PROOF entryfile=entry_cd23b78c7fa904da.json mode=0666 plaintext-value=true
```

The entry file legitimately holds the value (it is the store), but at 0644 in a 0755 directory it is
readable by every local user — unlike `kernel/creds` (`creds.go:212`, `0o600` + AES-256-GCM) and the
journal (`journal.go:81-83`). A `RatingSecret` entry is stored here in the clear.

**Remediation.** `0o600` / `0o700`, tightened in place on existing directories. Consider routing
`RatingSecret` values into `kernel/creds` instead of a bare JSON file.

---

## EXPOSE-005 — Webhook secret accepted in a URL query string

- **Severity:** Low · **Confidence:** 85 · **CWE-598** (Use of GET Request Method With Sensitive Query Strings)
- **File:** `kernel/webui/webui.go:1008-1011`

```go
secret := r.Header.Get("X-Agezt-Secret")
if secret == "" {
    secret = r.URL.Query().Get("secret")
}
```

`POST /hooks/<workflow>` is the one token-free mutating route on the default-on console
(`webui.go:860`), and its only credential is this per-workflow secret. Accepting it in the query
string puts a live credential into anything that records request lines: a reverse proxy's access log
(explicitly in scope per the threat model), browser history if a link is ever pasted, and `Referer`
headers.

**Verified mitigations — this is Low, not Medium.** I checked three things rather than assuming:
- The daemon itself logs **no** request URLs: `r.URL.String()`, `r.RequestURI` and `RawQuery` do not
  appear in `kernel/webui/`, `kernel/httpserver/`, or `kernel/restapi/` outside tests.
- `webui.go:1025-1031` explicitly strips `secret` from the payload placed on the bus
  (`if k == "secret" || len(v) == 0 { continue }`), so it never reaches the journal or SSE.
- Verification is constant-time (`kernel/controlplane/workflow.go:276`) and rate-limited pre-auth
  (`webui.go:1002-1007`).

**Remediation.** Deprecate the query-string form behind an opt-in flag; require `X-Agezt-Secret`.

---

## EXPOSE-006 — Second-tier redactors carry no literals; env-only credentials with no built-in pattern are never scrubbed

- **Severity:** Low · **Confidence:** 80 · **CWE-532**
- **File:** `kernel/redact/redact.go:217`, `:55-100`; instances at `kernel/openaiapi/openaiapi.go:50`, `kernel/controlplane/remote_mirror.go:256`, `plugins/builtintools/plugins.go:85`; literal seeding at `cmd/agezt/main.go:2631-2641`

The primary bus redactor is seeded with every vault value (`credSecrets`, `main.go:2631-2638`), which
covers vaulted credentials regardless of shape — that is the correct design and it substantially
limits this finding. Two gaps remain:

1. `redact.New()` returns `&Redactor{}` with **no literals** (`redact.go:217`). The three secondary
   redactors above are constructed that way, so they match only the built-in pattern list. The
   OpenAI-surface error redactor (`openaiapi.go:50`) is the one that scrubs strings returned over
   HTTP — it cannot mask an operator secret with no recognisable prefix.
2. `credSecrets` reads `store.Names()` — the **vault** only. A credential exported into the daemon's
   shell environment instead of stored in the vault is not a literal, so it is covered only if it
   matches a pattern at `redact.go:55-100`. That list has no rule for Twilio auth tokens (32 hex; the
   `sms` channel uses them, `plugins/channels/sms/sms.go:294`), SendGrid `SG.`, Discord *bot* tokens
   (distinct from the webhook URL rule at `:161-166`), Gotify/Zulip tokens, or generic
   `?api_key=`/`?token=` query parameters.

**Remediation.** Seed the secondary redactors from the same literal set. Extend `credSecrets` to
include the values of `AGEZT_*` variables present in the process environment. Add the missing
provider/channel patterns.

---

## EXPOSE-007 — File Manager returns raw OS error strings containing absolute host paths

- **Severity:** Low · **Confidence:** 82 · **CWE-209** (Error Message Information Leak)
- **File:** `kernel/webui/files_route.go:262`, `:271`, `:280`, `:343`, `:355`, `:411`, `:416`, `:421`, `:448`, `:453`, `:457`, `:480`, `:492`, `:503`, `:508`, `:532`

Every one is `http.Error(w, err.Error(), http.Status…)` on an `os.*` failure, so the response carries
the absolute host path (`open C:\Users\<user>\agezt\workspace\…: permission denied`). All are behind
`s.authorized(r)`, and the recipient is the operator who owns the filesystem — hence Low. It still
discloses the daemon's real base directory to anyone who reaches the console with a valid session,
including one opened with the default password from SECRET-002.

No stack traces are returned anywhere; panic recovery is in place (`kernel/controlplane/server.go:598-609`).

**Remediation.** Return a generic message plus a correlation id; log the detail server-side.

---

## EXPOSE-008 — Agent-gateway audit writes around the bus, bypassing redaction (latent)

- **Severity:** Low · **Confidence:** 90 · **CWE-532**
- **File:** `kernel/agentgw/audit.go:97`; sole caller `kernel/agentgw/gateway.go:275-284`

```go
if _, err := a.j.Append(spec); err != nil { ... }
```

`journal.Append` is called directly, so `bus.Publish`'s `redactSpecLocked` (`kernel/bus/bus.go:198`)
never runs. The bypass is structurally real and permanent (the journal is append-only with no purge).

**Recorded as Low because I verified it currently carries nothing sensitive.** The only non-test
caller (`gateway.go:275-284`) populates `Timestamp`, `TokenID` (`claims.ParentTokenID` — an
identifier, not the bearer token), `RunID`, `Subprocess`, `Operation` (`r.Method`), `Path`
(`r.URL.Path` — **path only, no query string**), `Success`, `ClientIP`. The `Error` and `Capability`
fields of `AuditEntry` (`kernel/agentgw/types.go:116-128`) are never set. So no secret reaches it
today; the hazard is that any future caller populating `Error` writes an unredactable secret into a
permanent log.

**Remediation.** Route through `bus.Publish`, or give `AuditLogger` a redactor.

---

## CRYPTO-002 — NIP-04 unauthenticated AES-CBC (protocol-mandated)

- **Severity:** Informational · **Confidence:** 85 · **CWE-353** (Missing Support for Integrity Check)
- **File:** `plugins/channels/nostr/nip04.go:24-69`

AES-256-CBC with a fresh `crypto/rand` IV per message (`:30-33`) and PKCS#7 — but **no MAC**, and
`nip04Decrypt` (`:41-69`) decrypts attacker-supplied ciphertext and returns three distinguishable
padding errors (`:82`, `:86`, `:90`).

**Recorded as Informational, not a finding.** NIP-04 is the wire format; adding a MAC would break
interoperability, and the file says so at `:16-21` ("NIP-44/NIP-17 are a possible future upgrade").
This lands squarely in the sc-crypto false-positive category "legacy compatibility with documented
migration plan". The padding-oracle distinction is also not observable to a remote party — a decrypt
failure drops the event rather than producing a distinguishable response. Worth tracking as a
migration item to NIP-44 (which is authenticated), not remediating in place.

---

## SECRET-003 — Vault-backed secret file mounts land inside the agent's own workspace

- **Severity:** Low · **Confidence:** 60 — **partly unverified, flagged explicitly**
- **File:** `kernel/executionprofile/secretfiles.go:140-144`, `:66-70`

```go
root := filepath.Join(workDir, secretFilesDir)   // secretFilesDir = ".agezt-secrets"
if err := os.MkdirAll(root, 0o700); err != nil { ... }
...
if err := os.WriteFile(hostPath, []byte(value), 0o600); err != nil { ... }
```

The file handling itself is **good**: 0600 files in a 0700 directory, and `cleanup` (`:52-57`)
`RemoveAll`s the root. The observation is that when `workDir` is non-empty the secret is materialised
*inside the tool's working directory* rather than the temp dir used on the `workDir == ""` path
(`:134-138`). If that working directory coincides with the workspace root the `file` tool is scoped
to, the agent could read its own mounted secret with a `file` call during the run.

**Explicitly not confirmed:** I did not trace every caller's `workDir` to establish whether it equals
`workspaceRoot(baseDir)` (`cmd/agezt/main.go:3836-3841`) in practice. The feature is also strictly
opt-in (`AGEZT_EXEC_SECRET_FILES_*`, unset by default) and its purpose is to hand the secret to the
child, so this may be entirely intended. Recorded so a later pass can settle it; do not act on it
without that trace.

---

## Secret inventory

| Secret type | At rest | In transit | In logs / journal | Who can read it |
|---|---|---|---|---|
| Provider API keys (vault) | `<base>/creds.json`, **0600 + AES-256-GCM**, PBKDF2-200k, machine-bound key by default (`creds.go:212`, `encrypt.go:166-205`, `machine.go:55-72`) | HTTPS to provider; `Authorization` header | Redacted as literals + patterns (`main.go:2631`, `bus.go:198`) | Same-user processes (machine key is same-user derivable, `machine.go:19-23`); **plugin children** (SECRET-001) |
| `AGEZT_*` channel/config secrets | Vault (as above), **also injected into daemon `os.Environ()`** at `main.go:3818-3825` | Per-channel API | Redacted (literals) | Any daemon-inherited child: **plugin children** (SECRET-001) |
| Console password | `AGEZT_WEB_PASSWORD` env / vault; **built-in default `"agezt"`** (`httpsurfaces.go:230`) | POST body (`/api/login`) | Not logged; compared constant-time (`session.go:224`) | Anyone reaching loopback (SECRET-002) |
| Console / SSE / REST / OpenAI tokens | Memory-only (console, SSE); `<base>/{openai,rest}.token` 0600 in 0700 (`auth/tokenfile.go:30-35`) | `Authorization: Bearer`; console token also in `?token=` boot URL | Not logged (no request-URL logging in any HTTP layer) | Same-user file read |
| Control-plane token | `<base>/runtime/control.token`, 0600, regenerated per boot (`controlplane/server.go:431-443`) | IPC body, constant-time (`:357`) | No | Same-user file read |
| Agent-gateway secret | `<base>/agentgw.secret`, hex, 0600, `O_EXCL` (`agentgw/secret.go:39-88`) | HS256 JWT, `hmac.Equal` (`token.go:124`) | Audit entries carry token **ID** only (EXPOSE-008) | Same-user file read |
| **MCP server env / headers** | `<base>/mcp/servers.json`, **0644 plaintext** (EXPOSE-001) | Injected into child env; sent as HTTP headers | Stripped from read API (`controlplane/mcp.go:30-49`) | **Every local user** |
| **Config Center values** | `<base>/configcenter/entry_*.json`, **0644 plaintext** (EXPOSE-004) | Control-plane IPC / console | **8-char cleartext prefix + unsalted SHA-256** in `audit_*.jsonl`, **0644**, around the redactor (EXPOSE-002, CRYPTO-001) | **Every local user** |
| Per-workflow webhook secrets | Workflow store (jsonstore, 0644) | Header **or `?secret=`** (EXPOSE-005) | Stripped from bus payload (`webui.go:1025-1031`) | Every local user |
| Tool output containing any secret | Journal, 0600/0700, redacted (`journal.go:81-83`, `bus.go:198`) | **Unredacted to the LLM provider** (EXPOSE-003) | Local record redacted — understates the exposure | The configured LLM provider |
| Secret file mounts | `<workDir>/.agezt-secrets/*`, 0600 in 0700, removed after run (`secretfiles.go:67`, `:141`) | Path passed via `SECRET_FILE_*` env | Names only (`ProfileSecretFileSummary`) | The child process; possibly the `file` tool (SECRET-003) |
| AWS credentials | `~/.aws/*` (not owned by AGEZT) | `credential_process` helper gets a **scrubbed** env (`aws.go:113-133`) | Redacted (`AKIA` pattern + `aws_secret_access_key=` template) | Same-user |

---

## Verified safe

Checks that came back clean, recorded so a later pass does not re-derive them.

**Cryptography**

- **The vault KDF is genuine PBKDF2-HMAC-SHA256, not a custom construction.** `deriveKeyPBKDF2`
  (`kernel/creds/encrypt.go:325-341`) computes `U_1 = HMAC(P, salt‖INT32BE(1))`, `U_j = HMAC(P, U_{j-1})`
  via `mac.Reset()` (which preserves the key), and XOR-accumulates every round — the exact RFC 8018
  construction for `dkLen == hLen`. `kernel/creds/kdf_known_answer_internal_test.go:35-60` cross-checks
  it **live against stdlib `crypto/pbkdf2`** across six cases including empty passphrase, empty salt
  and unicode. I ran it: `TestDeriveKeyPBKDF2_MatchesStdlib` and `TestDeriveKeyPBKDF2_KnownAnswers`
  both PASS. *This refutes Phase 1 DIVERGENCE 11.*
- Vault encryption: AES-256-GCM (AEAD), fresh 32-byte salt and 12-byte nonce per save from
  `crypto/rand` (`encrypt.go:170-186`), no nonce reuse, no static IV. Decrypt validates cipher id,
  KDF id, an iteration **floor** (100 000) and **ceiling** (10 000 000) against the envelope's own
  attacker-controllable `kdf_iter`, and checks nonce length **before** `gcm.Open` to avoid Go's panic
  (`encrypt.go:218-253`). All correct.
- Legacy KDF (`deriveKeyLegacyHMAC`, `:348-356`) is decrypt-only, pinned by golden vectors from an
  independent reimplementation (`kdf_known_answer_internal_test.go:68-82`), and new saves always write
  PBKDF2 (`encrypt.go:198`).
- **No `InsecureSkipVerify` anywhere in the tree.** The only two `tls.Config` literals
  (`plugins/channels/email/inbound.go:256`, `:274`) set `ServerName` and rely on Go's client default
  minimum of TLS 1.2.
- **No weak PRNG in a security role.** `math/rand` appears twice, both retry jitter:
  `kernel/governor/governor.go:631` and `plugins/providers/internal/retry/retry.go:206`. All tokens,
  session ids, nonces, salts and OAuth state use `crypto/rand`.
- **No non-constant-time secret comparison.** All 24 credential/MAC comparisons use
  `subtle.ConstantTimeCompare` or `hmac.Equal`, including every internet-facing channel webhook
  verifier (slack, discord, telegram, whatsapp, teams, line, zalo, dingtalk, feishu, wecom, onebot,
  sms, imessage, nextcloudtalk, chatwebhook, whatsappgw).
- **No MD5, DES, 3DES, RC4, or ECB mode anywhere.** SHA-1 appears three times, all protocol-mandated
  or non-security: `kernel/creds/sso.go:80` (AWS SSO cache **filename** derivation, per the AWS SDK
  convention), `plugins/channels/sms/sms.go:294` (Twilio's HMAC-SHA1 signature),
  `plugins/channels/onebot/onebot.go:327` (OneBot's HMAC-SHA1), `plugins/channels/wecom/wecom.go:426`
  (WeCom's `msg_signature`). HMAC-SHA1 remains sound as a MAC.
- **WeCom AES-CBC static IV is safe as implemented.** `wecom.go:449` uses the first 16 bytes of the
  AES key as the IV, per the WXBizMsgCrypt spec — but the plaintext format begins with 16 random
  bytes, and critically the signature is verified **before** decryption (`wecom.go:191-194`), so
  there is no reachable padding oracle. The trailing `receive_id` is then compared constant-time
  (`:202`).

**Secrets**

- **No hardcoded live credential in the tree.** `gitleaks detect` over all 1,693 commits (414 MB)
  returned exactly **one** hit, and it is a PEM-shaped string inside a previously committed file
  under `security-report/` — a prior assessment's own content, not application code. *(Not opened:
  that directory is out of read scope for this pass. Independently, the only PEM block anywhere in
  the source tree is the obviously synthetic `MIIEdeadbeef` at `kernel/redact/redact_test.go:67`.)*
- Every key-shaped literal in the tree is a documented example or a synthetic fixture:
  `AKIAIOSFODNN7EXAMPLE` (the AWS documentation constant), `ghp_xxxx…`, `xoxb-xxxx-…`,
  `sk-ant-api03-abcdefghij…`, `AKIA0123456789ABCDEF`. All in `_test.go` files or in
  `kernel/configcenter/classifier.go:83-92` where they are the pattern **examples**.
- `.env` exists at the repo root, is **untracked**, and is ignored four ways
  (`.gitignore:71-74`: `.env`, `.env.*`, `!.env.example`, `*.env`). Confirmed via
  `git check-ignore -v .env`. Contents not read. `.dev-home/` likewise ignored and not read.
- Only `.env.example` is tracked; every assignment in it is empty or commented.
- Non-Go surfaces are clean: no credential in `frontend/src`, `sdk/{typescript,python,rust}`,
  `scripts/`, `install.sh`, `install.ps1`, `dev.ps1`, `Makefile`, or `.github/workflows/`. The
  installers actively refuse to echo secret-named variables (`install.ps1:106`).

**Data exposure**

- `/api/config/values` never returns a secret value: `kernel/controlplane/settings.go:55-57` returns
  presence only (`entry["set"] = vault.Has(f.Env)`) for any field flagged `Secret`. The flag is
  reliable — the `pw()` helper at `kernel/settings/schema.go:85-87` hardcodes
  `Type: TypePassword, Secret: true`, and I confirmed every `TypePassword` field in the 204-field
  schema carries `Secret: true`, and that no credential-valued field lacks it.
- MCP read API strips `env` and `headers` values, returning sorted key names only
  (`kernel/controlplane/mcp.go:22-49`).
- OpenAI-surface error strings are pattern-redacted before leaving the process
  (`kernel/openaiapi/openaiapi.go:41-53`).
- Bus redaction covers **both** durable publishes and streaming deltas
  (`kernel/bus/bus.go:198`, `:247`), and `SetEscapeHTML(false)` (`:105-106`) stops JSON escaping from
  smuggling a secret past the scrubber.
- **No request-URL logging** in `kernel/webui`, `kernel/httpserver`, or `kernel/restapi` — so
  `?token=` / `?st=` / `?secret=` do not reach a daemon-written log.
- **Subprocess environment scrubbing is allowlist-first and consistent** across all six
  implementations (`kernel/envscrub/envscrub.go:15-43`, `plugins/tools/shell/env.go:25,67,91,115`,
  `plugins/tools/codeexec/codeexec.go:752,776,800,824`, `kernel/mcp/client.go:114`). Because the
  allowlist runs before the `isSecretName` deny-list, a credential-bearing variable with an
  unrecognised name (`DATABASE_URL`, `CONNECTION_STRING`) is dropped anyway. The shell tool
  additionally repoints `HOME`/`USERPROFILE`/`TMP` at the work dir (`env.go:57-63`) so a child does
  not land in the operator's real home. **The one exception is the plugin host — SECRET-001.**
- The `AGEZT_EXEC_SECRET_ENV_*` opt-in escape hatch's advertised `AGEZT_*` block **is** implemented,
  twice (parse time `kernel/executionprofile/env.go:145-147`, resolve time `:93`) — defence in depth,
  and the opt-out does not fail when configured.
- Frontend: no credential rendered, no credential in `localStorage`/`sessionStorage`, no credential in
  `console.*`, `sourcemap: false` in the production build (`frontend/vite.config.ts:24`), and **zero
  `.map` files** in the committed `kernel/webui/dist` bundle.
- All three SDKs transmit the token as an `Authorization: Bearer` header only — never a query string,
  never logged, never written to disk, never interpolated into an exception message
  (`sdk/typescript/src/client.ts:239-240`, `sdk/python/agezt/client.py:270`,
  `sdk/rust/src/client.rs:384-385`).

**Refuted / downgraded from Phase 1 recon**

| Recon claim | Verdict |
|---|---|
| DIVERGENCE 11: "vault KDF is a custom keyed-HMAC chain, not RFC 2898" | **Refuted.** Genuine PBKDF2, cross-verified live against stdlib `crypto/pbkdf2`. The recon read the *legacy* `deriveKeyLegacyHMAC` (decrypt-only) as if it were the current KDF. |
| DIVERGENCE 10(b): agentgw journal bypass leaks secrets | **Downgraded to Low (EXPOSE-008).** Structurally real, but the sole non-test caller sets no secret-bearing field, and `Path` is `r.URL.Path` without the query string. |
| `tunnelPublicURL` "prints the console token into the public URL" | **Not a leak.** `cmd/agezt/httpsurfaces.go:373-378` writes the tokened URL to the operator's own boot banner — the intended way to hand them a working link — not to any shared or published surface. |
| "Model context is never scrubbed" | **Confirmed and filed** as EXPOSE-003, reframed around the audit-record asymmetry, which is the actionable part. |
