# W5.3 — the frontend's boundaries are lint-enforced

**Status:** done (local)
**Date:** 2026-11-21
**Roadmap row:** W5.3 — *Frontend boundaries lint (features ↛ features, components/lib ↛ features); `designsystem.test` scans `features/`; delete test-only modules.*

## The claim

The console's module graph is now a **ratchet**, not a review habit: two rules,
enforced by a test, with an **empty** debt ledger. The design-system guard
sees the views, not only the shared components. And no module in the tree
exists only to be tested.

## The two rules

**R1 — a feature does not import another feature.** A feature owns its own
vocabulary; what two features share belongs in `lib/` (the shared layer) or in
`components/` (the design system).

**R2 — the shared layer does not reach up into a feature.** `components/` and
`lib/` sit *below* `features/`, so a shared component that needs a feature's
helper needs that helper in `lib/`.

Both rules are one sentence each because the tree is simple enough to say
them in one sentence. That is the point: a boundary rule nobody can state is a
boundary rule nobody can keep.

## The lint

`frontend/src/boundaries.test.ts` (145 lines) resolves the whole import graph
— the `@/` alias, the relative specifiers and the `.ts` / `.tsx` / `index.ts`
spellings — and reports every crossing.

**It has two teeth.** `ALLOWED` is the debt ledger, and:

- a crossing **not on the list** fails (new debt cannot land);
- a crossing **on the list that no longer happens** also fails — a stale
  allowance is a lie about the code, and a ledger that only rejects is a
  ledger that rots.

The list is **empty**: the debt was paid, not parked. A violation that would
need an allowance gets fixed instead.

## What the measurement found

The tree at `c6b8a2ec` (the commit before this slice) was measured on a
clean worktree of that exact commit — not on the working tree, and not from
memory:

| Measurement | Before | After |
|---|---|---|
| R1 crossings (feature → feature) | **35** | **0** |
| R2 crossings (shared layer → feature) | **32** | **0** |
| Total crossings | **67** | **0** |
| Test-only modules (no production importer) | **9** | **0** |
| Orphan modules (no importer at all) | 1 (`main.tsx`) | 1 (`main.tsx`) |
| Stale `vi.mock` paths | 1 | **0** |
| Files scanned | 430 | 411 |

The 67 crossings are the honest number, and they were paid, not reclassified:
nineteen modules moved down into `src/lib/`, the shared views into
`src/components/`, and each feature's *own* view into its feature. The two
feature cycles that existed (`agents ↔ incidents`, `autonomy ↔ incidents`)
came from the shared helpers both sides needed; those helpers now live in
`src/lib/`.

Two "features" were not features at all: `api-keys` and `configcenter` were
shared primitives wearing a feature's name, each reachable from three or more
other features. They are `src/components/apikeys/` and
`src/components/configcenter/` now.

## The design-system guard sees the views

`designsystem.test.ts` scanned `src/components/` **only**. All thirty-odd
views live in `src/features/`, so "one SectionPanel / one tone map / one
StatTile / Page-title-matches-nav" was, over the views, a rule about nothing
— and the codemap said so in as many words ("*effectively vacuous*").

Widening it to `FILES = [components, features]` found **two real copies**:
a local `Metric` in `features/observe/components/MissionControl.tsx` and a
local `Stat` in `features/jarvis/components/Jarvis.tsx`. Both are the shared
`<StatTile>` now.

## The test-only modules are gone

Nine modules were imported only by their own tests — leftovers of retired
views (Catalog, Insights, Routing, Backup snapshot, Toolbox, Dashboard) plus
two components nothing rendered:

`lib/{catalog,insights,routingSuggest,snapshot,telemetry,toolbox}.ts`,
`components/{FlightRecorder,JarvisPresenceCard}.tsx`,
`components/ui/advanced.tsx`.

**knip could not catch them**: it counts a test's import as usage, so it
reported the tree as clean while nine modules sat in it with no production
importer. The measurement did.

Deleting them fell a chain, and the chain had to be paid in full:

- `lib/replay.ts` existed only to serve the deleted `FlightRecorder`
  (`buildReplay` had **no** production caller even before that — the recorder
  imported only the *types*). It and its test are gone.
- `features/voice/index.ts` existed only to re-export `transcribeAudio`,
  which had to move down to `src/lib/voice.ts` because `components/MicButton.tsx`
  needs it and the shared layer may not reach up into a feature. A feature
  barrel forwarding a shared-layer function under a feature-shaped name is a
  second home for one symbol. It is gone, and its three consumers import from
  `@/lib/voice` now.

## What the measurement found that a code review would not

The move left **sixteen sibling tests behind** — the modules moved, their
tests did not. The cause was a bug in the move script: the sibling-test pair
was computed against a file set whose paths were relative to
`frontend/src/`, so the comparison silently failed for every module. The
tests still *passed* (their import specifiers had been rewritten), which is
what makes this the quiet kind of breakage: the tree looked right.

And **four `vi.mock` paths went stale** — a mock whose target moved stops
applying: vitest resolves the path, finds nothing, and the **real** module
runs instead. The test then passes for the wrong reason, or fails loudly, as
`VoiceSetup.test.tsx` did (it asserted on a button that the real
`FieldRow` renders and the mock never did). A fifth, `@/views/Files`, had
been broken long before this slice — `views/` was removed from the tree in
Day 23 — and was a no-op that only made the test look like it isolated
something.

**Only a measurement catches a test that stopped testing.** The lint catches
crossings; it says nothing about whether a test still tests.

## The mutation run

