# PHASE M860 — Data Lake bespoke views (calendar, habits, notes, bookmarks, contacts)

**Status:** shipped
**Milestone:** M860 (numbered M860 to avoid colliding with concurrent in-progress
M858 parallel-tools / M859 async-delegation work in the working tree).
**Theme:** Finish the app-like layouts for the remaining built-in collections,
after M856 did expense + tasks. Owner direction: keep making the Data Lake feel
like real apps (#41).

## What shipped

`frontend/src/views/Data.tsx` now renders a bespoke layout for every built-in
collection's `view` hint (generic table remains the fallback for custom ones):

- **`calendar` → CalendarView** — an agenda grouped into Upcoming / Past, soonest
  first, each event a date chip + title + time/location.
- **`habits` → HabitsView** — streak cards (a flame + the day-count, biggest
  streak first), with cadence + last-done.
- **`notes` → NotesView** — a card grid (title + body excerpt + tag badges).
- **`bookmarks` → BookmarksView** — a link list with openable URLs (new tab) and
  tags.
- **`contacts` → ContactsView** — a card grid (avatar + name + company, with
  mailto email and phone), alphabetized.

All reuse the shared `RowActions` (always-visible edit/delete, M855) and the
existing `/api/data/{update,delete}` routes. New helpers: `str`, `tagList`.

## Verification

- **Gate:** frontend builds; vitest **517 passed** (one ConfigCenter flake in the
  parallel run passed cleanly in isolation and on re-run); dist rebuilt (LF).
- **Live daemon smoke deliberately skipped:** another session was concurrently
  editing the Go kernel (M858/M859 work, mid-flux) — building a daemon would have
  compiled that in-progress state. These views are pure renders over the
  documented built-in schemas (kernel/datalake/builtins.go: calendar
  title/date/time/location, habits name/streak/last/cadence, notes title/body/tags,
  bookmarks title/url/tags, contacts name/email/phone/company), and the M855/M856
  CRUD + expense/task views already validated the data round-trip.

## Notes
- This milestone is **frontend-only** by design — it touches none of the files
  the concurrent M858/M859 work is editing (`agent.go`, `subagent.go`,
  `runtime.go`, `main.go`, etc.), so it merges without interfering.
- The Data Lake's seven built-in collections now all have app-like views;
  agent-created custom collections still use the editable table.
