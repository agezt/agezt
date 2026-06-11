# Phase M834 — Personal Data Lake (engine + `db` tool)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "custom veritabanı
yaratabilme … veri ekleme çıkarma arama … custom databases ajanların kullanımına
açık olmalı … birden çok agent tarafından kullanılabilir … chat ortamından
erişebilirim … database viewer … built-in expense/calendar/tasks/notes/habits/
bookmarks/contacts … personal data lake."

This is **milestone 1** of the Data Lake arc: the storage engine + the agent
tool. Built-in collections (expense/calendar/…) and the bespoke Web UI app views
follow in M835+.

## Decision: file-based, no DB dependency

The owner was unsure (sqlite vs postgres). AGEZT is a single static binary with a
frozen go.mod and a file-based architecture (journal, board, memory, artifacts
are all on-disk). Pulling in SQLite (CGO) or Postgres (a server) would break that.
So the Data Lake is a **pure-Go, dependency-free, on-disk structured store** —
the same shape as the existing stores — surfaced as a DB-like API.

## What shipped

**`kernel/datalake`** — a `Lake` of named **collections** (tables), each a set of
JSON **records** with an optional **schema**:

- Layout: `<base>/datalake/<collection>/_schema.json` + `…/rec/<id>.json` (one
  file per record), in-memory index loaded at Open, single mutex. Collections are
  shared across every agent on the daemon.
- API: `CreateCollection`/`EnsureCollection`(idempotent seed)/`DropCollection`
  (system collections protected); `Insert`/`Get`/`Update`(field-merge, nil
  deletes a key)/`Delete`; `Query{Search, Equals, SortBy, Desc, Limit, Offset}`
  (case-insensitive search across string fields, exact-match filter, numeric-or-
  string sort); `ListCollections`/`Schema`/`Count`.
- **Provenance** (serves the owner's "hangi agent ekledi" ask): every record and
  collection records `created_by` / `updated_by` (the run correlation, which maps
  to an agent in the journal) and `created_ms` / `updated_ms`.
- Schema carries a `view` hint (`table` | `expense` | `calendar` | `tasks` |
  `notes` | `habits` | `bookmarks` | `contacts`) + `icon`, so M835+ can render
  bespoke app-like front-ends.

**`plugins/tools/db`** — the `db` tool: ops `list_collections`,
`create_collection`, `drop_collection`, `insert`, `get`, `update`, `delete`,
`query`. Decoupled via a `Store` interface; the daemon injects `k.DataLake()`
after Open. Maps to the **memory** capability (structured durable knowledge) — no
new grant, no new env var.

## Wiring

- `kernel/runtime`: `datalake.Open` at boot, `k.DataLake()` accessor, struct field.
- `cmd/agezt`: register `db(data-lake)` in the tools line, inject the store.
- `kernel/edict/toolmap.go`: `case "db": return CapMemory`.

## Verification

- **Unit** (`kernel/datalake`): create/dup-reject/invalid-name; insert+provenance;
  query search/equals/sort/limit; update merge + nil-delete + provenance bump;
  delete; system-collection drop protection; EnsureCollection idempotency;
  persistence across reopen.
- **Unit** (`plugins/tools/db`): full create→insert→query→update→get→delete
  lifecycle over a real temp-dir lake; rejections (bad op, missing
  collection/id, unknown collection, no store); drop-missing soft error.
- **Live** (isolated home, real deepseek): one prompt had the agent create an
  `expenses` collection (`view=expense`), insert two records, and query sorted by
  amount desc — correct result; on disk, the schema + 2 record files carry
  `created_by` = the run correlation. `db(data-lake)` shows in the tools banner.

## Gate

datalake + db + edict + runtime + cmd/agezt tests green; vet + staticcheck +
linux cross-build clean; gofmt swept. go.mod unchanged (stdlib + ulid only).

## Next (Data Lake arc)

- M835: seed the built-in collections (expense/calendar/tasks/notes/habits/
  bookmarks/contacts) at boot via `EnsureCollection`, with field schemas + view
  hints.
- M836+: control-plane list/CRUD routes + a Web UI **Data** view — generic table
  viewer, then bespoke app views per built-in (expense tracker that looks like a
  real app, calendar, bookmarks, …).
