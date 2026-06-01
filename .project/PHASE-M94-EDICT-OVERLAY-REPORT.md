# Phase Report — Milestone M94 (`agt edict overlay`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / Edict observability.

## Why

Runtime policy changes (`agt edict level`/`mode`/`deny`) journal `policy.changed`
events, which the daemon folds into a net overlay at boot (M20,
`ProjectPolicyChanges`) to restore "what's in effect" across restarts. But that
NET overlay had no surface: `agt edict show` shows the BASE loaded config, `agt
edict log` shows the raw change events — neither shows the collapsed result. An
operator asking "what runtime policy is actually in effect right now?" had to
mentally fold the log. This is also the demoable, low-risk slice of the
`policy.changed` compaction idea: it surfaces exactly the state a compaction
would snapshot.

## What shipped

- **Server `handleEdictOverlay`** — walks the journal's `policy.changed` events
  and folds them through the *same* `edict.ProjectPolicyChanges` the boot replay
  uses (no new policy logic), returning the net level overrides, surviving
  runtime deny rules, mode override, and the count of changes folded.
- **CLI `agt edict overlay [--tenant <id>] [--json]`** — renders the overlay, or
  "no runtime policy overrides" when the engine runs on its loaded config.

## Design decisions

- **Reuse the boot fold verbatim.** `ProjectPolicyChanges` is the battle-tested,
  last-wins/add-rm-bookkeeping projection the daemon already trusts on every
  boot; the view calls it directly, so what `overlay` shows is exactly what a
  restart would restore. Zero risk of the view and the runtime disagreeing.
- **show = base, log = events, overlay = net.** Three complementary Edict views:
  loaded config, raw change history, and the collapsed current effect.

## Tests

- `TestEdictOverlay_FoldsPolicyChanges` — level last-wins (shell L1→L3 ⇒ L3), a
  mode override, and deny add/add/rm (r2 removed) ⇒ only r1 survives.

Test count: **1338 → 1339**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt edict overlay
  no runtime policy overrides (engine runs on its loaded config).
$ agt edict level shell L3 && agt edict mode deny
$ agt edict overlay
  runtime policy overlay (folded from 2 change(s)):
    mode      : deny
    levels:
      shell          L3
```
