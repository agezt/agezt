# PHASE M856 — Data Lake bespoke views (expense tracker + task checklist)

**Status:** shipped
**Milestone:** M856
**Theme:** App-like, per-collection layouts in the Data Lake instead of one
generic table. Owner ask (carried from the Data Lake thread): *"expense tracker
gerçekten app gibi olsun"* — the expense collection should look like a real app
(#41).

Each built-in collection already declares a `view` hint (`expense`, `tasks`,
`calendar`, …); M856 turns the two flagship hints into bespoke renderers, keeping
the generic table as the fallback for everything else.

## What shipped

`frontend/src/views/Data.tsx` dispatches on `activeCol.view`:

- **`expense` → ExpenseView** — an app-like layout: three summary cards (Total /
  This month / Records), a **by-category** breakdown with proportional bars
  (top 8), and the recent-expenses list (date · item · category badge · amount),
  each row with edit/delete. Amounts are summed/grouped from the records.
- **`tasks` → TasksView** — a checklist: pending above done, each task a clickable
  check (toggles `done` via a record update), with priority badge and due date;
  done tasks struck through. Plus edit/delete per row.
- Shared `RowActions` (always-visible pencil/trash, consistent with M855),
  `SummaryCard`, and small coercion helpers (`truthy`, `num`, `fmtMoney`).

No backend change — the views read the same `/api/data/records` and mutate via
the existing `/api/data/{update,delete}` routes.

## Verification

- **Gate:** frontend builds; vitest **517 passed**; dist rebuilt (LF).
- **Live (isolated home):** seeded two expenses (4.50 food, 22 transport) → the
  records carry numeric `amount` + `category`, total **26.50** (the summary +
  by-category math the view does); inserted a task and toggled `done:true` via
  `/api/data/update` (the checklist's toggle path) — confirmed it flipped.

## Notes
- The other collections (`calendar`, `habits`, `notes`, `bookmarks`, `contacts`)
  still use the generic table; their `view` hints are ready for the same
  treatment (a calendar agenda, a habit-streak grid, note/bookmark/contact card
  grids) as incremental follow-ups.
- Provenance (M851) shows through: the edit tooltip still surfaces who added/
  updated a record.
