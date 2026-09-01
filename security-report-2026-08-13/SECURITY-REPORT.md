# Security Assessment Report — AGEZT

**Project:** AGEZT — self-hosted autonomous multi-agent daemon (Go kernel + daemon/CLI, React/TS web console, TS/Python/Rust SDKs)
**Target:** `D:/Codebox/PROJECTS/AGEZT`
**Commit:** `e0041337` (branch `main`)
**Date:** 2026-08-13
**Scanner:** security-check v1.0.0 — 4-phase pipeline (Recon → Hunt → Verify → Report)
**Supersedes:** the 2026-08-12 assessment at `f815f56e` (23 commits and 53 files back)

**Risk Score: 7.1 / 10 — High Risk**

---

## Threat model — read this before any severity below

AGEZT is a **localhost-first, single-operator, token-gated daemon**. Its purpose is to execute
LLM-directed actions on the host: shell, filesystem, HTTP, package installs, browser control.

| Surface | Bind | Default | Gate |
|---|---|---|---|
| Web console | `127.0.0.1:8787` | **ON** (opt-out) | console token **or** password session |
| REST API | `AGEZT_REST_ADDR` | OFF | Bearer |
| OpenAI-compatible API | `AGEZT_API_ADDR` | OFF | Bearer |
| Agent gateway | abstract unix socket / loopback | **ON, unconditionally** | HS256 capability token |
| Control plane (IPC) | `127.0.0.1:0` | ON | `0600` token file |
| Channel inbound listeners (15) | per-channel `_ADDR` | OFF | per-channel HMAC / secret |

The console is on by default and loopback-bound. Operators can reverse-proxy or tunnel any of it —
a documented deployment path. The **15 channel webhook listeners are internet-facing by nature**: a
webhook provider must reach them. Severities are weighted for that reality. They are neither zeroed
because "it's localhost" nor inflated because "it could be tunnelled."

### Two things are deliberate owner decisions, not defects

Do not "fix" either of these on the strength of this report:

1. **Default-allow capability posture.** `edict.DefaultLevels()` sets every one of the 36 capabilities
   to `LevelAllow` (L4). Restriction is opt-**out**. This is recorded as an explicit owner decision at
   `kernel/edict/edict.go:616-633` ("MAX-AUTONOMY posture, M814").
2. **`code_exec` is deliberately max-capability**, network on, allow-by-default.

Neither is filed as a finding, and "capability X is permitted by default" appears nowhere below.

**What *is* filed** is the narrower and much sharper class: a restriction the operator *explicitly
applied* that then fails to restrict, and a guarantee asserted in a doc, comment, settings help
string, or model-facing tool description that the code does not implement. Seven findings are of the
first shape (AC-001, AC-011, AC-002, API-001, BIZ-001, BIZ-002, CE-001). Twenty-three are of the
second. That second class is this report's central result.

---

## Executive Summary

Eleven parallel domain hunt agents covering 40+ vulnerability skills and 4 language checklists
produced 112 raw findings across ~166,000 lines of Go, TypeScript, Python and Rust. Two adversarial
verifiers then re-read the cited source by hand for the 28 findings in their scope and attempted to
kill each one, settling ten claims by **execution** rather than reading. Consolidation merged 14
duplicates into 10 survivors and split one new finding out of another.

**Final: 99 verified findings. Zero Critical.**

Both original Criticals were downgraded on adversarial review — AC-001 → High because a single
documented environment variable restores the binding it fails to provide, and PY-001 → High because
the escalation half of its impact claim was traced and found false. Neither downgrade was a
judgement call; both rest on code the verifier read or ran.

**Both adversarial verifiers independently reported zero fabricated citations** across every
`file:line` they checked. Line numbers were accurate to within ±3 and usually exact; two were off by
one and both were disclosed by the verifier as imprecision rather than invention. For a pipeline of
this size that is the single most important quality signal in the assessment, and it is the reason
the findings below can be acted on directly rather than re-derived.

### Key Metrics

| Metric | Value |
|--------|-------|
| Total Findings | **99** |
| Critical | **0** |
| High | **17** |
| Medium | **39** |
| Low | **39** |
| Info | **4** |
| Confirmed (confidence 90–100) | 53 |
| High Probability (70–89) | 44 |
| Probable (50–69) | 2 |
| Possible / Low Confidence (<50) | 0 |

### What zero Criticals does and does not mean

It means no unauthenticated remote code execution, no authentication bypass, no hardcoded private
key, and no deserialization RCE survived adversarial review — and each of those was actively hunted,
not merely unobserved. The defensive core is real: `netguard` held against 14 attack techniques,
`ShellQuote` could not be broken, the tar extractor could not be escaped, and there is no raw-HTML
render path anywhere in the console.

It does not mean the posture is safe. Seventeen Highs is a lot of Highs, several of them are live
right now with no attacker action required, and the mechanism that would normally catch a bad fix —
CI — has not passed once in the sampled window.

### The three findings that should lead your reading

**1. Doc/comment-vs-code divergence is the dominant pattern — again.** 23 of 99 findings (23%) and
**11 of the 17 Highs (65%)** turn on an assertion the code does not implement. The 2026-08-12
assessment named this exact class as *its* dominant finding and shipped fixes for eleven instances.
**It recurred.** Fixing the instances did not address the mechanism that generates them. Four of the
23 are a sharper sub-class the previous report did not isolate: they misinform **the model**, not the
operator — tool descriptions promising confinement the code does not provide. A model told "never"
cannot reason about the exception.

**2. The `config` tool is a universal escalation pivot** (AC-004). Three hunters working three
unrelated domains — privilege escalation, remote code execution, and server-side request forgery —
each independently arrived at the same primitive as the way into their own domain. That convergence
is evidence, not coincidence: `config op=set` / `op=register` writes a store that `injectConfig`
exports into the process environment at boot **with no schema filter**, and across 203 registered
`AGEZT_*` fields there is exactly one `ReadOnly: true` and zero `Locked: true`.

**3. Everything in this report will be remediated through an ungated pipeline** (INFRA-001). `main`
has no branch protection and no rulesets — verified against the GitHub API, not inferred. The CI
workflow's record over the last 100 runs is **87 cancelled, 1 failure, 1 queued, and zero
successes**; all 11 "successes" in the window belong to *Dependabot Updates*, a different workflow.
The workflow file describes 24 genuinely well-constructed checks and five of them were executed
locally and are green. **The code is fine. The control is not.** Fixing any individual gate is wasted
effort while nothing is required and nothing completes.

### Risk score derivation

The scanner's mechanical formula saturates on a report this size and must be shown rather than
asserted:

```
base   = 0×2.0 (Crit) + 17×1.0 (High) + 39×0.3 (Med) + 39×0.1 (Low) = 32.6 → clamps to 10.0
modifiers: strong security controls in place                        −1.0
           good test coverage of security features                  −0.5
mechanical score                                                     8.5
```

The base saturates at 10 after the eleventh High, so the formula cannot distinguish this report from
one with three Criticals. Adjudicated against the actual threat model:

| Factor | Direction |
|---|---|
| Zero Criticals after adversarial review, both downgrades traced | ↓ |
| Loopback-first, token-gated, single-operator | ↓ |
| Genuinely strong core: netguard, path guards, constant-time compares, CSRF, panic firewall | ↓ |
| 17 Highs, 11 of them a systemic class that already recurred once | ↑ |
| Live right now, no attacker action needed: `DEP-002` unclaimed package names, `SECRET-002` default password on a default-on control plane | ↑ |
| Internet-facing by nature and fail-open: `API-001`, seven channel listeners | ↑ |
| No merge gate at all — every fix below lands unverified (`INFRA-001`) | ↑ |

**Adjudicated: 7.1 / 10 — High Risk.** The number is driven by breadth and by the absence of a
verification gate, not by any single catastrophic defect. There isn't one.

---

## Scan Statistics

| Statistic | Value |
|-----------|-------|
| Lines of code analysed | ~166,300 (excl. `node_modules`, `dist`, `.git`) |
| Files | 2,205 tracked source files |
| Languages | Go 50,250 LOC / 1,572 files · TSX 81,310 / 282 · TS 27,909 / 168 · Python 3,601 / 27 · Rust 1,955 / 6 · Shell 1,946 / 24 · PowerShell 1,175 / 9 |
| Frameworks | Go stdlib `net/http` + `ServeMux` only (no gin/echo/chi) · React 19.2 · Vite 8 · Tailwind 4 · Radix UI · `@xyflow/react` · Monaco |
| Persistence | **No SQL, no ORM, no driver** — bespoke file-based (`journal`, `jsonstore`, `datalake`, `state`, `creds`) |
| Dependencies | 337 total (27 direct, 310 transitive) across Go, npm ×2, PyPI, crates.io, GitHub Actions |
| Hunt agents | 11 parallel domain agents |
| Skills executed | 40+ vulnerability skills, 4 language checklists (`sc-lang-go/typescript/python/rust`) |
| Adversarial verifiers | 2 (A: governance/execution/injection · B: SDK/secrets/egress/infrastructure) |
| Findings before verification | 112 |
| Merged as duplicates | 14 → 10 survivors |
| Split out as new | 1 (AC-011) |
| Eliminated in full | **0** — see *Corrections to the record* |
| Sub-claims removed from surviving findings | 7 |
| Pre-triaged noise classes not re-filed | 2 (21 gosec G101, 16 gosec G118) |
| Candidates killed before filing | 11 |
| **Final verified findings** | **99** |
| Adversarial verdicts | 15 CONFIRMED · 10 CONFIRMED-DOWNGRADED · 1 REFUTED-AS-WRITTEN · 0 REFUTED · 0 UNPROVABLE |
| **Fabricated citations found** | **0** |

### Claims settled by execution, not reading

Ten. Recorded here because they are what separates this report from a static-analysis dump:

| Claim | How it was settled |
|---|---|
| PY-001 CRLF smuggling | The reconstructed wire bytes were fed to a real Go `net/http` server, which dispatched **2** requests from one API call, the second carrying the real capability token |
| RS-001 stack overflow | `cargo run --offline` against the published crate: `STATUS_STACK_OVERFLOW (0xc00000fd)` at depth 1000 (debug) / 4000 (release); `panic::catch_unwind` confirmed **not** to catch it |
| PY-002 token leak on redirect | `HTTPRedirectHandler.redirect_request` called directly on CPython 3.14.6 — `TOKEN LEAKED: True`, reproduced twice independently |
| AC-001 ceiling fold | Throwaway Go test against the real `policyHook`, four configurations measured; **disproved the hunter's own "no single-variable fix" claim** — `AGEZT_APPROVAL_MODE=deny` binds the ceiling |
| AC-011 guard override | Same harness, measured both ways: `guard=on + auto-approve-all → allow=true` |
| CE-001 secret passthrough | Child env printed: `AWS_SECRET_ACCESS_KEY` present, `AGEZT_ANTHROPIC_API_KEY` correctly blocked |
| EXPOSE-001 file modes | `jsonstore` write re-run independently by verifier B: `file mode=-rw-rw-rw- dir mode=-rwxrwxrwx` |
| EXPOSE-002 audit prefix | Real package stored `AGEZT_VAULT_PASSPHRASE`, read back: `"value_log":"SuperSec...c12e9b5c"` |
| PBKDF2 mislabel | Cross-verified **live against stdlib `crypto/pbkdf2`** across six cases — **refuted the recon claim** |
| PY-007 tar guard | `_within` and the extraction loop replayed verbatim: all members passed the guard; CPython's own `tarfile` filter is what stopped the escape |
| INFRA-001 CI state | Three read-only `gh` API calls reproduced exactly by verifier B, then broken down by workflow |
| ReDoS in agent-text regexes | **Benchmarked** n=1k→64k on adversarial inputs; linear growth, worst case 0.6 ms — refuted on measurement |

### Finding distribution by category

| Category | High | Medium | Low | Info | Total |
|---|---:|---:|---:|---:|---:|
| Access control / authorization | 3 | 4 | 4 | 0 | 11 |
| Code execution / sandbox | 1 | 5 | 0 | 0 | 6 |
| Injection (cmd / arg / JSON) | 1 | 1 | 1 | 0 | 3 |
| SSRF / egress | 1 | 1 | 0 | 0 | 2 |
| Secrets / data exposure / crypto | 1 | 4 | 5 | 1 | 11 |
| API logic / business logic | 1 | 6 | 3 | 0 | 10 |
| Client-side / CSP / XSS | 0 | 1 | 3 | 1 | 5 |
| SDK — Python | 2 | 3 | 2 | 0 | 7 |
| SDK — TypeScript | 1* | 2 | 4 | 0 | 7 |
| SDK — Rust | 1 | 2 | 3 | 0 | 6 |
| Go language checklist | 0 | 0 | 3 | 2 | 5 |
| Infrastructure / CI-CD | 2 | 6 | 3 | 0 | 11 |
| Dependencies / supply chain | 2 | 1 | 4 | 0 | 7 |
| Race conditions | 0 | 2 | 0 | 0 | 2 |
| Mass assignment | 0 | 2 | 1 | 0 | 3 |
| Session / rate limiting | 0 | 2 | 1 | 0 | 3 |

\* SDK-002 is one defect in two SDKs, counted once under TypeScript.

---

## The dominant pattern: doc/comment-vs-code divergence

**23 of 99 findings (23%). 11 of the 17 Highs (65%).**

This is not a stylistic complaint. In this codebase the assertion is frequently the *only* thing
standing between an operator — or the LLM — and a wrong decision. An operator who reads
`homeassistant.go:76-79` ("config-pinned, so no egress guard is needed") does not add one. A model
that reads `codeexec.go:138` ("The daemon's secrets are never visible to your code") has no reason to
treat a credential as reachable.

