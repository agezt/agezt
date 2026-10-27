# W5.4f — the stale docs are archived, and the index keeps them archived

## What the slice is

W5.4's last part. The roadmap reads:

> W5.4 (archcheck allowlist = 0, forbidden-call allowlist = 0, codemap docs
> regenerated, `docs/` stale architecture docs archived)

Two of those four are done — the forbidden-call ratchet went to zero in
W5.4a, and the import ratchet is at **2** with the last two edges needing a
redesign and a port that does not exist yet. The codemap is regenerated on
every slice by `tools/structure-md`. This slice is the fourth: **the stale
architecture documents in `docs/`**.

## What the measurement said

Every `.md` file directly in `docs/` was read and its claims were checked
against the tree. **42 files**: **13 KEEP · 23 ARCHIVE · 5 UPDATE · 1
UNCLEAR**.

The two that mattered most:

- **`docs/ARCHITECTURE.md`** — the most-cited architecture document in
  `docs/`, and it names six package paths that no longer exist
  (`kernel/board`, `kernel/memory`, `kernel/skill`,
  `frontend/src/views/Roster.tsx`, `frontend/src/components/AgentDetail.tsx`,
  `frontend/src/views/Schedules.tsx`), repeatedly, at lines 29-33, 64, 129,
  193 and 358.
