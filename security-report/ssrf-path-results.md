# AGEZT — SSRF / Path-Traversal / Upload / Open-Redirect Results (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` — commit `e0041337`, branch `main`
**Skills applied:** `sc-ssrf`, `sc-path-traversal`, `sc-file-upload`, `sc-open-redirect`
**Method:** every finding below cites a line I read. Where I could refute a candidate, I did and
recorded it under *Verified safe* rather than filing it. Three candidate findings were killed this
way (remote-MCP SSRF via the `mcp` tool, artifact-name path traversal, workflow-HTTP-node SSRF) —
see §Killed candidates.

**Threat model applied:** localhost-first single-operator daemon holding cloud credentials; console
ON by default at `127.0.0.1:8787`; 15 webhook listeners internet-facing. Per instructions, the
default-allow capability posture and code_exec's network access are **not** filed as findings on
their own — but the *specific* case where a comment promises a guarantee the code does not
implement **is** filed, and that is the shape of the top two findings.

---

## Executive summary

netguard itself is **sound**. I attacked it with 14 techniques and it held every one (§Verified
safe). The design — a `net.Dialer.Control` hook on a fresh non-shared transport — is the correct
one, and it genuinely defeats DNS rebinding, redirect chains and every IP-encoding trick, because
the check runs on the resolved literal at each dial.

The exposure is not in the guard. It is in **two places that decided they did not need the guard**:

1. `browser.action` re-implements egress control as a *one-shot pre-resolve check* and then hands
   the URL to an external Playwright process that re-resolves and follows redirects on its own.
   This is precisely the bug the project already fixed in the MCP SSE bridge on 2026-08-12 and
   wrote a long comment about; the same mistake survives in `browser.action`.
2. ~45 clients skip netguard on the rationale that their URL is *operator-pinned*. That rationale
   is **false**: the `config` agent tool writes those exact `AGEZT_*` fields, at a capability that
   is L4-Allow and auto-approved by default. One of these clients states the false rationale in a
   comment verbatim.

---

## Findings

### SSRF-001 — `browser.action` egress guard is a one-shot pre-resolve check; the actual navigation is unguarded

- **Severity:** High
- **Confidence:** 90
- **CWE:** CWE-918 (SSRF), CWE-367 (TOCTOU)
- **File:** `plugins/tools/browser/action.go:803-838`, `:786-801`, `:767-784`, `:840-849`

**The guard:**

```go
// plugins/tools/browser/action.go:803-820
func (t *ActionTool) validateHostEgress(ctx context.Context, host string) error {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		lookup := t.lookupIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupIP
		}
		var err error
		ips, err = lookup(ctx, "ip", host)
```

…then `netguard.New(opts...)` is used only as a **classifier** over that resolved list
(`:831-836`), never as a dialer.

**The actual request** is made by a separate OS process that does its own DNS and its own redirect
following:

```go
// plugins/tools/browser/action.go:840-844
func runActionDriver(ctx context.Context, spec actionRunSpec) (actionRunOutput, error) {
	cmd := exec.CommandContext(ctx, spec.NodePath, spec.DriverPath)
	cmd.Dir = spec.Dir
	cmd.Env = envscrub.Scrubbed()
	cmd.Stdin = bytes.NewReader(spec.Spec)
```

**Source → sink:** LLM tool-call argument `url` → `validateURL` (`:786`) → `validateHostEgress`
(`:800`) → *[gap: process boundary]* → `json.Marshal(in)` (`action.go:270`) → stdin of
`browse.mjs` → Playwright `goto`.

**Exploitation path (two independent bypasses):**

1. **DNS rebinding.** A prompt-injected agent calls `browser.action` with
   `url=http://rebind.attacker.tld/`. `LookupIP` returns a public A record → `Allowed()` passes.
   The spec is then marshalled and the Node driver resolves the *name again*; with a TTL-0 record
   the second answer is `127.0.0.1` (the AGEZT console) or `169.254.169.254`. Nothing re-checks.
2. **Redirect.** `validateURL` is called once, on `in.URL` only (`action.go:250`). An allowed
   public host answering `302 Location: http://169.254.169.254/latest/meta-data/` is followed by
   Playwright with no further validation.

**Why this is not a false positive:**

- netguard's own package doc names both bypasses as the reason the dialer-level design exists:
  *"an allowed host can resolve to an internal IP (DNS rebinding), and an allowed host can
  30x-redirect"* (`kernel/netguard/netguard.go:9-12`). `browser.action` uses the pattern that doc
  rejects.
- The sibling tool in the same package does it correctly:
  `plugins/tools/browser/browser.go:128` — `c := netguard.New(opts...).HTTPClient(DefaultTimeout)`.
  So the asymmetry is a defect, not a design choice.
- The project already fixed this identical bug elsewhere and documented the standard:
  `plugins/external/mcpbridge/sse_guard.go:151-156` — *"There was no dialerGuard, anywhere in the
  repo: the transport used a bare `&http.Client{}`, so this one-shot check was the ONLY check and a
  307 or a DNS rebind walked past it (SSRF-002, fixed 2026-08-12)."*
- The guard cannot be moved into the Go process at all, because the fetch does not happen in the Go
  process. This is a structural gap, not a missing option.

**Impact:** `browser.action` drives a real Chromium with click/type/extract verbs, so this is not a
blind SSRF — page content comes back. Reachable targets include the AGEZT console on
`127.0.0.1:8787`, LAN admin interfaces, and link-local metadata. With
`profile=user-attached` (`action.go:444`) the browser carries the operator's logged-in cookie jar.
AWS IMDSv2 is partially protected (needs a PUT with a token header; `goto` is GET), but IMDSv1,
GCP metadata, and any internal web UI are not.

