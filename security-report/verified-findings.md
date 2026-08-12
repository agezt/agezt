# Verified Security Findings — adversarial verification pass

**Target:** `D:\Codebox\PROJECTS\AGEZT` — `main` @ `f815f56e`
**Date:** 2026-08-12 · **Role:** sc-verifier (adversarial — the goal is to REFUTE)
**Scope of this pass:** PRIVESC-002, PRIVESC-003, PRIVESC-005, PRIVESC-006 from `security-report/injection-results.md`.
This file replaces an older full-scan output; it covers only the four findings re-attacked here.

**Method:** every claim was re-derived from source. Where the original finding asserted an absence
("no check", "no ceiling"), I looked for the check on the actual dispatch path rather than at the
site the finding quoted. Two posture rules were applied as instructed and are *not* treated as
vulnerabilities: (a) default-allow — a capability being `LevelAllow` is by owner decision; (b) trust
ceilings L1–L3 fold to `DecisionAllow` under the shipped default `AskPolicy=AskAllow`, so a ceiling
is never credited as protection when assessing impact.

---

## Verdicts

| ID | Verdict | Severity assigned | One-line reason |
|---|---|---|---|
| **PRIVESC-002** | **REFUTED** (as privesc) | **Low** (was High) | Authority *is* re-derived from the roster — `WithAgentProfile` re-applies the profile's ceiling and the ticket can only tighten it — and the only tools that can write the ticket (`shell`, `code_exec`) already give arbitrary host writes, where `roster.json` / `standing.json` are equally unsigned and strictly more powerful. |
| **PRIVESC-003** | **CONFIRMED** | **Medium** (was High) | `agent` / `run_id` are model-visible schema fields, nothing re-pins them downstream, and `Store.Claim` treats a name match as authority — but the attacker gains attribution and lease control, not capability. |
| **PRIVESC-005** | **CONFIRMED** | **High** | `overseer` is registered unconditionally, `Invoke` discards the context so no caller check is even possible, and `EditAgent` lets an agent clear its own `tool_deny` — the one per-agent restriction that survives the `AskAllow` fold — and raise its own `max_daily_mc`, durably. |
| **PRIVESC-006** | **PLAUSIBLE** | **Medium** (was High) | The missing `p.System` / `fleetLock` guard on `RepairAgent` is real and untested, but `op=wake` already reaches a guardian's prompt with attacker text, the write surface excludes every authority field, and the write depends on the guardian's model emitting the proposal. |

---

## PRIVESC-002 — Resume tickets are unauthenticated — **REFUTED**

**Verdict: REFUTED as a privilege escalation. Reclassify as Low (integrity hardening), confidence ~35.**

### What the original finding got right

Verified accurate, line by line:

- `kernel/resume/resume.go:201-242` — `getLocked` / `List` do `json.Unmarshal` and nothing else. No MAC, no signature, no provenance field. `List` filters on `.json` suffix and `t.Corr != ""` only — it does **not** filter on `Status`, and neither does `buildResumer`.
- `cmd/agezt/main.go:2984-3080` — the resumer checks `Resumable`, `Attempts < maxAttempts`, and that `AgentSlug` names an existing, un-retired, enabled profile. Nothing cross-checks `Corr` against the journal.
- `Messages` are injected verbatim as prior conversation (`WithResumeSeed`, consumed at `kernel/runtime/runtime.go:2116-2118`), including fabricated tool-result turns.
- `AllowsDelegationFrom` genuinely does not apply on this path.

### Why it is refuted

**1. The claim "omit `trust_ceiling` and the resumed run carries no ceiling" is false.**

The resumer applies `WithAgentProfile(rctx, *prof)` at `main.go:3016` — *before* the ticket's ceiling —
and `WithAgentProfile` itself re-applies the roster profile's own ceiling:

```go
// kernel/runtime/runctx.go:382
if ceiling := strings.TrimSpace(p.TrustCeiling); ceiling != "" {
    if lvl, err := edict.ParseTrustLevel(ceiling); err == nil {
        ctx = WithTrustCeiling(ctx, lvl)
    }
}
```

and `WithTrustCeiling` keeps the **minimum** (`runctx.go:134-136`). So the ticket's ceiling field can
only ever *tighten* what the roster already imposes; omitting it yields the target agent's normal
baseline ceiling, not "no ceiling". The finding's own exploit narrative targets
`system-health-guardian`, and every shipped guardian is seeded `TrustCeiling: "L2"`
(`plugins/builtinguardians/builtinguardians.go:215`) — so the forged ticket in the PoC would run at
L2, exactly as a legitimate resume would. Authority is re-derived, not reconstructed from the ticket:
Soul, ToolAllow/ToolDeny, ConfigOverrides, MemoryScope, Workdir, model chain and `MaxDailyMc` all come
from the live roster profile.

