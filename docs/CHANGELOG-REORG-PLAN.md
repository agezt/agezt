# CHANGELOG Reorg Plan (P0-5)

> **Date:** 2026-07-04 (last updated: 2026-07-06)
> **Branch:** `main` (HEAD: `ef7b412d`)
> **Status:** ARCHIVED — CHANGELOG reorganization completed (root slimmed to release index, per-version bodies in `CHANGELOG/`). Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Plan retained for historical reference.
> **Other references:** `docs/MISSING-PARTS-PLAN.md`, `docs/MISSING-PARTS-REPORT.md` (H-04).

---

## 1. Current State (verified 2026-07-04)

| Metric | Value |
|---|---|
| `CHANGELOG.md` size | **646,076 bytes (≈631 KB)** |
| Total lines | **7,914** |
| Top-level sections (`## `) | 3 |
| Subsections (`### `) | 57 |
| Published versions | `[1.0.0] 2026-06-03`, `[0.1.0] 2026-05-30` |
| Unreleased section | Lines 12–5795 (5,784 lines / **~635 KB**) |
| `[1.0.0]` section | Lines 5796–7835 (2,040 lines / ≈6.7 KB) |
| `[0.1.0]` section | Lines 7836–7914 (79 lines / ≈0.3 KB) |

**Problem:** The `[Unreleased]` block is **5.7K+ lines** and contains a summary of nearly every M-phase report. The diff is hard to understand in PR reviews; new M-phase commits keep adding lines to this block.

### 1.1 Internal Structure of Unreleased

There are 57 subsections, distributed across the Keep a Changelog categories:
- `### Added`: ~24 blocks
- `### Fixed`: ~15 blocks
- `### Changed`: ~7 blocks
- `### Security`: ~2 blocks
- `### Code quality`: ~1 block
- `### Tests`: ~1 block

Each block is a collective description of M-phase work. Example: `### Added — positioning, security, and SDK parity documentation` (line 14).

---

## 2. Goal

**Goals:**
1. Bring the main `CHANGELOG.md` below ~50 KB (only TOC + the last 1.0.0 + a new Unreleased).
2. Slice the Unreleased content by **milestone ranges**.
3. Let CI / lint verify this new structure.
4. Keep PR diffs small and readable (each PR only appends to the relevant milestone file).

**Non-goals:**
- **No deletion** of CHANGELOG content — we will split it, not remove it.
- No breaking of the Keep a Changelog format — section headings and categories stay the same.
- The M phase-report files (`PHASE-M*.md`) are untouched; they are already separate.
- No retroactive `git tag` additions to history (e.g. `v1.0.1` or similar) — for stability.

---

## 3. Strategy: Structural Splitting

The structure I propose:

```
CHANGELOG.md                                  (≈50 KB — TOC + Unreleased + 1.0.0 + 0.1.0)
├── CHANGELOG/
│   ├── README.md                            (TOC, milestone links, maintenance rule)
│   ├── unreleased/
│   │   ├── current.md                       (~100 KB — active development, last ~30 days)
│   │   ├── m600-m699.md                     (~50 KB — older M blocks)
│   │   ├── m700-m799.md
│   │   ├── m800-m899.md
│   │   ├── m900-m999.md
│   │   └── m1000+.md
│   ├── v1.0.0.md                           (≈7 KB)
│   ├── v0.1.0.md                           (≈0.3 KB)
│   └── REORG-LOG.md                        (split history)
```

### 3.1 Rationale for the Structure

- **Mirror option**: If tracking `git log -- CHANGELOG.md` becomes difficult, milestone files are smaller so diffs stay clean.
- **GitHub rendering**: GitHub can link `CHANGELOG/README.md` and `CHANGELOG/v1.0.0.md` separately; also suitable for marketplace/release pages.
- **Lint**: `make check` or `.github/workflows/ci.yml` can verify via a helper (e.g. `tools/changelog-lint`) that Unreleased lives under current.md.

### 3.2 How Are Milestone Ranges Determined?

**Data basis:** There are 697 `PHASE-M*.md` files on disk, from M1 to M923. The 5.7K lines of content in the CHANGELOG span 100+ M phases.

**Practical rule:**
- Ranges of 100 (M100-199, M200-299, ...) provide **sufficient slicing**.
- Ranges of 50 (M50-99, M100-149, ...) yield **smaller files**.
- Ranges of 25 would be too small: 50+ files, hard to manage.

**Recommendation:** Ranges of 100. About 10 files, ~100 KB each (sensible slices of the Unreleased block).

### 3.3 Breaking Up the Unreleased Block