Three mutations, each reverted. A ratchet nobody has tried to break is a
ratchet that does not work.

| # | Mutation | Expected | Result |
|---|---|---|---|
| **A** | `features/observe/components/MissionControl.tsx` imports `features/overseer/components/Overseer` | the boundary lint fails, naming the crossing | **caught** — `R1 features/observe/components/MissionControl.tsx -> features/overseer/components/Overseer.tsx` |
| **B** | that same crossing is **listed** in `ALLOWED` while it does not happen | the second tooth fails (stale allowance) | **caught** — the stale entry is named |
| **C** | `features/jarvis/components/Jarvis.tsx` defines its own `Tile` again | the widened design-system guard fails | **caught** — `src/features/jarvis/components/Jarvis.tsx` |

Mutation B is the one that matters most: a lint with a single tooth would
have accepted B silently, and the ledger would have grown a lie.

All three were reverted; `boundaries.test.ts` and `designsystem.test.ts` are
green (13 assertions) and the measurement is back at 0/0.

## The gates

**Frontend**

- `npx tsc --noEmit` — **0 errors**
- `npm run deadcode` (knip) — **0 issues**
- `npm run build` — the committed `kernel/webui/dist` rebuilt (141 changed
  paths; the bundle is hash-named, so a source move renames every chunk)
- `npx vitest run src/boundaries.test.ts src/designsystem.test.ts` — **13 passed**

**The full vitest package**, measured against a clean `c6b8a2ec` worktree under
the same conditions and compared **by failing test id** (a file's failure
count moves with the machine's load — the 5s timeout under a parallel run — so
the count alone would have been read as a regression):

| | suites | failed suites | tests | failed tests |
|---|---|---|---|---|
| `c6b8a2ec` (baseline) | 566 | 48 | 1495 | 80 |
| working tree | 540 | 40 | 1443 | 76 |

- **70 tests fail in both trees** — the machine's, not the slice's: this Node
  build has no `localStorage` (`ExperimentalWarning: localStorage is not
  available because --localstorage-file was not provided`), so every test that
  touches it fails at `localStorage.clear()`.
- **10 fail only in the baseline**; four of them are in files this slice
  deleted (`components/ui/advanced.test.tsx`,
  `features/configcenter/components/ConfigCenter.test.tsx` — they went with
  their modules), the other six are load-flaky.
- **6 fail only in the working tree** — and **all six pass when run on their
  own** (`Roster`, `Memory` ×2, `Runs`, `Schedules`, `World`: 175 tests,
  green). They are the same load-induced timeouts, landing on a different
  test of the same file.

So: **no test regressed because of this slice**, and the slice *removed* 26
suites and 52 tests (the deleted modules' own tests) while making the
frontend four failing tests fewer.

**Backend** — the Go tree is untouched except one line (below). The full gate
set is green: build, gofmt, vet, `go test ./...`, archcheck, dead-code,
dependency, SDK-generate, structure, changelog-lint, doc-claims, docs-index,
staticcheck.

**The SDKs** — Rust (`cargo fmt --check`, `cargo test`: 4 + 3 doc-tests),
TypeScript (`tsc --noEmit`, 27 tests) and Python (47 tests) are green.

**One Go line changed.** `cmd/agt/compare_data.go` names an **evidence path**
for the onboarding-console capability row, and the path pointed at
`frontend/src/features/configcenter/components/ConfigCenter.tsx` — a file this
slice moved. `TestCompareAudit_EvidenceMissingIsZero` exists precisely to
catch that class of drift (it asserts the right invariant, `EvidenceMissing
== 0`, where the older test masked it), and it went red. The path now names
the file's new home. The gate that caught the drift is the gate doing its
job.


## What this slice did NOT do

- **The kernel's allowlists are untouched.** W5.4 is the slice that takes the
  `archcheck` allowlist and the forbidden-call allowlist to zero.
- **`docs/ARCHITECTURAL-REPORT.md` still names `FlightRecorder`.** It is a
  *dated* report of the tree at `ef7b412d`, and the repo's rule for dated
  documents is that they are left alone. The live codemap
  (`architecture/11-frontend-console.md`) is updated.
- **The `localStorage` failures are not fixed here.** They are an
  environment defect of this machine's Node build, not a defect of the tree,
  and "fixing" them would mean changing tests that are correct.

## The two open ends W5.3 did not touch

- the **message row's second home**: the REST contract's row and the app
  layer's board projection name the same fields — a ratchet ties them, but
  one is a copy;
- the **Go SDK's control plane** (NDJSON) has no golden fixture yet — the
  REST surface is covered, the control plane is not.

## The lesson worth keeping

The measurement found three things a review would not have:

1. **a test that had stopped testing** (the stale `vi.mock`);
2. **a test that had stopped living beside its subject** (the sixteen
   orphaned sibling tests);
3. **a module that existed only to be tested** (the nine).

None of them is a boundary violation. The lint cannot see any of them —
it sees the *graph*, not whether a test still exercises what it names. The
honest reading is that "one home per vocabulary" has two halves: the
**production** graph and the **test** graph. This slice linted the first and
measured the second.

## Evidence files

- `frontend/src/boundaries.test.ts` — the lint (new)
- `frontend/src/designsystem.test.ts` — widened to `features/`
- `architecture/11-frontend-console.md` — the codemap (§6.3, §6.4, §6.5, §8.1,
  §9, §10, §11)
- `architecture/21-migration-roadmap.md` — W5.3 ✅
- `architecture/NEXT.md` — the W5.3 checkpoint
- `CHANGELOG/unreleased/current.md` — Removed + Changed
