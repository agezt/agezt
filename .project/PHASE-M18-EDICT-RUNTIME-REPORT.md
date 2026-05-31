# Phase Report — Milestone M18 (Runtime-managed hard-deny rules)

> Status: **shipped** · Date: 2026-05-31
> DECISIONS F4 / SPEC-06. M17 let operators *seed* extra hard-deny rules at
> boot (`AGEZT_EDICT_DENY`). M18 lets them *manage* the rules on a running
> daemon — add, list, remove — over the control plane, journaled for audit, no
> restart.

## Why

The hard-deny floor is the non-overridable layer of the policy engine: it fires
before any allow, regardless of trust level. M17 made it operator-extensible,
but only at startup — adding a rule (a new threat surfaces, an incident is in
progress) meant editing `AGEZT_EDICT_DENY` and bouncing the daemon, which drops
every in-flight run. A floor you can't adjust live isn't much of a floor during
an incident.

Two requirements shaped the design:

1. **You can tighten but never loosen the boot-time floor.** If runtime
   management could delete a built-in (`rm -rf /`) or an operator-seeded
   (`operator[N]`) rule, a prompt-injected agent that reached the control plane —
   or a careless operator — could disarm the kernel's own safety net. So runtime
   `rm` is restricted to runtime-added rules; the boot floor is immutable at
   runtime.
2. **A policy change is itself a security event.** Edict decisions are already
   journaled (`policy.decision`); a change to the *rules* must be equally
   auditable. Each add/rm emits a `policy.changed` event into the same
   hash-chained journal — so "who loosened the policy and when" is answerable
   from the tamper-evident log, not tribal memory.

## What shipped

- **Engine (`kernel/edict`)** — `AddHardDeny(rule) (HardDenyRule, error)` and
  `RemoveHardDeny(name) (bool, error)`. `Add` always assigns the rule a fresh
  `runtime[N]` name (overwriting any caller-supplied name) so runtime rules are
  unambiguously identifiable; it rejects a blank substring (same footgun
  `ParseDenyRules` guards). `Remove` refuses any name without the `runtime[`
  prefix — built-ins and `operator[N]` rules error rather than being dropped.
  The `RuntimeRulePrefix` constant + `IsRuntimeRule(name)` predicate encode the
  invariant in one place. Both are mutex-guarded; the change takes effect on the
  next `Decide`.
- **Control plane** — `edict_deny_list` (rules + a per-rule `removable` flag),
  `edict_deny_add` (parses one rule via the existing `ParseDenyRules`, so the
  CLI and env-var syntax are identical; rejects multi-rule or empty specs), and
  `edict_deny_rm`. Add and rm publish a `policy.changed` event
  (`actor=operator`, payload carries `action` = `deny.add`/`deny.rm`, the rule
  name/substring/scope, and the new rule count).
- **Event** — `event.KindPolicyChanged` (`"policy.changed"`), registered in the
  validation map.
- **CLI** — `agt edict deny list|add|rm`, wired under the existing `agt edict`
  dispatcher. `list` renders each rule tagged `[floor]` or `[runtime]`; `add`
  echoes the assigned name; `rm` reports the new count (exit 3 if the name
  wasn't found, mirroring `edict test`'s deny exit).

## Proven

- **Unit (engine):** `Add` assigns a runtime name + the rule fires through
  `Decide`; a caller-supplied name is overwritten and names are unique; empty
  substring rejected; `Remove` refuses both a built-in (`rm-rf-root`) and an
  `operator[1]` rule (and the built-in keeps firing afterwards), removes a
  runtime rule cleanly, and returns `false,nil` for an absent runtime name.
- **Unit (control plane):** full add→list→test→rm round-trip (the added rule
  appears `removable`, hard-denies via `edict_test`, then stops after removal);
  removing a floor rule errors; add rejects multi-rule and empty specs.
- **Unit (CLI):** `deny` appears in help; the `deny` dispatcher and its
  `add`/`rm` argument contracts (subcommand required, rule/name required,
  unknown-subcommand) all resolve before any daemon dial.
- **Live (daemon + `agt`):** booted with `AGEZT_EDICT_DENY="git push"`;
  `deny list` shows the built-ins + `operator[1]` all tagged `floor`;
  `deny add "shell:kubectl delete"` → `runtime[1]`, which then hard-denies
  `kubectl delete ns prod` via `edict test`; `deny rm rm-rf-root` and
  `deny rm operator[1]` are both **refused** ("the boot-time deny floor cannot
  be removed at runtime"); `deny rm runtime[1]` succeeds and the command goes
  back to `allow`. The journal holds two `policy.changed` events
  (`deny.add` then `deny.rm`, `actor=operator`) with the rule payloads.

12 new tests; suite **1161** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Per-tenant deny rules** (still global; M14 tenants share the operator floor —
  a tenant could carry its own *additional* runtime rules over its own token).
- **Runtime level changes** (`agt edict level <cap> <Ln>`) — the engine already
  has `SetLevel`; the same control-plane + journal pattern would expose it.
- **Glob/regex rules** beyond substring (named deferred in M17, unchanged).
- **Persistence of runtime rules** — today they live only in the running
  engine; a restart returns to the boot floor. A durable overlay (replayed from
  the `policy.changed` events already in the journal) would make runtime rules
  survive a bounce.