Unreleased is 5,784 lines. To slice those lines using a helper enriched with the **existing timestamps** or **M-number references**:

1. **Extract each paragraph's M-number reference**: with the regex `(PHASE-M\d+|M\d{3,4})`. Most entries already contain `M-XXX`.
2. **Assign paragraphs with an unclear M-number** (e.g. an addendum like "added dep doc alignment") to the nearest neighboring M reference.
3. **Place the lines into the slice files**.

This split can be done **scripted**. Plan + script draft below in §5.

---

## 4. Structural Design Detail

### 4.1 The Main `CHANGELOG.md` File (≤50 KB)

```
# Changelog
[intro paragraphs — 8-10 lines]

## [Unreleased] — currently at /CHANGELOG/unreleased/current.md
See CHANGELOG/unreleased/current.md for the in-flight changes.

## [1.0.0] — 2026-06-03
[full content (2,040 lines / ≈7 KB)]

## [0.1.0] — 2026-05-30
[full content (79 lines / ≈0.3 KB)]

## Older
See CHANGELOG/ dir for milestone-level subdivisions.
```

### 4.2 `CHANGELOG/unreleased/current.md`

The current development block (last 30 days). Every PR enters here as **one or a few items**.

### 4.3 `CHANGELOG/unreleased/mXXX-mYYY.md`

Each range file contains the `### Added` / `### Fixed` … categories that start with `### /`. A note is added to the range files: "moved from current.md → mXXX-mYYY, <date>".

### 4.4 `CHANGELOG/README.md`

```
# Changelog

This directory holds the per-milestone changelog for Agezt (the `agezt` daemon + the `agt` CLI).

## Structure

- `v0.1.0.md`, `v1.0.0.md` — published versions.
- `unreleased/current.md` — active development (last ~30 days).
- `unreleased/mXXX-mYYY.md` — older M-block ranges, created during the reorg.
- `REORG-LOG.md` — the split history of the milestone files.

## PR Order

1. New feature, fix, or change → added to `unreleased/current.md` (when opening the PR).
2. Once current passes the ~30-day cycle → moved to the relevant `mXXX-mYYY.md` range.
3. When a new version is cut → a new `vX.Y.Z.md` file is created.
```

### 4.5 `CHANGELOG/REORG-LOG.md`

```
# 2026-07-04 — Reorg v1

`CHANGELOG.md` was split from a single ~646 KB file into milestone-range files.

## Mapping

- `unreleased/current.md`: the Unreleased portion from the last ~30 days.
- `unreleased/m100-m199.md`: all paragraphs referencing M100-M199.
- `unreleased/m200-m299.md`: ...
- ... (each range)
- `v1.0.0.md` and `v0.1.0.md`: the original version blocks, unchanged.

## Tools

- `tools/changelog-split` (to be added in a PR): slices Unreleased automatically.
- `tools/changelog-lint` (to be added in a PR): verifies the TOC + file existence.

## Migrating Back

The main `CHANGELOG.md` is still kept inline (back-compat), but it is not "deleted". Mirror tooling can regenerate it automatically when needed.
```

---

## 5. Implementation Steps (sequence)

### 5.1 PR-1: Tool — `tools/changelog-split`

The `tools/changelog-split/` package:

```go
package main
// Reads `CHANGELOG.md`, parses ## [Unreleased] block,
// extracts per-paragraph M-references (regex),
// splits content into milestone-range files,
// emits `CHANGELOG/unreleased/*.md`.
// Dry-run mode, --verify mode, --emit mode.
```

Usage:
```
go run ./tools/changelog-split --dry-run  # show planned output
go run ./tools/changelog-split --emit     # write files
go run ./tools/changelog-split --verify   # check current state matches
```

**Tests:**
- `TestSplitByMRange` — assigns paragraphs between given M references to the correct file.
- `TestNoMReference` — paragraphs without a reference are written to current.md.
- `TestMergedSections` — merged when multiple paragraphs share the same M reference.

### 5.2 PR-2: Tool — `tools/changelog-lint`

```go
package main
// Checks CHANGELOG.md structure:
// - ## [Unreleased] is short and references /CHANGELOG/unreleased/current.md.
// - Milestone files exist under the CHANGELOG/ directory.
// - Each milestone file contains `### Added`, `### Fixed`, etc. categories.
// - At least one v0/v1 version lives in a separate file.
// - REORG-LOG.md exists.
```

It is added to `make check` or the CI gate.

### 5.3 PR-3: Reorg — write the files + shorten CHANGELOG.md

- Run `tools/changelog-split --emit`.
- The top-level content of `CHANGELOG.md` will keep only the TOC + the v1.0.0 and v0.1.0 versions.
- Create `CHANGELOG/README.md` + `CHANGELOG/REORG-LOG.md`.

### 5.4 PR-4: Migration helper (optional)

For older PRs, *if the author committed to Unreleased* and Unreleased has grown again, an automatic migrate command:

```
make changelog-migrate  # moves Unreleased content into the milestone files
```

---

## 6. Demo Gate (Phase 0 → Phase 1)

- ✅ `tools/changelog-split --verify` works: the split between `current.md` and the milestone files is correct.
- ✅ `tools/changelog-lint` is green in the CI gate: the key headings of `CHANGELOG.md` exist.
- ✅ The main `CHANGELOG.md` of the whole CHANGELOG tree is <50 KB.
- ✅ With the current `git log`, each milestone PR change touches only the relevant milestone file.

### 6.1 Duration Estimate

| Step | Duration | Dependency |
|---|---|---|
| PR-1 `tools/changelog-split` | 1-2 d | none |
| PR-2 `tools/changelog-lint` | 0.5 d | PR-1 |
| PR-3 reorg (writing files) | 0.5 d | PR-1, PR-2 (dry run) |
| PR-4 migration helper (optional) | 0.5 d | PR-3 |
| **Total** | **2-4 d** | |

---

## 7. Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Paragraphs without an M reference are lost during the split | Medium | `--no-m-reference` → `current.md` (default); visible in PR review; lint keeps Unreleased short. |
| Older PRs append to the main `CHANGELOG.md` without knowing the new structure | Low-Medium | PR-2 lint → CI fail; a strict "Unreleased is reference-only" rule is established in PR-3. |
| Concurrent reorg conflicts across multiple agents | Low | Coordinated via the PR-4 migration helper; `tools/changelog-lint` invariant test on every PR. |
| Loss of date formatting (especially `[1.0.0]` and `[0.1.0]`) | Low | The main `CHANGELOG.md` retains the original blocks; the sub-files are mirrors. |
| Deviation from the Keep-a-Changelog format | Very Low | The structure is an "alternative representation" of that format; the content is the same. |

---

## 8. Post-Reorg Scenario

After these PRs land:

- With `git log -- CHANGELOG/unreleased/current.md`, the diff of active development is small and readable.
- When a new M phase completes: PHASE-M*.md reports + an `unreleased/mXXX-mYYY.md` update + no Unreleased line in the main `CHANGELOG.md`.
- When a version is cut: a new `CHANGELOG/vX.Y.Z.md` file + a line in the main `CHANGELOG.md`.
- `make check` and CI verify the invariant on every PR.

### 8.1 PR Policies (going forward)

- **A newly added CHANGELOG line** goes to either `unreleased/current.md` (new feature), `unreleased/mXXX-mYYY.md` (backfill), or `vX.Y.Z.md` (version patch) — no other file.
- **Appending to the old CHANGELOG.md** → lint fails.
- **Tooling updates** (e.g. a `changelog-add` helper) arrive via PR.

---

## 9. Snapshot (2026-07-04)

At this planning stage:

- 1 file on disk: `CHANGELOG.md` (646 KB, 7,914 lines)
- After the reorg (expected):
  - `CHANGELOG.md` ≤ 50 KB
  - `CHANGELOG/README.md` ≈ 1 KB
  - `CHANGELOG/REORG-LOG.md` ≈ 1 KB
  - `CHANGELOG/v0.1.0.md` ≈ 0.3 KB
  - `CHANGELOG/v1.0.0.md` ≈ 7 KB
  - `CHANGELOG/unreleased/current.md` ≈ 100 KB
  - `CHANGELOG/unreleased/m100-m199.md`, `m200-m299.md`, ... (10 files, ~50 KB each)

Total: ≈ 50 + 1 + 1 + 0.3 + 7 + 100 + 50×10 ≈ **660 KB overall** (the cost of mirroring), but the main file shrinks by 92%.

---

## 10. Open Questions / Pending Decisions

- **Ranges of 100 vs 50**: 100 is more manageable but produces larger files. Decision: start with **100**, switch to 50 if needed.
- **Is `REORG-LOG.md` required**: useful for PR review, but it can grow with every split. Decision: **keep it**, merging older entries into a "summary".
- **Is the CI gate too early**: if `tools/changelog-lint` is merged together with PR-1/PR-2, CI does not exist yet. Suggestion: start with an `--off` flag in PR-2; switch to `--on` after PR-3.

---

*This plan is entirely within "release hygiene" scope. It adds no new features; it only splits the existing 646 KB into ~10 manageable files.*