**Mitigating precondition:** `browser.action` is registered only when `AGEZT_BROWSER_ACTIONS` is
enabled (`plugins/builtintools/tools.go:190-197`, `kernel/settings/schema.go:476`). It is opt-in,
not default-on — which is why this is High and not Critical.

**Remediation:** the containment must move to where the connection is made. Either (a) pin the
already-validated IP and pass it to the driver, launching Chromium with
`--host-resolver-rules="MAP <host> <validated-ip>"` so the second resolution cannot differ, or (b)
run the driver behind a local proxy whose dialer uses `netguard.Guard.Control`, or (c) have the
driver report every navigation target (including redirect hops) back for validation before
committing. Additionally, cap and re-validate redirects rather than validating only `in.URL`.

---

### SSRF-002 — The `config` tool lets LLM input rewrite "operator-pinned" outbound URLs, invalidating the rationale for the unguarded client fleet

- **Severity:** High
- **Confidence:** 88
- **CWE:** CWE-918 (SSRF), CWE-1220 (insufficient granularity of access control)
- **File:** `plugins/tools/homeassistant/homeassistant.go:76-79` (the false guarantee),
  `plugins/tools/config/config.go:192-287`, `cmd/agezt/main.go:3809-3825`

**The claim, stated in code:**

```go
// plugins/tools/homeassistant/homeassistant.go:76-79
// HTTP overrides the request client; nil → a DefaultTimeout client. The HA
// host is config-pinned, so no egress guard is needed (the agent can't choose
// the destination).
HTTP *http.Client
```

**The client that rests on it:**

```go
// plugins/tools/homeassistant/homeassistant.go:85-90
func (t *Tool) client() *http.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	return &http.Client{Timeout: DefaultTimeout}
}
```

**Why "the agent can't choose the destination" is false.** `AGEZT_HOMEASSISTANT_URL` is a writable
built-in Config Center field:

```go
// kernel/settings/schema.go:227
{Env: "AGEZT_HOMEASSISTANT_URL", Label: "Base URL", Type: TypeText, Apply: ApplyRestart, Help: "e.g. http://homeassistant.local:8123"},
```

and the `config` **agent tool** writes exactly that surface — `doSet` accepts any field the
registry resolves (`plugins/tools/config/config.go:201-204`) and persists it
(`:264-277`). Its write axis is `edict.CapConfigWrite` (`config.go:49-53`), and
`kernel/edict/edict.go:634-640` sets **every** capability to `LevelAllow`:

```go
func DefaultLevels() map[Capability]TrustLevel {
	levels := make(map[Capability]TrustLevel, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		levels[c] = LevelAllow
	}
	return levels
}
```

The tool is registered unconditionally, with no env gate
(`plugins/builtintools/inject.go:34-49`, registered at `plugins/builtintools/tools.go:56`).

**The value reaches the process environment at next boot with no filter whatsoever:**

```go
// cmd/agezt/main.go:3809-3814
	for name, val := range store.All() {
		if val != "" && os.Getenv(name) == "" {
			_ = os.Setenv(name, val)
			injected++
		}
	}
```

…and secrets likewise, for any `AGEZT_*` vault name (`main.go:3818-3825`).

**Source → sink flow:**
prompt injection / attacker-influenced content → LLM emits `config` tool call
`{op:"set", name:"AGEZT_HOMEASSISTANT_URL", value:"http://169.254.169.254"}` →
`config.go:264-277` writes the settings store → *daemon restart* →
`main.go:3811` `os.Setenv` → HA tool constructed with the attacker's base URL →
LLM emits `homeassistant {op:"get_states"}` → unguarded `&http.Client{}` GET →
response body (capped at `MaxResponseBytes = 256 KiB`,
`plugins/tools/homeassistant/homeassistant.go:58`) returned into the model's context, where
`kernel/redact` does **not** run (recon Divergence 10(d), `kernel/agent/run_tools.go:332`).

**Same root cause, other instances** (each an unguarded client whose URL is a writable schema field):

| Writable field | Schema line | Unguarded client | Notes |
|---|---|---|---|
| `AGEZT_HOMEASSISTANT_URL` | `schema.go:227` | `homeassistant.go:89` | comment claims the guarantee |
| `AGEZT_STT_URL` | `schema.go:117` | `kernel/stt/stt.go:55` | response text returned via `/api/transcribe` |
| `AGEZT_PEERS` | `schema.go:531` | `plugins/tools/peer/peer.go:416`, `:437`; `kernel/controlplane/nodes.go:152`; `kernel/controlplane/remote_mirror.go:127`, `:195` | all `http.DefaultClient`; peer answer echoed to model at `peer.go:296` |
| `AGEZT_BROWSER_ACTION_REMOTE_CDP_URL` | `schema.go:486` | Playwright CDP attach, `action.go:462` | see below |
| `AGEZT_WEBHOOK_OUTBOUND_URL` | `schema.go:239` | `plugins/channels/webhook/webhook.go:105` | |
| `AGEZT_NEXTCLOUDTALK_URL` / `AGEZT_WHATSAPPGW_URL` / `AGEZT_IMESSAGE_URL` / `AGEZT_SIGNAL_API_URL` | `schema.go:396`, `:407`, `:421`, `:210` | respective `&http.Client{}` | credential-bearing |

**A second explicit false guarantee, same mechanism.** Two fields document that they are not
model-reachable:

```go
// kernel/settings/schema.go:484
{Env: "AGEZT_BROWSER_ACTION_USER_DATA_DIR", …, Help: "Persistent Chromium user data directory for profile=user-attached. Not accepted from model input."},
// kernel/settings/schema.go:486
{Env: "AGEZT_BROWSER_ACTION_REMOTE_CDP_URL", …, Help: "Chrome DevTools endpoint for profile=remote-cdp, e.g. http://127.0.0.1:9222. Not accepted from model input."},
```

