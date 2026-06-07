# M492 — Mutation testing edict: pin the punctuation-adjacent whitespace contract

## Context
Third package in the mutation-testing pass: `kernel/edict`, the policy /
authorization engine. `go-mutesting .` scored **0.701** (181 killed / 77 survived /
258). Most survivors are error-message and equivalent mutants; two security-relevant
classes were checked and found to be equivalent (a positive result):

- **toolmap `return CapFileList` / `return CapFileDelete`** survivors are *equivalent*:
  removing the explicit return falls through to `return Capability("file." + p.Op)`,
  which yields `"file.list"` = `CapFileList` exactly (the constant equals `"file."+op`).
  So `CapabilityForToolCall` is provably correct either way — no gap.
- **TrustLevel constants** (`LevelAllow = 4`, …) are referenced symbolically, so a
  value change is a consistent rename (equivalent), and a value *collision* would be a
  duplicate `switch` case → compile error → auto-killed.

## Tooling hazard found (and contained)
`go-mutesting` **left a mutant in the working tree** on Windows: after the run,
`edict.go:322` held the injected `for j := i - 1; 1 < 1; j--` instead of the
committed `j >= 0`. HEAD was correct and the M490/M491 commits had staged only their
*test* files (never the mutated sources), so nothing corrupt was ever committed; the
working tree was restored with `git checkout --` and the stray `report.json` artifacts
removed. **Conclusion: do not rely on go-mutesting to restore sources on this
platform; treat the survivor list as data and verify fixes with manual negative
controls instead of re-running it.**

## The genuine gap (closed)
That stray mutant was itself a real survivor: `j >= 0` → never-true disables the
**backward** scan in `stripPunctAdjacentWhitespace`, the SPEC-06 §3.2 normalizer that
strips spacing-evasion from hard-deny floor rules (fork bombs, `rm -rf /`). The
function documents stripping a space bordering punctuation on *either* side, but the
existing fork-bomb tests exercise it only at the `Decide` level — and every optional
space in `:(){ :|:& };:` has punctuation on its **right**, so the forward scan alone
normalizes it. The left-side (backward) scan was never tested, so a regression
breaking it would let a variant whose only punctuation neighbour is on the left evade
floor-rule normalization.

## Fix
`kernel/edict/strip_whitespace_test.go` — `TestStripPunctAdjacentWhitespace`, a direct
unit test pinning the contract: left-only punctuation (`"x} y"` → `"x}y"`, requires the
backward scan), right-only punctuation, two words preserved (no merge — the M426
regression), a trailing space (guards the forward `j < len(rs)` bound), and the
canonical fork bomb.

## Negative control (manual, not go-mutesting)
- Backward-scan mutant `j >= 0` → `1 < 1`: the test fails (`"x} y"` left unstripped).
- Forward-bound mutant `j < len(rs)` → `j <= len(rs)`: the test fails (index-out-of-range
  panic on the trailing-space case).
Both restored byte-for-byte; `git diff --ignore-all-space` on `edict.go` is empty.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- `kernel/edict` suite green; `go.mod`/`go.sum` unchanged; tracked tree otherwise clean.

## Pass summary (mutation arc M490–M492)
redact (0.575→0.725, 4 tests), journal (rotation accounting + Tail trim, 2 tests),
edict (whitespace-normalizer contract, 1 test). Across the three highest-stakes
packages mutation testing found genuine, security/integrity-relevant test gaps that
the existing suites missed by construction; each is now pinned with a focused test and
a verified negative control. Remaining survivors are error-message or equivalent
mutants, which are not chased.