(The one governance field that *is* ticket-only is `MaxCostMc` — `WithAgentProfile` deliberately does
not apply the profile's per-run cost cap. Omitting it drops the per-run cap, leaving the per-agent
daily ceiling and global budget as the bounds. Minor, and financial rather than authority.)

**2. The precondition is already total compromise, so no privilege boundary is crossed.**

`<BaseDir>/resume` (`kernel/runtime/runtime.go:737`) is a sibling of the workspace
(`<BaseDir>/workspace`, `main.go:3836-3840`), i.e. **outside** the file tool's root — and the `file`
tool is genuinely confined (`withinRoot`, `ErrEscape`, symlink-resolved, `plugins/tools/file/file.go:756-838`),
so it cannot write there. The reachable writers are `shell` and `code_exec`. `shell` executes
`[shellBin, shellArg, in.Command]` (`plugins/tools/shell/shell.go:254-262`) under warden
`ProfileNamespace`, whose own header states what it is not:

> `kernel/warden/warden_linux.go:26-28` — "**What this is NOT.** No namespaces (CLONE_NEWUSER / CLONE_NEWNS / CLONE_NEWPID), no seccomp BPF, no cgroup v2."

`WorkDir` is a cwd, not a jail; on Windows there is no confinement layer at all. The F4 hard-deny list
(`edict.go:646-666`) blocks fork bombs, `rm -rf /`, `mkfs`, raw-device `dd`, and shutdown verbs — nothing
path-related. So the exploit's entry condition is **arbitrary host command execution as the daemon user**.

At that point the resume store is inside the TCB and confers nothing new. Every governance store in
`AGEZT_HOME` is equally unsigned plain JSON:

- `kernel/roster/roster.go:713` — "Store is the persistent roster, a single JSON file rewritten atomically."
- `kernel/standing/standing.go:160-178` — "the persistent set of standing orders, a single JSON file", loaded via `jsonstore.LoadFrom(dir, "standing.json", …)`.

Writing `roster.json` directly mints an arbitrary profile with any ceiling, any budget and any
tool policy — strictly more than a resume ticket, which is *constrained* by the profile it names.
Writing `standing.json` gives unattended, **recurring** dispatch as a chosen identity with no
3-attempt cap and no quarantine — a better version of the same primitive. The resume ticket is the
weakest of the three, not a unique escalation.

### What survives

A genuine but different threat model: an actor who can write one file into `AGEZT_HOME` *without*
already having code execution — a restore from an untrusted backup, an `AGEZT_HOME` sync, or another
OS user with write access to the daemon home. That is real, but it is identical for `roster.json`,
`standing.json`, and the config/settings stores, and should be filed once as **"AGEZT_HOME is unsigned
mutable state; the daemon trusts its own home directory"**, not as a resume-specific High. The
resume-specific hardening worth doing is narrow: treat seeded `Messages` as untrusted observations
(the taint machinery already exists), and require an explicit ceiling declaration rather than
inferring "absent" from `omitempty`.

---

## PRIVESC-003 — Workboard claim identity is caller-supplied — **CONFIRMED**

**Verdict: CONFIRMED. Severity Medium (down from High), confidence 90. CWE-863/CWE-345 rather than CWE-290.**

The end-to-end path is complete, deterministic, and reachable — I could not break it.

1. **The model is invited to supply the identity.** `agent` and `run_id` are first-class properties of
   the published input schema, with descriptions that advertise the override:
   `"agent": {"type":"string", "description":"For claim/heartbeat. Defaults to the acting agent when available."}`
   (`plugins/tools/workboardtool/workboard.go:80-81`). This is not an accidentally-accepted extra key.
2. **The context identity is a default only.** `applyContextDefaults` (`:214-224`) fills `Owner`/`Agent`/`RunID`
   only when blank, and is called once at `:147` with `agent.AgentFromContext(ctx)`.
3. **Nothing re-pins downstream.** `Kernel.ClaimWorkboardTask` (`kernel/runtime/workboard.go:34-41`) forwards
   `agent` and `runID` unchanged to the store and echoes them into the journal payload. The run's real
   `corr` is passed alongside but never compared to `runID`.
4. **The store treats a name match as authority.** `Store.Claim` (`kernel/workboard/workboard.go:396-404`):
   `if t.Claim != nil && !strings.EqualFold(t.Claim.Agent, agent) { return ErrClaimConflict }` — a matching
   name passes, then `t.Claim = &Claim{Agent: agent, RunID: runID, …}` **overwrites** the live claim with the
   attacker's `RunID`, and `t.Attempts = append(…, Attempt{Agent: agent, …})` durably records the victim's name
   against the attacker's attempt.
