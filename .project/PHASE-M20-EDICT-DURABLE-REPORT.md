# Phase Report — Milestone M20 (Durable runtime policy)

> Status: **shipped** · Date: 2026-05-31
> DECISIONS F3/F4 · B1 (event-sourcing). M18 and M19 made the policy engine
> runtime-manageable (deny rules, trust levels) but the changes were lost on
> restart. M20 makes them durable by replaying the `policy.changed` events that
> were already in the journal — no new storage, the journal *is* the store.

## Why

M18 (runtime deny rules) and M19 (runtime trust levels) both ended with the same
named follow-up: the changes live only in the running engine and revert to the
boot config on restart. For an operator who locked a capability down or added a
deny mid-incident, a daemon bounce silently undoing that is a safety regression
exactly when it matters.

The fix is the project's founding principle, not new machinery: **everything is
an event**. Every M18/M19 mutation already journals a `policy.changed` event
into the hash-chained log. The engine's runtime state is therefore a *projection*
of those events — and a projection can be rebuilt by replaying them. M20 closes
the loop: on boot, fold the `policy.changed` history into a net overlay and apply
it onto the freshly-built engine. No sidecar file, no new format, no second
source of truth that could drift from the journal.

**Opt-in, deliberately.** Restoring a deny rule is security-positive; restoring a
level *loosening* could be a footgun (an operator opens `http.post` for one task,
restarts a week later, and it's silently still open). So durability is requested
explicitly via `AGEZT_EDICT_DURABLE=on`, and the banner says exactly what was
restored. Default behaviour is unchanged: a fresh boot is a fresh policy.

## What shipped

- **`edict.ProjectPolicyChanges([]PolicyChange) PolicyOverlay`** — a pure fold,
  no engine and no I/O, so it is exhaustively unit-testable. `PolicyChange`
  mirrors the `policy.changed` payload (`action`, `capability`, `to`, `name`,
  `substring`, `applies_to`) so a journal payload unmarshals straight into it.
  Semantics: `level.set` is last-wins per capability; `deny.add`/`deny.rm` are
  bookkept by the rule's journaled name (an add later removed leaves no trace),
  survivors keep add order; malformed entries (blank capability/substring,
  unparseable level) are skipped — one bad historical event must never wedge a
  restart. `PolicyOverlay.IsEmpty()` lets the caller skip a no-op apply.
- **`Engine.ApplyOverlay(PolicyOverlay) (levels, rules int)`** — applies the
  overlay (levels via `SetLevel`, surviving deny rules via `AddHardDeny`, each
  re-assigned a fresh `runtime[N]` name) and returns the counts for the banner.
- **Daemon wiring (`cmd/agezt`)** — `replayPolicyOverlay(k)` ranges the journal,
  decodes `policy.changed` events into `[]edict.PolicyChange`, and projects them.
  Gated on `AGEZT_EDICT_DURABLE=on`, applied after the engine is built and the
  bus/redactor are wired but **before any Run is dispatched**. The policy banner
  gains `; durable=on (restored N level(s), M deny rule(s))`.

| Env var | Meaning |
|---|---|
| `AGEZT_EDICT_DURABLE` | `on` → replay journaled `policy.changed` events at boot to restore runtime deny rules + level changes. Default off (fresh boot). |

No `go.mod` change (`encoding/json` + the existing journal `Range` are stdlib /
in-tree). No new event kind — M20 *consumes* the `policy.changed` events M18/M19
already emit.

## Proven

- **Unit (engine):** `ProjectPolicyChanges` over a realistic history — two adds,
  one remove, a level changed twice (last wins), plus a bad-level and a
  blank-substring entry that must be skipped — yields exactly the surviving rule
  + final level; empty history → empty overlay; `ApplyOverlay` restores a level
  and a deny rule onto a fresh engine, the rule fires, and it is re-named with
  the runtime prefix.
- **Live (full daemon restart, mock provider):**
  - Session 1 (`AGEZT_EDICT_DURABLE=on`): `edict deny add "shell:kubectl delete"`
    → `runtime[1]`; `edict level http.post L0`. Both fire.
  - **Restart** (same base dir, durable on): banner reads
    `durable=on (restored 1 level(s), 1 deny rule(s))`; `kubectl delete` is still
    hard-denied by `runtime[1]` and `http.post` is still L0 — **without
    re-adding either**; `deny list` shows the rule tagged `[runtime]`; `rm -rf /`
    is still floored.
  - **Restart without durable:** banner has no restore clause; `kubectl delete`
    is back to allow and `http.post` back to its L1 boot default — the opt-in
    contract holds and the journal is left untouched (the events remain; they're
    simply not replayed).

3 new tests; suite **1171** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Per-tenant durability** — M14 tenants get their own engine but no reload
  hook; the same replay over each tenant's journal would make their runtime
  policy durable too (pairs with the still-open per-tenant deny/level items).
- **Compaction** — replay cost is O(all `policy.changed` events). For a daemon
  whose policy churns heavily over a long life, a periodic snapshot (or a
  journal compaction pass) would bound boot time; not a concern at current
  scale.
- **`AskPolicy` durability** — once a runtime `agt edict mode` setter exists
  (named deferred in M19), its events would flow through this same replay with
  no new mechanism.

## Arc

M20 closes the runtime-policy surface opened by M18/M19: the policy engine is now
fully manageable at runtime **and** that management is durable, all through the
one journal the rest of the system already trusts. Combined security arc to date:
M14 tenant isolation · M15 redaction · M16 egress guard · M17 operator deny floor
· M18 runtime deny rules · M19 runtime trust levels · M20 durable runtime policy.