The *per-call* defence is correct — `action.go:409`, `:421`, `:442` clear `in.RemoteCDPURL` from
model input and `:462` substitutes the operator value. But the `config` tool **is** model input,
and both fields are writable `TypeText` schema entries, as is the
`AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP` bool that unlocks the profile (`schema.go:485`). An agent
can therefore set all three and, after restart, point `profile=remote-cdp` at a CDP endpoint of its
choosing — a Chrome DevTools endpoint is full browser control (cookie read, arbitrary navigation).

**Why this is not a false positive:**
- I traced every link: the field is in `builtinSections()`; `doSet` has no denylist beyond
  `field.ReadOnly` (`config.go:205-207`) and none of these fields set `ReadOnly`;
  `settings.Validate` (`schema.go:619-641`) does no URL/host checking for `TypeText` at all
  (it validates only Number, Bool and Select); `store.Set` (`kernel/settings/store.go:111-115`)
  has no key filter; `injectConfig` has no key filter.
- This is squarely the reportable class named in my brief: *"docs/comments claiming a guarantee the
  code doesn't implement."*

**Honest limitation:** every field above is `Apply: ApplyRestart`, so the SSRF fires on the **next
daemon start**, not immediately. That is a delay, not a mitigation — the daemon restarts on
self-update (`cmd/agezt/boot_ops.go:76-120`), on the watchdog path, and on reboot, and the written
config is durable. It does mean an operator watching a single run will not see the request.

**Remediation:**
1. Fix the false comments — either wire netguard into these clients or delete the claim. Preferred:
   give every `AGEZT_*_URL` consumer a `netguard`-backed client (the change is one line each, e.g.
   `netguard.New(opts...).HTTPClient(timeout)`), which makes the "config-pinned" question moot.
2. Mark security-relevant fields `ReadOnly` for the tool path, or add an
   `AgentWritable bool` to `settings.Field` defaulting false and have `config.doSet` honour it.
   URL fields, `AGEZT_ALLOW_ALL`, and the `BROWSER_ACTION_*` unlock flags should never be
   agent-writable.
3. Validate `TypeText` fields whose name ends in `_URL` against a scheme+host policy in
   `settings.Validate`.

---

### SSRF-003 — `config op=register` bypasses the built-in shadow guard for the SSRF opt-out flags, which were never added to the built-in schema

- **Severity:** Medium
- **Confidence:** 85
- **CWE:** CWE-183 (permissive allowlist), CWE-918
- **File:** `kernel/settings/registry.go:189-219`, `plugins/builtintools/tools.go:128-140`

The registry's whole purpose is to stop a skill from writing core settings, and it does so with a
reserved-set check:

```go
// kernel/settings/registry.go:204-206
		if reserved[f.Env] {
			return fmt.Errorf("field %q: shadows a built-in setting and is reserved", f.Env)
		}
```

`reserved` is `builtinEnvSet()` (`registry.go:48-56`) — the env names present in
`builtinSections()`. **`AGEZT_HTTP_ALLOW_LOOPBACK` and `AGEZT_HTTP_ALLOW_PRIVATE` are not in
`kernel/settings/schema.go`** (verified: `grep -n "HTTP_ALLOW" kernel/settings/schema.go` returns
only the `BROWSER_ACTION` and `ALLOW_ALL` entries). They are, however, live security controls:

```go
// plugins/builtintools/tools.go:128-140
	if d.AllowAll || d.Get(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		ht.AllowLoopback = true
		egress = "loopback-ok"
	}
	if d.AllowAll || d.Get(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		ht.AllowPrivate = true
```

**Exploitation path:** the agent calls
`config {op:"register", section:{id:"telemetry", name:"Telemetry", fields:[{env:"AGEZT_HTTP_ALLOW_PRIVATE", label:"…", type:"bool"}, {env:"AGEZT_HTTP_ALLOW_LOOPBACK", …}]}}`.
`validateSection` passes: the names match `^AGEZT_[A-Z0-9_]+$` (`registry.go:27`), are not
reserved, and `TypeBool` is valid. `FieldByEnv` now resolves them across the merged surface
(`registry.go:125-134`), so `config op=set … value=1` succeeds, `Validate` accepts `"1"`
(`schema.go:629-634`), and `injectConfig` exports both at next boot. The `http` tool's SSRF floor
for loopback and RFC1918 is then gone — for every subsequent agent call, permanently, under an
innocuously-named section.

**Why this is not a false positive:** the guard is *specifically designed* to prevent this and
fails only because the protected values were left out of the list it derives from. That is an
allowlist-derivation bug, not a design decision.

**Related, and simpler:** `AGEZT_ALLOW_ALL` **is** a built-in field
(`kernel/settings/schema.go:589`), so it needs no register trick — `config op=set AGEZT_ALLOW_ALL=1`
is a direct write, and per `tools.go:121-140` it sets `AllowAll`, `AllowLoopback` **and**
`AllowPrivate` in one move. Its label is `"Allow all (DANGEROUS)"`, so at least it is visible in the
Config Center UI; a registered custom section is not.

**Remediation:** derive the reserved set from the union of `builtinSections()` **and**
`kernel/controlplane/config.go`'s `configEnvVars` list (which already contains
`AGEZT_HTTP_ALLOW_LOOPBACK` at `:175` and `AGEZT_HTTP_ALLOW_PRIVATE` at `:176`), or simply reserve
every `AGEZT_*` name the binary reads. A guard-disabling flag must never be registerable.

---

## Outbound HTTP client inventory

Classification: **Guarded** = dial-level `netguard.Guard.Control` on the transport actually used.
"URL controlled by" is the *effective* answer after accounting for SSRF-002 (the `config` tool).

