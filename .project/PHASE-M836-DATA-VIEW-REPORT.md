# Phase M836 — Data Lake Web UI (Data view + control-plane routes)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "bunları gösterecek
database viewer sistemi de lazım tabi webui da … hepsini iyi göster." Milestone 3
of the Data Lake arc (after M834 engine + M835 built-ins).

## What shipped

The human window onto the Personal Data Lake — see, search, and hand-edit the
collections agents build with the `db` tool.

**Control plane (`kernel/controlplane/datalake.go` + protocol/server):**
seven commands over the existing `s.k.DataLake()` —
`data_collections` (list + counts), `data_records` (query: search/sort/desc/
limit/offset), `data_insert`, `data_update`, `data_delete`, plus
`data_create_collection` / `data_drop_collection`. Operator-initiated rows are
stamped `created_by: operator` (provenance, distinct from an agent's run id).

**Web UI routes (`kernel/webui/webui.go`):** `/api/data/collections` (GET, read
allowlist), `/api/data/records` (GET, args), `/api/data/insert` + `/api/data/update`
+ `/api/data/collection` (POST JSON bodies), `/api/data/delete` + `/api/data/drop`
(POST query-arg). Same allowlist discipline as every other route.

**Frontend (`frontend/src/views/Data.tsx`, nav "Data Lake"):** a collection
sidebar (icon + title + count, built-ins marked with a lock) and a records panel
— a generic table whose columns come from the collection's schema fields (or are
inferred from record keys), with debounced search, an Add/Edit modal that
type-coerces inputs by field type (number/money → number, bool, tags → list,
note → textarea), and per-row delete. Bespoke per-`view` app layouts
(expense/calendar/…) layer on top in a later milestone.

## Verification

- **Unit/guard:** webui read-only guard extended for `data_collections`; webui +
  controlplane Go tests green; frontend `tsc` + 512 vitest tests green.
- **Live HTTP** (isolated home, real daemon on :8799): `GET /api/data/collections`
  → all **7 built-ins** with their `view` hints; `POST /api/data/insert` into
  `tasks` → record created (`created_by: operator`); `GET /api/data/records?
  collection=tasks` → the row back with `schema.view=tasks`.

## Gate

controlplane + webui Go tests green; frontend tsc + vitest green; vet +
staticcheck + linux cross-build clean; dist rebuilt and committed (LF, in sync
with a fresh build per `.gitattributes`). go.mod unchanged.

## Next

M837+: bespoke per-collection app views keyed off `schema.view` — an expense
tracker that looks like a real app (totals, charts), a calendar grid, a bookmarks
board, etc. — plus create/drop-collection from the UI.
