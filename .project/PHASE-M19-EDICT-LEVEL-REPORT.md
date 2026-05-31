# Phase Report — Milestone M19 (Runtime trust-level changes)

> Status: **shipped** · Date: 2026-05-31
> DECISIONS F3. M18 made the hard-deny *floor* runtime-manageable; M19 does the
> same for the other policy layer — the trust ladder (L0 deny .. L4 allow). `agt
> edict level <capability> <level>` changes a capability's level on a running
> daemon, journaled for audit, no restart.

## Why

Edict has two layers: the hard-deny floor (M17/M18) and the per-capability trust
ladder (F3). M18 made the floor adjustable live; the ladder was still boot-only —
set once via env (`AGEZT_APPROVAL_MODE`, per-cap defaults) and frozen until the
next restart. An operator who wanted to lock `shell` down the moment something
looked wrong, or open `http.post` for a known-good task, had to bounce the daemon
and drop every in-flight run. The kernel already had `Engine.SetLevel`; M19
exposes it over the control plane with the same auditing discipline as M18.

Why loosening is safe to allow at runtime (unlike floor `rm`): the trust ladder
is the operator's tuning knob by design — the floor is the catastrophe backstop.
The floor fires *before* and *regardless of* the level, so even setting a
capability to L4 cannot unlock a hard-denied command. That makes level changes
legitimately bidirectional where floor removal is not: `shell=L4` still blocks
`rm -rf /`. The level change is bounded by a guarantee the operator can't switch
off from this surface.

## What shipped

- **Engine (`kernel/edict`)** — `ParseTrustLevel(s) (TrustLevel, error)` accepts
  the canonical `L0`..`L4` labels (case-insensitive — the same vocabulary
  `TrustLevel.String()` emits) and word aliases `deny`/`ask`/`askfirst`/
  `askscoped`/`allow`, so `... shell L4` and `... shell allow` are equivalent.
  Unknown input errors rather than defaulting. `SetLevel`/`Level` already existed.
- **Control plane** — `edict_set_level` validates the capability is known
  (rejecting a typo that would otherwise create a default-deny phantom entry),
  parses the level, captures the previous level, applies the change, and
  publishes a `policy.changed` event (`action=level.set`, `capability`,
  `from`, `to`). Returns `{capability, from, to}`.
- **CLI** — `agt edict level <capability> <level> [--json]`, under the existing
  `agt edict` dispatcher, printing `shell: L2 → L0`. `agt edict show` remains the
  way to read the whole ladder.

## Proven

- **Unit (engine):** `ParseTrustLevel` table — every Lx label, every alias,
  case/space-insensitivity, and the error cases (`""`, `L5`, `permit`, …).
- **Unit (control plane):** a full round-trip flipping `shell` L2→L0 (with
  `from` correctly captured as the F3 default, `edict_test` then denying, and
  `edict_show` reflecting L0) and back to L4 via the `allow` alias; bad-input
  rejection (unknown capability, unparseable level); and the safety guarantee —
  setting `shell=L4` and confirming `rm -rf /` is **still hard-denied**.
- **Unit (CLI):** `level` in help; both-args-required contract; the level
  vocabulary shown in `level --help`.
- **Live (daemon + `agt`):** `edict level shell L0` → `edict test shell "echo hi"`
  denies; `edict level shell allow` → it allows; `edict test shell "rm -rf /"`
  still reports `hard-deny: rm-rf-root` at L4; `edict level shel L4` and
  `edict level shell L9` are both rejected with clear messages. The journal
  holds two `policy.changed` events (`level.set` L2→L0, then L0→L4).

7 new tests; suite **1168** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Per-tenant level changes** (the change is global; M14 tenants share the
  ladder — a tenant could carry its own level overrides over its own token),
  matching the per-tenant deny-rule item still open from M18.
- **AskPolicy at runtime** (`agt edict mode allow|deny|prompt`) — the engine has
  no setter yet; the same control-plane + journal pattern would expose it.
- **Persistence** — like runtime deny rules (M18), level changes live only in the
  running engine and revert to the boot ladder on restart; a durable overlay
  replayed from the `policy.changed` events already in the journal would make
  both survive a bounce. This is now a shared follow-up for the whole
  runtime-policy surface (M18 + M19), best done once for both.
