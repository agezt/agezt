# Phase Report — Milestone M85 (`agt memory log`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt memory list` shows the CURRENT memory records (the projection). Nothing
showed the HISTORY of how that state came to be — what the agent learned, forgot,
and replaced, and when. For a persistent-memory agent, that provenance is a trust
surface: it answers "why does it believe this?" and "when did it forget that?".
The journal already records `memory.written` / `memory.forgotten` /
`memory.superseded`; M85 adds `agt memory log` over them.

## What shipped

- **Server `handleMemoryLog` (`memory_log.go`)** — folds the three memory
  lifecycle events into one timeline (op, id, type, subject), newest-first,
  limited, with an `op` filter (written|forgotten|superseded, write/revive both
  matching `memory.written`) and the shared `--since` window.
- **CLI `agt memory log [N] [--op <o>] [--since <dur>] [--json]`** — renders
  `<time> <op> <type> <subject> (<id>)`.

## Design decisions

- **List = state, log = history.** The two are complementary: `memory list`
  answers "what does it know now?", `memory log` answers "how did it get there?".
- **Honour the real action verb.** `memory.written` carries `action`
  (create/revive); the log shows it verbatim so a reinforcement reads differently
  from a first write.

## Tests

- `TestMemoryLog_ListsAndFilters` — a write + forget + supersede: all three
  newest-first (supersede first), `--op forgotten` returns just the forget.

Test count: **1328 → 1329**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt memory add "deploy process" "..."   # then a second add, then forget the first
$ agt memory log
  2026-06-01 14:35:44  forget     deploy process  (ab25…)
  2026-06-01 14:35:44  create    FACT          user prefers  (a511…)
  2026-06-01 14:35:44  create    FACT          deploy process  (ab25…)
$ agt memory log --op forgotten
  2026-06-01 14:35:44  forget     deploy process  (ab25…)
```
