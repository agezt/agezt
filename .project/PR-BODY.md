# PR body — audit/2026-09-surface

Paste into https://github.com/agezt/agezt/pull/new/audit/2026-09-surface

---

## What

A full-surface audit of AGEZT, started from "the WebUI looks like a mess" and
ended up somewhere else: **the kernel was fine, the verification layer was
not.** Every gate the project already had was either red, unwired, or measuring
the wrong string — and the two real product bugs it found were both hiding
behind gates that were green.

## The two real product bugs

**`fix(shell)` — the output budget was enforced on the wrong string.**
`renderResult` applied `MaxOutputBytes` to the concatenated stdout+stderr and
prepended the status line *afterwards*, so a command that both overflowed the
budget and timed out shipped `MaxOutputBytes + len("timed out after 30s")`
bytes — 65,556 against a cap of 65,536. The code comment said "enforce the
model-facing budget"; the model reads `Result.Output`, not the concatenation.
A budget the model can exceed is not a budget.

It survived because the test meant to catch it cannot decide anything:
`TestInvoke_RealWarden_CombinedBudgetHeld` drives a real `cmd.exe` through the
real Warden, and its verdict turns on whether the command reaches
`DefaultTimeout` before overflowing. Killed in time, it passes; allowed to
overflow, it fails. Same code, two verdicts, decided by machine speed — and it
is Windows-only, so CI never saw it. Replaced with
`plugins/tools/shell/render_budget_test.go`: a table over all eight shapes,
deterministic, runs everywhere.

**`fix(frontend)` — the Conductor store grew without bound.**
Wired once from `App.tsx` and folding every `conductor.*` event off the whole
firehose. Entries were keyed by correlation id and nothing ever removed one,
and every fold rebuilt the entire map — O(events × runs) in a store that had no
reader at all since the view was retired. Capped at 20 most recent runs, which
is right whether or not the view comes back. Ordering needed a monotonic
insert counter, because `updatedMs` has millisecond resolution and a batch of
runs folds inside one millisecond, which made "newest first" a claim rather
than a guarantee — the first version of the regression test caught exactly
that.

## Everything else

| | |
|---|---|
| **59 package comments** | destroyed by the Day-50…211 god-file splits, which prepended a mechanical `Code extracted from …` header above the package clause. Recovered from `52234e77` — the last copy. Regenerating first would have destroyed it for good. |
| **98 packages** | declared more than one package comment, so `go doc` showed the split provenance first and the real docs below. Moved below the clause, verbatim. **Now 0.** |
| **636 files** | violated `gofmt` — CI's own gate, red since 2026-06-06. Plus 673 files whose CRLF made `gofmt -l` meaningless on a Windows checkout. |
| **CI was missing a gate** | `tools/structure-md -check` had no job at all, which is why three stale generated documents could sit on `main`. |
| **`e2e-smoke.ps1`** | broken since 2026-07-06: the shell twin got the token-file fix, the PowerShell twin never did, so its regex could never match. Invisible to CI. |
| **knip** | reported every live view's own export as unused — it cannot follow `lazyNamed(loader, key)`. Acting on that report would have broken the console. |
| **51 dead files, 53 dead exports** | removed — but not by taking the linter's word for it: both files it pointed at were alive. |
| **The changelog** | 14 appended blocks and a 510-line `Unclassified` pile whose first entry was a critical self-update finding (attacker-supplied manifest and hash → arbitrary code execution over `<baseDir>/bin/agezt`), unfindable. |
| **`kernel/governor`** | `doc.go` and `governor.go` documented the provider chain two different ways. The code settles it: `authModePriority` is purely a cost ranking, so subscription-first and cost have shipped and quality and latency have not. Both files now say that. |

## Metrics

| | at `0b6c8519` | now |
|---|---|---|
| `structure-md-check` | ❌ exit 1 | ✅ |
| `frontend-deadcode` | ❌ exit 1 | ✅ `{"issues":[]}` |
| packages with >1 package comment | 98 | **0** |
| `.go` files failing `gofmt` | 636 | **0** |
| `staticcheck` findings | 3 | **0** |
| tests whose verdict depends on machine speed | 3 | **0 fixed** |

`go test ./...` 192 packages / 0 failures · vitest 162 files / 1454 tests ·
`tsc` clean · cross-build 6/6 · e2e smoke 10 checks · webui e2e 6 tests ·
`govulncheck` no vulnerabilities · `gitleaks` 2,071 commits, no leaks.

## Reviewing this

`.project/REVIEW-MAP.md` splits the 17 commits into their review order and
carries the exact commands to reproduce every claim above — nothing here needs
taking on trust. `.project/AUDIT-2026-09-BACKEND-SURFACE.md` has the full
findings and the measurements behind them.

History is bisect-safe: 12 of the 13 pre-warden commits are fully green, and
the one that is not green at that commit is green from the next one on — it is
`TestInvoke_RealWarden_CombinedBudgetHeld`, the test `fix(shell)` replaces.

## Notes for the reviewer

- **`docs/CONSOLE-IA.md` records a decision made without asking**: 25 console
  destinations stay retired. Restoring them would have meant authoring ~12 UI
  surfaces whose implementations were deleted. No kernel handler was touched
  and every capability is still on the CLI; the doc says how to reverse it.
- **`kernel/governor` leaves one question open**, and it is a roadmap one
  rather than a docs one: whether quality and latency should become chain
  ordering dimensions. Today the ordering is cost only.
- I made five mistakes in this work — a dropped `//` prefix, a `wrap()` arity
  error, a regex that hit the import instead of the export, a wrong "SDKs
  untouched" claim, and one mis-scoped metric. All are recorded in the audit
  report rather than tidied away.
