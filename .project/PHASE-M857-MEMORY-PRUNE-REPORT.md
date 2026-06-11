# PHASE M857 — Memory prune (no memory-bomb)

**Status:** shipped
**Milestone:** M857
**Theme:** Stop memory growing without bound. Owner ask: *"memory arada bir
compact summary… "* — keep memory from becoming a memory-bomb (#36).

## Finding

The **compaction** half already exists: `CmdMemoryConsolidate` (M804) is the LLM
"sleep cycle" that clusters related records and merges each cluster into one,
**superseding** the originals. But consolidation and `Forget` are *soft* deletes
— the superseded/tombstoned records stay on disk forever. Over time that dead
weight is the actual unbounded-growth risk. The missing piece is a **hard prune**
of the soft-deleted graveyard.

## What shipped

- **`Store.Delete(id)`** (kernel/memory) — a hard removal, used only by prune.
- **`Manager.Hygiene(cutoff)`** → `{total, active, tombstoned, superseded,
  prunable}` — the store's health, including how many soft-deleted records are
  older than the cutoff.
- **`Manager.Prune(corr, cutoff, dryRun)`** — hard-removes records that are
  **tombstoned or superseded AND** whose last activity predates the cutoff.
  Active records are never touched (by construction it only deletes already
  soft-deleted, aged-out rows). Journaled as `memory.pruned`. New event kind
  `KindMemoryPruned`.
- **`CmdMemoryPrune {older_than_days, dry_run}`** — confirm-first flow like the
  M845 artifact collector: dry-run reports hygiene + the prunable count; the real
  call prunes. Default age threshold **30 days** (recent deletions stay
  recoverable).
- **Web UI:** a "Prune" button in the Memory view — dry-runs, shows how many old
  deleted records would go, confirms, then prunes.

## Surface

- `kernel/memory/memory.go` — `Store.Delete` + `FileStore.Delete`.
- `kernel/memory/manager.go` — `HygieneStats`, `Hygiene`, `Prune`.
- `kernel/event/kinds.go` — `KindMemoryPruned` (+ registry).
- `kernel/controlplane/{memory,protocol,server}.go` — `handleMemoryPrune`,
  `CmdMemoryPrune`, dispatch.
- `kernel/webui/webui.go` — `/api/memory/prune` write route.
- `frontend/src/views/Memory.tsx` — Prune button + dry-run/confirm flow.
- `kernel/memory/prune_test.go`.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `memory`/`controlplane` green; vitest **517 passed**; dist rebuilt. No new env;
  go.mod unchanged.
- **Unit:** prune removes exactly the tombstoned + superseded records and never
  an active one (incl. a supersession's live successor); the dry-run count
  matches; a recent soft-delete is NOT prunable under a past cutoff.
- **Live (isolated home):** add two records, forget one → hygiene reports
  `tombstoned:1, active:1`; prune correctly **declines** to remove the
  just-forgotten record (it isn't 30 days old — the safety threshold), and the
  active record survives. Removal-when-aged is covered by the unit test.

## Notes
- This complements, not replaces, consolidation: consolidation shrinks *live*
  knowledge into summaries (creating supersessions); prune later reclaims those
  aged-out supersessions. Together they bound memory.
- A periodic auto-prune (on the pulse/cadence) and the skill-usage cleanup half
  of the owner's ask (#37) remain as follow-ups.