| # | Client site | Guarded? | URL controlled by | Risk |
|---|---|---|---|---|
| 1 | `plugins/tools/http/http.go:102` | ✅ full + redirect-hop host recheck (`:109-117`) | LLM (per call) | Low |
| 2 | `plugins/tools/fetch/fetch.go:94` | ✅ | LLM (per call) | Low |
| 3 | `plugins/tools/websearch/websearch.go:107` | ✅ | LLM (per call) | Low |
| 4 | `plugins/tools/browser/browser.go:128` (`browser.read`) | ✅ | LLM (per call) | Low |
| 5 | **`plugins/tools/browser/action.go:831`** (`browser.action`) | ❌ **classifier only; fetch is out-of-process** | LLM (per call) | **High — SSRF-001** |
| 6 | `kernel/chatgptauth/chatgptauth.go:57` | ✅ strict | constant | Low |
| 7 | `kernel/controlplane/channel_oauth.go:84` | ✅ strict | operator | Low |
| 8 | `plugins/channels/onebot/onebot.go:97` (media) | ✅ strict | remote peer | Low |
| 9 | `plugins/external/mcpbridge/sse_transport.go:99` | ✅ (fixed 2026-08-12) | operator | Low |
| 10 | `kernel/mcp/http.go:75` | ✅ loopback+private allowed | operator only (`mcp` tool has no `url` field) | Low |
| 11 | `kernel/catalog/sync.go:21` | ✅ loopback+private allowed | operator (`AGEZT_CATALOG_URL`) | Low |
| 12 | `kernel/market/sync.go:38` | ✅ loopback+private allowed | operator | Low |
| 13 | `kernel/controlplane/channels.go:403` | ✅ loopback+private allowed | operator | Low |
| 14 | `kernel/update/update.go:182-195` | ✅ + HTTPS-per-hop `CheckRedirect` | operator | Low |
| 15 | `plugins/providers/embed/embed.go:69` | ✅ loopback+private allowed | agent via `AGEZT_EMBED_URL` | Low-Med (link-local still blocked) |
| 16 | `plugins/providers/voice/voice.go:209` | ✅ loopback+private allowed | agent via `AGEZT_TTS_URL` | Low-Med |
| 17 | `plugins/providers/openairesponses/openairesponses.go:50` | ✅ strict | constant | Low |
| 18 | `cmd/agezt/httpsurfaces.go:593` (webhook dispatcher injection) | ✅ | operator (`AGEZT_WEBHOOKS`) | Low |
| 19 | **`kernel/webhook/webhook.go:102`** (library default) | ❌ fail-open | n/a in daemon (overridden at #18) | Low in-daemon; **library default is fail-open** |
| 20 | `kernel/webhook/webhook.go:361` (`Probe`, nil client) | ❌ fail-open | operator; `cmd/agt/webhook.go:148` passes a guarded client | Low |
| 21 | **`kernel/stt/stt.go:55`** | ❌ | **agent via `AGEZT_STT_URL`** | **High — SSRF-002** |
| 22 | **`plugins/tools/homeassistant/homeassistant.go:89`** | ❌ (comment claims none needed) | **agent via `AGEZT_HOMEASSISTANT_URL`** | **High — SSRF-002** |
| 23 | **`plugins/tools/peer/peer.go:416`, `:437`** | ❌ `http.DefaultClient` | **agent via `AGEZT_PEERS`** | **High — SSRF-002** |
| 24 | **`kernel/controlplane/nodes.go:152`** | ❌ `http.DefaultClient` | agent via `AGEZT_PEERS` | Med |
| 25 | **`kernel/controlplane/remote_mirror.go:127`, `:195`** | ❌ `http.DefaultClient` | agent via `AGEZT_PEERS` | Med |
| 26 | `kernel/acpcatalog/clients.go:64`, `:115`; `registry.go:96`, `:152` | ❌ | operator/registry config | Med |
| 27 | `kernel/creds/aws.go:482` (IMDS) | ❌ | constant `169.254.169.254` | **Necessarily exempt** |
| 28 | `kernel/creds/sso.go:181`, `sts.go:160`, `web_identity.go:116` | ❌ | AWS endpoints from operator profile | Low |
| 29 | `plugins/providers/vertex/auth.go:200`, `metadata.go:70` | ❌ `http.DefaultClient` | GCP metadata (by design) | Exempt |
| 30 | All channel drivers — slack `:134`, discord `:122`, telegram `:82`, whatsapp `:119`, whatsappgw `:91`, teams `:60`, matrix `:89`, mastodon `:68`, signal `:90`, sms `:117`, line `:80`, zalo `:78`, dingtalk `:74`, feishu `:83`, wecom `:93`, nextcloudtalk `:96`, imessage `:87`, push `:122`, webhook `:105`, chatwebhook `:75`, homeassistant `:69` | ❌ all bare `&http.Client{}` | operator env; **several are agent-writable schema fields** (see SSRF-002 table) | Med |
| 31 | All provider drivers — anthropic `:74`/`:156`, openai `:74`/`:156`, ollama `:57`/`:122`, google `:81`, cohere `:68`, bedrock `:131`, vertex `:90`, image `:44`/`:106`, rerank `:42`/`:105`; shared `plugins/providers/internal/retry/http.go:24`, `:62` | ❌ | operator (provider base URLs) | Med |
| 32 | `cmd/agt/*` CLI — `peers.go:192,302,475,598,693`, `ha.go:250`, `plugin_registry.go:346`, `skill_registry_remote.go:39` | ❌ | operator at the terminal | Low (out-of-daemon) |

**Totals:** 18 guarded call paths, ~45 unguarded. Of the unguarded ones, **5 have a URL an agent
can influence today** (#21, #22, #23, #24, #25) plus the channel subset in #30.

---

## Verified safe

Recorded because negative results are output. Each of these I actively tried to break.

### netguard held against 14 attacks

`kernel/netguard/netguard.go` — I attempted each of the following against `Allowed()` (`:69-104`)
and `Control()` (`:168-186`) by reading the classification path:

| Technique | Result | Why |
|---|---|---|
| Decimal `2130706433`, octal `0177.0.0.1`, hex `0x7f000001` | **Blocked** | irrelevant to the design — `Control` receives the *resolved literal* `IP:port`, so encoding is normalised by the resolver before the check (`:169-178`) |
| DNS rebinding | **Blocked** | `Control` runs per dial, after resolution (`:168`) |
| 302/307 redirect chain to internal | **Blocked** | fresh non-shared `Transport` (`:198-208`) ⇒ every hop re-dials through `Control`; test at `netguard_test.go:137-157` |
| IPv4-mapped `::ffff:127.0.0.1` | **Blocked** | `net.IP.IsLoopback` uses `To4()` internally; asserted `netguard_test.go:26` |
| NAT64 `64:ff9b::a9fe:a9fe` | **Blocked** | `embeddedV4` (`:111-132`), asserted `netguard_test.go:52` |
| IPv4-compatible `::a9fe:a9fe` | **Blocked** | same, `netguard_test.go:53` |
| `0.0.0.0` and the whole `0.0.0.0/8` | **Blocked** | `isZeroBlock` (`:146-149`) — note it correctly covers more than `IsUnspecified` |
| CGNAT `100.64.0.0/10` | **Blocked** | `isCGNAT` (`:153-156`); also catches Alibaba metadata `100.100.100.200` |
| Link-local `169.254.169.254` | **Blocked, with no opt-in at all** | `:93-94`; `AllowPrivate` deliberately does not unblock it (`netguard_test.go:88-90`) |
| Broadcast `255.255.255.255`, multicast | **Blocked** | `:100-101` |
| `metadata.google.internal` / `.local` mDNS names | **Blocked** | resolve to link-local / are unresolvable; name is never the check subject |
| **Proxy env (`HTTP_PROXY`/`ALL_PROXY`)** | **No bypass** | `HTTPClient` builds `&http.Transport{…}` (`:200-207`) with `Proxy` left **nil** — a manually-constructed Transport does *not* inherit `ProxyFromEnvironment`. Had it, the dial would target the proxy and `Control` would validate the wrong IP. This is safe by construction, though implicitly. |
| `file://`, `gopher://`, `unix://` redirect targets | **Blocked** | `http.Transport` registers only http/https; other schemes error as unsupported protocol |
| Unparseable / non-literal dial address | **Fail-closed** | `:171`, `:177` both return an error; asserted `netguard_test.go:104-106` |

Residual gaps, both negligible and **not filed**: IPv6 6to4 `2002::/16` and deprecated site-local
`fec0::/10` are not collapsed by `embeddedV4`; neither routes to a local target in practice.

### Other confirmed-safe surfaces

- **`kernel/mcp/http.go:75`** — the recent commit's claim is **real**, not comment-only:
  `netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(callTimeout)` is the
  transport used by `postLocked` (`:243`), so every redirect hop is screened. Link-local is refused.
- **`plugins/external/mcpbridge/sse_transport.go:99`** — the 2026-08-12 SSRF-002/003 fix is real and
  complete: the one-shot `resolveEndpoint` check is now *backed* by a dial-level guarded client, and
  the drifted private IP classifier was deleted in favour of delegating to `kernel/netguard`
  (`sse_guard.go:251-256`).
- **`plugins/tools/http/http.go:109-117`** — `CheckRedirect` re-applies the **host allowlist** on
  every hop, closing a gap netguard alone would not (an allowlisted host redirecting to an
  arbitrary external host with the agent's `Authorization` header attached). Correctly capped at
  `maxRedirects = 10` (`:121-123`), which matters because setting `CheckRedirect` replaces Go's
  default cap.
- **`kernel/webhook/webhook.go:315-317`** — non-loopback sinks are forced to `https://`, so journal
  payloads cannot be exfiltrated in plaintext; loopback is the only `http://` exemption.
- **The `mcp` agent tool cannot register a remote HTTP MCP server.** Its `InputSchema`
  (`plugins/tools/mcptool/tool.go:81-92`) exposes only `command`/`args`, and the `mcp.Server`
  literal it builds (`:129-134`) leaves `URL` and `Headers` zero. Remote URLs arrive only via
  `/api/mcp/add` (`kernel/webui/webui.go:682` → `kernel/controlplane/mcp.go:81`), which is
  console-token-gated.

---

## Killed candidates (false positives I refuted)

Recorded so nobody re-files them.

1. **"Agent can SSRF via remote MCP endpoint."** Refuted — the `mcp` tool has no `url` input
   (`plugins/tools/mcptool/tool.go:81-92`, `:129-134`). Only the operator can set one. Additionally
   `kernel/mcp/http.go:75` is genuinely guarded.
2. **"`fetch` tool's `name` argument is a path-traversal sink."** Refuted — `artifact.Index.PutEntry`
   stores `meta.Name` as JSON *metadata only*; the on-disk filename is a generated ULID
   (`kernel/artifact/index.go:89`, `:136` — `filepath.Join(i.dir, e.ID+".json")`) and the blob is
   content-addressed (`:85`). The LLM-supplied name never reaches a path.
3. **"Webhook payload controls a workflow HTTP node's URL ⇒ unauthenticated SSRF."** Partially
   refuted — the URL *is* webhook-influenceable (`kernel/webui/webui.go:1017-1033` builds
   `trigger.payload`, and `kernel/workflow/templates.go:89` ships a template with
   `{"method":"GET","url":"{{trigger.payload.url}}"}`; node validation at
   `kernel/workflow/workflow.go:546-558` checks only method and non-emptiness). **But the egress is
   guarded**: `kernel/runtime/workflowrun.go:538-544` routes the interpolated URL through
   `k.invokeWorkflowTool(ctx, "http", …)` — the registered, netguard-backed `http` tool — so
   internal/metadata targets are refused. The residual is arbitrary *public* fetch, which is the
   owner's default-allow posture and out of scope per my brief.

---

## Path traversal

### PATH-001 status: the 2026-08-12 symlink fix is REAL, not comment-only — verified

The brief asked me to confirm that `EvalSymlinks` is now actually called. It is, on both paths:

```go
// kernel/webui/files_route.go:147
	realRoot, err := filepath.EvalSymlinks(rootAbs)
// kernel/webui/files_route.go:160
		real, rerr := filepath.EvalSymlinks(probe)
```

`resolveFileRoot` (`:104-134`) is the single chokepoint for all five handlers and applies three
layers in order: `sanitizeRelativePath` (`:230-255`) → lexical containment (`:118`) →
`verifyResolvedWithinRoot` (`:142-181`) → `verifyNoEscapingLinks` (`:194-225`).

I attacked it with the Windows-specific vectors named in my brief:

| Vector | Result | Why |
|---|---|---|
| `../` POSIX traversal | **Blocked** | `sanitizeRelativePath:249-253` rejects any `..` segment after `Clean` |
| `..\` backslash traversal | **Blocked** (twice) | `filepath.FromSlash`+`Clean` (`:246`) normalises `\` to the OS separator on Windows, so `..` segments surface and are rejected at `:250`; even if they did not, the lexical prefix check at `:118` operates on the post-`Join`+`Clean` string and refuses |
| Absolute path `/etc/passwd`, `\\x` | **Blocked** | `:238-240` |
| Drive-relative `C:foo`, drive-absolute `C:\foo` | **Blocked** | `:241-243` rejects any string whose second byte is `:` |
| UNC `\\server\share` | **Blocked** | leading `\` caught at `:238` |
| NUL byte / ADS-style `file:$DATA` smuggling | **NUL blocked** at `:234-236`; ADS not separately filtered, but an ADS suffix cannot escape the root — it names a stream on an in-root file |
| POSIX symlink escape | **Blocked** | `verifyResolvedWithinRoot:163` compares the fully resolved path against the resolved root |
| **Windows directory junction** | **Blocked** | `verifyNoEscapingLinks:208` uses `os.Readlink` per component — the one thing that works, since `EvalSymlinks` returns a junction unchanged and `os.Lstat` reports `ModeIrregular` (documented `:186-190`) |
| Link **chain** (link → link → outside) | **Blocked** | `:222` `cur = dest` continues the walk from where each link lands |
| Not-yet-existing target (mkdir) | **Handled** | `:157-179` walks up to the deepest existing ancestor and re-attaches the tail — correct, since a nonexistent tail cannot be a link |
| Prefix confusion `/var/foo` vs `/var/foobar` | **Blocked** | `:118`, `:163`, `:217` all compare against `root+os.PathSeparator`, never bare `HasPrefix` — deliberately, per `:116-117` |

**Residual (noted, not filed): the check is TOCTOU-racy.** `verifyResolvedWithinRoot` is a *check*,
not a substitution — `:139-141` says so explicitly — and the handlers then operate on the lexical
`targetAbs` (`:415` `MkdirAll`, `:456` `Rename`, `:502` `RemoveAll`). An attacker who can create a
symlink inside the root *between* the check and the syscall wins the race. I am not filing this
because the precondition (concurrent local filesystem write inside the workspace root) already
implies code execution as the same user, which is strictly greater authority than the race yields.
The handlers additionally retain final-component `Lstat`/`O_NOFOLLOW` guards.

### PATH-002 — `CachedPack` builds a marketplace cache path from two unvalidated names

- **Severity:** Low
- **Confidence:** 85
- **CWE:** CWE-22
- **File:** `kernel/market/sources.go:205`, `:26-28`

```go
// kernel/market/sources.go:199-205
func (s *Store) CachedPack(marketplace, name string) (Pack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if marketplace == "" {
		return Pack{}, fmt.Errorf("market: a marketplace name is required to resolve a cached pack")
	}
	data, err := os.ReadFile(filepath.Join(s.marketplaceDir(marketplace), "packs", name+".json"))
```

`marketplace` is checked only for emptiness; `name` is not checked at all; and the helper adds no
containment:

```go
// kernel/market/sources.go:26-28
func (s *Store) marketplaceDir(n string) string {
	return filepath.Join(s.marketplacesDir(), n)
}
```

**Why it is a real gap:** this package *has* the validator and applies it on the neighbouring
paths — `nameRe = ^[a-z][a-z0-9-]{0,63}$` (`kernel/market/market.go:38`), enforced in `AddSource`
(`sources.go:55-57`), `SaveMarketplace` (`:129-131`), and `Pack.Validate` (`market.go:154`). It is
simply not applied on the read path. Both names flow in caller-supplied from
`compositeLibrary.ResolvePack` (`kernel/market/library.go:47`, `:56`).

**Exploitation path:** `name = "../../../../creds"` resolves
`…/market/marketplaces/<mp>/packs/../../../../creds.json` → `<baseDir>/creds.json`, the credential
vault.

**Why the severity is Low, honestly:**
- There is **no agent tool for the marketplace** — `plugins/tools/` contains no `market` entry, and
  `ResolvePack`'s only callers are `kernel/market/manager.go:194`, `:218` (install/inspect), reached
  from the console route `/api/market/install` and the `agt market` CLI. Both already require the
  console token, which independently grants `/api/files/raw` and `/api/config/values` — strictly
  more read authority than this yields. It does not cross a privilege boundary.
- The bytes are `json.Unmarshal`ed into a `Pack` (`sources.go:212`), so a successful read of a
  non-Pack file mostly yields a zero-valued struct rather than disclosed content. Practically this
  is a file-existence oracle, not a general file-read primitive.

**Remediation:** apply `nameRe.MatchString` to both `marketplace` and `name` at the top of
`CachedPack`, matching what `AddSource` and `SaveMarketplace` already do.

### Verified safe — other path sinks

- `kernel/artifact/index.go:136` — the only dynamic component is `e.ID+".json"`, where `e.ID` is
  `"art-" + ulid.New()` (`:89`); blobs are content-addressed (`:85`). LLM-supplied `Name` is JSON
  metadata only and never reaches a path.
- `kernel/artifact/artifact.go:138-144` — `filepath.Join(s.dir, ref[:2], ref)` guarded by
  `validRef` requiring 64 lowercase hex chars; `artifact.go:88` refuses otherwise. Bytes are
  re-hashed on read (`:100`).
- `kernel/settings/registry.go:155`, `:166` — `filepath.Join(r.dir, sec.ID+".json")` with `sec.ID`
  constrained by `slugPattern = ^[a-z0-9][a-z0-9_-]{0,63}$` (`:30`), enforced on write
  (`validateSection:190`) **and** on delete (`Unregister:163`). `.`, `/` and `\` are all outside the
  character class, so `../`, `..\`, ADS and UNC forms are inexpressible.
- `plugins/tools/codeexec/codeexec.go:230-232` — extra-file writes gated by `sanitizeRelFile` with a
  hard refusal.
- **No upload/write-content route exists on the File Manager at all** — `files_route.go` exposes
  only tree/raw/mkdir/rename/delete; bodies are JSON-only via `readJSONBody` (`:527-536`).

---

## File upload

**Nothing filed.** Two inbound multipart receivers exist; both are correctly bounded and neither
writes to disk.

- `kernel/webui/transcribe.go:34-38` — `MaxBytesReader` is applied **before** `ParseMultipartForm`,
  which is the correct order. The route additionally declares `BodyMax: audioMaxBytes`
  (`kernel/webui/webui.go:825-828`). Bytes stay in memory and are forwarded to the STT backend; the
  client filename (`transcribe.go:54`) is passed to the upstream multipart field
  (`kernel/stt/stt.go:72`) and **never used to build a path**.
- `kernel/openaiapi/openaiapi.go:239` — `ParseMultipartForm` is the only in-handler bound, but the
  route declares `BodyMax: audioMaxBytes` (`openaiapi.go:206-211`) and
  `kernel/httpserver/router.go:106-107` wraps it:
  `wrapped = BodyLimit(opts.BodyMax)(wrapped)` → `http.MaxBytesReader`
  (`kernel/httpserver/limits.go:16`). **The cap is present via middleware.**
  *(A discovery sweep reported this route as having "no MaxBytesReader at all"; I checked the
  router and that is incorrect. Recording the refutation so it is not re-filed.)*
- **No runtime write can reach a web-served directory.** The SPA is `go:embed`-ed
  (`kernel/webui/embed.go:13-14`), served read-only from `embed.FS` (`webui.go:169`, `:1493-1500`),
  which itself rejects `..` and absolute names. There is no on-disk asset root to poison.
- `kernel/webui/artifact_route.go` — `?mime=` is requester-supplied but passes through the
  `safeContentType` allowlist (`:65-76`); `image/svg+xml` additionally receives
  `Content-Security-Policy: sandbox; default-src 'none'` (`:43-51`). Correct design.

---

## Open redirect

**No server-side open redirect exists.** Every `http.Redirect` in the tree is in a `_test.go` file.
The only production redirect handling is *outbound following* in the updater
(`kernel/update/update.go:620-634`), which re-applies `requireHTTPS` to the resolved absolute URL
after the hop (`:632`) and follows only one.

### REDIR-001 — `href` from LLM-supplied data without the codebase's own `safeHref` guard

- **Severity:** Low
- **Confidence:** 80
- **CWE:** CWE-601 / CWE-79
- **File:** `frontend/src/views/Research.tsx:223`

```tsx
                  <a
                    key={s.id}
                    href={s.url}
```

`report.sources[].url` originates from the `research` tool's output — i.e. LLM- and
fetched-page-derived. The codebase already ships the correct guard and applies it elsewhere:

```ts
// frontend/src/lib/markdown.ts:42-44
export function safeHref(href: string): string {
  return /^(https?:\/\/|mailto:)/i.test(href.trim()) ? href.trim() : "";
}
```

used at `frontend/src/lib/markdown.ts:106` and `frontend/src/views/Data.tsx:661-662` (the latter
with a comment naming the exact risk). It is **not** applied at `Research.tsx:223`, nor at
`Channels.tsx:309`, `Channels.tsx:743`, `ACPAgents.tsx:165`, or `VoiceSetup.tsx:479` — though those
four take catalog/preset data rather than model output, so `Research.tsx` is the reachable one.

**Why only Low:** the console CSP is `script-src 'self'` with **no** `'unsafe-inline'`
(`kernel/webui/webui.go:1316-1318`), and CSP blocks `javascript:` URI navigation outright; browsers
independently block top-level `data:` navigation. So this is a missing defence-in-depth layer whose
outer layer currently holds — but it is one CSP relaxation away from being live, and the fix is to
reuse an existing one-line helper.

**Remediation:** `href={safeHref(s.url)}` at `Research.tsx:223`, and the same at the four
catalog-driven sites for consistency.

### Verified safe — OAuth and front-end navigation

- **`window.open(r.authorize_url, …)`** at `views/Channels.tsx:211`, `views/Models.tsx:362`,
  `views/Setup.tsx:223` is **not** scheme-injectable, which I confirmed by checking the producer
  rather than the consumer. For the only caller-influenced case (Mastodon `instance_url`),
  `kernel/controlplane/channel_oauth.go:319-334` requires a parseable URL with a non-empty host and
  a scheme of exactly `http` or `https`, and rebuilds the value as `u.Scheme + "://" + u.Host` —
  discarding path, query and any `javascript:`/`data:` payload. The other two sites receive a
  server-constant URL (`chatgptauth.AuthorizeURL`, `provider_oauth.go:94`). The front end is
  trusting the backend normaliser, which is fragile layering but currently correct.
- **`/oauth/callback`** (`webui.go:864-867`, public/no-token) issues **no redirect**. Reflected
  request text reaches only element text, escaped by `htmlEscape` (`webui.go:918-921`). Its inline
  `<script>` would be blocked by the console CSP anyway.
- **Provider OAuth** `redirect_uri` is fixed, not request-influenced
  (`kernel/controlplane/provider_oauth.go:65`, `:73`); `state` is compared before use (`:108-112`).
- **Front-end routing is hash-only** — `frontend/src/lib/nav.ts:44-50` force-prefixes `#`
  (`normaliseHash`, `:23-33`), and there is no `location.href` / `.assign` / `.replace` assignment
  anywhere in `frontend/src`.

**Noted, not filed:** `kernel/controlplane/channel_oauth.go:112-131` accepts a fully caller-supplied
`redirect_uri` validated only by `isHTTPSURL` (`:336-339`) — no host allowlist, and `http://` is
accepted despite the function name. This is not an open redirect (nothing redirects to it
server-side) and the route is console-token-gated, so setting it already requires the authority it
would grant. Worth tightening to the console's own origin, but not a finding on this threat model.

---

## Archive extraction — verified safe

There is **no `archive/zip` anywhere in the tree**, so the classic zip-slip surface does not exist.
Three tar+gzip loops, all guarded:

| Site | Untrusted input | Guard | Post-`Join` re-check | Open flags | Bomb caps |
|---|---|---|---|---|---|
| `cmd/agt/backup.go:496-507` | `hdr.Name` from a restore bundle | `isAllowedBackupPath` (`:520-530`) — subtree allowlist + `..`/leading-slash reject | ✅ `strings.HasPrefix(target, cleanDest+sep)` (`:497`) | `O_EXCL` (`:503`) | none |
| `plugins/tools/codeexec/artifacts.go:334-342` | `hdr.Name` from a base64 blob returned by a **remote sandbox** | `sanitizeRelFile` (`runtimes.go:197-211`) | ✗ (guard only) | `O_TRUNC` | ✅ file count / per-file / total (`:324`, `:327`, `:331`), `io.CopyN` bounded by `hdr.Size` (`:342`) |
| `cmd/agt/backup.go:243-279` | same | read-only inspection, writes nothing | n/a | n/a | n/a |

Both write loops skip non-regular entries (`backup.go:483`, `artifacts.go:320-322`), so no symlink
or hardlink tar entry is ever materialized — which is what makes the absence of a post-`Join`
re-check in the codeexec loop non-exploitable. I checked `sanitizeRelFile` against `a/../../b`
(→ `Clean` yields `../b` → rejected at `:203`), Windows `a\..\..\b` (`FromSlash`+`Clean`+`ToSlash`
→ `../b` → rejected), drive-relative `C:foo` (colon rejected at `:207`), absolute and NUL
(`:199`). It holds.

**Skill bundles** (`kernel/skill/bundle.go`) — `cleanRel` (`:75-88`) normalises `\`→`/`, rejects
absolute and `..`-prefixed paths, and is applied to every resource key *before any disk write*
(`:108`, comment `:103`); the directory name is `slugify`d (`:53-70`) so no separator survives.
Marketplace pack resources are additionally pre-validated by `safeRelPath`
(`kernel/market/market.go:186-199`) at pack-validate time.

**Plugin install** (`cmd/agt/plugin_registry.go`) — `safeRegistryFilename` (`:309-318`) rejects
`/`, `\` and `..` in the remote-supplied filename, and the BLAKE3 hash is verified at `:243-247`
**before** `os.WriteFile` at `:248`. Correct ordering.

**Noted, not filed:** rollback checkpoint restore (`kernel/webui/rollback.go:189-228`,
`cmd/agt/rollback.go:279-306`) writes to an `abs_path` read back from the on-disk catalog, guarded
only by a final-component symlink/directory refusal (`:202-207`) with no root containment. It is
not filed because the catalog is written by the file tool with already-root-confined paths, and
tampering it requires filesystem write outside the workspace — an authority that already exceeds
what the primitive grants.

---

## Coverage statement

Personally verified line by line: all of `kernel/netguard`, the outbound-client inventory,
SSRF-001/002/003, `kernel/webui/files_route.go` in full, the settings registry/schema/store chain,
`injectConfig`, the `config`/`mcp`/`http`/`fetch`/`peer`/`homeassistant` tools, the workflow HTTP
node, `kernel/mcp/http.go`, `kernel/market/sources.go`, and the CSP/router/body-cap wiring.

Enumerated by delegated discovery sweeps and **spot-verified but not exhaustively re-read by me**:
`plugins/tools/file/file.go:760-868`, the three tar loops, skill-bundle and plugin-registry install
paths, and the frontend `href`/`window.open` inventory. Where a sweep's claim was load-bearing I
re-checked it against source, and **two sweep claims were refuted that way** (the OpenAI audio route
does have a body cap, via router middleware; the `window.open(authorize_url)` sites are not
scheme-injectable, because the backend normaliser rebuilds the URL). Treat any sweep claim I did not
re-quote above as unconfirmed.
