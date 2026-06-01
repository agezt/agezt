# Phase Report — Milestone M74 (`agt edict log` tool & capability filters)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / Edict observability.

## Why

`agt edict stats` breaks denials down by capability (`net: 5, fs: 2`), but `agt
edict log` couldn't drill into that breakdown — there was no way to ask "show me
the `net` denials" or "every decision for the `shell` tool". The stats pointed at
a problem; the log couldn't zoom in on it. M74 adds `--tool` and `--capability`
filters, the drill-down from the aggregate, and brings `edict log` to filter
parity with `agt tool log --tool`.

## What shipped

- **Server `tool` + `capability` filters** — `handleEdictLog` skips decisions
  whose tool/capability don't match, during the same journal fold (composes with
  the existing `--denied` and `--since` filters).
- **CLI `--tool <name>` and `--capability <cap>`** (with a `--cap` alias and
  `--tool=`/`--capability=`/`--cap=` forms), documented in `--help`.

## Design decisions

- **Both filters, AND-combined.** `edict log --capability net --denied` answers
  "the network denials" exactly; each filter narrows independently during the
  fold, so they stack predictably.
- **`--cap` alias.** `capability` is the precise term (and matches the JSON
  field + stats key), but `--cap` is the one operators actually type; both map to
  the same arg.

## Tests

- `TestEdictLog_ToolAndCapabilityFilters` — `--tool shell` → 1; `--capability
  net` → 2 (and every row's capability is `net`); `--capability net --denied` → 2.

Test count: **1316 → 1317**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt edict log --tool shell
  2026-06-01 13:57:29  allow  shell  shell  (level L2; AskPolicy=AskAllow …)
$ agt edict log --cap shell
  2026-06-01 13:57:29  allow  shell  shell  (level L2; AskPolicy=AskAllow …)
$ agt edict log --tool nonexistent
  no policy decisions journaled yet.
```
