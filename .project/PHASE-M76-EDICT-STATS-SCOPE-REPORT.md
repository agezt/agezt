# Phase Report — Milestone M76 (`agt edict stats` tool & capability scope)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / Edict observability.

## Why

M74 made `agt edict log` filterable by `--tool` / `--capability`, but the
aggregate `agt edict stats` still only windowed by `--since`. So an operator
could list one tool's decisions but couldn't ask "what's the denial RATE for the
shell tool?" — the per-surface filter set was asymmetric. M76 adds the same
`--tool` / `--capability` scope to `edict stats`, completing the symmetry
(log and stats both scope by tool/capability) and matching `tool stats --tool`.

## What shipped

- **Server scope** — `handleEdictStats` skips decisions whose tool/capability
  don't match before counting, so total / allowed / denied / rate /
  denied-by-capability are all computed over the scoped subset.
- **CLI `--tool <name>` / `--capability <cap>`** (alias `--cap`, `=`-forms),
  documented in `--help`, AND-combined with each other and `--since`.

## Design decisions

- **Scope, then aggregate.** The filter runs inside the fold, so every reported
  figure (including the denial rate) reflects the scoped subset — `edict stats
  --tool http` gives http's own denial rate, not the global one.
- **Symmetry over novelty.** Mirrors M74's exact flag surface so an operator who
  learned `edict log --tool` gets `edict stats --tool` for free.

## Tests

- `TestEdictStats_ToolScope` — unscoped total 3; `--tool http` → total 2, denied
  2, denial_rate 1.0.

Test count: **1318 → 1319**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt edict stats --tool shell
  policy decisions (over 1):
    allowed   : 1
    denied    : 0 (hard 0)
    denial    : 0.0%
$ agt edict stats --tool nonexistent
  no policy decisions.
```
