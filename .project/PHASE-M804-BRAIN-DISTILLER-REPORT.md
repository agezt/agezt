# Phase M804 — brain distiller (memory consolidation pass)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** vision gap #6 (the last
gap-analysis frontier).

## What — the brain's sleep cycle

The per-run auto-distiller extracts facts as tasks complete; over weeks
that accretes many small, overlapping records about the same things.
M804 adds the complementary pass: **consolidation** — cluster related
records, LLM-merge each cluster into ONE concise record, supersede the
originals. Soft, journaled, reversible; nothing destroyed.

**kernel/memory/consolidate.go**:
- `Clusters(rs, threshold, minSize)` — pure: active records embed via the
  M803 local vectors; greedy seed-clustering in deterministic store
  order; threshold 0.55 ("clearly the same thing"), min size 3.
  **Scope is a hard wall**: records private to different scopes never
  share a cluster, and the consolidated record inherits the cluster's
  scope — a private note can never leak into shared memory by being
  "summarized into" it.
- `Manager.DistillBrain(ctx, corr, provider, model)` — one pass: at most
  4 clusters (incremental, cheap timer ticks), each ≤12 records
  (prompt-size guard), TaskType "distill" (same budget class as per-run
  distillation). Per cluster: strict-JSON merge prompt → `Remember`
  (tags source=brain-distill) → supersede each original (idempotent —
  an already-linked record is skipped, so overlapping passes converge).
  Non-JSON answer skips the cluster; provider error aborts the pass
  (budget/network problems shouldn't burn more calls). Journals ONE
  `memory.consolidated` (new kind) with the counts + per-record
  `memory.superseded` — `agt why` explains every merge.

**Surfaces**:
- `agt memory consolidate [--json]` → controlplane `memory_consolidate`
  (sync, 5m cap) → `k.DistillBrain` (halted kernels refuse).
- `POST /api/memory/consolidate` (writeRoute, mirrors /api/reflect/run).
- **Standing surface**: `AGEZT_BRAIN_DISTILL_EVERY` (e.g. 24h) arms a
  daemon ticker (mirrors AGEZT_REFLECT_EVERY; banner shows the cadence;
  in configEnvVars so the guard test passes). Off by default.

## Tests (4 new suites; touched packages + full suite green)

- Clusters: groups the 3 near-duplicates, excludes the private-scope
  record about the SAME topic, excludes unrelated + tombstoned, minSize
- DistillBrain merge: 5 records → 1 consolidated SUMMARY (Frankfurt
  content), 3 superseded links, 1 provider call; second pass idle
  (idempotence, 0 extra calls)
- Non-JSON answer: cluster skipped, store untouched
- Provider required

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider MiniMax-M2)

Seeded 4 near-duplicate kubernetes facts (EN + Turkish mixed) + 1 pizza
fact. `agt memory consolidate` → "consolidated 1 of 1 cluster(s): 4
record(s) merged away (5 → ~2 active)". The real LLM merged across
languages into one clean record ("runs in the frankfurt region and
requires weekly node upgrades"); pizza untouched; memory.consolidated +
memory.superseded journaled under the correlation; M803 typo recall
("kubenetes upgrade") finds the consolidated record. Boot banner with
AGEZT_BRAIN_DISTILL_EVERY=24h: "brain distill : every 24h0m0s".

Gate note: one TestEngine_Start_FiresLive failure was a Windows TempDir
unlinkat cleanup race in kernel/cadence (untouched by this change);
-count=2 rerun green — the known Windows-timing flake class.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green (cadence flake re-run
clean); vet + staticcheck clean; linux cross-build OK; frontend
untouched; go.mod unchanged; AGEZT_BRAIN_DISTILL_EVERY added to
configEnvVars (guard green).

## GAP ANALYSIS COMPLETE (#1–#6)

Profiles · A2A · forge · MCP self-install · vector memory · brain
distiller — every ratified frontier shipped. Remaining backlogs are
polish (workflow refine/history/templates, provider embeddings opt-in,
forge promotion queue, alert routing) and owner-gated CI/billing items.
