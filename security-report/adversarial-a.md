# Adversarial Verification A — governance / execution / injection

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (`main`)
**Scope:** AC-001..AC-004, CE-001..CE-004, BIZ-001, BIZ-002, PE-007, API-001, INJ-001, INJ-002
**Method:** every cited `file:line` re-read by hand in this pass. Five claims settled empirically
with a throwaway Go test in `kernel/runtime` (deleted; `git status` shows no source or test
artifact of mine — only the pipeline's own `security-report/*` edits).

**No fabricated citations.** Every `file:line` in all four assigned result files resolved to code
that says what the hunter said it says, with one exception noted under AC-003 (the *inference*
from correctly-quoted code is wrong, not the quote). Line numbers were accurate to within ±3
throughout and usually exact.

---

## Summary

| ID | Original | Verdict | My severity | Conf |
|---|---|---|---|---|
| AC-001 | Critical | **CONFIRMED-DOWNGRADED** | High (+ an understated High sub-finding) | 92 |
| AC-002 | High | **CONFIRMED** | High | 96 |
| AC-003 | High | **REFUTED-AS-WRITTEN** | Medium | 90 |
| AC-004 | High | **CONFIRMED** | High | 94 |
| CE-001 | High | **CONFIRMED** | High | 93 |
| CE-002 | High | **CONFIRMED-DOWNGRADED** | Medium | 90 |
| CE-003 | High | **CONFIRMED-DOWNGRADED** | Medium | 88 |
| CE-004 | High | **CONFIRMED** | High | 90 |
| BIZ-001 | High | **CONFIRMED-DOWNGRADED** | Medium | 85 |
| BIZ-002 | High | **CONFIRMED-DOWNGRADED** | Medium | 85 |
| PE-007 | High | **CONFIRMED** (= AC-002) | High | 96 |
| API-001 | High | **CONFIRMED** | High | 92 |
| INJ-001 | High | **CONFIRMED-DOWNGRADED** | Medium | 88 |
| INJ-002 | High | **CONFIRMED** | High | 88 |

Killed outright: none. Downgraded: 6. Mechanism refuted: 1 (AC-003).
**Understated:** one sub-claim inside AC-001 (see below) is a bigger finding than the headline.

---

## AC-001 — Trust ceilings are operationally inert

**Verdict: CONFIRMED-DOWNGRADED (Critical → High) · Confidence 92**

Every link verified individually, by reading and by running code.

| # | Link | Cited | Holds? |
|---|---|---|---|
| 1 | ceiling clamp | `kernel/edict/edict.go:804-807` | **Yes** — exact quote |
| 2 | Ask-class folds to Allow | `kernel/edict/edict.go:846-853` | **Yes** — exact quote, `default: // AskAllow` → `DecisionAllow` |
| 3 | AskAllow is default *and* typo fallback | `cmd/agezt/main.go:3845-3859` | **Yes** — `case "", "allow"` → AskAllow; `default:` → AskAllow |
| 4 | `RequiresApproval` reaches `autoApproveCap` | `kernel/runtime/policy.go:187-192` | **Yes** — exact quote |
| 5 | empty `AGEZT_AUTO_APPROVE_CAPS` = every capability | `cmd/agezt/main.go:3893-3898` | **Yes** — exact quote |
| 6 | injected daemon-wide into every run | `kernel/runtime/runtime.go:1970-1971` | **Yes** — exact |

Start points also verified: `plugins/builtinguardians/builtinguardians.go:78` (`defaultTrustCeiling
= "L2"`) and `:215`; `runctx.go:382-386` applies it; `main.go:2771-2773` is the standing-order
fail-safe; `main.go:2899` and `:3028-3030` re-apply ceilings on the standing/resume paths.

**Empirical result** (temporary test against the real `policyHook`, since deleted):

```
AskAllow      + ceiling L2 + auto-approve-all → allow=true  "…AskPolicy=AskAllow…(clamped to ceiling L1)"
AskPrompt     + ceiling L2 + auto-approve-all → allow=true  (auto-approve satisfied the prompt)
AskDeny       + ceiling L2 + auto-approve-all → allow=FALSE "…AskPolicy=AskDeny (clamped to ceiling L1)"
AskAllow      + ceiling L2 + auto-approve OFF → allow=true
```

### Two corrections to the write-up

**1. "No single-variable configuration fixes this" is false.** `AGEZT_APPROVAL_MODE=deny` alone
makes the ceiling bind, *even with the auto-approve default left at "all"* — proven above. The
mechanism is that `AskDeny` (`edict.go:827-833`) returns `DecisionDeny` with `RequiresApproval`
left **false**, so `policy.go:187` is never reached. The hunter names this option but frames it as
not-a-fix; it is a fix, with a real cost (`DefaultLevels()` at `edict.go:634-640` puts *every*
capability at L4, so Ask-class arises only from a ceiling — meaning `deny` denies **everything**
inside a ceilinged run, which is the reason it is not a usable posture, not the reason it fails).

**2. The doc-contradiction claim is half-right.** `runctx.go:254-261` does say "session-scoped
operator grant … NOT a daemon-wide policy change" — but the `Config` field's own doc at
`kernel/runtime/runtime.go:260-265` says, verbatim, *"a **daemon-wide** operator grant … applied
to every run and inherited by sub-agents."* Code and its own field doc agree. Only the helper's
comment drifted.

### Downgrade rationale (Critical → High)

`edict.go:630-632` states the fold as an explicit owner decision: *"the owner ran
AskPolicy=AskAllow, which folded every ask to allow in practice — this makes the real posture the
explicit one."* Combined with `DefaultLevels()` = all-L4, the inert ceiling is the documented
consequence of a documented posture, and a single documented variable restores it. That is High,
not Critical.

### The part the hunter UNDERSTATED — file this separately, High

Buried in "why not a false positive" is the observation that the same `policy.go:187` branch also
satisfies the prompt-injection guard. I verified this is exactly true and it is worse than the
ceiling case:

- `AGEZT_PROMPT_INJECTION_GUARD` defaults to **warn**, not on (`kernel/runtime/runctx.go:313-328`,
  pinned by `autoapprove_test.go:117-131`). So an operator who wants blocking must *explicitly*
  set `on`/`block` — a deliberate, single-purpose hardening.
- With it set, `policy.go:169-180` sets `requiresApproval = true`, `verdict.Allow = false` …
  and `policy.go:187-192` immediately flips `Allow` back to `true`, because the shipped
  `AGEZT_AUTO_APPROVE_CAPS` default covers every capability. Measured:

```
guard=on + auto-approve-all → allow=true  reason="capability set to L4 (allow)"
guard=on + auto-approve OFF → allow=false reason="approval timeout: no response within timeout"
```

Note the reason string: the verdict the journal records is a plain L4 allow. Nothing in
`policy.go`'s verdict says the injection guard fired and was overridden — only a side-channel
`publishAutoApprove` event. `runtime.go:262-264` promises the grant "never overrides hard-deny,
explicit tool-deny, SSRF, budgets, or other fail-closed guards" and omits the injection guard,
which it *does* override.

**This is the cleanest instance in the whole scan of "an opt-in restriction the operator
explicitly applied fails to restrict."** It needs no ceiling, no guardian, no schedule.

---

## AC-002 / PE-007 — `overseer op=pause` / `op=retire` reach System guardians

**Verdict: CONFIRMED (High) · Confidence 96 · These are ONE finding.**

The two hunters agree on mechanism in every particular — same call chain, same missing checks,
same `0cdd3799` precedent, same remediation placement. Independent convergence; no disagreement.

Verified myself:

- `plugins/tools/overseertool/tool.go:149-159` (`pause`/`unpause`) and `:161-174` (`retire`)
  dispatch with **no** target lookup, no `System` check, no `fleetLock` check.
- `plugins/tools/overseertool/kernelsource.go:77-86` — `SetAgentEnabled` / `SetAgentRetired` are
  bare pass-throughs, exactly as quoted.
- `kernel/runtime/runtime.go:1221-1235` / `:1240-1254` — no `System` check.
- `kernel/roster/roster.go:786-806` / `:811-836` — no `System` check; `:825` sets
  `p.Enabled = false` on retire.
- Contrast confirmed: `kernelsource.go:105` (`fleetLock`) + `:112-114` (`System`) on `EditAgent`;
  `tool.go:341-345` (`System`) on `op=repair`; `runtime.go:1301-1303` on `RemoveProfile` — whose
  error message literally reads *"pause or retire it instead of removing."*
- Bulk variants: `kernelsource.go:341-352` / `:354-371`, same gap.
- **Survives restart, verified:** `plugins/builtinguardians/builtinguardians.go:234-262`
  re-clamps `ToolDeny`/budgets/`TrustCeiling`/`MemoryScope`/`NoisePolicy` and never touches
  `Enabled`/`Retired`; and it could not — `kernel/roster/roster.go:853-854` restores both from the
  pre-mutation snapshot.

**One correction to AC-002's doc argument.** `docs/THREAT-MODEL.md:479` reads exactly as quoted,
but read literally it promises the fleet lock blocks *"edit, create, or delete"* and that
*"System-guardian edits or deletion are always refused"* — pause/retire appear in neither list, so
the doc is not strictly falsified. PE-007's version of the argument is the stronger one and it
holds: `kernel/roster/roster.go:128` says System agents "can still be paused, retired, and
**edited** like any agent" — and `edit` *is* blocked on the agent-reachable path, which proves the
sentence describes operator privilege, not the tool surface.

Severity stands at High: `CapOversee` is `LevelAllow` by default (`DefaultLevels`, all-L4), the
seeded guardians hold `overseer`, six tool calls kill the fleet, and the effect is durable.

---

## AC-003 — `op=wake` "drops prompt-injection taint"

**Verdict: REFUTED-AS-WRITTEN · Confidence 90 · Real severity: Medium**

The quoted code is real and correct (`plugins/tools/overseertool/kernelsource.go:410-416`,
`ctx := kernelruntime.WithAgentProfile(context.Background(), p)`). The **inference is wrong.**

**What the hunter missed.** The untrusted-observation taint is never in the tool's context to
begin with. `kernel/agent/run_tools.go:188-191` builds a *separate* `policyCtx` and attaches the
taint to that one only:

```go
policyCtx := WithPolicyToolDef(ctx, def)
scopedTaint := s.untrustedTaint
scopedTaint.DirectiveLike = s.directiveActive(iter)
policyCtx = WithUntrustedObservationTaint(policyCtx, scopedTaint)
verdict = s.cfg.Policy(policyCtx, tc)
```

`policyCtx` is discarded after the verdict. The tool is invoked from the *outer* `ctx`:
`executeToolJobs(ctx, cfg, jobs)` → `invokeToolJob(ctx, …)` → `toolCtx := WithCorrelation(ctx, …)`
→ `job.tool.Invoke(toolCtx, …)` (`run_tools.go:275`, `:326-332`).

So the `overseer` tool's `Invoke` context carries **no taint**, and `context.Background()`
discards nothing of the kind. The recommended fix — thread `ctx` in and use
`context.WithoutCancel(ctx)` — would **not** propagate taint either. The claim that "the
delegation path deliberately does the opposite" is also inapt for taint: `subagent.go:549-562`
and `:242` are real and quoted correctly, but they propagate depth/actor/correlation/profile —
never taint, for the same structural reason.

**What is actually true, and worth keeping:**

1. `WakeAgent` has **no `System` check and no `fleetLock` check** — independently verified,
   `kernelsource.go:389-418`. Same family as AC-002; an agent can wake a protected guardian.
2. The fresh context genuinely loses the caller's **trust ceiling**, intent frame, auto-approve
   set, tenant stamp and correlation. The ceiling loss is real (`WithTrustCeiling` is on the run
   context and does reach tools) — though inert today for AC-001's reason.
3. Attacker-controlled text does become the woken agent's **run intent** — untrusted content
   promoted to the operator position. That is a genuine confused-deputy step; it just isn't
   mediated by the taint mechanism the finding names.
4. Guardians are wakeable: `DirectCallable` is unset on the seeded profiles and
   `roster.go:224-226` defaults `nil` → true.

Medium, not High: the mechanism that made it High (defeating an armed injection guard) does not
exist.

---

## AC-004 — `config` tool rewrites the daemon's security posture

**Verdict: CONFIRMED (High) · Confidence 94**

Counted and re-read, not taken on trust:

- `kernel/settings/schema.go`: **203** `Env: "AGEZT_…"` fields, **exactly 1** `ReadOnly: true`
  (`:99`, `AGEZT_CHATGPT_OAUTH`), **0** `Locked: true`. Matches the finding.
- Every field in the impact table exists at the cited line and is writable. Spot-verified:
  `AGEZT_WEB_PASSWORD` `:459`, `AGEZT_WEB_PASSWORD_STRICT` `:463`, `AGEZT_AGENTGW_SOCKET` `:467`,
  `AGEZT_APPROVAL_MODE` `:585`, `AGEZT_AUTO_APPROVE_CAPS` `:586`,
  `AGEZT_PROMPT_INJECTION_GUARD` `:588`, `AGEZT_ALLOW_ALL` `:589`. All exact.
- Gate: `plugins/tools/config/config.go:44-53` maps `set`/`register`/`unregister` to
  `CapConfigWrite` — `LevelAllow` by default. Only constraint is `field.ReadOnly`
  (`config.go:205-207`) and `field.Locked && value == ""` (`:212-214`).
- Persistence loop confirmed end to end: `config.go:264-277` writes the store;
  `cmd/agezt/main.go:209` calls `injectConfig` **before** `daemonconfig.Load` at `:219`;
  `main.go:3809-3814` `os.Setenv`s every stored key that is unset in the real environment.
- The stated mitigation is real: `main.go:3810` is `if val != "" && os.Getenv(name) == ""`, so a
  real-environment pin wins. Also `main.go:3793-3799` records those pins.

The High rating (rather than Critical) is correctly reasoned by the hunter and I agree.

---

## CE-001 — code_exec secret scrub disableable from inside its own tier

**Verdict: CONFIRMED (High) · Confidence 93**

- `plugins/tools/codeexec/codeexec.go:283` — exact:
  `env := executionprofile.AppendEnvPassthrough(scrubEnv(dir), profileID)`. Same wrapper on
  `plugins/tools/shell/shell.go:259`.
- Advertised guarantee real: `runtimes.go:117-119` ("This is the load-bearing safety property of
  the whole tool"), and the **model-facing** string at `codeexec.go:138` ("The daemon's secrets
  are never visible to your code").
- `kernel/executionprofile/env.go:48-50` re-reads `os.Getenv` **per invocation** via
  `EnvPassthroughPolicyFromEnv`, so a live `os.Setenv` takes effect on the next call in the same
  run. Confirmed.
- The secret-list branch blocks only the `AGEZT_` prefix: `env.go:93` and `:145-147`. `IsSecretEnvName`
  (`:120-128`) covers KEY/TOKEN/SECRET/AWS_ — and the `SecretEnvNames` loop deliberately does not
  consult it. So `AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, `ANTHROPIC_API_KEY` all pass.
- All four possible blocks verified absent: `AGEZT_EXEC_SECRET_ENV_LOCAL` at `schema.go:499` is
  `TypeCSV`, `ApplyLive`, not `Secret`/`ReadOnly`/`Locked`; and `settings.Validate`
  (`schema.go:619-641`) has cases for **only** `TypeNumber`, `TypeBool`, `TypeSelect` — no
  `TypeCSV` branch, so any string is accepted.
- Live-apply path confirmed at `config.go:279-284`.

Holds as written, at High. This is a restriction (the scrub) that the confined tier can switch
off, while the string shipped to the model asserts it unconditionally.

---

## CE-002 — `tool_forge` promotion has no human in the loop by default

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · Confidence 90**

Mechanism exact: `cmd/agezt/internal/daemonconfig/daemonconfig.go:393` reads
`!strings.EqualFold(…, "off")` → unset means **true**; wired `main.go:435` → `:516`;
`kernel/runtime/scripttool.go:134-140` short-circuits the approval registry. The "tested"
invariant is self-attested — `scripttool.go:87` is `ok := !isErr` on the agent's own script and
input. All quoted docs verified: `forgetool/tool.go:5-7`, `:77-79` (model-facing),
`scripttool.go:116-118`.

**Why Medium.** The finding says "three docs and the model-facing tool description promise it."
Both *operator*-facing surfaces are honest, and I read them:

- `kernel/settings/schema.go:587`: *"on/default = tested agent-forged tools go live when the agent
  requests promotion; off = require operator promotion."*
- Boot banner, `cmd/agezt/main.go:1590-1594`: prints `auto-promotes tested tools` vs
  `operator promotes` — on every start.

So the wrong surfaces are two Go package comments plus a model-facing string. A model being
misinformed is not a security control. And the security delta is bounded: the same agent already
holds `code.exec` at `LevelAllow`, so promotion buys **persistence and cross-agent reach**, not
new execution authority. Real, worth fixing (the doc drift is unambiguous), but Medium.

---

## CE-003 — `mcp op=add` forces `Enabled=true`; boot auto-attach is ungated

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · Confidence 88**

Verified: `kernel/mcp/store.go:218` is literally `srv.Enabled = true`, unconditional.
`kernel/runtime/mcptool.go:29-30` says *"Registration alone spawns nothing — attach does"* —
falsified. `kernel/mcp/store.go:11-12` says `mcp.install` is *"Ask by default"* — falsified by
`DefaultLevels()` (`edict.go:634-640`), which is all-`LevelAllow`. `AttachEnabledMCPServers`
(`mcptool.go:178-191`) and `AttachMCPServer` contain no `policyHook`/`Decide`/`approval`
reference — I read the whole function. `cmd/agezt/main.go:1616` calls it from a plain boot step.

**Why Medium.** The hunter's own reachability argument undercuts the severity: the doc-promised
second step (`op=attach`) is `mcp.install` at `LevelAllow`, so the agent can attach the same
payload **immediately, in the same run**, with a policy decision that returns Allow. The
incremental risk of the forced `Enabled` flag is therefore *persistence across restart*, not new
execution. The two falsified doc statements are the real, cheap-to-fix finding.

---

## CE-004 — `config op=register` + `op=set` reopens the raw-command path

**Verdict: CONFIRMED (High) · Confidence 90**

Every link verified:

- `AGEZT_ACP_AGENT_CMD` and `AGEZT_CODING_CMD` are **not** in `kernel/settings/schema.go` (grep:
  zero hits in that file; they appear only in `kernel/controlplane/config.go:41,91`, a different
  list, and in the tools that read them). Therefore not in `builtinEnvSet()`
  (`kernel/settings/registry.go:48-56`), which is the only reserved set `validateSection` checks
  (`registry.go:204-206`).
- Both names match `envNamePattern` `^AGEZT_[A-Z0-9_]+$` (`registry.go:27`).
- `Registry.Register` (`registry.go:140-156`) validates and writes; `FieldByEnv`
  (`registry.go:125-133`) then finds the registered field via the merged `Sections()`.
- `TypeText` has no `Validate` branch (`schema.go:619-641`) → any string.
- `injectConfig` (`cmd/agezt/main.go:3809-3814`) iterates `store.All()` with **no schema filter
  at all** — verified by reading the loop.
- Sink: `plugins/tools/acpagent/acpagent.go:244-246` is `exec.Command(shell, arg, cmdStr)` under a
  `SECURITY:` comment at `:238-243` asserting the value is operator-only. Quoted accurately.
- The self-consistency the hunter did not spell out and I checked: after the restart the env var
  *is* set, so `plugins/builtintools/envgated.go:89` registers the `acp_agent` tool that reads it.
- The hunter's own refutation of the live-apply variant is correct (`registry.go:144-146`,
  `:114-116` force `ApplyRestart`), so it costs a restart.
- `AGEZT_TUNNEL_CMD` (`schema.go:543`, `TypeText`, not read-only) needs no `op=register` — one
  `op=set` reaches `kernel/tunnel/tunnel.go`. Verified present in the schema.

Holds at High.

---

## BIZ-001 — Unpriced model bills $0, defeating every spend ceiling

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · Confidence 85**

Mechanism exact at every citation: `kernel/governor/preflight.go:185-192` (the gate and its
`Off by default` comment), `plugins/providerboot/providerboot.go:309` (only literal `on` arms it),
`kernel/governor/pricing.go:117-125` ("Unknown models cost nothing"),
`kernel/governor/governor.go:1283-1293` (`spentToday.Add(cost)` with `cost == 0`),
`kernel/governor/budgetgate.go:119` (`spent >= ceiling`). The `schedule` tool's free-text `model`
input exists at `plugins/tools/schedule/schedule.go:120`.

**Why Medium.** The governor's comment at `preflight.go:185-189` describes this exact failure
mode, names it, and ships the one-variable fix (`AGEZT_PRICING_STRICT=on`). It is a documented
off-by-default hardening, not an unknown hole. Exploitation also needs a model that *routes* but
is unpriced — a real Ollama/Azure/new-release id, not an arbitrary string. The budget ceiling is
still an operator-applied restriction that fails, which is why I am not refuting it.

---

## BIZ-002 — Trust ceiling laundered into an uncapped future run via `schedule`

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · Confidence 85**

Mechanism verified precisely. `scheduledRunContext` (`cmd/agezt/main.go:3327-3341`) applies
`WithAgentProfile` and `WithMaxCost` and **no `WithTrustCeiling`** — I read the whole function.
`runCtx` is the scheduler tick's context (`main.go:3166`), not the creating run's. The contrast is
real: `main.go:2899` (standing) and `:3028-3030` (resume) both call `WithTrustCeiling` explicitly,
the latter under a comment naming it a "Governance invariant (M1002)". `applyActingAgent`
(`schedule.go:161-167`) is indeed the only identity binding. And a non-System profile's
`TrustCeiling` really is empty — `applySystemGuardianDefaults` (`roster.go:496-497`) returns
immediately for `!p.System`, so the `"L4"` default at `:518` never applies to a normal agent.

**Why Medium.** The hunter's own note concedes it: the ceiling this launders is inert today for
AC-001's reason, so there is no observable effect on a stock install. It becomes a genuine High
*after* AC-001 is fixed. Filing it at High today overstates present exposure.

---

## PE-007 — see AC-002

**Verdict: CONFIRMED (High) · Confidence 96.** Same finding, independently found, mechanisms
identical. PE-007's handling of the `roster.go:128` counter-argument is the better-argued of the
two and I verified it. Deduplicate into one item.

---

## API-001 — Seven channel listeners authenticate with `if secret != ""`

**Verdict: CONFIRMED (High) · Confidence 92**

I read all eight cited sites. All seven have the fail-open shape and the eighth (baseline) fails
closed:

- `chatwebhook.go:157-158` — `if c.cfg.Token == "" { return true }`, with the doc comment saying so.
- `dingtalk.go:139`, `onebot.go:163` — `if c.cfg.Secret != "" && !valid…`.
- `feishu.go:162` — `if c.cfg.VerifyToken != "" && …`.
- `zalo.go:143`, `imessage.go:158`, `whatsappgw.go:153` — `if c.cfg.Secret != "" { … }`.
- Baseline `plugins/channels/webhook/webhook.go:260-263` — `if c.secret == "" || sig == "" { return false }`,
  documented as *"An empty secret fails closed (no unsigned inbound)."*
- Factory contrast verified: `plugins/builtinchannels/factories.go:1201-1210` gates dingtalk on
  `ADDR`, passing `DINGTALK_SECRET` through whatever it is; `factories.go:950-961` shows `line`
  refusing to construct the two-way channel without a secret. The invariant genuinely lives in the
  wrong layer.

**One nuance the hunter states honestly and I confirmed.** `kernel/channel/channel.go:129-132`
denies everyone when the allowlist is empty, so an operator must also set e.g.
`AGEZT_DINGTALK_USERS` — but they must set it anyway for the channel to function, and the key is a
display name / staff id, not a secret. It is a second gate, not authentication. High stands.

---

## INJ-001 — Hard-deny rails inert for every non-shell execution capability

**Verdict: CONFIRMED-DOWNGRADED (High → Medium) · Confidence 88**

Mechanism is exactly right and I re-derived it: `kernel/edict/edict.go:373-378` short-circuits on
capability *before* the substring test; all sixteen entries of `DefaultHardDeny()`
(`edict.go:646-667`) carry `AppliesTo: []Capability{CapShell}` — I counted, no exceptions;
`kernel/edict/toolmap.go:25-27` (`forge_*`→`CapCodeExec`), `:30-32` (`mcp_*`→`CapMCP`),
`:135-136` (`code_exec`), `:137-144` (`conductor`), `:171-172` (`coding`), `:173-174`
(`acp_agent`) all resolve outside `CapShell`. So `code_exec` really can run `rm -rf /`.

**Why Medium — the "advertised guarantee" argument is weaker than claimed.** I read both cited
comments in both directions:

- `edict.go:622-624` lists "the F4 hard-deny strings" among what MAX-AUTONOMY *"does NOT relax."*
  That is literally true: the posture does not relax them; they were narrowly scoped from birth.
- `cmd/agezt/main.go:287-289` says the rails *"deliberately stay"* under `AGEZT_ALLOW_ALL`. Also
  literally true — `main.go:278` keeps `DefaultHardDeny()` in `edictOpts` regardless.
- And `DefaultHardDeny`'s own doc at `edict.go:642-644` states the scoping outright: *"only
  checked for the specified capabilities so they don't false-positive on unrelated tool input."*

Neither cited comment claims the rails cover every capability, and the rule set documents that
they do not. `edict_test.go:399-407` locks the scoping in as intentional. What remains is a real
defense-in-depth gap on a **self-destruction guardrail** — in a system where `code.exec` is
`LevelAllow` by owner decision, so the same agent already has unrestricted execution. Cheap and
correct to fix; not a High.

---

## INJ-002 — JSON/argument injection into workflow tool-node arguments

**Verdict: CONFIRMED (High) · Confidence 88 — with one supporting claim killed**

Mechanism confirmed at every line:

- `kernel/runtime/workflowrun.go:449-455` interpolates the **serialized JSON text** of `c.Args`
  and re-casts it with `json.RawMessage(args)`. Exact.
- `kernel/workflow/template.go:20-39` is pure text substitution; `renderValue` (`:69-82`) returns
  `case string: return t` verbatim — no escaping layer anywhere. Exact.
- `kernel/workflow/workflow.go:313-316` — `Args json.RawMessage // templated JSON`. Exact.
- The correct sibling really is one function away: `workflowrun.go:531-541` (`NodeHTTP`)
  interpolates leaves and then `json.Marshal`s a `map[string]any`. This contrast is what makes it a
  bug rather than an engine property, and it holds.
- The gate runs on post-injection args: `invokeWorkflowTool` (`workflowrun.go:736-752`) calls
  `ValidateToolInput` at `:742` and `policyHook` at `:745` against the already-substituted
  `args`. Confirmed.
- No re-interpolation loop (`template.go:37` advances past the substitution) — second-order
  template injection is genuinely absent, as claimed.

**Killed:** the "shipped-template proof" in the Notes-for-orchestrator section. I read
`kernel/workflow/templates.go:138-141`: that workflow's trigger node is
`{ID:"start", Type: NodeTrigger, Config: raw(`{"kind":"manual"}`)}` — **manual**, not webhook. The
vulnerable `save` node at `:141` is real, but nothing shipped feeds it attacker-controlled data.
The claim "it is not a theoretical pattern the operator would have to opt into" is wrong: the
operator must wire a webhook/channel/schedule trigger into a tool node themselves. The finding's
own body states this correctly ("An attacker who can reach the trigger…"); only the orchestrator
note overreaches.

The `/hooks/` source is real and unauthenticated-but-secreted as described
(`kernel/webui/webui.go:1008-1030`; non-JSON bodies ride verbatim as a string at `:1020`,
`payload := map[string]any{"kind":"webhook","body": body}` at `:1030`). High stands.

---

## Cleanup

Temporary test file `kernel/runtime/zzadvverify_test.go` created and deleted. `git status` shows
only pipeline-owned `security-report/*` changes and the pre-existing untracked `.wrongstack/`.
No source file was modified. The daemon was not started. `.dev-home/` was never read.
