# Phase M835 — Data Lake built-in collections (seeded at boot)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "built-in olarak
expense tracker, calendar, tasks, notes, habits, bookmarks, contacts gibi
veritabanları olsun, bunları personal data lake olarak isimlendir." Milestone 2
of the Data Lake arc (after M834's engine + `db` tool).

## What shipped (`kernel/datalake/builtins.go`)

- `BuiltinSchemas()` — the seven out-of-the-box collections the owner named, each
  with a title, lucide icon, a `view` hint for the bespoke Web UI app view to
  come, and a field schema (typed hints text/number/money/date/bool/url/tags/note):
  **expenses** (view=expense), **calendar**, **tasks**, **notes**, **habits**,
  **bookmarks**, **contacts**. All marked `Builtin` + `System` (always present,
  undroppable — records inside stay fully agent/user managed).
- `(*Lake).SeedBuiltins(actor)` — `EnsureCollection`s each; idempotent (existing
  ones and their data untouched); returns the names newly created.
- `kernel/runtime` seeds them at boot right after the lake opens — best-effort so
  a seed hiccup never blocks startup, retried next boot.

## Verification

- **Unit:** first seed creates all 7; second seed creates none (idempotent); each
  built-in is present with `Builtin`+`System`+`View` set and refuses `Drop`
  (ErrSystem); a record can still be inserted into a built-in.
- Runtime tests still green (kernel Open seeds without disturbing other suites).

## Gate

datalake + runtime tests green; vet + staticcheck + linux cross-build clean.
go.mod unchanged. (Local Windows `gofmt -l` flags CRLF working-copy files — a
false positive; committed blobs are LF and the linux CI gate is authoritative.)

## Next

M836+: control-plane list/CRUD routes + a Web UI **Data** view — generic table
viewer first, then the bespoke per-built-in app views (expense tracker like a
real app, calendar, bookmarks, …) keyed off each schema's `View`.
