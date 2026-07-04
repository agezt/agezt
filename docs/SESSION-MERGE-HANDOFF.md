# Session merge handoff — `integration/session-merge`

> **Generated:** 2026-07-04. One integration branch = `main` (`967de333`) + the five
> branches produced in session `sess_01KWM73A…`. All merged, all verified. `main`
> itself is **untouched** — the final landing step needs a human (branch-guard blocks
> writes to `main`/`master`).

## What's on this branch

`origin/integration/session-merge` @ `93810ae6` merges, in dependency order:

| Merge commit | Branch | Summary |
|---|---|---|
| `2f30a95e` | `feat/log-cursor-pagination-phase2` | **A2 log pagination (P1–P4)** — cursor on all 12 log endpoints (Go handlers + `seq` row-ids + WS route registration), a shared `components/ui/load-more-footer.tsx`, and the 3 always-visible log views (Providers, Approvals, Tools) wired end-to-end. |
| `b481424f` | `fix/wsl-runner-gocache-tmpfs` | **CI runner fix** — stage `GOCACHE` + `GOTMPDIR` to per-runner tmpfs in `setup-go-safe`, closing the ext4 write→exec race behind the recurring `fork/exec …: invalid argument` flake. |
| `4f016d2b` | `refactor/c2-voice-merge` | **C2 P1** — fold 5 voice modules into `lib/voice/` behind an `index.ts` barrel. |
| `984940a0` | `refactor/c2-store-merges` | **C2 P2** — `conductorStore→conductor`, `councilStore→council`, `languages→language`. |
| `93810ae6` | `refactor/c2-colocate-views` | **C2 P3/P4 plan corrections** — reclassify `snapshot` (cross-cutting → KEEP) and `language` (→ KEEP after P2); flag the flat→folder structural prerequisite blocking colocation. |

## Verification (run on the fully-merged tree)

- `go build ./...` — clean
- `tsc --noEmit` (frontend) — clean
- `vitest run` (frontend) — **1461/1461 green** (177 files)
- `vite build` — ok; `kernel/webui/dist` regenerated from merged sources and committed

## Conflict resolution note

The **only** conflicts during the merge were **228 rename/rename collisions in
`kernel/webui/dist/`** — the two frontend branches each rebuilt the bundle, producing
different content-hash filenames for the same chunks. **Zero source conflicts.** The C2
plan doc (`docs/REFACTOR-C2-LIB-CLASSIFICATION.md`) auto-merged with no markers.

Resolution: `git rm -r kernel/webui/dist`, then `vite build` to regenerate dist fresh
from the merged sources (the authoritative state), then stage. This is the correct way
to handle a build-artifact conflict — never hand-merge minified chunk hashes.

## To land on `main`

`main` is a strict ancestor of this branch, so a fast-forward is clean. Pick one:

**A — via PR (recommended):** open one PR `integration/session-merge → main`. Lets CI
run — and it exercises the `fix/wsl-runner-gocache-tmpfs` commit, so this PR is the test
of whether the runner fix lets a Go CI run complete.

**B — fast-forward (needs branch-guard lifted for `main`):**
```
git checkout main
git merge --ff-only integration/session-merge
git push origin main
```

## Safety net

`backup/main-before-merge-967de333` holds `main`'s exact pre-merge state (`967de333`).
If anything looks wrong after landing: `git reset --hard backup/main-before-merge-967de333`.

## Still blocked (not addressable by an agent here)

- **GitHub Actions billing** — hosted runners are off (`payments have failed / spending
  limit`), which is why everything runs on the self-hosted WSL runners in the first place.
- **C2 P3/P4 colocation** — needs a team decision on the view/component folder convention
  (all `views/*` and `components/*` are flat files today). See the corrections in
  `docs/REFACTOR-C2-LIB-CLASSIFICATION.md`.