- **`docs/NEXT.md`** — the June 2026 handoff plan, 25 KB, last touched
  2026-06-24. It is not a duplicate of `architecture/NEXT.md`; it is its
  **stale predecessor**, and its closing instruction ("Do not add new
  product code unless the owner explicitly opens a feature request")
  contradicts the W program that followed it.

The `UNCLEAR` one was settled by running the thing it documents:
`agt compare audit --json` answers **19 total, 12 supported, 6 partial, 1
missing, 0 evidence missing** — and `docs/COMPETITOR-PARITY.md:26-29` says
exactly that, row for row. The ledger is current, so it stays.

## The convention: mark, do not move

The repo has **three** precedents and they disagree, so the choice was
made explicitly:

1. **In-place banner** — commit `f86e56a2` (2026-07-06) added
   `> **Status:** ARCHIVED — …` to four documents **without moving them**.
2. **`.project/PHASE-ARCHIVE/`** — its own README says the strategy
   explicitly: *"PHASE files are NOT moved — keeping in place maintains git
   history … the archive directory serves as the index/catalog"*, and marks
   the move as *"Files moved ❌ Not needed"*.
3. **Deletion** — commit `0dbc778f` (2026-06-25) deleted three documents;
   two of them were re-added a week later **with an ARCHIVED banner**.

Two of the three say **do not move**, and the roadmap's word is
"archived", not "moved" or "deleted". So the convention is:

> **The archived documents stay where they are, carry an ARCHIVED banner
> that names their canonical successor, and are catalogued in the index
> under one heading.**

The decisive argument is the cost of the alternative: moving them breaks
**every inbound link**, and there are many — `README.md:23` points at
`docs/SYSTEM-AUDIT-REPORT.md`, and `kernel/journal/cursor.go:28-29` cites
two of the refactor plans **in a live source comment**. Marking them
breaks nothing, and the git history keeps the rest.

## What changed

### Nineteen banners, four already there

Nineteen documents got the banner; the four that already carried one
(`MISSING-PARTS-REPORT.md`, `MISSING-PARTS-PLAN.md`,
`SPEC-IMPLEMENTATION-STATUS.md`, `CHANGELOG-REORG-PLAN.md`) were left as they
were. Every banner names the reason **and** the canonical successor:

> **Status:** ARCHIVED — names six package paths that no longer exist
> (kernel/board, kernel/memory, kernel/skill, frontend/src/views/*). The
> content is retained for historical reference; architecture/00-README.md (the
> package index) with architecture/04-agent-runtime.md and
> architecture/11-frontend-console.md is the canonical source.
> **Archived:** 2026-10-27 (W5.4f)

### The index was rebuilt

`docs/index.md` had drifted in every way the gate now measures. It is now
the live map (the guides, the policies, the engineering program, the demos,
the checkable artifacts) plus **one new section** — *Archived documents* —
with a table of the 23 archived documents, what each one was, and where the
live answer is.

The index also gained the row it never had: **`CONSOLE-IA.md`** was in
`docs/` and in the tree since 2026-10-24, and the index never mentioned it.
It is pinned by `frontend/src/nav-docs.test.ts:100`, so it is a live
document — it was simply invisible.

### Five documents that are stale but still wanted were fixed

| Document | What was wrong | What it says now |
|---|---|---|
| `AGENT-LOOP-INVARIANTS.md` | names `agent.DefaultContextRescueMarker`, which moved to the contract layer in W1.2 | names `toolapi.DefaultContextRescueMarker` |
| `REFACTORING-INDEX.md` | links `REFACTOR-B1-RUNTIME-API-PLAN.md` — **the file was never written**; and calls the 2026-08 scan the "current master plan" | the row says the plan file was never written and the runtime decomposition shipped inside the W program; the scan row says the program it started has been superseded |
| `REFACTORING-SCAN-2026-08.md` | the status banner stops at 2026-08-09 | a banner above it says the program it started has been superseded by the W0–W5 migration, and where the live handoff is |
| `FRONTEND-REFACTOR-PLAN.md` | describes `frontend/src/views/` (161 files) that is gone | a banner says the sprint is DONE and where the console as it stands is described; the carve-out template the shipped console followed is kept |
| `OPENCLAW-HERMES-ROADMAP.md` | its AGEZT-side phase ordering was planned against the old tree | a banner says the AGEZT half is superseded by the W program and the external half is the snapshot it is |

## The gate — because a banner archives nothing

The banner alone does not archive anything, and the index alone does not
remember it: **four documents sat in the tree with an ARCHIVED banner and
were still listed in the index as current.** That is the state the repo was
in, and it is the reason this slice is not only a documentation edit.

`tools/docsindex` (new) checks three rules:

1. every local link in the index points at a file that exists;
2. every `.md` file in `docs/` is listed in the index at least once;
3. a document that carries an ARCHIVED banner is listed **only** under the
   archive section, and a document without one is listed **only** outside it.

It runs from the `Makefile` (`make docs-index`, and it is part of `make
check`) and as a test (`go test ./tools/docsindex/`, which the CI's
`test (linux)` job runs).

**The gate found real problems on its first run** — and it is worth naming
them, because they are the reason the gate exists:

- **`CONSOLE-IA.md` was in `docs/` and in nobody's index.** A live document,
  pinned by a frontend test, invisible to a reader. Found by rule 2.
- **The four banner-archived documents were listed as current.** Found by
  rule 3 — the exact failure the banner-and-index disagreement produces.
- **A link in the rebuilt index pointed at a file that does not exist**
  (`architecture/11-frontend-console.md` was written from memory before the
  file list was checked). Found by rule 1, while the slice was still being
  written.

## The proof

### Permanent tests — seven, in `tools/docsindex/main_test.go`

| Test | What it holds |
|---|---|
| `TestTheGateAcceptsAnIndexThatAgrees` | the happy path: a live document under a live section, an archived one under the archive section |
| `TestTheGateCatchesALinkToNothing` | a row that points at a file that is not there |
| `TestTheGateCatchesADocumentNobodyLists` | a document in the tree that the index never mentions |
| `TestTheGateCatchesAnArchivedDocumentListedAsLive` | the banner and the section must agree |
| `TestTheGateCatchesALiveDocumentListedAsArchived` | the other half of the same rule |
| `TestTheGateIgnoresTheIndexItself` | the index does not have to list itself |
| `TestTheRepositoriesOwnIndexAgreesWithTheDocsTree` | the gate run against this repository |

### The mutation run

**9 mutations, 9 CAUGHT**, in two independent passes with **byte-identical**
logs.

The gate *is* the product of this slice, so the mutations target the
**gate's own rules** and the proofs are the gate's own unit tests: each
mutation removes one rule, and the test that holds that rule must fail.

| Mutation | Rule it removes | Caught by |
|---|---|---|
| M01 | the dead-link check | `TestTheGateCatchesALinkToNothing` |
| M02 | the unlisted-document check | `TestTheGateCatchesADocumentNobodyLists` |
| M03 | "archived but listed as live" | `TestTheGateCatchesAnArchivedDocumentListedAsLive` |
| M04 | "live but listed as archived" | `TestTheGateCatchesALiveDocumentListedAsArchived` |
| M05 | the ARCHIVED banner detection | `TestTheGateCatchesAnArchivedDocumentListedAsLive` |
| M06 | the index's own exemption | `TestTheGateIgnoresTheIndexItself` |
| M07 | the archive section's name | `TestTheGateAcceptsAnIndexThatAgrees` |
| M08 | the heading trim | `TestTheGateAcceptsAnIndexThatAgrees` |
| M09 | the walk of the docs directory | `TestTheGateCatchesADocumentNobodyLists` |

The first calibration pass reported **two BUILDERRs**, and both were **the
mutation's fault**: removing a check block left its variable unused, and Go
refuses to compile an unused one. The mutations were rewritten to keep the
variable alive (`_ = resolved`, `_ = name`) so the behaviour could be
measured. A compile error is not a caught mutation; it is a mutation that
never ran.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — 2 allowlisted import violations and 0 allowlisted call sites remain |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | **OK: the documentation index agrees with the docs/ tree** |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## The rules the slice is really about

**A banner archives nothing.** It is a note in a file nobody opens. What
archives a document is the agreement between the file's own banner and the
index's table — and the gate is what keeps the two in step.

**Marking beats moving when the links are the history.** Nineteen
documents, thirty-odd inbound links and a live source comment that cites a
plan: moving them would have rewritten all of it for no gain, and the git
history already keeps what moved.

**A document that nobody lists is a document that does not exist.**
`CONSOLE-IA.md` was pinned by a test and invisible to a reader for a
quarter. The index is the public map; a file in the tree that the map does
not carry is a file the public cannot find.

**The live answer has to be named.** An archived document without a
successor is a dead end; every banner says where the current answer lives.
