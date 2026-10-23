# W3.11 — the providers module's first half: the catalog and the ChatGPT auth have a home

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 10 —
`modules/providers` (catalog, chatgptauth, providerboot).

**Scope:** the step's **mechanical half**. The providerboot half is a different
kind of work (it needs a ports rewrite, and it is the next slice) — the reason
is in "What is deliberately left" below.

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/catalog` (12 files) | `kernel/modules/providers/catalog` | the live provider/model registry: the store, the discovery, the family and capability helpers, the pricing, the catalog sync |
| `kernel/chatgptauth` (8 files) | `kernel/modules/providers/chatgptauth` | the OAuth token manager for the "Sign in with ChatGPT" provider — the same subscription auth Codex CLI uses |

Both were already one module — the layer map named `kernel/catalog` and
`kernel/chatgptauth` as layer 3 in the module `providers` — but they sat at the
top of `kernel/`, outside the `kernel/modules/` tree W3 establishes as the
target home of the bounded contexts. The package names do not change, so the
move is an import-path change and nothing else: **77 importer files**
repointed.

The placement is now *derived* (`kernel/modules/*/...` → module `providers`)
and the **two hand-written layer rules are gone**.

## The ratchet did not move

**275 packages, 126 import violations, 13 call sites** — the same 126/13, with
**no new edge**. The module's name does not change (`providers`), so every
edge keeps its shape and only its path changes; the eight allowlist entries
moved with their edges:

- `cross-module kernel/runtime → kernel/modules/providers/catalog`
- `cross-module kernel/runtime/accessors → kernel/modules/providers/catalog`
- `cross-module kernel/runtime/runexec → kernel/modules/providers/catalog`
- `cross-module kernel/cadence/systemtasks → kernel/modules/providers/catalog`
- `upward kernel/governor → kernel/modules/providers/catalog`
- `plugin-reach plugins/providerboot → kernel/modules/providers/catalog`
- `plugin-reach plugins/providerboot → kernel/modules/providers/chatgptauth`
- `plugin-reach plugins/providers/compat → kernel/modules/providers/catalog`

## The proof

The moved packages keep their own suites — the catalog's family, capability,
pricing, credential, down-route and fuzz tests, and the ChatGPT auth's store,
proactive-refresh, code-exchange, import and logout tests — and they are the
proof that the move did not quietly stop testing anything.

**Four tests were added or strengthened**, and the mutation run says why: the
moved packages had four gaps on their critical paths, and the move is the
moment to close them.

- `TestHasCredentials_NilLookupRefuses` (catalog) — a provider that needs
  credentials and a **nil** lookup must be refused, not crash on a nil call.
- `TestToken_EmptyAccessTokenRefreshes` (chatgptauth) — a stored refresh token
  with **no** access token is not a usable session: the access token must be
  fetched, not returned empty.
- `TestToken_NearExpiryWithoutRefreshTokenDoesNotCallTheProvider`
  (chatgptauth) — a token about to expire but with **no refresh token** cannot
  be refreshed, so the daemon must keep using it rather than burn a round trip
  that cannot succeed. The test counts the provider calls, not just the token.
- `TestLogout_RemovesTheVaultSecret` (chatgptauth) — the logout must remove
  the secret from the **vault**, not only from the manager's memory. The old
  test asserted `HasTokens()` (the in-memory state) and would have passed with
  the bytes still on disk.

## Mutations — 10 mutations, 10 caught

| # | Mutation | Caught by |
|---|---|---|
| M01 | a provider that needs no credentials is refused | the catalog's credential suite |
| M02 | a nil credential lookup is accepted | `TestHasCredentials_NilLookupRefuses` (added) |
| M03 | a family without native JSON mode claims it | the catalog's family suite |
| M04 | the input price is off by a factor of a thousand | the catalog's pricing suite |
| M05 | the output price is off by a factor of a thousand | the catalog's pricing suite |
| M06 | an empty access token does not force a refresh | `TestToken_EmptyAccessTokenRefreshes` (added) |
| M07 | a token near expiry is never refreshed | the chatgptauth's proactive-refresh suite |
| M08 | the refresh token is not required for a refresh | `TestToken_NearExpiryWithoutRefreshTokenDoesNotCallTheProvider` (added) |
| M09 | a stored token set counts as no tokens | the chatgptauth's store suite |
| M10 | the logout leaves the tokens in the vault | `TestLogout_RemovesTheVaultSecret` (added) |

**The run found four gaps in the tests and they are fixed and named.** They
are the same class as the ones W3.8 and W3.10 found: the test asserted the
*state* and not the *effect*.

1. **M02 escaped.** The credential test covered "any env set", "none set" and
   "no env list", but never the nil lookup — a nil lookup with a provider that
   needs credentials is a crash, and nothing said so.
2. **M06 escaped.** The refresh test stored an *expired* token; a stored
   refresh token with an **empty** access token took the other path and was
   never exercised.
3. **M08 escaped.** The "no refresh token" test covered `ForceRefresh` with no
   tokens at all — not a token near expiry whose refresh would fail.
4. **M10 escaped.** The logout test asserted the manager's memory, not the
   vault: a logout that left the secret on disk passed it.

One further note, recorded honestly: M07's first anchor did not compile
(`exp` became unused) and was re-run with a compiling, opposite-sense anchor.
A build failure is not an escape and is not counted as one.

## The gates

All green: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (**275 packages, 126 import violations, 13 call sites** — the
ratchet unchanged, two hand-written layer rules gone), `tools/structure-md`
(+ its `-check`), `tools/changelog-lint`, `tools/docclaimscheck`,
`staticcheck`, and the full `go test ./...`.

## What the move broke, and what it cost

A move that renames a path breaks the references that name the path — and one
of them was **in CI**, where it would have been silent:

- **The race job's package list.** `.github/workflows/ci.yml` runs
  `go test -race` over `./kernel/chatgptauth/...`; the path moved with the
  package. A stale path there is not a failure — it is a package that quietly
  stops being race-checked. Fixed.
- **The dead-code checker's path prefix** (none for these two packages) and
  the **parity ledger** (no rows for them) needed nothing.
- **The layer map's two hand-written rules** are gone.
- **The documentation and the code comments** that named the old paths: the
  architecture set (`05`, `08`, `03`), the provider plans and specs
  (`.project/SPEC-01`, `SPEC-15`), the docs (`CONNECT`,
  `SPEC-IMPLEMENTATION-STATUS`, `VERIFICATION-GATES-REPAIR-PLAN`,
  `REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN`), the frontend's comment and the
  comments in the governor, the control plane, the market, the settings
  registry and the two provider plugins. All updated. The **dated** reports
  (`.project/PHASE-*`, `.project/AUDIT-*`, `.project/REVIEW-MAP.md`,
  `CHANGELOG/unreleased/m*.md`, `docs/ARCHITECTURAL-REPORT.md`) describe the
  tree as it was and are left alone — `REVIEW-MAP.md` still lists
  `kernel/roster`, `kernel/selfrepair` and `kernel/mcp`, which the earlier
  slices moved too.

## What is deliberately left

- **The providerboot half of this step.** `plugins/providerboot` is not a
  library that happens to sit under `plugins/` — it is a library that imports
  the **concrete** provider packages (`plugins/providers/compat`, `mock`,
  `openairesponses`) precisely so the kernel never grows a kernel→plugins
  edge; its own doc says so. Moving it into the module (L3) would make that
  edge **upward**, and it would also add a cross-module edge to `kernel/agent`
  (the middleware stack). So the module has to ask for the provider set
  through a **port** and the composition root has to supply it — the same
  shape as W3.7's workflow engine and W3.9's reaper. It is the next slice.
- **`modules/runs` last**: what remains of `kernel/runtime` + `runexec` + the
  agent loop + resume + delegation + intervention; `kernel/runtime` is
  deleted when empty.
- **The agents module's overseer ops** (step 8's third item) — the api door
  it needed now exists (W3.9).
