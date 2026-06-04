# M383 — Granular `agt journal export --scope task:<correlation>` (SPEC-09 §3)

## SPEC audit (read-vs-code)
SPEC-09 §3 ("Granular export"): "Event-sourcing + IDs make surgical extraction
natural — you can 'cut' any subgraph by following correlation_id/causation_id …
`agt export --scope agent:<ulid>` etc."

**Verified gap:** `agt journal export` (M101) supported `--since <dur>` (a
contiguous time window) and `--out`, but **no `--scope`** — there was no way to
extract just one run's events. Confirmed by reading the CLI + the
`handleJournalExport` control-plane handler (only `since_ms` was honoured).

The realizable scope today is **Task/workflow** = one run = one `correlation_id`
(the SPEC table's "a single task's DAG + its events — reproducible bundle of one
run"). Agezt agents are ephemeral per-run, so `agent:`/`tenant:`/`skill:`/
`memory:` group differently and are explicitly NOT claimed (rejected with a clear
message) rather than mis-cut.

## Key design point — verification of a non-contiguous cut
A scope is a **cut**, not a window: its events do not chain to each other
(prev_hash links into the full journal), and the last event is not the chain
head. So the existing `verifyBundleEvents` (prev-hash continuity) and
`checkBundleCompleteness` (reaches-head) checks do not apply. A scoped bundle is
re-verified offline by `verifyScopedBundleEvents`: per-event BLAKE3 recompute
(tamper detection) + every event belongs to the scope's correlation (no foreign
event smuggled in). Together these prove the cut is untampered and is exactly the
named run's subgraph.

## What
- **`kernel/controlplane/journal_export.go`** — `handleJournalExport` honours an
  optional `correlation` arg: events not matching are skipped; the result echoes
  `correlation`. Composes with `since_ms`.
- **`cmd/agt/journal_export.go`** — `--scope <spec>` flag; `scopeCorrelation`
  parses `task:<corr>` / bare `<corr>` (rejects the unsupported prefixes);
  manifest gains a `scope` field marking the cut; `cmdJournalVerify` branches to
  the scoped path when `scope` is set; new `verifyScopedBundleEvents`.

## Verification
- **`cmd/agt/journal_export_scope_test.go`** — `TestScopeCorrelation` (parse
  matrix incl. trimmed/empty/unsupported prefixes); `TestVerifyScopedBundleEvents`
  (non-contiguous cut verifies; tampered payload rejected; foreign-correlation
  event rejected; empty ok).
- **`kernel/controlplane/journal_export_test.go`** —
  `TestJournalExportScopedByCorrelation`: two runs, scoped export returns only
  the target run's events (count < full), every event carries the target
  correlation and still hash-verifies.
- **Negative control:** disabling the daemon-side correlation filter → the
  scoped export leaks the other run's events → the membership assertion FAILs;
  restored `journal_export.go` byte-identical.
- **Live demo:** two runs (ALPHA, BRAVO); `agt journal export --scope
  task:<ALPHA-corr> --out alpha.bundle` → "exported 6 event(s) for scope …";
  `agt journal verify --bundle alpha.bundle` → "scoped bundle OK: 6 event(s)
  verified for task:…" (offline); the bundle contains only ALPHA's correlation
  (BRAVO excluded) and the `scope` manifest field.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2165** passing (was 2162; +3). CHANGELOG (Added, user-visible).

## Scope notes
- Closes the **Task** row of SPEC-09 §3. The remaining scopes
  (agent/tenant/skill/memory/standing-order) are larger groupings (multi-run,
  world-model neighborhoods) — recorded in next.md, not claimed, and rejected by
  the CLI with a clear message so the surface never silently mis-cuts.
- `agt restore` already does point-in-time (M371); restoring a scoped cut into a
  fresh home would need the import path to accept a non-contiguous set — a
  candidate follow-up (the export + offline verify is the SPEC-09 §3 deliverable).
