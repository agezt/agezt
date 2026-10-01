# PR body — audit/2026-09-surface

Paste into https://github.com/agezt/agezt/pull/new/audit/2026-09-surface

---

## What

A full-surface audit of AGEZT, started from "the WebUI looks like a mess" and
ended up somewhere else: **the kernel was fine, the verification layer was
not.** Every gate the project already had was either red, unwired, or measuring
the wrong string — and the two real product bugs it found were both hiding
behind gates that were green.

## The finding that outranks all the others

**20 of this workflow's 22 checks could not run, and had not for months.**

```console
$ gh api repos/agezt/agezt/actions/runners
{"total_count":0,"runners":[]}
```

No self-hosted runner is registered for this repo, while fifteen job
definitions still carried `runs-on: [self-hosted, Linux, X64]`. `multi-arch` is
a six-leg matrix, so those lines accounted for 20 of 22 checks. The runners they
name live on another host (`ops/wsl-runners/README.md`: `WHITE`) and are not
reporting.

This was found while trying to *merge* this branch. On the run before the fix,
`updatedAt` never moved past `createdAt`; the only two checks that concluded
were the two already on `ubuntu-latest`, and the other twenty sat in `queued`
indefinitely. **A queued job is neither pass nor fail** — nothing turns red and
no notification fires, so a dashboard reading "no failing checks" is
indistinguishable from a healthy pipeline. `main` requires the `CI` check, so
this also meant no PR could merge at all, including this one.

All 17 jobs now run on `ubuntu-latest`. The repo is public, so hosted minutes
are free: the self-hosted design's only advantage was cost, and it had cost the
entire gate. The rule was already written in `ci.yml`'s own header on 2026-09-06
— *"a job that never gets a runner enforces nothing"* — and had only ever been
applied to new gates.

Turning the jobs back on surfaced **six more defects that the dead pool had been
hiding**, and two coverage ratchets that were red on `main` and not introduced
here: `kernel/tunnel` at 99.0% on Linux (one uncovered statement, the fallback
arm of `killProcessTree`), and the voice/Jarvis ratchet, which had been
pointing at source files that moved to feature slices and no longer existed —
so it had never run, and once repaired reported 99.67% statements / 94.83%
branches against a 100% threshold. Ten new tests, four ratchets at 100%.

Five of the six hidden defects, since a reviewer will want them: `setup-go-safe`
detected a hosted runner by testing `RUNNER_NAME` for emptiness, which GitHub's
own documentation contradicts, so its WSL2 workaround had been staging ~600MB
into `/dev/shm` on every hosted job; `ci-go-retry.sh` deleted the staged
`GOCACHE`/`GOTMPDIR` and re-staged only `GOROOT`, so every retry died before the
command ran; `sdk/typescript/package-lock.json` disagreed with `package.json`,
so `npm ci` could not succeed; `kernel/creds` fixtures wrote unquoted
`credential_process` paths, which split on GitHub's space-containing `TMPDIR`;
and two `plugins/tools/file` TOCTOU tests asserted the wrong channel for a
refusal. Full detail in Finding I of the audit report.

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
| **CI checks that can reach a conclusion** | **2 of 22** | **22 of 22** |
| `structure-md-check` | ❌ exit 1 | ✅ |
| `frontend-deadcode` | ❌ exit 1 | ✅ `{"issues":[]}` |
| packages with >1 package comment | 98 | **0** |
| `.go` files failing `gofmt` | 636 | **0** |
| `staticcheck` findings | 3 | **0** |
| tests whose verdict depends on machine speed | 3 | **0 fixed** |

`go test ./...` 192 packages / 0 failures · vitest 163 files / 1469 tests ·
`tsc` clean · cross-build 6/6 · e2e smoke 10 checks · webui e2e 6 tests ·
`govulncheck` no vulnerabilities · `gitleaks` 2,071 commits, no leaks.

And, for the first time, the twenty checks that could not run have run:
`race-breadth` and `race-depth` green on linux/cgo, `rust-sdk` green,
`frontend-dist-in-sync` green — which is the one that answers whether the
committed `kernel/webui/dist` matches a rebuild, and it does, byte for byte.

## Reviewing this

`.project/REVIEW-MAP.md` splits the commits into their review order and
carries the exact commands to reproduce every claim above — nothing here needs
taking on trust. `.project/AUDIT-2026-09-BACKEND-SURFACE.md` has the full
findings and the measurements behind them.

**On bisect-safety**, precisely and without overstating it: the first 13 commits were each checked out and
built and tested individually. Twelve are fully green, and the one that is not
is green from the next commit on — it is
`TestInvoke_RealWarden_CombinedBudgetHeld`, the test `fix(shell)` replaces, so
a `git bisect` landing there finds the bug it is meant to find. The commits
added after that sweep were **not** each re-walked; that is a real gap in the
guarantee, not a formality. The later ones are single-file and
independently-verifiable — six touch only `.project/*.md`, one only
`kernel/warden/warden_test.go` (a test bound), one only `.gitleaks.toml` (a
comment, with the scan re-run green afterwards), one only two `kernel/governor`
doc comments — but the three CI commits that repointed 17 jobs and rewired a
composite action are exactly the kind a bisect should stop on. What has been
re-verified is the tip: `go test ./...` 192 packages / 0 failures, `gofmt`,
`go vet`, `GOOS=linux go vet` on the packages carrying Linux-only tests,
`structure-md -check`, `deadcodecheck`, `sdk-parity`, `changelog-lint`,
`npm ci` in `sdk/typescript`, the voice ratchet at 100% on all four metrics,
`kernel/tunnel` re-measured at 100% inside a `golang:1.26` container, and a full
22-check CI run on the branch tip.

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