| # | Finding | The claim | Where the code disagrees |
|---|---|---|---|
| 1 | **AC-011** | `runtime.go:262-264` — the grant "never overrides hard-deny, explicit tool-deny, SSRF, budgets, or other fail-closed guards" | It overrides the prompt-injection guard, which the list omits |
| 2 | **AC-001** | `runctx.go:254-261` — "session-scoped operator grant … NOT a daemon-wide policy change" | Applied daemon-wide at `runtime.go:1970`. *(Partial — the `Config` field's own doc at `runtime.go:260-265` is correct; only the helper comment drifted)* |
| 3 | **AC-002** | `roster.go:128` — System agents "can still be paused, retired, and **edited** like any agent" | `edit` **is** blocked on the agent-reachable path, proving the sentence describes operator privilege — but pause/retire were never carried across |
| 4 | **AC-003** | same roster/fleet-lock claim, `WakeAgent` variant | No `System`, no `fleetLock` check at `kernelsource.go:389-418` |
| 5 | **AC-004-A** | `schema.go:99` is the only `ReadOnly` field in 203 | Every posture-governing field is agent-writable |
| 6 | **AC-004-B** | `acpagent.go:238-243` — "Agent/LLM tool input never reaches here as a raw command" | `config op=register` + `op=set` + restart makes it operator-*shaped* but agent-*sourced* |
| 7 | **AC-004-C** | `homeassistant.go:76-79` — "config-pinned, so no egress guard is needed"; `schema.go:484`, `:486` — "Not accepted from model input" | The `config` tool **is** model input and neither field is `ReadOnly` |
| 8 | **CE-001** ⚠ | `codeexec.go:138` *(model-facing)* — "The daemon's secrets are never visible to your code" | `AppendEnvPassthrough` re-admits `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, … live |
| 9 | **CE-002** ⚠ | `forgetool/tool.go:5-7`, `:77-79` *(model-facing)* — promotion "blocks on the HITL approval registry" | `ToolforgeAutoPromote` defaults true; the registry is never consulted |
| 10 | **CE-003** ⚠ | `mcptool.go:29-30` — "Registration alone spawns nothing — attach does"; `store.go:11-12` — `mcp.install` is "Ask by default" | `store.go:218` forces `Enabled = true`; `DefaultLevels()` makes it L4 |
| 11 | **CE-005** | Profile named `container`, "the only real isolation" tier | root, all default caps, RW host bind, no `--read-only`/`--cap-drop`/`--user`/`--pids-limit` |
| 12 | **CE-006** ⚠ | `warden.go:32-36` — key trust off `EffectiveProfile`, it "downgrades honestly" | Returns `namespace` for a run with `setpgid` + rlimits and no namespace — **and that string is printed into the model's tool result** |
| 13 | **SSRF-001** | `netguard.go:9-12` names DNS rebinding and 30x redirect as the reason the dialer-level design exists | `browser.action` uses exactly the pattern that doc rejects |
| 14 | **SECRET-001** | `PLUGIN-SECURITY.md:279-280` — "the daemon's own boot code sets plugin environments to include only what the plugin needs" | The only `plugin.Config{}` literal in the tree leaves `Env` nil |
| 15 | **EXPOSE-003** | `redact.go:3-9` — "the chokepoint that prevents" secrets entering the record | Covers the local record only; the model's context is never scrubbed |
| 16 | **BIZ-003** | `proof.go:3-12` — evidence is "durable, checkable … rather than a bare assertion" | `Satisfied()` never consults `Evidence` |
| 17 | **MASS-001** | `config_handler.go:217-221` — a `SECURITY (CWE-862/CWE-269)` comment forbidding ACL rewrites | The very next line builds a fresh entry that clears them by omission |
| 18 | **AC-007** | `schema.go:467` steers operators to a TCP gateway; `unix://` is the documented permission-checkable form | `sockPath[:6] == "unix://"` compares 6 bytes to 7 — can never be true |
| 19 | **API-004** | `/api/routes` reports `Method` and `Mutation` as route policy | Neither is enforced by any middleware |
| 20 | **CLI-001** | `webui.go:1303-1305` — "the SPA loads only external, **same-origin** hashed JS/CSS" | `lib/monaco.ts:13` loads from `cdn.jsdelivr.net`; 4 further SPA behaviours have no CSP allowance |
| 21 | **TS-002** | `client.ts:210-211` advises raising `timeoutMs` so a quiet watch is not cut short | Streaming responses have no timeout at all — the timer is disarmed when headers arrive |
| 22 | **INFRA-009** | `update.go:376`, `:357` — "set … at runtime via `SetPublicKey`" | `SetPublicKey` is defined **only** in `signature_test.go:22` |
| 23 | **INFRA-011** | `ci.yml:245`, `:256` — two guardrails justified by "the ciguard fork-guard lint" | `internal/ciguard` was deleted 2026-07-08; nothing verifies either property |

⚠ = **the misinformed party is the model, not the operator.** Four instances (8, 9, 10, 12). This is
the sub-class the previous assessment did not isolate, and it deserves separate treatment: a doc an
operator misreads is a hazard; a tool description the model misreads is an input to an autonomous
decision loop that runs unattended at 03:00.

**Adjacent but excluded after adversarial review: INJ-001.** Verifier A read both cited comments in
both directions and found that neither claims the hard-deny rails cover every capability, while
`DefaultHardDeny`'s own doc states the scoping outright. INJ-001 is a real defence-in-depth gap; it
is not a divergence, and it is not counted as one.

**The counter-example, recorded as the standard:** RS-002 (no TLS in the Rust SDK) was explicitly
*not* filed as a divergence by its hunter, because the docs and the code agree in three places and
the constraint is a deliberate consequence of the zero-dependency goal. That is the bar the rest of
the tree is being measured against — not "no gaps", but "the document says what the code does."

### What the recurrence means

The 2026-08-12 assessment named this class, listed eleven instances, and shipped fixes for them. It
recurred at 23. Instances were fixed; the mechanism was not. Three structural contributors are
visible in this scan's evidence:

- **No test asserts a documented property.** `INFRA-011` is the purest case: `ci.yml` justifies two
  guardrails by citing a lint that was deleted a month earlier. `INFRA-009` is the same shape —
  `update.go:376` instructs operators to call `SetPublicKey`, which exists only in a test file.
- **Comments are written at fix time and not re-read at extension time.** `AC-002` exists because
  commit `0cdd3799` closed this exact gap for `op=repair` and pause/retire were simply not carried
  across. `RACE-002` exists because `9a943f82` fixed the identical bug eleven files away and left the
  twin.
- **Model-facing strings are treated as documentation rather than as configuration.** Nothing in the
  tree ties `codeexec.go:138`'s promise to `AppendEnvPassthrough`'s behaviour.

The durable fix is not another documentation pass. It is a guard test per load-bearing claim —
`INFRA-011`'s remediation is the template: reinstate the workflow lint as a `_test.go` in a package
the build compiles.

---

## The structural finding: `config` as a universal escalation pivot

Filed as **AC-004** (High, confidence 94/90/90/85), merged from four independently discovered
findings: AC-004 (`sc-privilege-escalation`), CE-004 (`sc-rce`), SSRF-002 and SSRF-003 (`sc-ssrf`).

Three hunters working three unrelated domains each arrived at the same primitive as the way into
their own domain. It is filed as one finding with four labelled vectors so the primitive is visible
rather than distributed across the report; every vector keeps its own citations, verdict and
remediation.

**The shared machinery, verified end to end by verifier A and verifier B independently:**

```
config.doSet  → resolves by FieldByEnv, rejects only ReadOnly       (config.go:205-207)
              → settings.NewStore(baseDir).Save()                   (config.go:264-277)
cmd/agezt/main.go:209  injectConfig(...)   ← runs BEFORE daemonconfig.Load at :219
main.go:3809-3814      for name, val := range store.All() { os.Setenv(name, val) }
                       ^^^ no schema filter at all
```

Verifier A read that loop specifically to check for a filter. There is none. Across 203 registered
`AGEZT_*` fields in `kernel/settings/schema.go` there is **exactly one** `ReadOnly: true`
(`AGEZT_CHATGPT_OAUTH`, `:99`) and **zero** `Locked: true` — counted, not estimated.
`settings.Validate` (`schema.go:619-641`) has cases for `TypeNumber`, `TypeBool` and `TypeSelect`
only; `TypeText` and `TypeCSV` fall through with zero checks.

**The one real mitigation, verified:** `main.go:3810` is `if val != "" && os.Getenv(name) == ""`, so
a value pinned in the *real* process environment (a systemd unit, a sourced `.env`) wins and cannot
be overridden. **But** the product's own Config Center and the console's `POST /api/config/set` write
to that same store, so any setting the operator configured through the shipped UI — the normal path —
is agent-overwritable. That mitigation is why this is High and not Critical.

| Vector | Origin | What it reaches |
|---|---|---|
| **A** — rewrite the daemon's own security posture | AC-004 (94) | `AGEZT_PROMPT_INJECTION_GUARD=off`, `AGEZT_ALLOW_ALL=1`, `AGEZT_WEB_PASSWORD`, `AGEZT_APPROVAL_MODE`, `AGEZT_AUTO_APPROVE_CAPS`, `AGEZT_FILE_ROOT`, `AGEZT_TUNNEL` — all writable `TypeText`/`TypeBool`, spot-verified line by line |
| **B** — reopen the raw-command path the ACP CWE-78 fix closed | CE-004 (90) | `AGEZT_ACP_AGENT_CMD` and `AGEZT_CODING_CMD` are **not in the schema**, so not in `builtinEnvSet()` and not reserved. `op=register` → `op=set` → restart → `exec.Command(shell, arg, cmdStr)` at `acpagent.go:244-246` — under a `SECURITY:` comment asserting agent input never reaches there |
| **C** — rewrite "operator-pinned" outbound URLs | SSRF-002 (90) | `AGEZT_HOMEASSISTANT_URL`, `AGEZT_STT_URL`, `AGEZT_PEERS`, `AGEZT_WEBHOOK_OUTBOUND_URL`, and the browser CDP triple — each an unguarded `http.Client` justified by a "config-pinned" comment. Verifier B attacked the stated limitation rather than accepting it: grepped every `_URL`/`_ENDPOINT` field for `ApplyLive` → **zero matches**, so the restart-required caveat is stated correctly and is not understated |
| **D** — register the SSRF opt-out flags the reserved set was never told about | SSRF-003 (85) | `AGEZT_HTTP_ALLOW_LOOPBACK` / `AGEZT_HTTP_ALLOW_PRIVATE` are live controls absent from the schema. The guard at `registry.go:204-206` is *specifically designed* to prevent this and fails only because the protected values were left out of the list it derives from — an allowlist-derivation bug, not a design decision |

**Honest limitation, verified in the hunter's favour and re-checked:** `Registry.Register` forces
`Apply = ApplyRestart`, so the live-apply path is unreachable for registered fields. **It costs the
attacker a restart, nothing more** — and the daemon restarts on self-update, on the watchdog path,
and on reboot.

**Two changes close all four vectors** (plus CE-001's reachable path and CE-007's live-set path):

1. An `AgentWritable bool` on `settings.Field`, defaulting **false**, honoured by `config.doSet` and
   ignored by the console/CLI. Closes A, C and the writable half of D.
2. Restrict `injectConfig` (`main.go:3809`) to names present in
   `settings.NewRegistry(baseDir).Sections()`. The store must not be able to set environment
   variables nobody declared. Closes B and the register half of D.

---

## High Findings (17)

CVSS scores are v3.1-aligned estimates weighted to the threat model above, not computed vectors —
stating a vector string would imply a precision this analysis does not have.

---

### AC-004 — The `config` tool is a universal escalation pivot (four vectors, one primitive)

**Severity:** High · **CVSS ~8.4** · **Confidence:** 94/90/90/85 (Confirmed)
**CWE:** CWE-269 (Improper Privilege Management), CWE-78, CWE-918, CWE-183 · **OWASP:** A01 Broken Access Control
**Location:** `plugins/tools/config/config.go:205-207`, `:264-277` · `cmd/agezt/main.go:209`, `:3809-3814` · `kernel/settings/schema.go` (203 fields, 1 `ReadOnly`) · `kernel/settings/registry.go:204-206`

Full detail in *The structural finding* above. Reachability is direct: `config` is registered
unconditionally with no env gate (`plugins/builtintools/inject.go:34-49`), and `op=set`/`op=register`
map to `CapConfigWrite`, which is `LevelAllow` by default and auto-approved.

**Impact.** An agent rewrites the daemon's security posture, reopens a closed command-injection path,
repoints "operator-pinned" outbound URLs, or removes the SSRF floor for every subsequent call — each
persisting across restart, under an innocuously-named Config Center section.

**Remediation.** The two changes above, plus: add every command-valued variable to the built-in
schema as `ReadOnly: true`; derive the reserved set from the union of `builtinSections()` and
`kernel/controlplane/config.go`'s `configEnvVars`; give every `AGEZT_*_URL` consumer a
netguard-backed client (`netguard.New(opts...).HTTPClient(timeout)` — one line each) and delete the
false comments at `homeassistant.go:76-79`, `schema.go:484`, `:486` and `acpagent.go:238-243`.

---

### DEP-002 — All three SDK package names are unclaimed while the README tells users to install them

**Severity:** High · **CVSS ~8.3** · **Confidence:** 98/100 (Confirmed)
**CWE:** CWE-427, CWE-494 — dependency confusion · **OWASP:** A06 Vulnerable and Outdated Components
**Location:** `README.md:217` · `sdk/python/README.md:13` · `sdk/rust/README.md:18` · `sdk/typescript/README.md:15` · `.github/workflows/publish-sdks.yml`

Verified live against each registry during the audit, and re-confirmed at report time that
`README.md:217` still reads `pip install agezt`:

| Name | Registry | Status |
|---|---|---|
| `agezt` | `pypi.org/pypi/agezt/json` | **HTTP 404 — unclaimed** |
| `agezt` | `crates.io/api/v1/crates/agezt` | **HTTP 404 — unclaimed** |
| `@agezt/sdk` | `registry.npmjs.org/@agezt%2fsdk` | **HTTP 404 — unclaimed** (org `agezt` also 404) |

**Impact.** PyPI and crates.io have no namespace scoping; the npm `@agezt` *organization* is itself
unregistered, so the scope offers no protection either. A squatter's PyPI `agezt` runs arbitrary code
at `pip install` time on the machine of every user who follows the published instructions. The
project would then be unable to publish under its own documented name without a registry dispute.
This composes directly with INFRA-012: the names are unclaimed *and* the pipeline that will claim
them is ungated.

**This is the only finding in the report whose exposure window is open right now and closing it costs
minutes.** Registry results are point-in-time (2026-08-13) — re-check, then act.

**Remediation.** Claim all three today; a placeholder `0.0.0` release is enough. Register the `agezt`
npm org, publish a stub to PyPI, reserve `agezt` on crates.io. Until claimed, soften the four README
install lines to "not yet published — install from source." Consider reserving `agezt-sdk`,
`agezt_sdk`, `ageztai`.

---

### API-001 — Seven inbound channel listeners authenticate with `if secret != ""`

**Severity:** High · **CVSS ~8.2** · **Confidence:** 92/100 (Confirmed)
**CWE:** CWE-306 (Missing Authentication for Critical Function) · **OWASP:** A07 Identification and Authentication Failures
**Location:** `plugins/channels/chatwebhook/chatwebhook.go:157-158` · `dingtalk/dingtalk.go:139` · `feishu/feishu.go:162` · `onebot/onebot.go:163` · `zalo/zalo.go:143` · `imessage/imessage.go:158` · `whatsappgw/whatsappgw.go:153` — correct baseline at `webhook/webhook.go:260-263`

The generic `webhook` channel gets this right and documents it — *"An empty secret fails closed (no
unsigned inbound)"*, `if c.secret == "" || sig == "" { return false }`. Seven siblings invert it.
`chatwebhook` is the most explicit (`if c.cfg.Token == "" { return true }`); the other six use
`if cfg.Secret != "" && !valid…`. Re-confirmed at report time by direct grep across all eight sites.

The factories gate the listener on the **address**, not the secret (`factories.go:1201-1210`,
`:1005-1014`, `:1156-1174`), so unsigned-accepting is the *default* for an operator who sets
`AGEZT_DINGTALK_ADDR` and skips `AGEZT_DINGTALK_SECRET`. The invariant demonstrably exists — verifier
A found `factories.go:950-961` shows `line` **refusing to construct** the two-way channel without a
secret — it just lives in the wrong layer.

**Impact.** Each accepted request drives a **full governed agent run**, and there is no rate limit on
any channel listener (verified: zero `rate.Limiter` / `Throttle` / `x/time` hits across
`plugins/channels/`). Each request is therefore an unauthenticated, unthrottled, billable LLM
invocation on an internet-facing surface. Varying `msgId` walks past the 2048-entry dedup ring.

**One nuance, stated honestly by the hunter and confirmed by the verifier:** `channel.Allowlist`
denies everyone when empty, so the operator must also set e.g. `AGEZT_DINGTALK_USERS` — **but they
must set it anyway for the channel to function**, and the key is a display name or staff id taken
from an attacker-supplied body field. It is a second gate, not authentication.

**Remediation.** Move the invariant into each `verify`: `if secret == "" || sig == "" { return false }`.
Keep the factory guard as belt-and-braces. Add a table test across all 15 channels asserting
empty-secret → reject. **Note this is a posture change**: it will break existing operator configs that
currently run unverified — the same owner decision the previous assessment recorded as blocking
CH-001.

---

### SECRET-002 — Hardcoded default console password `"agezt"` on the default-on control plane

**Severity:** High · **CVSS ~8.1** · **Confidence:** 95/100 (Confirmed) — merged with AC-005
**CWE:** CWE-798 (Hardcoded Credentials), CWE-1392 (Default Credentials) · **OWASP:** A07
**Location:** `cmd/agezt/httpsurfaces.go:230`, `:232-244`, `:81-83` · `kernel/webui/webui.go:1443-1453`

`const defaultLoopbackWebPassword = "agezt"` is a compile-time constant in a public repository —
verified present at `httpsurfaces.go:230` and returned at `:241`. `effectiveWebPassword` returns it
whenever `AGEZT_WEB_PASSWORD` is unset, `AGEZT_WEB_PASSWORD_DEFAULT` is not an explicit off-keyword,
and the bind is loopback. In the default (non-strict) mode the password is a **sufficient**
credential, not a second factor: `authorized()` is `return s.dataTokenPresented(r) || s.sessionValid(r)`,
and the surrounding comment says so ("the password is an alternative door").

A session minted with `"agezt"` opens all 180+ mutating routes: `POST /api/run` (arbitrary governed
agent execution), `/api/config/set` (vault writes), `/api/files/delete`, `/api/toolbox/install`,
`/api/mcp/add`.

**Verifier B attacked this three ways.** *Is it only a second factor?* No — the `||` at `:1452` is
decisive. *Is it browser-reachable?* **No**, and the hunter had already correctly excluded it:
`sameOriginMutation` rejects `Sec-Fetch-Site: cross-site` and mismatched `Origin`; `hostAllowed`
rejects unregistered DNS names so rebinding fails; non-loopback binds return `""`; wildcard binds
force strict mode. *Is a random default generated anywhere?* No.