5. **The victim is locked out.** `Heartbeat` (`:419`) fails on `t.Claim.RunID != runID`, so the victim cannot
   refresh its own lease; and the completion write at `kernel/controlplane/workboard.go:538` is gated on
   `current.Claim.RunID == corr`, so the victim's finished work is silently discarded.

No race, no timing window, no operator involvement.

**Why Medium rather than High.** The attacker gains no capability, no data access, no execution, and no
budget. What it gains is (a) denial of another agent's work lease, (b) a forged actor name in the durable
attempt record, and (c) discard of the victim's completed work. That is integrity-of-attribution plus
availability, inside a single-tenant fleet the operator owns end to end. "Authentication Bypass by Spoofing"
(CWE-290) overstates it — there is no authentication being bypassed; there is a missing authorization check on
a self-asserted actor field. The remediation in the original report is correct as written.

---

## PRIVESC-005 — Self-escalation via `overseer op=edit` — **CONFIRMED**

**Verdict: CONFIRMED. Severity High, confidence 88 — but for `tool_deny` and the budgets, not for the trust ceiling.**

**Reachability is not in doubt.** I checked the two things that could have killed this:

- The tool is registered **unconditionally** — `toolreg.Register(specOverseer())` at
  `plugins/builtintools/tools.go:70`, with no env gate (contrast the "env-gated externals" block at `:82-85`).
- `CapOversee` is `LevelAllow`, but not as a special case: `DefaultLevels()` (`kernel/edict/edict.go:634-640`)
  sets **every** capability to `LevelAllow`. Per the posture rule, that alone is not the finding.

**The structural fact that makes a caller check impossible.** `func (t *Tool) Invoke(_ context.Context, raw json.RawMessage)`
(`plugins/tools/overseertool/tool.go:86`) — the context is discarded at the signature. The overseer tool cannot
identify its caller even in principle, so `ref == self` cannot be rejected at the tool layer, and `EditAgent`
(`kernelsource.go:104`) takes no caller argument either. The only two guards are `fleetLock` (`:105`, off by
default and documented as deliberately opt-in) and `cur.System` (`:112`). There is no monotonicity check on any
field: `trust_ceiling` (`:136`), `max_cost_mc` (`:141-143`), `max_daily_mc` (`:144-146`), `tool_allow`/`tool_deny`
(`:162-167`), `retry_policy` (`:150-152`), `config_overrides` (`:168-170`) are all patched wholesale.

**Impact, split by what actually bites:**

- **`trust_ceiling` raise — no impact.** Under the shipped `AskPolicy=AskAllow`, L1–L3 already resolve to
  `DecisionAllow`, so raising a ceiling to L4 changes nothing that was not already permitted. I decline to
  count this, per the posture rule. (It becomes live the day PRIVESC-001 is fixed.)
- **`tool_deny` clearing — real, and the strongest half.** The per-agent tool denylist is the one per-agent
  restriction that is *not* subject to the `AskAllow` fold: `agentToolPolicyDenial` (`kernel/runtime/policy.go:115-121`)
  sets `HardDenied: true` and returns immediately. It is what still constrains the shipped guardians
  (`ToolDeny: ["memory"]`). An agent clearing its own `tool_deny` escapes the only effective per-agent tool
  restriction the product has.
- **`max_daily_mc` / `max_cost_mc` raise — real.** Governor budget refusals are hard and unaffected by `AskPolicy`.
  The restricted party removing its own operator-set spend ceiling is a straightforward control bypass.
- **Durability confirmed.** `reconcileExistingGuardian` (`plugins/builtinguardians/builtinguardians.go:233-260`)
  re-clamps `ToolDeny` / `MaxCostMc` / `MaxDailyMc` / `TrustCeiling` at every boot — but it returns immediately
  `if !p.System`. A non-System agent's self-granted budget and cleared denylist persist indefinitely.

**Why this is not just the default-allow posture.** Default-allow says a capability *starts* permitted;
it does not say a restriction, once an operator sets it, may be removed by the party it restricts. The
codebase already accepts exactly this reasoning — it is the stated rationale for the `cur.System` guard
eleven lines above (`kernelsource.go:97-103`). The gap is that the same reasoning was not extended to
self-targeting. `op=create` is the same escalation in one step (`CreateAgent` forces `System = false` but
accepts any `max_daily_mc` and any empty `tool_deny`), and `op=wake` then dispatches the new identity
with an arbitrary intent and no delegation-authorization check (`kernelsource.go:389-408`).

