# PHASE M864 — Built-in git-ops skill bundle

**Status:** shipped
**Milestone:** M864 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Backlog #34 (more out-of-the-box capability) with a direct line to #42
(self-improvement): a fifth built-in skill bundle that gives agents a safe,
disciplined git + GitHub workflow for changing code and shipping it as a PR.

## What shipped

A built-in `git-ops` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — the rules (never commit on `main`, one change per branch, clear
  messages, never force-push shared branches, open a PR don't self-merge), the
  `gh auth` preflight, and the conflict-resolution path.
- `scripts/gitflow.sh` — a POSIX safe-workflow helper: `sync` (fetch + ff the
  default branch), `branch`, `save` (add -A + commit; **refuses on the default
  branch** so you can't push to main by reflex), `pr` (push + `gh pr create`
  against the resolved default branch), `status`. Resolves origin's default
  branch via `refs/remotes/origin/HEAD` with a main/master fallback.
- `reference/recipes.md` — clone→PR end to end, auth check, undo/amend,
  cherry-pick, rebase conflict resolution, throwaway worktrees, and the
  self-improvement loop.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is editing
for M858/M859). The seeder auto-loads it. It tests in isolation:
`go test ./plugins/builtinskills/` compiles just this package + `kernel/skill`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `sh -n gitflow.sh` syntax-clean. Package suite green —
  `TestSeedAll_InstallsGitOps` asserts the bundle seeds **active** and materializes
  `scripts/gitflow.sh` + `reference/recipes.md`; bundle-count assertions now cover
  five bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` / daemon smoke deliberately
  skipped — they'd compile the concurrent in-progress Go edits.

## Notes
- Five seeded bundles now ship out of the box: browser-use, computer-use,
  data-analysis, docker-services, git-ops. The helper's "refuse to commit on the
  default branch" guard mirrors the operator's own discipline (branch-per-PR).
