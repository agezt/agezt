# Phase Report — Milestone M86 (`agt world log`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt world list` shows the CURRENT knowledge graph (the projection). Nothing
showed the HISTORY of how it formed — what entities and relations the agent
observed, reinforced, and forgot, and when. M86 adds `agt world log`, the
world-model analogue of `agt memory log` (M85): both are knowledge stores, both
now keep an audit timeline of how their state came to be.

## What shipped

- **Server `handleWorldLog` (`world_log.go`)** — folds
  worldmodel.entity.upserted / relation.upserted / forgotten into one timeline
  (op verb, what=entity|relation, label), newest-first, limited, with a `kind`
  filter (entity|relation) and the shared `--since` window.
- **CLI `agt world log [N] [--kind entity|relation] [--since <dur>] [--json]`** —
  renders `<time> <op> <what> <label>` (entity name, or the relation triple).

## Design decisions

- **Faithful to the journal.** Entity ops show the entity name + kind; relation
  ops show the `from verb to` triple as journaled (the from/to are the resolved
  entity ids — the audit form, not re-resolved to names, so the log reflects
  exactly what was recorded).
- **Symmetry with memory log.** Same shape (`ops: [{ts, op, …}]`), same `--op`/
  `--kind` + `--since` filter philosophy, so the two knowledge-store audits read
  alike.

## Tests

- `TestWorldLog_ListsAndFilters` — an entity upsert + relation upsert + forget:
  all three newest-first; `--kind relation` returns just the relation with its
  `from verb to` label.

Test count: **1329 → 1330**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt world add "Ada Lovelace" --kind person   # + a project entity + a relation
$ agt world log
  2026-06-01 14:38:46  create    relation  <id> wrote-notes-for <id>
  2026-06-01 14:38:46  create    entity    Analytical Engine [project]
  2026-06-01 14:38:46  create    entity    Ada Lovelace [person]
$ agt world log --kind relation
  2026-06-01 14:38:46  create    relation  <id> wrote-notes-for <id>
```
