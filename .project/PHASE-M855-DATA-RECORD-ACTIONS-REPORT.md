# PHASE M855 — Data Lake record edit/delete made visible

**Status:** shipped
**Milestone:** M855
**Theme:** Make per-record **edit and delete** usable in the Data Lake browse
view. Owner ask: *"data lake icinde data browse icinde delete edit vs lazım
kayıtlara"* (records in the data-browse view need delete/edit).

## Finding

The capability already existed end to end — control-plane `CmdDataInsert` /
`CmdDataUpdate` / `CmdDataDelete`, the `/api/data/{insert,update,delete}` routes,
and the Data view's `saveRecord` / `delRecord` handlers + edit form. The actual
gap was **discoverability**: the per-row edit/delete controls were rendered
`opacity-0` and only revealed on row hover (`group-hover:opacity-100`), so they
looked absent — especially without a precise hover, or on touch.

## What shipped

- `frontend/src/views/Data.tsx` — the per-row actions are now **always visible**:
  a clear pencil (edit) and trash (delete) icon button per record, with hover
  highlight (accent / bad) and aria-labels. The edit tooltip also surfaces
  provenance ("added by … · updated by …", M851).

No backend change — the routes, handlers, and store CRUD were already present and
verified.

## Verification

- **Gate:** frontend builds; vitest **517 passed**; dist rebuilt (LF).
- **Live (isolated home), the exact routes the UI calls:** `POST
  /api/data/insert` → got an id; `POST /api/data/update` → field edited,
  `updated_by=operator` (provenance flows to the data lake too); `POST
  /api/data/delete` → `deleted:true`; `/api/data/records` → 0 left. The whole
  edit/delete path is functional, now reachable from the UI.

## Notes
- The Data view is still the generic table; bespoke per-collection app layouts
  (expense charts, calendar grid, …) remain the larger #41 follow-up. This
  milestone makes the table's record actions actually usable.
