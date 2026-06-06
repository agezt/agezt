# M490 — Mutation testing the redactor: close real test gaps (0.575 → 0.725)

## Context
Every gate so far (vet, staticcheck, gitleaks, govulncheck, errcheck, gosec,
golangci-lint, build matrix) measures whether the *code* is clean. **Mutation
testing** measures whether the *tests* would catch a regression: it injects a
one-token change ("mutant") and re-runs the suite; a mutant the suite still passes
"survives" — a gap where a real refactor could silently break behaviour unnoticed.

Ran `go-mutesting` on `kernel/redact` — the security chokepoint that scrubs secrets
before they hit the permanent, hash-chained journal. (go-mutesting panics on Windows
absolute paths with a drive letter; run from inside the package directory as
`go-mutesting .` to avoid it.)

## Baseline
Score **0.575** — 23 mutants killed, **17 survived** of 40.

## Triage of the 17 survivors
Split into genuine test gaps vs. equivalent mutants (semantically identical to the
original — unkillable by any black-box test, the well-known floor of the technique):

**Genuine gaps (6, now closed):**
- `len(v) < minLiteralLen` → `<=`, and `minLiteralLen 8` → `7` / `9`: the literal
  length floor. Nothing pinned whether an *exactly-8-char* secret is redacted (it must
  be) or a *7-char* value is left intact (it must be — too likely an ordinary substring).
- `continue` → `break` on the length-filter branch: a leading too-short value would
  abort the whole loop, dropping every later secret — unredacted. Nothing caught it.
- `continue` → `break` on the dedup branch: a leading duplicate would likewise drop
  later secrets.
- sort-comparator no-op: defeats the longest-first ordering, so a secret that is a
  prefix of another gets the shorter one replaced first, leaving the longer secret's
  tail exposed. Nothing exercised overlapping secrets.

**Equivalent mutants (11, correctly unkilled):**
- `r.mu.RUnlock()` removal (no functional-test observable; would need a deadlock probe).
- `seen[v] = struct{}{}` removal and the dedup-`continue` removal: duplicate literals
  redact *identically* (same `ReplaceAll` result), so output is unchanged.
- `if s == ""` / `len(b) == 0` early-returns mutated to `_ = s` / `== -1` / `== 1`:
  the loops over an empty/short string produce the same result, so behaviour is identical.
- comparator `>` → `>=` and `<` → `<=`: differ only for equal-length, equal-value
  items, which cannot occur after dedup.

## Fix
`kernel/redact/redact_m490_test.go` — four tests, each asserting a property whose
violation leaks a secret:
- `TestSetSecrets_MinLengthBoundaryIsEight` — 8-char redacted, 7-char intact.
- `TestSetSecrets_ShortValueDoesNotStopLaterSecrets` — `["ab", real]` → real redacted.
- `TestSetSecrets_DuplicateDoesNotStopLaterSecrets` — `[dup, dup, other]` → other redacted.
- `TestSetSecrets_OverlappingRedactedLongestFirst` — inputs shortest-first; the longer
  secret's unique tail must not survive.

## Result
Score **0.725** — 29 killed, 11 survived. The 6 genuine gaps are closed; the
remaining 11 are the equivalent mutants enumerated above, so **every non-equivalent
mutant is now killed**. These tests would fail if a future edit flipped the length
floor, turned a `continue` into a `break`, or broke the longest-first sort — each a
silent secret-leak regression the suite previously missed.

## Verification / gate
- `kernel/redact` tests pass (incl. the 4 new); `go vet` + `staticcheck` clean;
  gofmt-clean on the staged LF blob (the new M489 CI gofmt job flagged an initial
  comment-alignment slip, which was fixed — the gate earning its keep immediately).
- Full `go test ./...` exit 0; `go.mod`/`go.sum` unchanged.

## Note
Mutation testing is the one gate that grades the tests rather than the code. redact
was chosen first as the highest-stakes package (a redaction regression leaks secrets
into a permanent journal). The same method applies to the other integrity-critical
packages (journal hash-chain, edict policy) as a follow-up.
