# Dead Code Audit

Generated during the 2026-06-28 surgical dead-code cleanup.

## 1. Findings Table

| # | File | Line(s) | Symbol | Category | Risk | Confidence | Action |
|---|------|---------|--------|----------|------|------------|--------|
| 1 | `internal/generic/generic.go` | all | whole file | UNREACHABLE_DECL | HIGH | 100% | DELETE |
| 2 | `internal/ciguard/ciguard.go` | all | production helpers only used by tests | UNREACHABLE_DECL | HIGH | 95% | DELETE |
| 3 | `kernel/agent/config.go` | all | unused grouped config API | UNREACHABLE_DECL | HIGH | 100% | DELETE |
| 4 | `kernel/controlplane/server_config.go` | all | unused server config wrapper layer | UNREACHABLE_DECL | HIGH | 100% | DELETE |
| 5 | `tools/jsonschemagen/file.go` | all | unused helper file | UNREACHABLE_DECL | HIGH | 100% | DELETE |
| 6 | `frontend/package.json` | deps | `@radix-ui/react-dropdown-menu`, `@radix-ui/react-scroll-area` | PHANTOM_DEP | HIGH | 100% | DELETE |
| 7 | `cmd/agezt/main.go`, `cmd/agt/commands.go`, `kernel/agent`, `kernel/runtime`, `kernel/controlplane`, plugins | multiple | test-only constructors/helpers in production code | UNREACHABLE_DECL | HIGH | 95% | DELETE |
| 8 | `frontend/src/**` | multiple | unused exports and imported symbols | PHANTOM_DEP | HIGH | 100% | DELETE |
| 9 | `kernel/netguard/netguard.go` | option block | `AllowLinkLocal` | UNREACHABLE_DECL | HIGH | 95% | DELETE |
| 10 | `sdk/approvals.go`, `sdk/events.go`, `sdk/mailbox.go`, `sdk/sdk.go` | multiple | public SDK methods reported by `deadcode` | UNREACHABLE_DECL | LOW | 55% | MANUAL_VERIFY |

## 2. Cleanup Roadmap

### Batch 1: High-risk cleanup, already applied

- Estimated LOC removed: 2,815 tracked source lines deleted.
- Estimated net source LOC reduction: about 2,040 lines after test-only helper relocation.
- Potential bundle or binary impact: small binary impact from removed Go helpers; frontend dependency graph is smaller after removing two unused packages.
- Suggested order: delete whole unreachable files first, move test-only helper behavior into `_test.go` files second, remove frontend unused exports and dependencies third, then regenerate the committed web UI bundle.

### Batch 2: Medium-risk cleanup

- Estimated LOC removed: 0 remaining.
- Potential bundle or binary impact: none.
- Suggested order: no medium-confidence findings remain after `staticcheck`, `knip`, `depscheck`, tests, and build verification.

### Batch 3: Low-risk/manual review

- Estimated LOC removed: 0 unless a breaking SDK change is approved.
- Potential bundle or binary impact: negligible.
- Suggested order: review SDK usage policy before deleting anything under `sdk/`. The remaining `deadcode` report is limited to public SDK methods that are documented as beta external API in `docs/API-STABILITY.md` and `CHANGELOG.md`.

## 3. Executive Summary

| Metric | Count |
|--------|-------|
| Total finding groups | 10 |
| High-confidence deletes | 9 |
| Estimated gross source LOC removed | 2,815 |
| Estimated net source LOC removed | about 2,040 |
| Estimated dead imports/dependencies | 2 package dependencies plus frontend unused imports/exports |
| Files safe to delete entirely | 5 |
| Estimated build time improvement | Small |

Verification commands completed successfully:

- `go test ./...`
- `go vet ./...`
- `staticcheck ./...`
- `npm run typecheck`
- `npm test`
- `npm run build`
- `npx knip --reporter json`
- `go run ./tools/depscheck`
- `go run ./tools/sdkparity -check docs/SDK-PARITY.md`
- `git diff --check`

Follow-up guardrails added after the cleanup:

- `go run ./tools/deadcodecheck` runs Go `deadcode` with an explicit public-SDK allowlist.
- `npm run deadcode` runs `knip --reporter json` from the pinned frontend devDependency.
- `make check` now includes both dead-code guards.
- `.github/workflows/ci.yml` now runs the Go dead-code guard in `deps-check` and the frontend dead-code guard before frontend tests.

The only remaining dead-code tool output is the public Go SDK surface. It is intentionally not deleted because external callers are not visible to repository-local reachability analysis.

Overall codebase health: the cleanup removed several stale abstraction layers, production-only test helpers, and frontend dependency/export drift without leaving analyzer debt. The highest-impact next actions are to keep `knip` and Go dead-code checks in CI, require test helpers to live in `_test.go`, and make SDK API removal a versioned compatibility decision instead of a dead-code sweep.