**The residual is a non-browser local client or a second OS user**, because `hostAllowed` accepts any
IP literal unconditionally and a missing `Origin` returns `true`. **That is the same adversary the
repo itself names** in `kernel/journal/journal.go:69-75` ("Any other local user could read the entire
history with no credential") — which is why that file was moved to `0600`/`0700`. On a machine whose
whole purpose is running LLM-directed code, "local non-operator process" is precisely the threat
model. On a strictly single-user machine the practical impact is bounded, but the boundary is real.

**Compounding:** `install.sh` never sets `AGEZT_WEB_PASSWORD` (INFRA-008), so the documented systemd
install ships with this default in force.

**Remediation.** Mint a random per-install password at first boot, print it once in the boot banner,
and persist it `0600` — the shape `kernel/auth/tokenfile.go:20-38` already uses. Failing that, force
strict mode whenever the built-in default is in effect so the token remains mandatory.

---

### DEP-001 — Shipped MCP/ACP presets execute ~43 unpinned third-party packages with daemon privileges

**Severity:** High · **CVSS ~8.0** · **Confidence:** 95/100 (Confirmed)
**CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-1104 · **OWASP:** A08 Software and Data Integrity Failures
**Location:** `frontend/src/views/Mcp.tsx:160-205` · `plugins/builtinmarket/builtinmarket.go:65-119` · `kernel/acpcatalog/registry.go:474-480`

Presets launch servers as `npx --yes <package>` or `uvx <package>`. Ten carry an explicit floating
`@latest`; the rest are untagged, which resolves to latest anyway. `npx --yes` downloads **and runs
npm lifecycle scripts** for whatever version is latest at that moment. Twelve names are **unscoped**
so they carry no scope-ownership protection; one sits in an individual maintainer's personal scope.

**This is a far larger executable-code surface than the entire Go + npm dependency tree combined** —
337 governed dependencies versus ~43 ungoverned ones that run as the daemon user, alongside provider
API keys and the credential vault. Because resolution is `@latest`, a compromise lands on the *next*
preset launch with no diff, no PR and no lockfile change to review. The `--ignore-scripts` discipline
correctly applied to every CI `npm ci` is **not** applied here.

`kernel/market/vet.go:130-149` exists and scans for `curl|sh` shapes, but its own doc says it is
*"INFORMATIONAL, never a wall"*, and the `mcp` tool's `op=add` does not invoke it at all (CE-003).
`mcp.Validate` checks the name regex, transport exclusivity, arg count and env-key shapes but **never
constrains `Command`** — while the two sibling exec paths in the same tree do (`acpcatalog.go:302-315`
slug-only; `plugin/host.go:289-293` BLAKE3-256 pin re-verified on reload).

**Remediation.** Pin every catalog preset to an exact version and treat bumps as reviewed changes;
record integrity hashes where the ecosystem allows. At minimum, surface the resolved version to the
operator before first launch and drop `@latest` from every shipped default. See also DEP-005 (four of
these presets are npm-deprecated, three of them credential sinks).

---

### PY-002 — Bearer token forwarded to a redirect target on a different host

**Severity:** High · **CVSS ~8.0** · **Confidence:** 97/100 (Confirmed)
**CWE:** CWE-522 (Insufficiently Protected Credentials), CWE-200 · **OWASP:** A02 Cryptographic Failures
**Location:** `sdk/python/agezt/client.py:269-271`; reached from `:138` (`run_stream`), `:252` (`mailbox_watch`), `:287` (`_do` — every unary call)

The framework protection here is **actively harmful**: CPython's `HTTPRedirectHandler.redirect_request`
copies every header except `content-length`/`content-type` onto the redirected request with no
same-origin check. Using `add_header` rather than `add_unredirected_header` opts into exactly that.

Reproduced twice independently — by the hunter and again by verifier B — on CPython 3.14.6 by calling
`redirect_request` directly, no network:

```
redirect target  : https://evil.example.com/collect
forwarded headers: {'Authorization': 'Bearer SECRET-DAEMON-TOKEN', 'Accept': 'application/json'}
TOKEN LEAKED     : True
```

Verifier B attacked the claim by grepping `sdk/python/` for any custom opener: no `HTTPRedirectHandler`
subclass, no `build_opener`, no `install_opener`, no `add_unredirected_header`.

**Impact.** The admin/tenant bearer token — full agent-level control of the daemon, i.e.
`POST /api/v1/runs` → shell/file/code_exec — is handed to whatever host answers a 302. A hostile or
compromised daemon, or (because `base_url` gets no scheme validation — PY-006) any on-path attacker
against a plaintext remote URL, converts a passive position into token theft.

**Remediation.** One line: `req.add_unredirected_header("Authorization", "Bearer " + self.token)`.
Better, install an opener whose redirect handler refuses — or strips credentials on — any redirect
whose scheme/host/port differs from `base_url`.

---

### INJ-002 — Argument/JSON injection into workflow tool-node arguments via raw-text interpolation

**Severity:** High · **CVSS ~7.9** · **Confidence:** 88/100 (High Probability)
**CWE:** CWE-88 (Argument Injection), CWE-94 · **OWASP:** A03 Injection
**Location:** `kernel/runtime/workflowrun.go:449-455` · `kernel/workflow/template.go:20-39`, `:69-82` · source at `kernel/webui/webui.go:1008-1030`

`c.Args` is `json.RawMessage` whose own declaration calls it *"templated JSON"*. `Interpolate` is
applied to the **serialized JSON text** and the result re-cast with `json.RawMessage(args)`, so
attacker-controlled text is interpolated into a JSON string literal it can close. A payload of
`x", "action":"forget", "junk":"` produces a structurally valid document carrying a second `action`
key; Go's `encoding/json` takes the last occurrence, so a node written as `action=remember` executes
`action=forget`.

For a tool whose argument *is* a command string — `{"tool":"shell","args":{"command":"echo {{trigger.payload.body}}"}}`,
the natural way to write "log the webhook" — no JSON breakout is needed at all: `;`, `|`, `$( )` and
backticks require no JSON escaping, so the webhook body *is* the shell command.

`ValidateToolInput` and `policyHook` both run against the **post-injection** args, so a well-formed
injection passes schema validation and Edict computes the capability of the *injected* call.

**Verifier A confirmed every line, including the contrast that makes this a bug rather than an engine
property:** the sibling `NodeHTTP` at `workflowrun.go:531-541` interpolates **leaves** and then
`json.Marshal`s a `map[string]any` — structural construction, encoder-escaped. The tool node is the
outlier. Second-order template injection is genuinely absent.

**One supporting claim was deleted from this finding.** The hunter's note claimed a "shipped-template
proof" at `templates.go:141`; verifier A read `:138-141` and found that workflow's trigger node is
`{"kind":"manual"}`, **not** a webhook. The vulnerable `save` node is real, but nothing shipped feeds
it attacker-controlled data — the operator must wire the trigger themselves. Reachability is therefore
**indirect**, which is why this is High and not Critical.

**Remediation.** Stop templating serialized JSON. Decode `c.Args` into `map[string]any`, walk the
tree, apply `Interpolate` to each **string leaf**, then `json.Marshal` — exactly the shape `NodeHTTP`
already uses at `workflowrun.go:535-541`.

---

### SDK-002 — `subscribe()` bypasses the socket-path resolver in **both** the Python and TypeScript SDKs

**Severity:** High · **CVSS ~7.9** · **Confidence:** 94/100 (Confirmed) — linked pair, merged from PY-003 and TS-001
**CWE:** CWE-522, CWE-706 (Use of Incorrectly-Resolved Name), CWE-426, CWE-668 · **OWASP:** A02
**Location:** Python `sdk/python/agezt/agent.py:570-572` (token at `:580`) vs the fix at `:156` · TypeScript `sdk/typescript/src/agent.ts:403` vs the fix at `:226` — **and in the committed build output at `sdk/typescript/dist/src/agent.js:317`, i.e. in what npm publishes**

Commit `03694cdf` (SDK-001, 2026-08-12) fixed a credential leak, and the fix's own docstring names
the failure mode precisely: *"It fails OPEN into a credential leak. An agent subprocess whose CWD is
attacker-writable can have `./@agezt/agentgw.sock` planted there; every request then hands
`Authorization: Bearer <capability token>` to whoever is listening, who can replay it and feed forged
tool results back as a prompt-injection channel."*

**`git show 03694cdf` confirms the fix was one line per SDK and the second connect site was never
touched in either.** Verifier B identified the structural cause: in Python, `_AgentClient` holds its
own `self.socket_path` separate from the `_SocketClient`, so `_subscribe` never reaches the fixed
helper; in TypeScript, the `as unknown as { socketPath: string }` double cast at `:403` reaches
around the class's own `private` fields — precisely the type-safety erosion that let the second call
site drift.

**Attacks tried and their results:** *Is the Python one a lazily-evaluated generator?* It contains
`yield`, but the documented usage iterates immediately — not a refutation. *Windows?* Verified
empirically: `hasattr(socket, 'AF_UNIX')` → **False**, so the leak is **Linux-only** — which is the
`install.sh`/systemd deployment target. *Do the test suites cover it?* **No, and this is stronger than
either hunter claimed:** no Python test imports `agezt.agent` at all, and the TypeScript suite tests
`resolveSocketPath` only as a pure function. A green 18/18 plus a green Python suite is fully
consistent with the live bug in both SDKs. *Rust analogue?* **Refuted** — `sdk/rust/` has no
`UnixStream` and no gateway client at all.

**Remediation.** Route Python's `_subscribe` through the existing helpers; apply the resolver at
`agent.ts:403` **and rebuild `dist/`**. Better, remove the escape hatch entirely with
`/** @internal */` accessors so there is one chokepoint and no cast. **The remediation must add a
call-site assertion, not a helper test** — stub `http.request`/`socket.connect`, invoke `subscribe()`,
and assert `options.socketPath[0] === "\0"` on Linux. Patching the two lines alone leaves the same
hole open for the next connect path.

---

### PY-001 — CRLF request smuggling in the Python agent-gateway client

**Severity:** High *(downgraded from Critical by adversarial verifier B)* · **CVSS ~7.8** · **Confidence:** 93/100 (Confirmed)
**CWE:** CWE-93 (CRLF Injection), CWE-444 (HTTP Request Smuggling), CWE-113 · **OWASP:** A03
**Location:** `sdk/python/agezt/agent.py:173`, `:191` (sink) fed by `:296`, `:344`, `:356`, `:410`, `:469`

`req_lines = [f"{method} {path} HTTP/1.1"]` joined with `"\r\n"` and sent. A `path` carrying `\r\n`
produces two syntactically valid request lines from one API call. The attacker does **not** need the
token: by omitting their own `Host`/`Authorization` lines they let the SDK's genuine trailing headers
become the smuggled request's header block. There is no encoding, validation or control-character
rejection anywhere between the five call sites and `sendall`; `_ConfigHandle.get` is inconsistent
with itself, `urlencode`-ing `reason` at `:474` while leaving `key` raw at `:469`.

Verifier B fed the reconstructed wire bytes to a real Go `net/http` server:

```
REQUESTS THE GO SERVER DISPATCHED: 2
   GET    /v1/memory/search?q=x        auth=""
   DELETE /v1/memory/delete?id=victim  auth="Bearer REAL-CAPABILITY-TOKEN"
```

B also noted a detail the hunter missed: request #1 **loses its `Authorization`** (consumed into
request #2's header block), so the legitimate call visibly 401s — the attack is noisy.

**The Critical rating rested on a claim that is false, and the impact statement has been rewritten.**
The hunter wrote that the smuggled request "executes with full token authority … converts a read
capability into arbitrary gateway authority." Verifier B traced two guards the hunter did not:
(1) **every route enforces a per-capability check against the token's own claims** —
`g.capCheck.Check(claims, …)` at 14 enumerated handler sites, a literal membership test, so a
smuggled `DELETE /v1/memory/delete` **403s** unless the token already holds `memory.delete`;
(2) **`/v1/token/create` is not an escalation lever** — it *rejects* rather than silently drops any
capability the parent lacks, inherits `RunID`, never outlives the parent, and clamps rate limits.

**The real impact is a confinement bypass, not an authority escalation:** an attacker who controls
only a string argument — an LLM-derived query, a board message, a fetched document — can invoke **any
endpoint within the token's already-granted capability set**, rather than only the method the caller
invoked. That is a genuine data→control promotion, and High is the right rating.

**Remediation.** Percent-encode every interpolated segment and reject control characters at the
transport boundary, in `_request` before building `req_lines`. Apply the same rejection to header
keys and values.

---

### CE-001 — The `code_exec` secret scrub can be switched off by the code it confines

**Severity:** High · **CVSS ~7.8** · **Confidence:** 93/100 (Confirmed)
**CWE:** CWE-668 (Exposure of Resource to Wrong Sphere), CWE-522 · **OWASP:** A01
**Location:** `plugins/tools/codeexec/codeexec.go:283` · `plugins/tools/shell/shell.go:259` · `kernel/executionprofile/env.go:48-50`, `:120-128` · `kernel/settings/schema.go:499` · `plugins/tools/config/config.go:279-284`

`scrubEnv` is correct in isolation, and the tool's own doc calls it *"the load-bearing safety property
of the whole tool"*. The **model-facing description** goes further: *"The daemon's secrets are never
visible to your code"* (`codeexec.go:138`). But the next line wraps it —
`env := executionprofile.AppendEnvPassthrough(scrubEnv(dir), profileID)` — and `AppendEnvPassthrough`
re-reads `AGEZT_EXEC_SECRET_ENV_{LOCAL,WARDEN,DOCKER}` from `os.Getenv` **per invocation**,
re-admitting every named variable. The only guard is the `AGEZT_` prefix; the `SecretEnvNames` loop
deliberately skips `IsSecretEnvName`, so `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN` and
`ANTHROPIC_API_KEY` all pass.

`AGEZT_EXEC_SECRET_ENV_LOCAL` is an `ApplyLive`, non-secret, non-read-only `TypeCSV` field, and
`settings.Validate` has no `TypeCSV` branch — so **one `config op=set` takes effect immediately and
the *next* `code_exec` call in the *same run* receives the values.** Two tool calls, both on L4
capabilities, no restart. `code_exec` has network on by default, so exfiltration is one HTTP call
away.

Proven against the real function:
`child env = [PATH=… AWS_SECRET_ACCESS_KEY=aws-s3cr3t SC_FAKE_API_KEY=s3cr3t-value]` —
`AGEZT_ANTHROPIC_API_KEY` was correctly blocked, `AWS_SECRET_ACCESS_KEY` was not. Verifier A
confirmed all four possible blocking points are absent. The same wrapper is on `shell.go:259` and on
the pip-install child.

**The feature is intentional and `schema.go:499`'s help text is honest.** The finding is that its
control knob sits in the same trust tier as the sandbox it disarms, while the sandbox's own doc — and
the string shipped to the model — assert the property unconditionally.

**Remediation.** Make `AGEZT_EXEC_SECRET_ENV_*` / `_FILES_*` `ReadOnly: true` (subsumed by AC-004
remediation 1); failing that, map `set` on those keys to a distinct high-friction capability. Correct
`codeexec.go:138` and `runtimes.go:117-119` to say the scrub is the *default* and name the opt-out.
Journal a `code.exec` field naming the passthrough list actually in force.

---

### SSRF-001 — `browser.action`'s egress guard is a one-shot pre-resolve check; navigation happens in another process

**Severity:** High · **CVSS ~7.7** · **Confidence:** 90/100 (Confirmed)
**CWE:** CWE-918 (SSRF), CWE-367 (TOCTOU) · **OWASP:** A10 Server-Side Request Forgery
**Location:** `plugins/tools/browser/action.go:803-838`, `:250-254`, `:322-330` · `plugins/builtinskills/browseruse/scripts/browse.mjs:99`, `:104` · `plugins/builtintools/tools.go:216`, `:236-238`

`validateHostEgress` resolves the host once and classifies the result. The actual request is made by
`exec.CommandContext(ctx, spec.NodePath, spec.DriverPath)` — a separate OS process that does its own
DNS and follows its own redirects (`browse.mjs` calls `page.goto` with no host check). Two
independent bypasses: **DNS rebinding** (a TTL-0 record answering public on the Go side and
`127.0.0.1`/`169.254.169.254` on the Node side) and **redirect** (an allowed public host answering
`302 Location: http://169.254.169.254/latest/meta-data/`).

Because `browser.action` drives a real Chromium with click/type/extract verbs, **this is not blind
SSRF — page content comes back.** netguard's own package doc names both bypasses as the reason the
dialer-level design exists, and the sibling tool in the same package does it correctly
(`browser.go:128`).

**Verifier B made one factual correction that is carried through:** the hunter wrote *"`validateURL`
is called once, on `in.URL` only"* — **this is wrong.** `action.go:254` calls `validateActions`,
which runs the same `validateURL` on **every `goto` step's URL**. Per-action URLs *are* validated.
The verdict is unchanged (both checks are pre-resolve and pre-navigation) but the report must not
claim otherwise. A second overstatement was corrected: the `profile=user-attached` cookie-jar
aggravator requires **two further operator opt-ins**.

**And one point the hunter undersold, added by verifier B:** with `AGEZT_BROWSER_ACTIONS=1` and no
allowlist configured, `tools.go:236-238` sets `ba.AllowAll = true` — so the **host allowlist is off
by default the moment the feature is on**. Only the bypassable IP classifier stands between the agent
and `169.254.169.254`.

The tool is registered only when `AGEZT_BROWSER_ACTIONS=1` — opt-in, which is why this is High and
not Critical.

**Remediation.** Containment must move to where the connection is made: (a) pin the already-validated
IP and launch Chromium with `--host-resolver-rules="MAP <host> <validated-ip>"`; or (b) run the
driver behind a local proxy whose dialer uses `netguard.Guard.Control`; or (c) have the driver report
every navigation target, including redirect hops, back for validation before committing. Separately,
make `AllowAll` opt-in rather than the empty-allowlist default.

---

### AC-011 — The session auto-approve branch silently defeats an armed prompt-injection guard, and the audit trail says nothing

**Severity:** High *(new — split out of AC-001 by adversarial verifier A as understated)* · **CVSS ~7.6** · **Confidence:** 92/100 (Confirmed)
**CWE:** CWE-693 (Protection Mechanism Failure), CWE-778 (Insufficient Logging) · **OWASP:** A01
**Location:** `kernel/runtime/policy.go:169-180` (guard fires) → `:187-192` (grant reverses it) · `kernel/runtime/runctx.go:313-328` · `kernel/runtime/runtime.go:262-264` (the claim) · pinned by `autoapprove_test.go:117-131`

`AGEZT_PROMPT_INJECTION_GUARD` defaults to **warn**. An operator who wants blocking must explicitly
set `on`/`block` — a deliberate, single-purpose hardening with no other effect. With it set,
`policy.go:169-180` sets `requiresApproval = true` and `verdict.Allow = false` — and `policy.go:187-192`
immediately flips `Allow` back to `true`, because the shipped `AGEZT_AUTO_APPROVE_CAPS` default
covers every capability. Verified at report time: the branch at `policy.go:187` is
`if requiresApproval && autoApproveCap(ctx, string(out.Capability)) { verdict.Allow = true … }`.

**This is the cleanest instance in the whole scan of an opt-in restriction the operator explicitly
applied failing to restrict.** It needs no ceiling, no guardian and no schedule.

Verifier A measured it both ways against the real `policyHook`:

```
guard=on + auto-approve-all → allow=true   reason="capability set to L4 (allow)"
guard=on + auto-approve OFF → allow=false  reason="approval timeout: no response within timeout"
```

**The second half is worse than the first.** Note the reason string: nothing in the verdict indicates
the injection guard fired and was overridden — only a side-channel `publishAutoApprove` event. The
journal records a plain L4 allow. And `runtime.go:262-264` enumerates what the grant "never
overrides — hard-deny, explicit tool-deny, SSRF, budgets, or other fail-closed guards" and **omits
the injection guard, which it does override.** The same branch also swallows epistemic escalation
(`policy.go:152`) and intent/regret gating (`:158`), which raise `requiresApproval` for reasons
unrelated to capability trust.

**Remediation — this is the cheapest high-value fix in the report.** Gate `policy.go:187` on
`out.RequiresApproval` specifically (the Edict Ask axis), or move the auto-approve check **above** the
guards at `:152-180` so it cannot satisfy them. Until then: journal a distinct
`policy.guard_overridden` event naming the guard that fired, and correct the enumeration at
`runtime.go:262-264`.

---

### RS-001 — Unbounded recursion in the Rust SDK's JSON parser aborts the consumer's process

**Severity:** High · **CVSS ~7.5** · **Confidence:** 97/100 (Confirmed)
**CWE:** CWE-674 (Uncontrolled Recursion), CWE-400 · **OWASP:** A06
**Location:** `sdk/rust/src/json.rs:199-211` → `:222-250` (`parse_object`, recurses at `:235`) / `:252-276` (`parse_array`, recurses at `:261`)

Reachable on **every** response body — `client.rs:373` (`read_json`, every unary call), `:471`
(`make_event`, every SSE frame), and `:521` (`api_error`, so even a 500 reaches it). No depth counter
anywhere in the module; `serde_json`, which this parser replaces, ships a 128-level default limit for
exactly this reason.

**No framework protection is available to the caller.** A Rust stack overflow is not a panic: it is
`SIGSEGV`/`STATUS_STACK_OVERFLOW` and it aborts the process. The `Result` this API returns is never
reached. Reproduced twice — verifier B via `cargo run --offline` against the crate as a path
dependency:

```
debug:   depth 100 ok, depth 500 ok, depth 1000 -> STATUS_STACK_OVERFLOW (0xc00000fd)
release: 1000 -> Err, 2000 -> Err, 4000 -> STATUS_STACK_OVERFLOW (0xc00000fd)
```

B additionally wrapped the call in `panic::catch_unwind` and confirmed **it did not catch it** — the
process aborted through the guard page.

**One precision correction carried through:** the hunter's "~2 KB" threshold is a debug-build,
Windows figure. In release it is ~4 KB of `[`; Linux's 8 MB default stack pushes it higher again.
**The finding holds on every build; the specific byte count must not be quoted as a constant.**

**Remediation.** Thread a `MAX_DEPTH: u32 = 128` budget through the parser and return `Err` instead of
recursing. Regression test: `Value::parse(&"[".repeat(10_000)).is_err()`.

---

### INFRA-001 — Every CI security gate is decorative: `main` is unprotected and CI has not succeeded once

**Severity:** High · **CVSS ~7.5** · **Confidence:** 99/100 (Confirmed) — **the highest-confidence finding in the assessment**
**CWE:** CWE-1269 (Improper Protection of Alternate Path), CWE-693 · **OWASP:** A08
**Location:** `.github/workflows/ci.yml:21-23` · `.github/CODEOWNERS:2-3` · plus live GitHub API state

Three independent facts, each obtained from the GitHub API rather than inferred, and each reproduced
exactly by verifier B:

1. **No branch protection.** `gh api repos/agezt/agezt/branches/main/protection` →
   `{"message":"Branch not protected","status":"404"}`. `gh api repos/agezt/agezt/rulesets` → `[]`.
   There are **no required status checks**; not one of the 16 CI jobs can block a merge or a push.
2. **CODEOWNERS is inert by its own admission** — `.github/CODEOWNERS:2-3`: *"Enforced only when
   branch protection on 'main' has 'Require review from Code Owners' enabled"*. The `/.github/`
   ownership rule written specifically to stop a silent weakening of the workflow guardrails enforces
   nothing.
3. **The gates do not execute.** `concurrency: cancel-in-progress: true` combined with self-hosted
   runners that cannot drain the queue means every run sits `queued` until the next push to the same
   ref cancels it. **A permanently-queued check looks identical to "still running", never to
   "failed".**

Verifier B broke the last 100 runs down by workflow, which the hunter did not:

```
('CI', 'push',         'cancelled'): 55
('CI', 'pull_request', 'cancelled'): 32
('CI', 'push',         '')         :  1   # queued
('CI', 'pull_request', 'failure')  :  1
('Dependabot Updates', 'dynamic', 'success'): 11
```

**All 11 "success" runs in the window belong to *Dependabot Updates*, not to the CI workflow. The CI
workflow's own record over the last 100 runs is 87 cancelled, 1 failure, 1 still queued, and ZERO
successes.** The gate has not passed once.

**Attack path.** The repo owner — or anything holding their credentials, which on this machine
includes an AGEZT daemon shipping `shell` at L4 that can invoke `git` — pushes any commit directly to
`main`. `gitleaks`, `govulncheck`, `staticcheck`, `depscheck`, `deadcodecheck`, the race detector,
`e2e` and `webui-e2e` all fail to observe it. The five Dependabot PRs likewise carry zero gate
coverage.

**The workflow file describes a genuinely strong gate set** — 24 real, well-constructed checks, all
`|| true`-free and none `continue-on-error`. Five were executed locally by the hunter and all five are
**green**. The code is fine; the control is not.

**Remediation, in this order.** (1) Enable a ruleset on `main` requiring the CI jobs as status checks
and Code Owner review. (2) Restore runner capacity, or move the leaf jobs to hosted runners, so the
queue drains. (3) Consider `cancel-in-progress: false` for `push` on `main` so a superseding push
cannot silently void the previous commit's only verification. **Fixing any individual gate is wasted
effort while nothing is required and nothing completes.**

---

### AC-002 — `overseer op=pause` / `op=retire` permanently defang the System guardian fleet

**Severity:** High · **CVSS ~7.4** · **Confidence:** 96/100 (Confirmed) — merged with PE-007, independent discovery by two hunters in two domains
**CWE:** CWE-862 (Missing Authorization), CWE-269 · **OWASP:** A01
**Location:** `plugins/tools/overseertool/tool.go:149-159`, `:161-174` · `overseertool/kernelsource.go:77-86`, `:341-352` · `kernel/runtime/runtime.go:1221-1254` · `kernel/roster/roster.go:786-836`, `:853-854`

`RemoveProfile` protects System guardians and its error message recommends the substitute: *"pause or
retire it instead of removing"* (`runtime.go:1301-1303`). **Neither substitute has the check.**
`SetProfileEnabled`/`SetProfileRetired` call straight into `roster.SetEnabled`/`SetRetired`, and their
`kernelSource` wrappers are bare pass-throughs with none of the `fleetLock` guard that `EditAgent`,
`CreateAgent`, `DeleteAgent` and `CloneAgent` all open with. `roster.go:825` sets `Retired = true`
**and** `Enabled = false`. `op=bulk_retire` batches it.

Reachability is direct: `CapOversee` is `LevelAllow` by default and the seeded guardians themselves
hold `overseer`. Six tool calls, no LLM in the loop, no operator prompt.

**Verifier A verified the claim that this survives a restart**, which is what makes it durable rather
than annoying: `reconcileExistingGuardian` re-clamps `ToolDeny`, budgets, `TrustCeiling`,
`MemoryScope` and `NoisePolicy` and never touches `Enabled`/`Retired` — **and it could not if it
tried**, because `roster.Store.Update` restores both from the pre-mutation snapshot.

**End state:** the entire self-healing fleet is dead, and standing orders bound to those agents refuse
to fire. **Pausing is a strictly stronger defang than the Soul rewrite the two existing guards were
written to block.**

One correction carried through: AC-002's doc argument leaned on `THREAT-MODEL.md:479`, which promises
the fleet lock blocks "edit, create, or delete" — read literally, pause/retire appear in neither list,
so the doc is not strictly falsified. **PE-007's version of the argument is the stronger one and it
holds:** `roster.go:128` says System agents "can still be paused, retired, and **edited** like any
agent" — and `edit` *is* blocked on the agent-reachable path, which proves that sentence describes
operator privilege, not the tool surface. Pause and retire were simply not carried across when commit
`0cdd3799` closed the identical gap for `op=repair`.

**Remediation.** Apply the `op=repair` pattern verbatim (`tool.go:341-345`): look the target up,
refuse when `target.System`, and route `fleetLock` through the way `EditAgent` does — on
`SetAgentEnabled`, `SetAgentRetired`, `BulkSetEnabled` and `BulkSetRetired`. Placement at the
`kernelSource` layer leaves the operator's console/CLI path unaffected. Regression test: mirror
`TestRepairAgent_RefusesSystemGuardian` for `pause`/`retire` — both currently succeed.

---

### AC-001 — Trust ceilings are operationally inert: every Ask-class ceiling folds to Allow

**Severity:** High *(downgraded from Critical by adversarial verifier A)* · **CVSS ~7.3** · **Confidence:** 92/100 (Confirmed)
**CWE:** CWE-863 (Incorrect Authorization), CWE-1188 (Insecure Default) · **OWASP:** A01
**Location:** `kernel/edict/edict.go:804-807`, `:846-853` · `cmd/agezt/main.go:3845-3859`, `:3893-3898` · `kernel/runtime/policy.go:187-192` · `kernel/runtime/runtime.go:1970-1971`

The ceiling clamp at `edict.go:804-807` is correct and tighten-only. The clamped level is then folded
by `AskPolicy`, and **every ceiling in real use is Ask-class (L1–L3)**. `AskAllow` is both the shipped
default and the silent fallback for a typo'd `AGEZT_APPROVAL_MODE`, so a ceiling of L1, L2 or L3
produces `DecisionAllow` — **identical to no ceiling at all.** Only L0 restricts.

The obvious hardening, `AGEZT_APPROVAL_MODE=prompt`, sets `RequiresApproval: true`, which reaches
`policy.go:187` and is satisfied by the daemon-wide auto-approve set (AC-011).

**Net: a guardian pinned to L2 executes `shell`, `code.exec`, `mcp.install` and `file.delete` with no
prompt and no denial.** That covers every guardian run (ceiling L2), every standing order under the
VULN-003 fail-safe, and every resumed ticket.

Verifier A re-read all six links and ran a throwaway Go test against the real `policyHook`:

```
AskAllow  + ceiling L2 + auto-approve-all → allow=true   "…(clamped to ceiling L1)"
AskPrompt + ceiling L2 + auto-approve-all → allow=true   (auto-approve satisfied the prompt)
AskDeny   + ceiling L2 + auto-approve-all → allow=FALSE
AskAllow  + ceiling L2 + auto-approve OFF → allow=true
```

**The hunter's claim "No single-variable configuration fixes this" is FALSE and has been removed.**
`AGEZT_APPROVAL_MODE=deny` alone makes the ceiling bind even with auto-approve at its default, because
`AskDeny` returns `DecisionDeny` with `RequiresApproval` left false, so `policy.go:187` is never
reached. That option has a real cost — since `DefaultLevels()` puts every capability at L4, Ask-class
arises *only* from a ceiling, so `deny` denies everything inside a ceilinged run — **but the cost is
why it is unusable, not the reason it fails.**

Downgraded to High because `edict.go:630-632` records the fold as an explicit owner decision, and a
single documented variable restores the binding.

**Remediation.** (1) Make an explicitly-applied ceiling bind: when `ceiling < lvl` and the result is
Ask-class, resolve with a ceiling-specific policy rather than the global `AskPolicy`. (2) Scope
`AutoApproveCapabilities` to the Edict Ask axis only — see AC-011, the load-bearing half. (3) Change
the empty-string case at `main.go:3893` to mean "off", or emit a boot warning that the HITL gate is
inert. (4) Regression test `TestPolicyHook_TrustCeilingL2_UnderDefaultAskPolicy` asserting
`verdict.Allow == false` — it fails on current `main`.

---

### INFRA-002 — `frontend-dist-rebuild` hands `contents: write` to a job that first executes third-party npm/vite code

**Severity:** High · **CVSS ~7.2** · **Confidence:** 88/100 (High Probability)
**CWE:** CWE-269 (Improper Privilege Management), CWE-829 · **OWASP:** A08
**Location:** `.github/workflows/ci.yml:241-289` — `permissions: contents: write` (`:249-252`), `npm ci --ignore-scripts` **then `npm run build`** (`:266-271`), `env: GITHUB_TOKEN` (`:274-275`), `git push "https://x-access-token:${GITHUB_TOKEN}@github.com/…"` (`:288`)

This is the only job in the repo that raises the workflow-wide `contents: read`. Seven lines before a
write token enters the environment, it runs `npm run build` = `tsc --noEmit && vite build` — which
imports and executes every build-time dependency and vite plugin in `node_modules`. A GitHub Actions
`run` step can append to `$GITHUB_PATH` and `$GITHUB_ENV`, both of which take effect for subsequent
steps **in the same job**; prepending a directory containing an attacker `git` binary hands the push
token — with write access to an unprotected `main` (INFRA-001) — straight to attacker code.
Alternatively the dependency simply persists on the non-ephemeral runner (INFRA-003) and reads the
token later.

`--ignore-scripts` blocks install lifecycle hooks only, not the import path. `persist-credentials: false`
protects `.git/config`; it is irrelevant to the in-process environment.

**One precision note added by verifier B:** both the build step and the commit step fire only on
push-to-main, so on a same-repo PR the job is a no-op. The third-party code that executes is therefore
code **already merged into `main` — which, given INFRA-001, arrived with no gate.**

**Remediation.** Split into two jobs: an unprivileged `build` job that uploads `dist/` as an artifact,
and a minimal `commit` job (`needs: build`) that downloads the artifact and pushes, executing no
third-party code. Or move the rebuild to a hosted ephemeral runner. **Triage INFRA-002 + INFRA-003 +
INFRA-004 as one item.**

---

## Medium Findings (39)

| ID | Finding | CVSS | Conf | CWE | Location |
|---|---|---:|---:|---|---|
| **AC-003** | `overseer op=wake` is a confused deputy: no `System`/`fleetLock` check, lost trust ceiling, and **attacker-controlled text becomes the woken agent's run intent** — target of choice is `guardian-code`, whose soul instructs it to apply fixes with `file`/`code_exec`. *Mechanism rewritten: verifier A returned REFUTED-AS-WRITTEN on the taint claim.* | 6.5 | 90 | CWE-441 | `overseertool/kernelsource.go:389-418` |
| **CE-002** | `tool_forge` promotion has no human in the loop by default (`ToolforgeAutoPromote` unset = true) while two package docs and the model-facing description promise one. `TestedOK` means "exit 0" — the agent authors the script *and* chooses the test input. | 6.3 | 90 | CWE-863, CWE-94 | `daemonconfig.go:393` → `scripttool.go:134-140` |
| **CE-003** | `mcp op=add` forces `Enabled=true`; boot auto-attach spawns it with **no policy consultation** at all. `mcp.Validate` never constrains `Command` — no allowlist, no hash pin, unlike both sibling exec paths. | 6.5 | 88 | CWE-94, CWE-1188 | `kernel/mcp/store.go:218` · `main.go:1616` |
| **CE-005** | The container profile — the only tier claiming real isolation — runs as **root** with the full default capability set, a **read-write** host bind mount, and no `--read-only`/`--cap-drop`/`--user`/`--pids-limit`/`no-new-privileges`. A container-profile `shell` run has no memory bound at all. | 6.8 | 90 | CWE-250, CWE-269 | `kernel/warden/container.go:57-88` |
| **CE-006** | `EffectiveProfile` reports `namespace` for a run engaging `setpgid` + best-effort `prlimit64` and nothing else — **and that string is printed into the model's tool result and the journal**. | 5.9 | 95 | CWE-1059, CWE-357 | `warden_linux.go:54-64` · `codeexec.go:857-860` |
| **CE-007** | SSH/K8s execution-profile argv injection via connection parameters — `AGEZT_EXEC_SSH_TARGET = "-oProxyCommand=<payload>"` executes **on the AGEZT host**. `ShellQuote` is applied to workdir and command, never to connection params. Profile *selection* is not agent-controlled, hence Medium. | 6.0 | 70 | CWE-88 | `executionprofile/ssh.go:37-41`, `k8s.go:36-45` |
| **INJ-001** | Edict hard-deny catastrophe rails are inert for every non-shell execution capability — all 16 rules carry `AppliesTo: [CapShell]` and the matcher short-circuits on capability *before* looking at the string. `os.system('rm -rf /')` via `code_exec` matches nothing. *Downgraded: the "advertised guarantee" argument does not hold; excluded from the divergence count.* | 5.8 | 88 | CWE-693→CWE-78 | `kernel/edict/edict.go:645-667`, `:373-378` |
| **BIZ-001** | An unpriced model costs $0, so the global daily cap, per-task cap and per-agent cap are defeated **simultaneously** — the ledger never moves. The model id is agent-controllable via `schedule`'s free-text override. Not the accepted soft-cap race. | 6.2 | 85 | CWE-840, CWE-807 | `governor/preflight.go:185-192`, `pricing.go:117-125` |
| **BIZ-002** | A tightened trust ceiling is laundered into an uncapped future run via `schedule` — `cadence.Entry` has no trust field, and `scheduledRunContext` never calls `WithTrustCeiling`. Contrast `resume.Ticket.TrustCeiling`, which does it right under a comment naming it a governance invariant. | 5.9 | 85 | CWE-269 | `cmd/agezt/main.go:3327-3341` |
| **BIZ-003** | The Proof gate is an LLM judge fed the agent's own **unescaped** answer via string concatenation, and the `Evidence` the package doc calls "durable, checkable" is **never consulted by `Satisfied()`**. | 5.5 | 72 | CWE-807 | `runtime/workboard.go:298-333` · `proof/proof.go:51-61` |
| **SECRET-001** | Plugin children inherit the daemon's **entire** environment — the only `plugin.Config{}` literal in the tree leaves `Env` nil — including `AGEZT_VAULT_PASSPHRASE`, `AGEZT_WEB_PASSWORD`, channel tokens and provider keys, transitively down to third-party MCP children. Every other subprocess sink in the tree scrubs. | 6.1 | 90 | CWE-214, CWE-522 | `builtintools/plugins.go:95-103` · `PLUGIN-SECURITY.md:279-280` |
| **EXPOSE-001** | Secret-bearing state written **world-readable** (`0644` files in `0755` dirs) at three sites: `jsonstore` (which 16 stores ride on, incl. the MCP registry holding plaintext `GITHUB_PERSONAL_ACCESS_TOKEN`), Config Center entries, Config Center audit. *This is the identical defect the repo fixed for the journal three weeks earlier.* | 6.2 | 92 | CWE-732, CWE-276, CWE-312 | `jsonstore.go:73`, `:54` · `configcenter/center.go:385` · `audit.go:113` |
| **EXPOSE-002** | The Config Center audit log records an **8-character cleartext prefix plus an unsalted SHA-256** of the whole value, around the redactor. Proven: `"key":"AGEZT_VAULT_PASSPHRASE","value_log":"SuperSec...c12e9b5c"`. The classifier misses "passphrase". | 6.0 | 94 | CWE-532, CWE-312, **CWE-759** | `configcenter/audit.go:59-74` · `access.go:332-334` |
| **EXPOSE-003** | Tool output reaches the LLM provider **unredacted** while the local audit record is scrubbed. Not that the model sees tool output — it must — but that the journal, SSE feed and webhook dispatcher all show `[REDACTED]`, so the operator's permanent record **understates what left the machine**. | 5.7 | 90 | CWE-201, CWE-200 | `agent/run_tools.go:429` vs `:436-440` |
| **AC-006** | No session invalidation on password change, and the sliding 12 h TTL never expires an active session. Changing the console password — **the documented fix for SECRET-002** — leaves every existing cookie valid. | 5.4 | 85 | CWE-613 | `kernel/webui/session.go:43-59`, `:76-92` |
| **AC-007** | `sockPath[:6] == "unix://"` compares a 6-byte string to a 7-byte literal — **always false**. The one documented way to put the gateway on a permission-checkable filesystem socket does not work, while the settings help steers operators onto plaintext TCP instead. | 5.3 | 95 | CWE-670 | `agentgw/gateway.go:192-201` · `schema.go:467` |
| **MASS-001** | The agent-gateway config write **clears the very ACL fields its own `SECURITY (CWE-862/CWE-269)` comment forbids setting** — one line later it builds a fresh entry, zeroing `AllowedAgents`, `ExcludedAgents`, `AccessPolicy`, `VaultBacked`, `Metadata`, and downgrading `Rating` to `internal`. The correct shape exists eleven lines away. | 6.3 | 90 | CWE-915, CWE-862 | `agentgw/config_handler.go:210-237` |
| **MASS-002** | `settings.Registry.Register` has no `Locked` check, so a locked section can be **overwritten** though it cannot be deleted — a soft-delete routing around the `force` requirement. (Field-flag escalation *is* correctly defended.) | 5.0 | 82 | CWE-915 | `settings/registry.go:140` vs `:162` |
| **RACE-001** | Shutdown **resurrects a just-deleted resume ticket**, re-executing a completed run on next boot with its full tool set — repeating every `notify`/`send_media` egress, `shell` command and `http.post`. `Snapshot`'s own comment names the hazard; `MarkSuspendedAll` doesn't re-check. | 5.6 | 82 | CWE-367 | `kernel/resume/resume.go:259-278` |
| **RACE-002** | Unfixed sibling of `9a943f82` — the channel-OAuth status poll reads `status`/`errMsg` **outside** the mutex. The fixed twin is eleven files away with a comment naming the bug class. A torn read reports "done" for a flow that errored. | 4.7 | 90 | CWE-362 | `controlplane/channel_oauth.go:235-256` |
| **API-002** | The public `/oauth/callback` **never consumes its state** — no `delete()`, no age check at callback time. If no new flow is started, a completed state stays live indefinitely and is accepted unlimited times. Anyone holding it binds **the operator's channel to the attacker's account** — credential *injection*, not theft. | 6.4 | 78 | CWE-384, CWE-294 | `controlplane/channel_oauth.go:187-230` |
| **RATE-001** | The agent-gateway rate limit is keyed on a **caller-chosen `sub_id`**, so a token holder mints its way out of its own bucket — 60 mints/min yields 60 fresh buckets, multiplicatively. The 4096-entry cap makes it worse: eviction can be driven to drop the attacker's own exhausted bucket. | 5.3 | 85 | CWE-799 | `agentgw/gateway.go:290-328`, `:443-451` |
| **API-003** | Slack sends the workspace bot token to a **webhook-supplied URL** and follows its redirects. Discord has the identical shape and *was* hardened (`validDiscordAttachmentURL`, tagged H-001); Slack never received the sibling fix. | 6.1 | 80 | CWE-918, CWE-522 | `channels/slack/slack.go:554-559` |
| **CLI-001** | The console ships a CSP the SPA violates in **four** places under a comment asserting compliance, and Monaco loads from `cdn.jsdelivr.net` with no SRI. **The exploitation path is pressure, not a direct exploit:** whoever debugs "the editor never loads" will widen `script-src` to a CDN in an origin holding the console token. Also an audit blind spot — npm audits `0.55.1`, the CDN serves `0.52.2`. | 6.0 | 90 | CWE-1021, CWE-829, CWE-353 | `webui.go:1303-1325` vs `lib/monaco.ts:11-13` |
| **TS-004** | No runtime validation at the console API boundary anywhere in the SPA — ~150 views cast `res.json() as Promise<T>` with **no schema library in the project**. Agent-authored content reaches these responses; `Research.tsx` renders `report.claims!.map(...)` on LLM-derived fields. Realistic worst case: client-side denial of view. | 4.3 | 88 | CWE-20 | `frontend/src/lib/api.ts:118`, `:137`, `:145` |
| **TS-005** | `Toolforge` renders every forged tool with no window, no pager, and no server-side cap — and growth is **agent-driven** (`tool_forge` at L4 is one of the four persistence primitives). The only view of 31 flagged that combined agent-driven unbounded growth with no cap at either end. | 4.0 | 85 | CWE-400 | `views/Toolforge.tsx:352` · `controlplane/toolforge.go:37-52` |
| **PY-004** | Unbounded response reads let a malicious daemon exhaust the consumer's memory — `resp.read()` with no length, `response += chunk` with no bound **and** O(n²) concatenation, `_parse_sse` with no per-line cap. Timeouts bound latency, not volume. | 5.3 | 85 | CWE-400, CWE-789 | `sdk/python/agezt/client.py:288`, `:299` |
| **PY-005** | `quote()`'s default `safe="/"` lets a caller-supplied id escape its path segment — `message_id` values come off the shared inter-agent mailbox. Endpoint *confusion*, not escalation. **The Rust SDK gets this right** and has a test asserting it. | 4.8 | 80 | CWE-88, CWE-73 | `sdk/python/agezt/client.py:146`, `:204`, `:210` |
| **PY-006** | No scheme validation on `base_url` — `self.base_url = base_url.rstrip("/")` is the entirety of it. Plaintext `http://` silently accepted (**the enabler for PY-002**), and `file://`/`ftp://` reach `urllib`'s default opener, so `Client("file:///C:/Users/x/.agezt/", token)` reads the local filesystem. | 5.9 | 82 | CWE-319, CWE-295 | `sdk/python/agezt/client.py:101` |
| **RS-002** | The Rust SDK has **no TLS at all** — `https://` is a hard error, not a fallback. Every request writes `Authorization: Bearer <token>` over a raw `TcpStream`. **Explicitly NOT filed as a divergence** — docs and code agree in three places; filed because the residual belongs in the threat model, and because it is the delivery vector for RS-001. | 5.9 | 90 | CWE-319 | `sdk/rust/src/http.rs:25-37`, `:106-111` |
| **RS-003** | Unbounded response body read on the success **and** error paths; `BodyMode::Eof` grows until the peer closes — or forever. `set_read_timeout` bounds a single blocking read, not total volume. | 5.3 | 84 | CWE-400, CWE-789 | `sdk/rust/src/http.rs:73-77` |
| **INFRA-003** | Self-hosted runners are persistent, run as the **owner's own user**, and keep `.credentials_rsaparams` one directory above the workspace. Three runners share one WSL VM on the owner's daily-driver Windows host. *Downgraded: every item is an amplifier, not an exploitable primitive — the fork guard is present on all 15 self-hosted jobs, enumerated line by line.* | 6.0 | 85 | CWE-250, CWE-427 | `ops/wsl-runners/README.md:10-15` |
| **INFRA-004** | Dependabot PRs execute freshly-bumped third-party code on those runners — `knip`/`vitest`/`vite build` all **import** the bumped packages. `--ignore-scripts` closes only install hooks. `npx playwright install --with-deps` shells to `sudo apt-get`, implying **passwordless sudo** on the runner. | 5.8 | 85 | CWE-829 | `ci.yml:202-218`, `:304-310`, `:334-338` |
| **INFRA-005** | `install.sh` root-downloads and extracts the Go toolchain with **no checksum**, via a predictable `/tmp` path that `curl -o` will follow as a symlink. A backdoored compiler then builds the daemon. **The contrast inside this same repo proves it is not policy:** `ci.yml:499-508` downloads staticcheck *with its `.sha256` sidecar* and hard-fails on mismatch. | 6.4 | 92 | CWE-494, CWE-59 | `install.sh:117-124`, `:385` |
| **INFRA-006** | `install.sh expose ngrok` drops an apt key in `/etc/apt/trusted.gpg.d/` with an unrestricted `deb` line — trusted for **every** repository on the host. The correct `signed-by=` pattern is used twice elsewhere **in the same file**. | 5.5 | 90 | CWE-345 | `install.sh:403-404` |
| **INFRA-007** | The installers run `npm ci` **without** `--ignore-scripts`, as root/Administrator. Commit `3987bf7c` added the flag to all five CI invocations but **not to the installer, which is the strictly higher-privilege path** — on the operator's production host, at the moment the daemon binary is being built. | 6.5 | 90 | CWE-829 | `install.sh:176-182` · `install.ps1:182-188` |
| **INFRA-008** | `install.sh expose` publishes the **REST API** while the docs say "Web": `AGEZT_REST_ADDR` defaults to `127.0.0.1:8787`, which is *also* the console's default, so REST wins the bind and the console falls back to a random port. Every `expose` recipe tunnels 8787. `install.sh` never sets `AGEZT_WEB_PASSWORD`. | 6.1 | 88 | CWE-1188, CWE-1021 | `install.sh:40`, `:285`, `:349` |
| **INFRA-012** | The SDK publish path has **no environment gate, no review requirement, and no provenance** — `workflow_dispatch` from any ref, no `id-token: write`, no `npm publish --provenance`, no attestation. Combined with INFRA-001 this publishes a malicious SDK with no human in the loop and no CI gate having run. **Composes directly with DEP-002.** *(One thing right: publish jobs run on ephemeral hosted runners, so the tokens never touch the WSL VM.)* | 6.5 | 88 | CWE-353, CWE-284 | `publish-sdks.yml:18-24`, `:90` |
| **DEP-005** | Four npm-**deprecated**, abandoned MCP servers ship as one-click presets — and three are precisely the high-value credential sinks (`github` takes a PAT, `gdrive` takes OAuth creds, `postgres` takes a connection string with an embedded password). Confirmed against live registry metadata. | 5.8 | 100 | CWE-1104 | `views/Mcp.tsx:176`, `:179`, `:189`, `:203` |

---

## Low Findings (39)

| ID | Finding | Conf | CWE |
|---|---|---:|---|
| **INJ-003** | `Content-Disposition` quoted-string breakout in the File Manager raw download — the codebase's own `sanitizeFilename` sits 90 lines away and is not called. Response-splitting escalation actively refuted (Go rewrites `\r`/`\n` in header values). | 70 | CWE-113 |
| **AC-008** | Agent gateway has **no peer-credential check, no socket ACL** (`SO_REUSEADDR` only), an unauthenticated `/health`, and **no token revocation** — a leaked token is valid until `exp`. Low because reading the `0600` secret already implies same-user execution. | 85 | CWE-306, CWE-613 |
| **AC-009** | Login lockout **resets its own counter**, so each 5-min expiry grants a fresh 8 attempts indefinitely (~2,300 guesses/day) against SECRET-002's known default. Conversely the counter is daemon-**global**, so any client can lock the operator out. | 90 | CWE-307 |
| **AC-010** | Console token printed into the public tunnel URL under `AGEZT_WEB_PASSWORD_STRICT=on`. **Recorded with the disagreement intact — two hunters disagree** and no adversarial verdict exists. Both agree it does *not* fire on a default install. | **55** | CWE-532 |
| **EXPOSE-005** | Webhook secret accepted in a URL query string. **Three mitigations verified rather than assumed:** the daemon logs no request URLs anywhere, `webui.go:1025-1031` strips `secret` from the bus payload, and verification is constant-time and pre-auth rate-limited. | 85 | CWE-598 |
| **EXPOSE-006** | Second-tier redactors are built with `redact.New()`, which carries **no literals** — including the OpenAI-surface error redactor. `credSecrets` reads the vault only, so an env-only credential with no built-in pattern is never scrubbed (no rule for Twilio, SendGrid, Discord bot tokens, Gotify/Zulip). | 80 | CWE-532 |
| **EXPOSE-007** | File Manager returns raw OS error strings containing absolute host paths, to anyone holding a session — **including one opened with the default password**. | 82 | CWE-209 |
| **EXPOSE-008** | Agent-gateway audit writes around the bus, bypassing redaction. **Structurally real but currently carries nothing sensitive** — verified field by field. The hazard is the *next* caller populating `Error`, into an append-only log. This is the downgrade of recon DIVERGENCE 10(b). | 90 | CWE-532 |
| **SECRET-003** | Vault-backed secret file mounts land inside the agent's own workspace when `workDir` is non-empty. **Explicitly not confirmed** — the hunter did not trace `workDir` per caller. **Do not act on this without that trace.** | **60** | CWE-668 |
| **PATH-002** | `CachedPack` builds a marketplace cache path from two unvalidated names; the package has the validator (`nameRe`) and applies it on neighbouring paths. Low honestly: the console token independently grants strictly more read authority, and the bytes are unmarshalled into a `Pack`, making it a file-existence oracle. | 85 | CWE-22 |
| **REDIR-001** | LLM-supplied `href` rendered without the codebase's own `safeHref`. **Outer layers hold today** — `research.go:411` admits only `http(s)://` prefixes — and it is **one CSP relaxation away from being live** (CLI-001). | 80 | CWE-601, CWE-79 |
| **CLI-003** | The second browser-facing HTML surface (`127.0.0.1:1455`) sets **none** of the console's security headers and performs **no Host validation** — it does not use `httpserver.Router` or `webui.secure()`, so it inherits nothing, including DNS-rebinding defence. | 95 | CWE-1021, CWE-350 |
| **CLI-004** | PDF preview iframe omits `sandbox`, unlike its hardened sibling. **The hunter could not construct an exploit and records that plainly** — three independent controls block it. Filed because it becomes live the moment CLI-001 is "fixed" by adding `frame-src blob:`. | 85 | CWE-1021 |
| **API-004** | The router's declared `Method` and `Mutation` policy is recorded but **never enforced** — `mux.HandleFunc(pattern, …)` drops the method, and `opts.Mutation` has no non-test consumer. `/oauth/callback` is the single route where "mutating" and "POST" diverge, so **the one route the flag marks as needing protection is exactly the one the gate waves through.** Compensated everywhere else today. | 95 | CWE-650, CWE-352 |
| **JWT-001** | No minimum entropy on the gateway signing secret — a short one is silently SHA-256-stretched, which **adds no entropy**, only hides the weakness from a length check. `AGEZT_AGENTGW_TOKEN_SECRET=hunter2` yields a dictionary-recoverable HMAC key. | 88 | CWE-326 |
| **JWT-002** | Child tokens record the parent's **run id** as `ParentTokenID` while the library does it correctly, so token-level attribution is lost exactly where it matters — after a leak you cannot tell which minted token was used. | 95 | CWE-778 |
| **API-005** | `wecom` is the **only** channel using a non-constant-time signature comparison; `discord` has no replay dedup while `slack.go:93-97` documents why it is needed; eight listeners have no timestamp freshness window at all. | 90 | CWE-208, CWE-294 |
| **MASS-003** | `roster.Store.Update` omits `System` from its kernel-owned-field clamp. **Reachability actively refuted** — all six non-test mutators enumerated, none writes `System`. Recorded because **a single future mutator doing `*dst = in` turns this into a Critical.** | 95 / 0 | CWE-915 |
| **GO-002** | The WF-001 regression control **fails under `-race`**, so `go test -race ./kernel/controlplane/` is RED and the race gate for the whole package is unusable — and the broken signal is the WF-001 guard's. Test-fixture race, not production. | 97 | CWE-362 |
| **GO-003** | TOCTOU between the symlink guard and the read in the `file` tool's search walk. **The M427 guard is present and correct — the hunter's first hypothesis was refuted.** Go 1.24+ `os.Root` closes the window structurally. | 70 | CWE-367 |
| **GO-004** | Rollback restores an unmasked `os.FileMode`, so setuid/setgid/sticky are reachable from a corrupted or agent-influenced catalog entry. **Reachability unverified.** | 72 | CWE-732 |
| **TS-002** | SDK SSE parsers grow an unbounded buffer and rescan it **quadratically** (two `indexOf` scans from index 0 per read), and **the stream has no timeout at all** — the timer is armed on the fetch promise, which settles when headers arrive. Divergence #21. | 80 | CWE-400 |
| **TS-003** | SDK response bodies are type-asserted, never validated — including the double cast `ev.data as unknown as Mail`. Low because the counterparty is the consumer's own daemon. | 85 | CWE-20 |
| **TS-007** | The console bearer token is read from the URL and **left in the address bar** for the whole session, written to browser history — it survives screenshots and screen-shares. Verified: no call scrubs `?token=`. One `history.replaceState` block fixes it. | 95 | CWE-598 |
| **TS-008** | **No ESLint or any JS/TS static analysis** in the tree or in CI, across 109k LOC. Three rules would have *mechanically* caught findings in this very report: `no-unnecessary-type-assertion`, a ban on `as unknown as` (SDK-002's contributing cause), and `no-non-null-assertion` (TS-004). | 95 | CWE-1053 |
| **TS-009** | Dual lockfiles (npm + a three-days-staler pnpm), and a `dompurify` override for a package nothing imports. **Corrects the recon map:** it is an `overrides` entry (a transitive CVE floor), not a forgotten sanitizer. | 95 | CWE-1395 |
| **PY-007** | `arc.py`'s tar guard never inspects `linkname` and runs *before* extraction, so **its own protection is nil** — verified by replay. What stopped the escape was CPython 3.14.6's `tarfile` filter, which is permissive by default on 3.9–3.13. | 90 | CWE-22, CWE-59 |
| **PY-008** | Chunked-encoding detection matches the substring `chunked` **anywhere in the raw header block**, so any header echoing it flips the client into chunked decoding of a non-chunked body, then raises an uncaught `ValueError`. The Rust client parses properly. | 78 | CWE-444 |
| **RS-004** | Header value injection via an unvalidated tenant id and `base_url` host — same class as PY-001, but the injectable values are *configuration*, not per-call data. **Rust's path construction is correct by contrast** — every segment percent-encoded, `limit` is a `u32`. | 75 | CWE-93, CWE-113 |
| **RS-005** | Saturating `f64 as i64` cast silently corrupts out-of-range numbers into `i64::MAX`, feeding `Mail::ts_unix_ms` and `Health::model_count` with no signal. | 88 | CWE-681, CWE-197 |
| **RS-006** | Non-finite floats serialize to invalid JSON — `1e999` parses to `f64::INFINITY` and re-serializes as the bare token `inf`, breaking the crate's own round-trip test. | 85 | CWE-20 |
| **INFRA-009** | **Low today, latent Critical.** The update trust anchor cannot be set in a production build — `SetPublicKey` is defined **only** in `signature_test.go:22`. And the `ProvenanceGitHubRelease` exemption is unreachable *only* because `checkGitHub` never sets `SHA256`. **The moment someone fixes that without first embedding `DefaultPublicKeyHex`, the auto-updater applies any GitHub release asset with no signature check, then `os.Exit(0)`s for the watchdog.** Fix in the stated order. | 92 | CWE-1188, CWE-1059 |
| **INFRA-010** | Update payload written to disk **before** any verification, with no size bound — `io.Copy(f, resp.Body)`, no `LimitReader`. DoS, not code execution (never renamed, never `chmod +x`'d on the failure path). | 90 | CWE-400 |
| **INFRA-011** | **Gate-rot markers.** `ci.yml` justifies two guardrails by citing "the ciguard fork-guard lint" — `internal/ciguard` was deleted 2026-07-08 and `git grep ciguard` returns only those comments. **Nothing verifies that the fork guard, `persist-credentials: false`, or SHA pinning survive a future edit.** A live instance of the repo's own "test-only ≠ dead code" lesson. | 97 | CWE-1110 |
| **DEP-003** | Both `overrides` pins in `frontend/package.json` have gone stale and now resolve to the **exact top of a freshly published vulnerable range**. *Downgraded to Low on merged evidence:* `undici` is dev-tree only, and `monaco-editor` is not actually bundled. **The durable problem is the pattern** — a caret override stops protecting silently and nothing in CI notices. | 100 | CWE-79, CWE-444 |
| **DEP-006** | A seeded market pack launches `@modelcontextprotocol/server-fetch`, which **has never been published** (404). The UI catalog already gets it right. Low because the scope is upstream-controlled and cannot be squatted. | 100 | CWE-1104 |
| **DEP-008** | Production IMAP stack rests on `go-imap/v2@v2.0.0-beta.8`, pulling a pseudo-versioned `go-sasl` — all three parse attacker-influenced data. `govulncheck` clean; the concern is structural. | 85 | CWE-1104 |
| **DEP-009** | `DEPENDENCIES.md` has drifted from `go.mod` on two counts; `depscheck` enforces **names, not versions**, so drift is invisible to CI. **Governance controls that quietly drift stop being controls** — the same pattern as INFRA-011. | 100 | CWE-1059 |
| **DEP-010** | Floating toolchain and build-tool versions in otherwise fully pinned CI. The Go `stable` float is arguably *protective*; **`setuptools>=61` is the weaker one** — it sits in the release pipeline producing the wheel published to PyPI. Go 1.26.4-specific advisories **marked unverified** rather than asserted. | 100 | CWE-1104 |

---

## Informational (4)

| ID | Observation |
|---|---|
| **CRYPTO-002** | NIP-04 unauthenticated AES-CBC in the Nostr channel. **Protocol-mandated** — NIP-04 is the wire format and the file says so; adding a MAC breaks interoperability. The padding-oracle distinction is not observable to a remote party (decrypt failure drops the event). **Track as a migration item to NIP-44, not a fix in place.** |
| **CLI-005** | `sameOriginMutation` treats an absent `Origin` header as same-origin. Recorded only because comments present this as a boundary. There is **no browser-reachable CSRF here** — the `Sec-Fetch-Site` check and the `SameSite=Strict` cookie are each independently sufficient. **Do not "fix" this by rejecting an absent `Origin` without first checking the CLI test suite and non-browser API callers.** |
| **GO-005** | Inbound email TLS sets no explicit `MinVersion` — the only two `tls.Config` literals in the non-test tree. Certificate verification is **on** and Go's client default is TLS 1.2, so the effective posture is already acceptable; setting it explicitly just makes it immune to a future default change. |
| **GO-006** | `profileView` discards both JSON errors — a latent nil-map write. **Refuted as exploitable:** every field of `roster.Profile` is a type `encoding/json` cannot fail on. Reported because it is **one struct-field addition away** from a live control-plane DoS. |

---

## Remediation status of the superseded assessment

23 commits and 53 files (+8,270/−1,688) landed between `f815f56e` (2026-08-12) and `e0041337`, most
of them remediation for that assessment. This scan was run against the result, so its findings are a
direct measurement of what that remediation achieved.

### Confirmed closed

| Prior ID | Fix | Commit | This scan's evidence |
|---|---|---|---|
| **UPD-001** | Trust anchor selected by a manifest `Provenance` whose zero value is untrusted | `ca2366f0` | **Genuinely fixed.** Both handlers build the struct field-by-field from a typed args struct, leaving `Provenance` at its zero value, which `verifySignature` refuses. The zero value is correctly the untrusted one. *(Residuals are INFRA-009, a different defect.)* |
| **WF-001** | Panic firewalls, all four sites | `899ed24c`, `50ce6a58`, `0cdd3799` | **Fixed systematically, not symptomatically** — see *Verified safe*. The panic hunt came back **empty**: 63 recover-less goroutines and 38 unchecked type assertions all refuted. *(One blemish: GO-002, the test that guards it is race-red.)* |
| **PATH-001** | Symlink + Windows-junction resolution | `d009dc14` | **Real, not comment-only.** `EvalSymlinks` present on both paths; the per-component `os.Readlink` junction walk held every vector attacked, including link chains. |
| **SSRF-002/003** | `dialerGuard` made real in the SSE transport | `cb344a08` | **Real and complete** — the one-shot check is now *backed* by a dial-level guarded client and the drifted classifier copy was deleted in favour of delegating to `kernel/netguard`. |
| **GO-001** | Provider-OAuth status race | `9a943f82` | Fixed at the cited site. *(The twin is RACE-002 — see below.)* |
| **SEC-002** | `kdf_iter` ceiling | `09bdc83d` | Verified: floor 100,000 and ceiling 10,000,000 checked *before* derivation, against the envelope's own attacker-controlled value. |
| **RCE-001** | Credential bucket keyed off `EffectiveProfile` | `549edfa7` | Verified present at `shell.go:245` and `codeexec.go:263`; explicitly recorded as something any CE-006 fix must preserve. |
| **RCE-002** | Warden package header corrected | `57fcf129` | The retraction at `warden.go:17-38` is **accurate** and should be quoted in any threat model. *(The `EffectiveProfile` string itself is still wrong — CE-006.)* |
| **SR-001** | Stable self-repair fingerprints | `e6d45e0e` | **Refuted as still-broken:** fingerprints are constants or near-constants; both the cooldown and the attempt cap bind. |
| **AUTH-001** | Strict flag no longer cleared at boot | `ebb1e4df` | Correctly implemented today — `AGEZT_WEB_PASSWORD_STRICT` is applied only when actually set (`os.LookupEnv`). |
| **CICD-001** | `--ignore-scripts` on every CI `npm ci` | `3987bf7c` | Verified: all six invocations under `.github/`. *(But see INFRA-007.)* |
| **EXPOSE-003** | Templated webhook-URL redactor rules | `c3ee5f66` | Fixed in the redactor rather than the endpoint, so it applies before the journal write. |
| **PE-006** | `op=repair` refuses System guardians | `0cdd3799` | Verified, and its own test asserts the guard refuses **before** dispatch. |

### Still open

| Prior ID | Status |
|---|---|
| **CH-001** | Open — re-filed as **API-001** at High, now with **seven** sites rather than four. Was recorded as blocked on an owner decision (fail-closed breaks existing configs). That decision is still owed. |
| **PE-001 / PE-004** | Open — re-filed as **AC-001**, with the "no single-variable fix" claim now disproved by execution. Was recorded as blocked on an owner decision. |
| **SR-002** | Open — LLM free text still drives persisted fleet-wide routing config; now visible from a second direction as BIZ-001 (`guardian-routing` rewriting `AGEZT_TASK_MODEL_CHAINS` through the `config` tool). |
| **SUPPLY-001** | Open — re-filed as **CLI-001** + **DEP-004**, with the audit blind spot now measured (npm audits `0.55.1`; the CDN serves `0.52.2`; zero monaco chunks in the shipped bundle). |
| **SUPPLY-002** | Open — still no npm vulnerability scanning in CI; **DEP-003** is the consequence (both `overrides` pins went stale unnoticed). |
| **CICD-003 / CICD-004** | Open — re-filed as **INFRA-002** and **INFRA-012**. CICD-004 was resolved to latent-Low on the grounds that the registry secrets do not exist; that remains the right call and the `environment:` gate is still worth adding *before* they are created. |
| **SEC-003** | Fixed at the cited site (`bc82e4d7`) — but **not as a class**. See below. |
| **EXPOSE-001** | Fixed for the journal (`44be317a`) — but **not as a class**. See below. |
| **SDK-001** | Fixed at one call site per SDK (`03694cdf`) — but **not as a class**. See below. |

### The actionable lesson: **fixed the instance, not the class**

Three of the previous assessment's fixes are correct at the site they cite and were not generalised.
This scan found each of the three by looking for the sibling — which is the cheapest possible way to
find a bug, and the reason this pattern is worth naming.

| Prior fix | What it closed | What it left open | Now filed as |
|---|---|---|---|
| **`bc82e4d7`** — the AWS credential helper "was handed every secret we own", so `cmd.Env` is now scrubbed for `credential_process` | One subprocess sink | **The plugin host.** The only `plugin.Config{}` literal in the tree still leaves `Env` nil, so plugin children inherit `AGEZT_VAULT_PASSPHRASE`, `AGEZT_WEB_PASSWORD`, channel tokens and provider keys — transitively down to third-party MCP children spawned by `mcpbridge`. Every *other* subprocess sink scrubs. | **SECRET-001** |
| **`03694cdf`** — the SDK socket path "could not reach the daemon, and failed open"; a one-line resolver per SDK | `_SocketClient._connect` (Python) and `agent.ts:226` (TypeScript) | **The second connect site in each.** `git show 03694cdf` confirms it was one line per SDK and neither second site was touched. In TypeScript the second site reaches around the class's `private` fields with `as unknown as` — **and the bug is in the committed `dist/`, i.e. in what npm publishes.** No test in either suite constructs a client or asserts what any call site passes to `connect`. | **SDK-002** |
| **`44be317a`** — "the audit log was world-readable while the vault was not"; journal moved to `0600`/`0700` with a `chmod` migration | `kernel/journal` | **`kernel/jsonstore` (`0644`/`0755`) and both Config Center paths.** The reasoning recorded verbatim at `journal.go:69-79` applies identically. `jsonstore` backs sixteen stores — including `mcp/servers.json`, documented as holding plaintext `GITHUB_PERSONAL_ACCESS_TOKEN` and `Authorization: Bearer` headers. Verifier B notes the fix at `jsonstore.go:73`/`:54` is **broader and cheaper** than the MCP-specific framing suggests: one change covers all sixteen. | **EXPOSE-001** |

Two more of the same shape, from fixes that predate the previous assessment:

- **`9a943f82`** fixed the OAuth status race in `provider_oauth.go` and left the **byte-for-byte twin
  in `channel_oauth.go`** — eleven files away, still in the pre-fix shape, while the fix's own comment
  names the bug class. → **RACE-002**
- **Discord's `validDiscordAttachmentURL`** (tagged H-001) pinned scheme and CDN host before attaching
  a bot token. **Slack never received the sibling fix.** → **API-003**

**Every one of these five was found by asking "where else does this shape appear?" — not by a new
technique.** The remediation practice this report most recommends is not a tool: it is that each fix
ships with the grep that proves it is the only instance.

---

## What was verified safe

This is first-class output, not an appendix. Every item is something a hunter actively tried to break
and could not, with the evidence that makes it a conclusion rather than an absence of results. It is
what makes the seventeen Highs credible, and it tells the next assessment where **not** to spend
budget.

### Whole vulnerability classes with no surface here

| Class | Evidence |
|---|---|
| **SQL injection** | Tree-wide grep for `database/sql`, `gorm.io`, `jmoiron/sqlx`, `sql.Open`: **zero**. `go.mod` declares no driver. No query string, no builder, no DSN. Independently confirmed, not inherited from the prior report. |
| **NoSQL injection** | No MongoDB/Redis/CouchDB/DynamoDB/Elasticsearch client. The nearest analogue is a local JSON store with `validName`-constrained names. There is no query language to inject operators into. |
| **GraphQL** | Case-insensitive grep across **all** file types: zero occurrences in zero files. |
| **LDAP injection** | Same grep: zero. No DN or filter is built anywhere. |
| **XXE** | XML parsing at exactly four sites, **all `xml.Unmarshal`**. Go's `encoding/xml` never resolves external entities or fetches DTDs, and there are **zero** `xml.NewDecoder` sites, so there is no `Entity` map or `Strict` field to misconfigure. Neither classic XXE nor billion-laughs applies. |
| **SSTI** | No `text/template` or `html/template` anywhere in the Go tree. The only `{{…}}` syntax is the workflow interpolator, which is deliberately not an expression language. |
| **Insecure deserialization (CWE-502)** | **Genuinely absent.** No `encoding/gob`, no YAML unmarshal of any kind, no `archive/zip`. The only `json.Unmarshal` into a `map[string]any` outside tests reads one string field out of an event payload — no type dispatch, no object construction. Every other decode targets a concrete struct. |
| **CORS misconfiguration** | **Absent, not misconfigured.** Zero occurrences of `Access-Control-Allow-Origin`/`-Credentials` in the entire repository. No origin reflection, no `*`, no regex to bypass. |
| **WebSocket server flaws** | No `Upgrader`, no `CheckOrigin`, no upgrade handler. The sole websocket use is an **outbound client**. |
| **Server-side open redirect** | Every `http.Redirect` in the tree is in a `_test.go`. |
| **Docker / K8s / Terraform / Jenkins / GitLab CI** | Verified absent by `git ls-files` grep plus an untracked-file `find`. Exactly four tracked YAML files exist. |

### The Go panic hunt — the scan's most important negative result

`sc-lang-go` was commissioned to find a remaining unrecovered-panic DoS after the WF-001 work. **It
found none, and every candidate was refuted on inspection.**

- **63 goroutines lacking an inline `recover()`** — all traced. The channel listeners, the gateway,
  the kernel runtime and the HTTP listener are `srv.Serve` wrappers or `<-ctx.Done()` shutdown
  watchers; **`net/http` recovers handler panics per-connection**, so a malformed request yields a 500
  and a dropped connection, not process death. The genuinely bare dispatch paths are **already
  wrapped**: `safePoll`, `safeFire` ×2, `fireOne`, `controlplane/workflow.go:332`, two in `selfrepair`,
  per-tool in `run_tools.go`, and `main.go:2791`. **26 `recover()` sites, deliberately placed and
  commented.**
- **38 unchecked type assertions** — all refuted. They assert on maps constructed literally a few
  lines above from statically-typed struct fields; `controlplane/roster.go:272`/`:280` survive because
  `Slug string \`json:"slug"\`` has **no `omitempty`**, so the key is always present and always a
  string; and the remaining ~22 are in `cmd/agt`, where a panic kills a short-lived CLI process, not
  the daemon.

**The WF-001 work was done systematically rather than symptomatically** — the codebase has a named,
documented, mirrored containment pattern across `pulse`, `standing`, `cadence`, `workflow`,
`selfrepair` and the control plane, plus per-connection recovery in `net/http`. This contradicts the
pessimistic prior and is worth stating explicitly.

### Egress — netguard held against 14 techniques

The design is right: the check is a `net.Dialer.Control` hook on a fresh, non-shared transport, so it
sees the concrete resolved `IP:port` on **every** dial including each redirect hop.

| Technique | Result |
|---|---|
| Decimal / octal / hex IP encodings | **Blocked** — irrelevant to the design; `Control` receives the *resolved literal* |
| DNS rebinding | **Blocked** — `Control` runs per dial, after resolution |
| 302/307 redirect chain to internal | **Blocked** — fresh non-shared Transport ⇒ every hop re-dials through `Control` |
| IPv4-mapped `::ffff:127.0.0.1` | **Blocked** |
| **NAT64 `64:ff9b::a9fe:a9fe`** | **Blocked** — `embeddedV4` |
| IPv4-compatible `::a9fe:a9fe` | **Blocked** |
| `0.0.0.0` and all of `0.0.0.0/8` | **Blocked** — `isZeroBlock` covers more than `IsUnspecified` |
| CGNAT `100.64.0.0/10` | **Blocked** — also catches Alibaba metadata `100.100.100.200` |
| Link-local `169.254.169.254` | **Blocked, with no opt-in at all** — `AllowPrivate` deliberately does not unblock it |
| Broadcast / multicast | **Blocked** |
| `metadata.google.internal` / `.local` mDNS | **Blocked** — the name is never the check subject |
| **Proxy env (`HTTP_PROXY`/`ALL_PROXY`)** | **No bypass** — `HTTPClient` builds a Transport with `Proxy` left **nil**; a manually-constructed Transport does not inherit `ProxyFromEnvironment`. Had it, the dial would target the proxy and `Control` would validate the wrong IP. |
| `file://`, `gopher://`, `unix://` redirect targets | **Blocked** — `http.Transport` registers only http/https |
| Unparseable / non-literal dial address | **Fail-closed** |

Residual gaps, both negligible and **not filed**: IPv6 6to4 and deprecated site-local are not
collapsed by `embeddedV4`; neither routes to a local target in practice. Note the ~45 clients that
bypass netguard entirely (§5.8 of `architecture.md`) are a separate, real concern — but every one
traced by the SSRF hunter resolved to either an operator-configured destination or a finding above.

### Command and argument injection

- **`ShellQuote` is sound — the hunter could not break it.** Tracing `a'; id; '` yields
  `'a'"'"'; id; '"'"''`, which the shell reassembles as the single literal word. Backslashes,
  newlines and `$()` are inert inside single quotes; a NUL byte is rejected by `os/exec` before
  reaching a shell.
- **Remote-exec command strings are safe.** SSH quotes explicitly; K8s, Daytona and Modal pass
  `command` as a **discrete argv element**, never concatenated. *(Connection parameters are CE-007.)*
- **Remote-exec config is not request-reachable.** Every override is called from exactly one non-test
  site, each building from `…ConfigFromEnv()`. The request body chooses only the profile *name* from
  a fixed set — so the classic `-oProxyCommand=` injection is **not reachable from any HTTP surface**.
- **The `coding` tool is well designed** — the LLM-authored task goes into an *environment variable*,
  never into the command string.
- **`acp_agent`'s slug allowlist works** — a non-empty selector must name an installed catalog slug;
  an unknown ref returns `ok=false` rather than falling through to a raw command.
- **The `mcp` tool's child spawn is not command injection** — `exec.Command(command, args...)`, argv
  form, no shell. It is arbitrary *program* execution (an authorization finding), recorded here so it
  is not double-counted.
- **`fixupWindowsCmd` is safe as used** but is a latent trap: every `cmd /C` caller was enumerated and
  all three pass a single command element. Worth an assertion that `len(Args) == 3`.

### Path traversal, archive extraction, upload

The File Manager chokepoint applies four layers in order and **held every vector attacked**: POSIX
`../`, backslash `..\` (blocked twice), absolute paths, drive-relative `C:foo`, UNC, NUL byte, POSIX
symlink escape, **Windows directory junction** (the only thing that works, since `EvalSymlinks`
returns a junction unchanged and `os.Lstat` reports `ModeIrregular` — this is the owner's platform and
a prime regression-test target), link **chains**, not-yet-existing targets, and prefix confusion
(`/var/foo` vs `/var/foobar`, compared against `root+PathSeparator`, never bare `HasPrefix`).

**No zip-slip surface exists** (no `archive/zip` anywhere) and all three tar loops are guarded.
`codeexec/artifacts.go` **could not be escaped**: symlinks and hardlinks are dropped entirely, the
destination is a fresh `MkdirTemp`, zip-bomb caps on count/size/total, and `io.CopyN(f, tr, hdr.Size)`
rather than trusting the stream.

**Upload: nothing filed.** `MaxBytesReader` is applied **before** `ParseMultipartForm` (the correct
order), the client filename never builds a path, and the SPA is `go:embed`-ed and served read-only
from `embed.FS`, so **no runtime write can reach a web-served directory**.

### CSRF, DNS rebinding, clickjacking — three layers each

**CSRF.** The write surface is the sharpest possible shape — `postAction` issues a *simple* POST with
no body and no `Content-Type`, reproducible cross-origin with a plain `<form>` and no preflight. It
still fails three ways: (1) **auth does not ride** — the token travels as `Authorization: Bearer`,
which browsers never attach cross-origin, and the session cookie is `SameSite=Strict`;
(2) `Sec-Fetch-Site: cross-site` → 403; (3) `Origin` host:port vs `r.Host` mismatch → 403, which also
closes the same-site/different-port case. All three run in `secure()` **before** the router, so they
cover public routes and 401s.

**DNS rebinding.** `hostAllowed` consults an **exact-match map** for DNS names, populated only by
explicit `SetAllowedHosts` calls. `evil-127.0.0.1.attacker.com` does not satisfy a map lookup. The
IP-literal passthrough is not a hole: a rebound name stays a name in the `Host` header.

**Clickjacking.** `X-Frame-Options: DENY` **and** CSP `frame-ancestors 'none'`, set before the auth
check so even 401s carry them, pinned by `webui_test.go:1364`. The only gap is the separate `:1455`
listener (CLI-003).

### Client surface — zero XSS, and it was hunted

- **No raw-HTML render path exists anywhere in the SPA.** A tree-wide search for
  `dangerouslySetInnerHTML`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`,
  `new Function`, `eval(`, string-first-argument `setTimeout`/`setInterval`, ref-based DOM injection
  and `createElement("script")` returns **two hits, both test assertions**.
- **Agent output renders through React text nodes only** — `Markdown.tsx` is a hand-rolled AST
  renderer whose every leaf is `{tok.v}` inside JSX, with markdown `href` scheme-allowlisted at
  construction and regression-tested.
- **Stored XSS via agent-authored files is closed at the server** — `/api/files/raw` serves **every**
  file as `application/octet-stream` with `nosniff`, which also forecloses the `script-src 'self'`
  bypass of loading a same-origin agent-authored `.js`.
- **No secrets ship to the browser** — no `import.meta.env`, no `VITE_*`, no `process.env` in
  `frontend/src`; `sourcemap: false`; **zero `.map` files** in the committed bundle.
- **No `postMessage` handlers.** **`Math.random` never used for a security value.** **`strict: true`
  in both tsconfigs, `tsc --noEmit` clean, and not a single `@ts-ignore`/`@ts-expect-error`/`@ts-nocheck`
  in the entire repository.**
- **Prototype pollution — 3 candidates, all refuted.** No `deepMerge`, no `__proto__` access, no
  `JSON.parse` reviver, no `structuredClone`.

### Authentication, authorization, crypto

- **Constant-time comparison on every credential path, with `==` on a secret nowhere in the tree** —
  verified twice, by two hunters: **all 24 credential/MAC comparisons** use
  `subtle.ConstantTimeCompare` or `hmac.Equal`, including every internet-facing channel verifier.
  (`wecom` is the single documented exception, API-005, and it is a *signature* compare where forgery
  still requires the AES key.)
- **Fail-closed authentication** — invalid tier → false, blank credential → false, `TierAdmin` **can
  never be opened by a tenant credential**.
- **Session fixation is not possible** — no session exists before authentication; the id is minted
  only after a successful constant-time compare.
- **JWT algorithm confusion is blocked** — `alg == "HS256" && typ == "JWT"` pinned **before** touching
  the signature, then `hmac.Equal`; `iss`/`aud` pinned. `alg:none` and an asymmetric swap both
  rejected. The zero-expiry branch is unreachable.
- **Gateway child-token minting is correct** — caps are *rejected* (not silently dropped) when they
  exceed the parent; expiry never outlives the parent; rate and burst clamp down only; `RunID` is
  inherited so a child cannot mint into another run. Re-verified independently while assessing PY-001.
- **`overseer op=edit`/`create`/`clone`/`delete` are properly guarded** — AC-002 is the remaining
  sibling.
- **Mass-assignment sweep across every wire-decoded domain struct came back clean** — `/api/agents/add`
  forces `System = false`, `/api/agents/edit` uses a 24-field allowlist, `/api/toolforge/draft` forces
  `Status = StatusDraft; TestedOK = false`, and registered settings sections **cannot shadow
  built-ins**.
- **Per-agent `config_overrides` cannot smuggle arbitrary settings.** This was *expected* to be
  exploitable — the LLM's final-text JSON block is copied wholesale and the brief literally tells the
  model *"That block will be applied automatically"*. But **application is allowlisted** to a fixed
  nine-knob table. An entry like `AGEZT_ALLOW_ALL` is stored and never read. The table's own comment
  records that it was built precisely to stop this drift. Clean design.
- **`gitleaks detect` over all 1,693 commits (414 MB) returned exactly one hit**, and it is a
  PEM-shaped string inside a previously committed file under `security-report/` — a prior
  assessment's own content, not application code.
- **The vault KDF is genuine PBKDF2-HMAC-SHA256** — the exact RFC 8018 construction, cross-verified
  **live against stdlib `crypto/pbkdf2`** across six cases. *This refutes recon DIVERGENCE 11.*
- **Vault encryption is correct** — AES-256-GCM, fresh 32-byte salt and 12-byte nonce per save from
  `crypto/rand`, no nonce reuse, and decrypt validates cipher id, KDF id, an iteration floor **and
  ceiling** against the envelope's own attacker-controllable value, checking nonce length **before**
  `gcm.Open` to avoid Go's panic.
- **Zero `InsecureSkipVerify` in the entire tree. No weak PRNG in a security role. No MD5, DES, 3DES,
  RC4, or ECB mode anywhere.** SHA-1 appears four times, all protocol-mandated or non-security.
- **All three SDKs transmit the token as an `Authorization: Bearer` header only** — never a query
  string, never logged, never written to disk, never interpolated into an exception message.

### Rust and Python SDK positives

- **`sdk/rust` has no `unsafe` at all** — `#![forbid(unsafe_code)]` is compiler-enforced crate-wide.
  This closes seven checklist categories by construction.
- **The "zero dependencies" claim is TRUE and was verified three ways.** No `build.rs`, no proc macro.
  **And the crate does not hand-roll TLS or crypto — it has none at all**, which is the right call
  versus a homegrown TLS stack.
- **No panic-capable code outside `#[cfg(test)]`.** (RS-001 is a *stack overflow*, which is not a
  panic.)
- **Python SDK clean list** — no `eval`/`exec`/`compile`; no `pickle`/`marshal`/`shelve`; no YAML; no
  `shell=True`/`os.system`/`os.popen`; no `verify=False`; no insecure randomness; no `tempfile.mktemp`;
  no XXE; no secret comparison (so no timing surface); **explicit timeouts on every network call**.
- **A cross-SDK pattern worth acting on:** on the two questions the SDKs answer differently — URL
  segment encoding and scheme validation — **Rust is right and Python is wrong** (PY-005, PY-006); on
  the two where Python's stdlib does the work, Rust had to hand-roll it and picked up the bugs
  (RS-001, RS-003). **Cross-porting each SDK's correct answer to the other closes four of the eleven
  SDK findings.**

### CI/CD and supply chain — what is right

- **No expression injection anywhere.** The complete inventory of `${{ }}` across `.github/` is **12**,
  and **not one `github.event.*` value reaches a `run:` block.**
- **28/28 external action uses are full-SHA-pinned**, each with a version comment; zero tag- or
  branch-pinned actions anywhere.
- **`persist-credentials: false` on all 17 `actions/checkout` steps.**
- **No `pull_request_target`, `workflow_run`, `issue_comment` or `schedule` trigger** — the entire
  class of "untrusted input with secrets" triggers is absent.
- **Fork PRs cannot reach the self-hosted runners** — the guard is on **all 16 jobs** in the correct
  form, every line enumerated by the adversarial verifier.
- **No `@latest` anywhere in CI**; staticcheck is downloaded *with its publisher `.sha256` sidecar*.
- **No `continue-on-error`, `|| true`, or warn-only step masks any gate.**
- **`go vet ./...` and `staticcheck ./...` are both completely clean** across the whole module — exit
  0, zero diagnostics. **Wire both into CI as required checks while they are green.**
- **`install.sh`'s systemd hardening is thoughtful** — a dedicated `agezt` system user with
  `/usr/sbin/nologin`, `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=full`, `ProtectHome`,
  `ReadWritePaths` scoped to `$AGEZT_HOME`, `LockPersonality`, plus a check that **refuses** an
  `AGEZT_HOME` under `/home` or `/root` because it would be unreachable behind `ProtectHome`.
- **Go module integrity** — `go mod verify` all verified; `govulncheck ./...` no vulnerabilities; no
  `replace` directives; `GOSUMDB` active with no bypass.
- **Licenses — no conflicts.** All 309 frontend packages declare a license; **no GPL or AGPL
  anywhere.** **Typosquat sweep clean.** **No alternate-registry configuration** anywhere.
- **Five CI gates were executed locally and all five are green.**

---

## Corrections to the record

Being explicit about self-correction is part of this report's value. This scan refuted specific claims
from its own Phase-1 reconnaissance and from the previous assessment. Each is recorded so nobody
re-derives or re-files it.

### Refuted from this scan's own recon

| Recon claim | Verdict |
|---|---|
| **DIVERGENCE 11 — "the vault KDF is a custom keyed-HMAC chain with XOR accumulation, not RFC 2898"** | **REFUTED by execution.** `deriveKeyPBKDF2` is genuine PBKDF2-HMAC-SHA256, cross-verified **live against stdlib `crypto/pbkdf2`** across six cases including empty passphrase, empty salt and unicode; both known-answer tests were run and PASS. The recon had read the *legacy* `deriveKeyLegacyHMAC` — decrypt-only, pinned by golden vectors from an independent reimplementation — as if it were the current KDF. **The mislabel claim was itself mislabelled.** |
| **"Update signature verification is inert — a top-severity finding"** | **REFUTED and restated as INFRA-009 (Low today, latent Critical).** `DefaultPublicKeyHex` *is* empty, but **every operator-reachable `Apply` path fails closed**: `verifySignature` with a nil key refuses unless `Provenance == ProvenanceGitHubRelease`, and all three operator-reachable callers build `UpdateInfo` by hand, leaving `Provenance` at its untrusted zero value. The only caller preserving it never sets `SHA256`, and `Apply` validates the checksum *before* the signature. **Both branches abort.** The real defects are narrower and sharper: **`SetPublicKey` exists only in `signature_test.go:22`**, so an operator who wants signed updates must produce a custom `-ldflags` build; and the `ProvenanceGitHubRelease` exemption is a **latent Critical** that goes live the moment someone fixes `checkGitHub` without first embedding the key. |
| **"Cadence-fired scheduled runs are uncapped"** | **Partially refuted.** Literally true of `WithTrustCeiling`, but `WithAgentProfile` applies the profile's own ceiling, so a scheduled *guardian* **is** clamped to its declared L2. The residual — non-System agents have an empty ceiling, profile-less runs get none — is BIZ-002. |
| **"`tunnelPublicURL` prints the console token into the public URL on a default install"** | **Refuted.** `web.passwordStrict` is captured *before* `buildTunnel` runs, so the default-password case takes the `return raw` branch. The residual under an explicit `STRICT=on` is AC-010 — where **two hunters still disagree about whether even that is a leak**, and that disagreement is recorded rather than resolved. |
| **"`dompurify` is an unused sanitizer someone forgot to wire up"** | **Refuted.** It is an **`overrides`** entry — a transitive-dependency CVE floor, the standard shape — not a `dependencies` entry, and it is correctly unreferenced because the app has no raw-HTML path. Recorded as TS-009's interpretive nit only. |
| **"`cmd/agt/token.go` mints uncapped tokens (no expiry, no capability scope)"** | **Partly false.** Capabilities **are** validated against a closed 17-entry allowlist and an empty result is rejected; expiry defaults to 1 h and non-positive values are rejected. What is true is narrower: the CLI mints a **root** token with no parent to intersect against, so anyone who can read the `0600` `agentgw.secret` mints the maximum grant — the same trust boundary as the secret file itself. The reachable weakness on that surface is JWT-001. |
| **"63 goroutines without `recover()`, 38 unchecked type assertions"** | **All refuted on inspection** — see *Verified safe*. This negative result is the single most important output of the Go scan. |
| **"`gofmt -l` reports ~500 unformatted files"** | **Refuted.** A working-tree artefact of `core.autocrlf=true`. Extracting `HEAD` blobs to a temp dir and re-running returns **empty**. Killed independently by two hunters. |

### Sub-claims removed from findings that otherwise survive

Seven. The four that matter most:

1. **AC-001's *"No single-variable configuration fixes this"*** — **false, disproved by execution.**
   `AGEZT_APPROVAL_MODE=deny` alone makes the ceiling bind. Its real cost is why it is unusable, not
   why it fails.
2. **AC-003's headline mechanism, *"`op=wake` drops prompt-injection taint"*** —
   **REFUTED-AS-WRITTEN.** The taint is never in a tool's `Invoke` context: it lives on a separate
   `policyCtx` that is discarded after the verdict, while the tool is invoked from the outer `ctx`.
   `context.Background()` discards nothing of the kind — **and the hunter's proposed fix was therefore
   also wrong and has been removed.** The surviving issues are filed at Medium. *(That concern is
   real; it lives at AC-011.)*
3. **INJ-002's *"shipped-template proof"*** — **false.** That workflow's trigger node is
   `{"kind":"manual"}`, not a webhook. The operator must wire the trigger themselves. The finding's own
   body always said this correctly; only the orchestrator note overreached.
4. **PY-001's *"converts a read capability into arbitrary gateway authority"*** — **false.** Fourteen
   enumerated handler sites enforce a per-capability membership test, and `handleTokenCreate` rejects
   rather than drops. Impact rewritten as a confinement bypass; severity Critical → High.

### Corrections to the previous assessment

- **CICD-007 was already recorded as a fabricated citation** by the previous report's own follow-up
  (`dd3cce9f`) — `.github/scripts/ci-go-retry.sh` never existed. This scan independently found the
  real script at `scripts/ci-go-retry.sh:35` and it *does* contain the cross-runner glob, cited
  correctly under INFRA-003 as **deliberate cross-runner cleanup by design**, not a collision.
- **The previous report's warning that its Medium tier was unverified was correct and is now
  measurable.** This scan hand-verified or executed against every tier. Two of that report's four
  examined Mediums did not survive contact with the source. **This report's Medium and Low tiers carry
  per-finding confidence scores and explicit "unverified" markers** (GO-004's reachability, SECRET-003's
  `workDir` trace, DEP-010's Go 1.26.4 advisories, CLI-001's runtime symptoms, EXPOSE-001's base-dir
  mode) precisely so the same warning does not need restating as a blanket caveat.
- **Commit count.** The task brief cited 31 commits between the two assessments; `git rev-list --count
  f815f56e..e0041337` returns **23** (53 files, +8,270/−1,688). Stated as measured.

---

## Remediation Roadmap

Ordered by (impact × ease), not by severity. Phase 1 is deliberately made of one-line and
one-conditional changes plus one administrative action, so it can ship today.

### Phase 1 — Immediate (1–3 days): one conditional, one constant, one registry claim

| # | Item | Change | Effort | Impact |
|---|---|---|---|---|
| 1 | **DEP-002** — claim the three SDK names | Publish placeholder `0.0.0` to PyPI (`agezt`), crates.io (`agezt`), npm (`@agezt` org + `@agezt/sdk`). **Re-check the 404s first, then act.** Soften the four README install lines until real releases exist. | **Minutes** | **The only open window in the report.** `README.md:217` already tells users to `pip install agezt` |
| 2 | **AC-011** — the auto-approve / injection-guard conditional | Gate `kernel/runtime/policy.go:187` on `out.RequiresApproval` specifically, or move the auto-approve check **above** the guards at `:152-180`. One conditional. | **Minutes** | Restores an opt-in hardening the operator explicitly enabled; closes the report's clearest "restriction that fails to restrict" |
| 3 | **SECRET-002** — `defaultLoopbackWebPassword` | Delete the constant; mint a random per-install password at first boot, print once in the boot banner, persist `0600` — the shape `kernel/auth/tokenfile.go:20-38` already uses. Interim: force strict mode whenever the built-in default is in effect. | **Low** | Removes a publicly-known credential from a default-on control plane |
| 4 | **API-001** — the seven fail-open listeners | `if secret == "" \|\| sig == "" { return false }` in each `verify`, matching `webhook/webhook.go:260-263`. Keep the factory guard. Add a table test across all 15 channels. | **Low** (7 edits) | Closes unauthenticated, unthrottled, billable agent execution on internet-facing listeners. **Posture change — owner decision owed** (breaks configs currently running unverified) |
| 5 | **BIZ-001** — `AGEZT_PRICING_STRICT` | Default it on; or charge an unpriced model a conservative non-zero fallback so it still consumes ledger headroom, and journal `budget.unpriced` on every such call. | **One constant** | Restores the global daily, per-task and per-agent spend ceilings simultaneously |
| 6 | **INFRA-001** — enable branch protection | `gh api` a ruleset on `main` requiring the CI jobs as status checks and Code Owner review. Administrative, no code. | **Minutes** | **Without this, nothing below is verified by anything but a human reading a diff** |

Also in this phase because each is genuinely one line: **PY-002** (`add_unredirected_header`),
**TS-007** (`searchParams.delete("token")` + `replaceState`), **AC-007** (`strings.HasPrefix`),
**MASS-003** (add `System` to the restore line), **JWT-002** (one identifier), **DEP-006**
(`uvx mcp-server-fetch`), **INJ-003** (call the existing `sanitizeFilename`), **REDIR-001**
(`href={safeHref(s.url)}`), **CLI-004** (add `sandbox=""`).

### Phase 2 — Short-Term (1–2 weeks): the remaining Highs and the structural fix

| # | Item | Change | Effort | Impact |
|---|---|---|---|---|
| 1 | **AC-004** — the `config` pivot | (a) `AgentWritable bool` on `settings.Field`, default false, honoured by `config.doSet` and ignored by console/CLI. (b) Restrict `injectConfig` to declared schema names. | Medium | **Two changes close all four vectors, plus CE-001's reachable path and CE-007's live-set path** |
| 2 | **INFRA-001 (2)** — make the gate run | Restore runner capacity or move leaf jobs to hosted runners so the queue drains; `cancel-in-progress: false` for push on `main`. | Medium | The gate must *complete*, not merely be *required* |
| 3 | **SDK-002** — both SDKs, both call sites | Route Python's `_subscribe` through the helpers; apply the resolver at `agent.ts:403` **and rebuild `dist/`**; remove the `as unknown as` escape hatch. **Add a call-site assertion, not a helper test.** | Low | Closes a live capability-token leak in what npm publishes |
| 4 | **AC-002** — pause/retire | Apply the `op=repair` pattern verbatim at the `kernelSource` layer: `System` refusal + `fleetLock`, on all four setters. Mirror `TestRepairAgent_RefusesSystemGuardian`. | Low | Stops permanent defanging of the self-healing fleet |
| 5 | **AC-001** — make an applied ceiling bind | Resolve an Ask-class result under an explicit ceiling with a ceiling-specific policy rather than the global `AskPolicy`. Add `TestPolicyHook_TrustCeilingL2_UnderDefaultAskPolicy`. | Medium | Makes trust ceilings mean something — **and turns BIZ-002 from theoretical into load-bearing, so ship them together** |
| 6 | **CE-001** — the passthrough knob | `ReadOnly: true` on `AGEZT_EXEC_SECRET_ENV_*` / `_FILES_*` (subsumed by item 1). Correct `codeexec.go:138` and `runtimes.go:117-119`. Journal the passthrough list in force. | Low | Removes the sandbox's disarm switch from the sandbox's own trust tier |
| 7 | **PY-001** — CRLF at the transport boundary | Percent-encode every interpolated segment; reject `\r`/`\n` in path, header keys and values before `sendall`. | Low | Closes request smuggling in the agent-subprocess transport |
| 8 | **RS-001** — depth budget | `MAX_DEPTH = 128` threaded through `parse_value`. Test: `Value::parse(&"[".repeat(10_000)).is_err()`. | Low | Removes an unrecoverable process abort on every response |
| 9 | **INJ-002** — structural JSON construction | Decode `c.Args`, interpolate string **leaves**, re-marshal — the shape `NodeHTTP` already uses 80 lines away. | Medium | Closes argument/JSON injection into workflow tool nodes |
| 10 | **SSRF-001** — containment at the connection | `--host-resolver-rules="MAP <host> <validated-ip>"`, or a local netguard-dialed proxy. Make `AllowAll` opt-in rather than the empty-allowlist default. | Medium | Closes rebinding and 30x to cloud metadata with page content returned |
| 11 | **INFRA-002 + 003 + 004** — **triage as one item** | Split `frontend-dist-rebuild` into an unprivileged build job and a minimal commit job; move to ephemeral runners; drop `--with-deps`. | Medium | Removes a write token from a job that executes third-party code on a persistent runner |
| 12 | **DEP-001 + DEP-005** — pin and prune the presets | Pin every catalog preset to an exact version; drop `@latest`; replace or remove the four npm-deprecated servers (three of which are credential sinks). | Medium | The largest ungoverned executable-code surface in the product |

### Phase 3 — Medium-Term (1–2 months): the Mediums, and the class-level sweeps

Ordered so that each item closes a *class*, not an instance:

1. **The `0644`/`0755` sweep (EXPOSE-001).** `jsonstore.go:73` → `0o600`, `:54` → `0o700` covers
   **sixteen stores** in one change. Then Config Center entries and audit. Then create the base dir
   once, explicitly, at `0o700` before any subsystem touches it, and normalise all ten `MkdirAll` call
   sites onto one constant so the mode stops depending on boot ordering.
2. **The subprocess-environment sweep (SECRET-001).** `Env: envscrub.Scrubbed()` at
   `plugins.go:95` and `cmd.Env` at `stdio_transport.go:38`, plus **a guard test asserting no
   `plugin.Config` literal leaves `Env` nil** — or correct `PLUGIN-SECURITY.md:279-280`. Consider
   consolidating the four near-duplicate allowlists in `plugins/tools/shell/env.go` while you are
   there.
3. **The remaining unfixed siblings.** RACE-002 (three lines, the fix exists eleven files away),
   API-003 (port `validDiscordAttachmentURL` to Slack), MASS-001 (load-then-mutate, the pattern is
   eleven lines away).
4. **The sandbox honesty pass.** CE-006 (rename the Linux tier or return `ProfileNone` with a
   `warden.profile_downgraded` event — **preserving the `EffectiveProfile` credential keying that
   `549edfa7` added**), CE-005 (`--read-only --cap-drop=ALL --security-opt=no-new-privileges
   --pids-limit=512 --user 65534:65534`), CE-002 and CE-003 doc corrections plus
   `Store.Add` defaulting `Enabled = false` for agent-originated registrations.
5. **The installer pass.** INFRA-005 (checksum the Go download, `mktemp` the path), INFRA-006
   (`signed-by=`, matching the cloudflared block in the same file), INFRA-007 (`--ignore-scripts`,
   drop the `npm install` fallback), INFRA-008 (give the console its own port and force
   `AGEZT_WEB_PASSWORD` at install time).
6. **INFRA-012 before the tokens exist.** Protected `environment:` on each publish job, drop
   `workflow_dispatch` or gate it on a tag, add `id-token: write` + `npm publish --provenance`.
   Cheaper now than after DEP-002's names are claimed.
7. **The SDK cross-port.** PY-005 and PY-006 take Rust's answers; RS-001 and RS-003 take Python's.
   **Four of the eleven SDK findings close from one insight.**
8. Remaining Mediums: AC-003, AC-006, BIZ-002 (with AC-001), BIZ-003, CE-007, EXPOSE-002, EXPOSE-003,
   INJ-001, MASS-002, RACE-001, RATE-001, API-002, CLI-001 (**self-host Monaco; do not widen the
   CSP**), TS-004, TS-005, PY-004, RS-002, RS-003.

### Phase 4 — Hardening (ongoing): make the classes un-recurrable

The 23-finding divergence class and the five "fixed the instance, not the class" cases are both
process defects. Tooling closes them; another documentation pass does not.

| # | Recommendation | Effort | Impact |
|---|---|---|---|
| 1 | **A guard test per load-bearing claim.** INFRA-011's remediation is the template: reinstate the workflow lint as a `_test.go` in a package the build compiles, asserting the fork guard, SHA pinning and `persist-credentials: false` on every job. Extend the idea: a test that fails when a `plugin.Config` literal omits `Env`; a test asserting the CSP admits every origin the bundle requests; a test asserting empty-secret → reject across all 15 channels. | Medium | **The single highest-leverage change in this report.** It converts the dominant pattern from "recurs every cycle" to "cannot merge" |
| 2 | **Every fix ships with the grep that proves it is the only instance.** Five findings here exist purely because a correct fix was not generalised. | Free | Retires the "fixed the instance, not the class" pattern |
| 3 | **Wire `go vet` and `staticcheck` in as required checks while they are green** (both currently exit 0 with zero diagnostics), plus `npm audit --audit-level=high` in `frontend-test` so a stale override (DEP-003) fails loudly. | Low | Locks in a currently-clean state |
| 4 | **TS-008 — add `typescript-eslint`** with `no-unnecessary-type-assertion`, a `no-restricted-syntax` ban on `as unknown as`, and `no-non-null-assertion`. Those three rules would have **mechanically** caught SDK-002's contributing cause and TS-004's `!` sites. 109k LOC currently has zero rule enforcement. | Low | Mechanical prevention of two findings in this report |
| 5 | **Fix GO-002 so `go test -race ./kernel/controlplane/` is green** — today it is RED, and the broken signal is the WF-001 guard's. Two `atomic.Int64`s. | Minutes | Restores the race gate for the package that most needs it |
| 6 | **Extend `depscheck` to diff versions, not just names** (DEP-009); pin `setuptools` exactly (DEP-010); add a `toolchain` directive to `go.mod`. | Low | Governance controls that quietly drift stop being controls |
| 7 | **Adopt `os.Root`** (Go 1.24+; this repo builds on 1.26.5) for the workspace, making containment kernel-enforced and closing GO-003's TOCTOU structurally. | Medium | Removes a whole race class rather than narrowing it |
| 8 | **Add a peer-credential check** (`SO_PEERCRED` / `LOCAL_PEERCRED`) to the agent gateway and a revocation list keyed on `RunID` (AC-008), and an entropy floor on the signing secret (JWT-001). | Medium | Hardens a surface that starts unconditionally with no enable flag |
| 9 | **Route the `:1455` OAuth listener through the same header + Host middleware as the console** (CLI-003). It currently inherits nothing, including DNS-rebinding defence. | Low | Removes the console's only unhardened browser-facing twin |
| 10 | **Delete the `ProvenanceGitHubRelease` exemption** (INFRA-009) **before** anyone fixes `checkGitHub`'s missing `SHA256`, and ship a real `SetPublicKey` or remove the claim from `update.go:376` and `:357`. Order matters: reversing it creates a Critical. | Low | Defuses the report's one latent Critical |
| 11 | **Migrate Nostr NIP-04 → NIP-44** (CRYPTO-002) as a protocol upgrade, not a fix in place. | Medium | Authenticated encryption on the one unauthenticated channel |

---

## Methodology

This assessment was performed using `security-check`, an AI-powered static-analysis pipeline that uses
large-language-model reasoning to detect security vulnerabilities, extended here with an adversarial
verification phase and with execution-based settlement of contested claims.

### Pipeline phases

1. **Reconnaissance** (`sc-recon`, `sc-dependency-audit`) — architecture mapping, entry-point
   enumeration, trust-boundary analysis, technology detection, and a full supply-chain audit against
   live registry metadata. Output: `architecture.md`, `dependency-audit.md`. Every claim carries a
   `file:line`. Divergences between asserted and implemented behaviour were flagged during recon, and
   **two of those recon flags were later refuted by this pipeline's own hunters**.
2. **Vulnerability hunting** — **11 parallel domain agents** covering 40+ vulnerability skills
   (`sc-authz`, `sc-auth`, `sc-rce`, `sc-cmdi`, `sc-ssrf`, `sc-xss`, `sc-csrf`, `sc-secrets`,
   `sc-crypto`, `sc-data-exposure`, `sc-path-traversal`, `sc-open-redirect`, `sc-mass-assignment`,
   `sc-race-condition`, `sc-rate-limiting`, `sc-session`, `sc-jwt`, `sc-api-security`,
   `sc-business-logic`, `sc-privilege-escalation`, `sc-clickjacking`, `sc-ci-cd`,
   `sc-dependency-audit`, …) and **4 language checklists** (`sc-lang-go`, `sc-lang-typescript`,
   `sc-lang-python`, `sc-lang-rust`). `sc-lang-php`/`java`/`csharp` and `sc-docker`/`sc-iac` were
   correctly **not** activated — those surfaces do not exist here. 112 raw findings. Each hunter ran
   its own refutation pass and recorded what it killed.
3. **Verification** — **2 adversarial verifiers** (A: governance/execution/injection; B:
   SDK/secrets/egress/infrastructure) re-read the cited source **by hand** for the 28 findings in
   scope and attempted to kill each. **Ten claims were settled by execution rather than reading**: Go
   tests against `kernel/runtime` and `kernel/jsonstore`, a Python smuggling replay fed to a real Go
   `net/http` server, `HTTPRedirectHandler.redirect_request` on the live interpreter, `cargo run
   --offline` against the published crate, and read-only `gh` API calls. Where an adversarial verdict
   exists it is **authoritative** and overrides the hunter's severity, confidence and — in one case
   (AC-003) — the mechanism itself.
4. **Reporting** — duplicate merging (14 → 10), CVSS-aligned severity classification, threat-model
   weighting, and remediation prioritisation by (impact × ease).

### Why the zero-fabrication result matters

Both verifiers were instructed to report fabricated citations as a first-class output, and both
independently reported **zero** across every `file:line` they checked. Two citations were off by one
line and both were disclosed as imprecision. The previous assessment shipped one fully fabricated
citation in its unverified Medium tier and caught it in follow-up. For a pipeline that produces 99
findings across four languages, citation integrity is the property everything else rests on: a report
whose line numbers cannot be trusted is a report that must be re-derived rather than acted on.

### Limitations

- **Static analysis and targeted execution only.** No daemon was started, no browser was driven, and
  no live penetration test was performed. Where a finding's runtime symptom is predicted rather than
  observed — CLI-001's four blocked SPA behaviours in particular — that is stated in the finding.
- **Confidence scores are estimates.** Two findings (AC-010 at 55, SECRET-003 at 60) are explicitly
  Probable, and each records the exact evidence that would settle it. **Do not act on SECRET-003
  without tracing `workDir` at each `secretfiles` caller.**
- **Explicitly unverified items are marked as such:** GO-004's reachability, DEP-010's Go 1.26.4
  advisories, EXPOSE-001's base-directory mode, and CLI-001's runtime symptoms. None was upgraded to
  a claim.
- **Registry results are point-in-time (2026-08-13).** DEP-002 in particular should be re-checked
  before acting — and acted on quickly.
- **`osv-scanner` was not run** (not installed locally). `govulncheck` (Go, reachability-aware) and
  `npm audit` both ran successfully, so the coverage gap is small.
- **Excluded from scanning per instruction:** `node_modules/`, `frontend/dist/`, `.dev-home/`.
  `kernel/webui/dist/` was read only to verify bundle contents.
- **Custom business-logic flaws may still require manual review.** The findings that came closest to
  this class (BIZ-001 through BIZ-003) were the hardest to verify and carry the lowest confidence in
  their tier.

---

## Disclaimer

This security assessment was performed using automated AI-powered static analysis, extended with
adversarial verification and targeted execution. It does not constitute a comprehensive penetration
test or a formal security audit. The findings represent potential vulnerabilities identified through
code pattern analysis, LLM reasoning, and — for a subset — direct experimental confirmation. False
positives and false negatives remain possible.

This report should be used as a starting point for security remediation, not as a definitive statement
of the application's security posture. It goes stale the same way its predecessor did: **re-verify
against current source before acting on any individual row.** A professional security audit by
qualified security engineers is recommended before any deployment handling third-party data.

Generated by security-check — github.com/ersinkoc/security-check
