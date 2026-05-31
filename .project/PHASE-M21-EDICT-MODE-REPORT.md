# Phase Report — Milestone M21 (Runtime approval-mode changes)

> Status: **shipped** · Date: 2026-05-31
> DECISIONS F3. The third runtime policy knob: after deny rules (M18) and trust
> levels (M19), `agt edict mode <allow|deny|prompt>` changes the engine-wide
> approval mode on a running daemon — and inherits M20's durability for free.

## Why

Edict's `AskPolicy` decides how Ask-class levels (L1..L3) are folded while live
approval routing matures: `allow` (fold to Allow + journal the would-ask),
`deny` (strict — only L4 runs), `prompt` (block for `agt approve|deny`). It was
boot-only config (`AGEZT_APPROVAL_MODE`). An operator who wanted to slam the
daemon into strict mode the moment something looked wrong — or open it to live
prompting for a supervised session — had to restart and drop every in-flight run.

M18 and M19 made the other two policy layers (the deny floor, the trust ladder)
runtime-manageable; the approval mode was the conspicuous gap. M21 closes it with
the exact same pattern, and because the change is journaled as a `policy.changed`
event it rides M20's replay with **no new durability mechanism**.

The safety story is unchanged and worth restating: the hard-deny floor fires
*before* `AskPolicy` is ever consulted, so no mode — not even `allow` — can relax
a hard-deny. The mode knob only governs the Ask-class middle of the ladder.

## What shipped

- **Engine (`kernel/edict`)** — `AskPolicy.String()` (`allow`/`deny`/`prompt`),
  `ParseAskPolicy(s)` (the inverse; unknown input errors, never a silent
  default), and `SetAskPolicy(p)` (mutex-guarded runtime setter). The control
  plane's old `askPolicyLabel` is now a thin alias over `String()`, removing a
  duplicated enum→label mapping.
- **Durable projection** — `PolicyOverlay` gains a `Mode *AskPolicy` (pointer so
  "no mode change in history" is distinct from `allow`=0); `ProjectPolicyChanges`
  folds `mode.set` events last-wins (skipping unparseable ones); `ApplyOverlay`
  applies the restored mode; `IsEmpty` accounts for it.
- **Control plane** — `edict_set_mode` parses the mode, captures the previous
  mode, applies it, and publishes `policy.changed` (`action=mode.set`,
  `from`/`to`). Returns `{from, to}`.
- **CLI** — `agt edict mode <allow|deny|prompt> [--json]`, printing
  `approval mode: allow → deny`. `agt edict show` already surfaces `ask_policy`.
- **Daemon banner** — when durable replay restores a mode, the banner's durable
  clause appends `; mode=<m>` so the (otherwise boot-derived) mode label isn't
  silently stale.

## Proven

- **Unit (engine):** `ParseAskPolicy`/`String` round-trip + unknown-mode error;
  `SetAskPolicy` takes effect through `Decide` (default allow → `SetAskPolicy(deny)`
  flips an L2 capability to deny) and the floor still fires regardless of mode;
  `ProjectPolicyChanges` folds `mode.set` last-wins (skipping a bogus value) and
  `ApplyOverlay` flips a fresh engine into the projected mode.
- **Unit (control plane):** a round-trip to `deny` (with `from` captured as the
  default `allow`, `edict_show` reflecting `deny`, and an Ask-class shell call
  now denying), a follow-on flip to `prompt`, and an unknown-mode rejection.
- **Unit (CLI):** `mode` in help; mode-required contract; the allow/deny/prompt
  vocabulary in `mode --help`.
- **Live (daemon + restart, mock provider):** `edict mode deny` →
  `approval mode: allow → deny`, `edict show` shows `ask_policy: deny`, and
  ask-class shell denies; an unknown mode is rejected. After a **full restart**
  with `AGEZT_EDICT_DURABLE=on` the banner reads
  `durable=on (restored 0 level(s), 0 deny rule(s); mode=deny)`, `ask_policy` is
  back to `deny` **without re-setting**, and the journal holds the
  `{action:mode.set, from:allow, to:deny}` event.

7 new tests; suite **1178** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc — runtime policy surface complete

M18–M21 make every layer of the policy engine runtime-manageable **and** durable
through the one journal:

| Layer | Set at runtime | Durable (M20 replay) |
|---|---|---|
| Hard-deny floor | `agt edict deny add/rm` (M18) | ✓ `deny.add` / `deny.rm` |
| Trust ladder | `agt edict level` (M19) | ✓ `level.set` |
| Approval mode | `agt edict mode` (M21) | ✓ `mode.set` |

All three emit `policy.changed`, all three replay, all three sit above an
unmovable hard-deny floor. There is no remaining policy state that requires a
restart to change.

## Deferred — named

- **Per-tenant policy** — the recurring item from M18/M19/M20: M14 tenants get
  their own engine but no runtime-management routing or reload hook. Extending
  all three knobs + durability per-tenant (over each tenant's token + journal)
  is the natural fusion of the whole arc, and is now the single largest
  outstanding policy item.
- **Compaction** of the `policy.changed` history (shared with M20) for daemons
  whose policy churns heavily over a long life.