---

## PRIVESC-006 — `op=repair` bypasses the System-guardian guard — **PLAUSIBLE**

**Verdict: PLAUSIBLE — the guard-parity bug is real and worth fixing, but three of the finding's impact claims
do not hold. Severity Medium (down from High), confidence ~70.**

### Confirmed

`RepairAgent` (`plugins/tools/overseertool/kernelsource.go:506-519`) checks exactly four things: the agent
exists, is not retired, is enabled, and `AllowsDirectCall()`. There is **no `p.System` check and no `fleetLock`
check** — verified against `EditAgent` (`:105`, `:112`), `CreateAgent` (`:200`) and `DeleteAgent` (`:211`), all of
which have one or both. Guardians qualify: seeded `System: true` with no `DirectCallable`
(`builtinguardians.go:206-218`), and `AllowsDirectCall()` returns `p.DirectCallable == nil || *p.DirectCallable`
(`roster.go:224-226`) → true. `reason` is interpolated verbatim (`repair.go:28-30`), the brief ends with
"That block will be applied automatically" (`repair.go:104`), and `applyRepairProposal` (`repair.go:207-236`)
writes the parsed fields through `UpdateProfile`. No test covers the System case for repair, while
`overseer_test.go` covers it for edit/create/clone/delete — so this reads as an oversight, as the report says.

### Why it is downgraded

**1. Reaching a guardian's prompt with attacker text is not novel.** `op=wake` (`kernelsource.go:389-408`)
performs the *same* four checks — exists, not retired, enabled, `AllowsDirectCall()` — with no System guard
and no fleetLock, and then runs the agent with a **fully attacker-authored intent**
(`RunWith(ctx, corr, intent)`). That is a documented, first-class op in the tool schema. So "an attacker can
steer a System guardian's run" is already available by design; the only delta `repair` adds is the
auto-applied profile write.

**2. The write surface excludes every authority field.** `applyRepairProposal` writes `Soul`, `Model`,
`Fallbacks`, `TaskType`, `ConfigOverrides` — and nothing else. No `trust_ceiling`, no `tool_allow`/`tool_deny`,
no `max_cost_mc`/`max_daily_mc`. The `ConfigOverrides` reach is bounded by a closed table of nine knobs
(`kernel/runtime/agentconfig.go:51-70`: `AGEZT_MODEL`, `MAX_ITER`, `MAX_AUTO_CONTINUE`, `AUTO_CONTINUE_WAIT`,
`PARALLEL_TOOLS`, `TOOL_DISCOVERY_MAX`, `CONTEXT_BUDGET`, `OBSERVATION_DELTAS`, `DISABLE_HEURISTIC_BYPASS`) —
none of which is a budget, routing, or authority knob. The report's phrase "per-agent `AGEZT_*` runtime knobs"
is accurate but reads broader than the nine-entry reality.

**3. The write is not deterministic.** It requires the *guardian's own model* to emit a fenced JSON block in
its final answer, steered by the injected `reason`. That is a prompt-injection success probability, not a
guaranteed write — materially weaker than `op=edit`, which writes unconditionally.

**4. Partially self-healing — with one exception that is the real finding.** `reconcileExistingGuardian`
(`builtinguardians.go:233-260`) runs at every boot for System profiles and re-clamps `ToolDeny`, `MaxCostMc`,
`MaxDailyMc`, `TrustCeiling`, `MemoryScope` and `NoisePolicy`. It does **not** touch `Soul`. So a
repair-injected **system-prompt rewrite of a protected guardian persists across restarts** — that is the
concrete, durable harm, and it is exactly what the `EditAgent` System guard's rationale comment says the guard
exists to prevent ("rewrite a guardian's Soul … and behaviorally defang it").

The remediation stands as written — add `cur.System` and `fleetLock` to `RepairAgent`, ideally via one shared
precondition helper — but this is a guard-parity/consistency bug with a probabilistic, prompt-shaped trigger
and a bounded write surface, not a High-severity authority bypass.

---

## Notes on this pass

- Read-only. No source file was modified, nothing was executed, and nothing was run against the owner's real `~/.agezt`.
- Findings not in scope of this pass (PRIVESC-001, PRIVESC-004, PRIVESC-007/008, all BIZ-*, RACE-*, SESS-*)
  were not re-verified here and carry their original ratings from `injection-results.md`.
- One cross-cutting observation for the fix plan: PRIVESC-002's residual and the `roster.json`/`standing.json`
  equivalence are the same issue. If the daemon home is to be a trust boundary, it needs one authentication
  scheme across every store; if it is not, the resume ticket needs no MAC and the finding is closed.
