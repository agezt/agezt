# M388 — Correct post-M387 stale comment + lock the retrieval-pool exclusion (priority-C)

## Audit (read-vs-code)
Final category-C sweep of the remaining `deferred/not-yet/unimplemented` comments.
Result:

- **`kernel/skill/skill.go:55`** — "StatusQuarantined … (v1: operator-driven;
  **auto-quarantine deferred**)" is now **STALE**: M387 shipped auto-quarantine.
- `skill.go:50` (auto shadow-test deferred) — accurate (LARGE, deferred).
- `compat.go:43` (ADC unimplemented) — BLOCKED (external/federated ADC, never
  guessed per the standing rules).
- `creds/creds.go` (at-rest encryption, hot reload) — accurate; at-rest needs an
  OS-keychain platform dep (BLOCKED-class).
- `creds/sso.go:17` — an accurate *historical* note (explains SSO does NOT depend
  on SigV4; quotes a past report that was wrong), not a live deferral.
- `warden_linux.go`, `proc_windows.go` — platform backends (Linux full-namespace,
  Windows Job Object), BLOCKED-class.
- `host.go:737`, `check.go:626`, `agent.go:692` — accurate current descriptions,
  not deferrals.

So `skill.go:55` is the only stale comment; the rest are accurate descriptions of
genuinely LARGE-deferred or BLOCKED items.

## What
- **`kernel/skill/skill.go`** — rewrote the `StatusQuarantined` doc: operator-
  driven OR automatic past the failure threshold (M387), and noted that
  quarantined skills are excluded from the retrieval pool.

## Verification (lock-in)
The corrected comment asserts "quarantined skills are excluded from the retrieval
pool" — a property `Retrieve` enforces (`if !sk.Active() { continue }`,
retrieve.go) but that had **no test** (`retrieve_test.go` didn't exist).

- **`kernel/skill/retrieve_pool_test.go`**:
  - `TestRetrieve_OnlyActiveSkills` — a keyword-matching skill in
    draft/shadow/quarantined/archived is never retrieved; the same skill, active,
    is. Locks the retrieve.go:21 contract directly.
  - `TestAutoQuarantine_RemovesFromRetrievalPool` — ties M387 to retrieval
    end-to-end: an active skill is retrievable; after it auto-quarantines on 3
    failures it is gone from the pool (the "pulled from production" guarantee).
- **Negative control:** disabling the `!sk.Active()` filter in `Retrieve` → both
  tests FAIL (archived/quarantined skills get retrieved); restored byte-identical.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2174** passing (was 2172; +2). No CHANGELOG (developer comment + test only;
  the behaviour shipped in M387).

## Scope notes
- With this, the category-C offline sweep is clean: no remaining comment hides
  implemented functionality. The remaining deferred markers are accurate
  descriptions of LARGE-deferred or BLOCKED work (recorded in next.md's
  completeness summary).
