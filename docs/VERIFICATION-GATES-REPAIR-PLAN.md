# Verification-layer repair plan

> **Generated:** 2026-08-12, from a full-tree scan at `8f577e1e`.
> **Scope:** the checks that guard the tree — not the tree itself.
> **Companion to:** [`REFACTORING-SCAN-2026-08.md`](REFACTORING-SCAN-2026-08.md) (the Phase 0–4 arc whose
> extractions produced most of the drift below).

## Summary

The running system is healthy. Every behavioural check passes, on a real daemon, against the
bundle that actually ships. What is broken is the **verification layer**: two of `make check`'s
seven gates are red, one of them will destroy a correct document if you follow its own error
message, and a third guard is blind by construction to 13 environment variables.

None of the three findings is a runtime defect. All three are ways the tree can now drift
without anything noticing — which is the failure mode the gates exist to prevent.

### What was verified green (executed, not assumed)

| Gate | Command | Result |
|---|---|---|
| Build | `go build ./...` | ✅ 183 packages |
| Vet | `go vet ./...` | ✅ |
| Tests | `go test ./...` | ✅ all packages |
| Static analysis | `staticcheck ./...` (2026.1) | ✅ 0 findings |
| Vulnerabilities | `govulncheck ./...` (v1.4.0) | ✅ none |
| Formatting | `gofmt -l` over git index content | ✅ 0 files |
| Contract codegen | `jsonschemagen` diff vs `contract/gen/types.gen.go` | ✅ in sync |
| Dependency allowlist | `go run ./tools/depscheck` | ✅ 24 deps justified |
| Changelog layout | `go run ./tools/changelog-lint` | ✅ |
| Coverage ratchets | 10 pinned packages @ 100% | ✅ 10/10 at 100.0% |
| Frontend types | `tsc --noEmit` | ✅ |
| Frontend tests | `vitest run` | ✅ 187 files / 1543 tests |
| Frontend dead code | `knip --reporter json` | ✅ `{"issues":[]}` |
| Embedded bundle | rebuild `frontend/src` → diff `kernel/webui/dist` | ✅ zero drift |
| E2E smoke | `scripts/e2e-smoke.sh` (real daemon, isolated `AGEZT_HOME`) | ✅ PASS, 0 panics |
| Web UI E2E | `scripts/webui-e2e.sh` (Playwright vs embedded SPA) | ✅ 2/2 |
| SDKs | Python 21 · TypeScript 14 · Rust 13+3 · `cargo fmt --check` | ✅ |
| Repo hygiene | `git ls-files -ci --exclude-standard` | ✅ empty |

---

## Finding 1 — `tools/sdkparity` route discovery is dead

**Severity: high.** CI job `deps-check` has been red since **2026-07-26**, and the remedy the
tool prints is destructive.

### Evidence

`tools/sdkparity/main.go:36` reads exactly one file and greps one shape:

```go
routes, err := extractRoutes(filepath.FromSlash("kernel/restapi/restapi.go"))
...
re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`)
```

Commit `c7dded37 refactor(http): centralize route auth and body limits` (2026-07-26) replaced
that registration style with a policy-carrying router:

```go
router.Handle("/api/v1/health", userRoute, s.handleHealth)
```

Measured: **zero** occurrences of `mux.HandleFunc` across all 14 files in `kernel/restapi/`.
`extractRoutes` therefore returns an empty slice.

### Why it is worse than a stale doc

`docs/SDK-PARITY.md` is **still correct** — its 13-row `/api/v1/*` table matches the live router
exactly. The generator is what regressed. But `-check` compares generated (empty) against the
file (correct), reports "is stale", and instructs:

```
docs/SDK-PARITY.md is stale; run `go run ./tools/sdkparity -out docs/SDK-PARITY.md`
```

Running that deletes all 13 rows and rewrites every SDK's coverage line from `9/11` to `0/0`.
Following the error message destroys accurate documentation and reports the REST API as absent.

The tool's own suite passes, because nothing asserts the extractor finds a non-empty route set.

### Fix

1. `extractRoutes`: match `router\.Handle\("([^"]+)"`. All 13 registrations are still in
   `restapi.go`, so the single-file scan stays valid. `/healthz`, `/readyz`, `/metrics` are
   registered the same way but are already filtered by the existing `/api/v1/` prefix check —
   behaviour preserved.
2. Add a guard test: `extractRoutes` over the real `restapi.go` must return `len(routes) > 0`.
   This is the assertion whose absence let a silent 17-day regression through.
3. Regenerate and confirm the diff against the committed doc is empty. If it is empty, the doc
   never rotted and no content change lands.

**Risk:** none. Generator-only change; the expected outcome is a byte-identical document.

---

## Finding 2 — `deadcodecheck` reports 15 unreachable functions

**Severity: medium.** Same CI job, same red. All 15 are refactor residue from Phases 2.1, 2.2,
2.6 and the recent `budgetgate` change. The allowlist exempts only public-SDK findings, so every
one is a hard failure.

`tools/deadcodecheck` runs `deadcode ./...` **without** `-test`, so "reachable" means "reachable
from a binary". That is a deliberate and useful policy: it is exactly what surfaced item (a)
below. The findings split into three classes that want three different answers.

### (a) Production wiring that never landed — the substantive part

| Symbol | Situation |
|---|---|
| `channelwire.BuildAll` | Zero callers anywhere. `cmd/agezt/main.go:1206` hand-writes the identical `for _, m := range channel.Manifests() { BuildKind(...) }` walk. A literal duplicate of shipped code. |
| `channelwire.Describe` | Zero references, tests included. The boot-banner formatter Phase 2.1 wrote so banner text would stay uniform; the banner never adopted it. |
| `toolreg.Set.NetguardGaps` | The netguard drift alarm. Only its own unit test calls it, against a fixture `Set`. **Boot never runs it against the real built `Set`** — so a genuinely netguard-unaware spec ships undetected. The alarm exists but is not armed. |
| `channelwire.MissingFactories` | The manifest↔factory drift alarm. Redundant: `plugins/builtinchannels/factory_ratchet_test.go` re-implements the same loop inline over `channelwire.Lookup` instead of calling it. The alarm *is* armed — just not through this function. |

### (b) Test-only seams

`budgetScopeNamed`, `Governor.budgetExceeded`, `Governor.taskBudgetExceeded`, `toolreg.Lookup`,
`toolreg.Names`, `channelwire.Kinds`.

These are real, exercised API — `budget_boundary_internal_test.go` and
`plugins/builtintools/ratchet_test.go` drive them — but no binary reaches them. The
`budgetExceeded` comment ("kept as a named helper because it is the boundary the budget tests
drive directly") is accurate and is precisely why the analyzer flags it.

### (c) Legacy compatibility shim

`Server.SetDiskWatch`, `Server.SetProbeWatch`, `Server.legacyObservers`,
`funcObservers.AddDiskObserver`, `funcObservers.AddProbeObserver`.

Self-described in `pulse_control.go:178` as "the legacy per-func setters". Production now injects
a `pulseObserverAdmin` (`cmd/agezt/main.go:3527`) and goes through the interface; only tests
still take the shim path.

### Decision required: how the gate should treat class (b)

Two coherent policies. They are mutually exclusive and the choice is yours.

**Option A — per-item cleanup, keep the strict policy (recommended).**
Delete (a)'s two duplicates, arm or delete the two alarms, retire the (c) shim, and add a named,
commented allowlist category for (b) — "registry introspection whose only consumer is the drift
ratchet in another package". Cost: one allowlist section that must be maintained. Benefit: the
gate keeps meaning "nothing reaches this from a binary", which is the property that just caught
`NetguardGaps`.

**Option B — add `-test` to the analyzer invocation.**
`deadcode -test ./...` counts test-reachable code as live. Thirteen of the fifteen findings
disappear with no code change; only `BuildAll` and `Describe` remain. Cost: it permanently hides
the `NetguardGaps` class — a safety net that only its own test calls would look healthy forever.
That is the exact bug this scan found, so switching to `-test` would retire the check that found it.

Recommendation: **Option A.** Option B trades away the gate's only interesting property to save a
maintained list.

### Fix (assuming Option A)

1. Delete `channelwire.BuildAll` and `channelwire.Describe` (zero-reference).
2. `NetguardGaps` — **behaviour decision, see Open questions.** Either call it in
   `cmd/agezt/boot_tools.go` after `toolreg.BuildAll` and log/refuse on a non-empty result, or
   delete it and accept that the ratchet test is the only coverage.
3. `MissingFactories` — point `factory_ratchet_test.go` at it instead of re-implementing the loop,
   then allowlist it under (b); or delete it as redundant. Either is defensible; deleting is less
   code, reusing is less duplication.
4. Retire the (c) shim: migrate `pulse_control_test.go` to the `observers` interface the daemon
   actually injects, then delete `funcObservers`, `legacyObservers`, `SetDiskWatch`, `SetProbeWatch`.
5. Add the (b) allowlist category with one comment line per symbol naming its test consumer.

**Risk:** steps 1, 3–5 are mechanical and behaviour-preserving. Step 2 changes boot behaviour and
must be decided first.

---

## Finding 3 — 13 daemon env vars are in no configuration surface

**Severity: medium.** They are absent from **both** inventories: `controlplane.configEnvVars`
(so `agt config show` never reports them) and `kernel/settings/schema.go` (so Config Center
cannot display or set them).

### The 13

| Variable | Read at | Note |
|---|---|---|
| `AGEZT_AGENTGW_TOKEN_SECRET` | `kernel/agentgw/secret.go` | **security** — gateway token signing secret |
| `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED` | `kernel/creds/aws.go` | **security** — permits `credential_process` execution |
| `AGEZT_AGENTGW_SOCKET` | `kernel/runtime/runtime.go` | |
| `AGEZT_CHATGPT_OAUTH` | `kernel/chatgptauth/chatgptauth.go` | |
| `AGEZT_FILE_ROOT` | `kernel/webui/files_route.go` | Files browser root |
| `AGEZT_FILE_ROOT_MAX_BYTES` | `kernel/webui/files_route.go` | |
| `AGEZT_FILE_ROOT_MAX_ENTRIES` | `kernel/webui/files_route.go` | |
| `AGEZT_RETRY_PRESSURE_THRESHOLD` | `kernel/runtime/reaper.go` | |
| `AGEZT_RETRY_PRESSURE_WINDOW` | `kernel/runtime/reaper.go` | |
| `AGEZT_ANTHROPIC_THINKING_BUDGET` | `plugins/providers/compat/compat.go` | |
| `AGEZT_GOOGLE_THINKING_BUDGET` | `plugins/providers/compat/compat.go` | |
| `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET` | `plugins/providers/compat/compat.go` | |
| `AGEZT_AZURE_API_VERSION` | `plugins/providers/compat/compat.go` | |

Counted across every non-test file under `kernel/`, `plugins/`, `internal/`, `cmd/agezt`:
**417 distinct reads**, against 386 entries in `configEnvVars` and 239 in the settings schema.
`cmd/agt` is excluded — CLI-only vars such as `AGEZT_SHOW_REASONING` do not belong in a daemon
inventory.

Good news, also measured: **`configEnvVars` has zero stale entries.** Nothing is listed that is
not read. The list only under-reports, never over-reports.

### Root cause is structural, not clerical

`TestConfigEnvVars_CoversCmdAgeztReads` (`kernel/controlplane/config_inventory_test.go:30`)
scans a hardcoded **inclusion** list of eight directories. None of `kernel/agentgw`,
`kernel/creds`, `kernel/chatgptauth`, `kernel/webui`, `kernel/runtime`, or
`plugins/providers/compat` is in it, so the guard cannot see these reads.

The list has been extended reactively after each refactor phase — the comments read "Phase 2.5:
…", "Phase 2.6: …", "Phase 2.1/2.2/2.4 …". Every extraction that moves an env read into a new
package silently shrinks the guard's coverage until someone notices. The test's own docstring
says the invariant "had silently rotted" once before; it has rotted again, the same way.

Meanwhile `config.go:30` promises more than the guard enforces: *"every `Getenv("AGEZT_...")` in
`cmd/agezt/` (plus the daemon's kernel/plugin reads)"* — the parenthetical is unenforced.

### Fix

1. **Invert the scan** from an inclusion list to an exclusion list: walk every package under
   `kernel/`, `plugins/`, `internal/`, and `cmd/agezt`, skipping `_test.go` files and an explicit,
   commented set of exclusions. A new package then defaults to *covered* instead of *invisible*.
2. Add the 13 to `configEnvVars` (alphabetical, as the file requires). This is presence-reporting
   only — `config.go:451` records `env[name] = true` and never reads the value — so the secret
   var is safe to list.
3. Decide separately which of the 13 deserve an editable Settings field. If
   `AGEZT_AGENTGW_TOKEN_SECRET` is given one it must use the `pw()` password-field helper.
4. Align the `config.go:30` comment with whatever the guard actually enforces.

**Risk:** low. Steps 1–2 are additive and change no runtime behaviour; step 3 is UI surface.

---

## Hygiene items

Small, independent, no ordering constraints.

| # | Item | Detail |
|---|---|---|
| H1 | ~~Duplicate frontend lockfiles~~ — **withdrawn, was wrong** | The claim was that `frontend/pnpm-lock.yaml` is a stale committed lockfile. It is not tracked: `.gitignore:110` ignores it explicitly ("npm is the source of truth for frontend (VULN-010); keep a stray pnpm lock out"), and `security-report/dependency-audit.md` DEP-001 records it as resolved on 2026-07-26. The 2026-06-27 date I read from `git log` was the commit that **removed** it. The file on disk is an untracked local leftover in this working copy only — deleting it is a local tidy, not a repo change. |
| H2 | Docs not indexed | 21 of 39 files in `docs/` are absent from `docs/index.md`. It is a curated "start here" page, so plans and scans reasonably sit outside — but `ARCHITECTURE-DEEP.md`, `AGENT-SDK-ARCHITECTURE.md`, `SPEC-IMPLEMENTATION-STATUS.md` and `SYSTEM-AUDIT-REPORT.md` are reference material a reader would expect linked. |
| H3 | Stale security report | `security-report/` last regenerated 2026-07-29. Known-stale class per project convention: re-verify any finding against current source before acting on it. |
| H4 | Refactor doc header lag | `REFACTORING-SCAN-2026-08.md:3` still reads "EXECUTION STATUS (2026-08-06): Phases 0–2 COMPLETE" while the body tracks 3.1–3.6 and 4.1. Rows for 3.5 and 4.1 say "(this commit)"; both are now past commits and want their SHAs. |
| H5 | Thin Web UI E2E | One spec, two tests (`frontend/e2e/webui.spec.ts`) for a ~55-view console. Vitest carries the real load at 1543 tests, so this is a gap in *real-daemon* coverage rather than in coverage overall. |
| H6 | Local CRLF noise | 620 of 1571 tracked `.go` files have CRLF in the working tree, so a bare `gofmt -l .` reports 620 false positives locally. The git index is LF and formats clean, and CI checks out LF on Linux, so CI is unaffected. Worth recording so nobody "fixes" the phantom — verify with `git ls-files '*.go' \| xargs gofmt -l` against index content, never the working tree. |
| H7 | `.wrongstack/project.json` un-ignored but never added | **Restated — my first reading was wrong.** `.gitignore:125-127` is a deliberate three-line pattern: `!/.wrongstack/` un-ignores the directory, `/.wrongstack/*` ignores its contents, `!/.wrongstack/project.json` carves out one file. It works exactly as written — `AGENTS.md` and `statusline.json` are ignored, `project.json` is not. The real symptom is that `project.json` was carved out so it *could* be committed and then never was, so `git status` shows `?? .wrongstack/project.json` forever. It holds a third-party tool's workspace id (`{"version":1,"projectId":"proj_…"}`), so whether that belongs in the repo is an owner call: `git add` it, or drop line 127. **Left untouched.** |

---

## Finding 4 — the `secrets` gate was red too, and the original scan missed it

**Severity: medium (a red gate, not an exposure).** Found only when step 7 got round to actually
running `gitleaks` — the original scan reasoned from `security-report/`, which asserts the scan
"came back empty (`[]`)". That was true when written and had since stopped being true. **A gate
you have not executed is not a gate you have checked**, which is the same mistake that let
findings 1 and 2 sit for weeks.

Reproduced with CI's exact pinned version and command
(`go install github.com/zricethezav/gitleaks/v8@v8.30.1` → `gitleaks detect --no-banner --redact -s .`):
`generic-api-key` fires on `kernel/auth/auth_test.go:75`.

The value is `"0123456789abcdef"` written twice — a synthetic 32-char hex token whose only purpose
is to assert `WriteTokenFile` truncates it to the display prefix `0123…cdef`. No real secret.

`kernel/auth` was created by `c7dded37` on **2026-07-26** — the same route-auth centralisation that
broke `sdkparity`. One commit took out two CI gates, and neither failure was visible because the
runners were not reporting.

**Fixed** by adding the path to `.gitleaks.toml`'s allowlist with its justification, matching the
six existing test-fixture entries. `gitleaks` now reports `no leaks found` across all 1,657 commits.

## Execution record (2026-08-12)

All four findings are fixed. `deadcodecheck` reports
`OK: no unexpected dead code; 37 public SDK findings and 1 cross-package test seams allowlisted.`,
`sdkparity -check` passes, and `gitleaks` is clean.

**Finding 1.** `extractRoutes` matches the registration call shape rather than the `mux`
receiver name. Two guard tests added: a non-empty extraction against the real `restapi.go`
anchored on `/api/v1/health` + `/api/v1/runs`, and a fixture proving both the historical
`mux.HandleFunc` and current `router.Handle` forms parse while `/healthz` stays out. The
regenerated report is **byte-identical** to the committed document, confirming the doc was
never stale.

**Finding 2.** Answers taken: warn-and-continue for `NetguardGaps`, Option A for the policy.
- `Set.NetguardGaps` is now called in `cmd/agezt/boot_tools.go` after `toolreg.BuildAll` and
  warns on stderr. The alarm is armed against the real Set for the first time.
- Deleted with no replacement: `channelwire.BuildAll` (a duplicate of the walk in
  `main.go:1206`), `channelwire.Describe`, `channelwire.MissingFactories` (the real check is
  `TestEveryManifestHasFactoryOrTODO` over actual manifests) and its fixture-only test.
- Retired the legacy pulse shim: `funcObservers`, `legacyObservers`, `SetDiskWatch`,
  `SetProbeWatch`, plus the type switch they forced on every availability check —
  `diskWatchAvailable`/`probeWatchAvailable` collapse into one `watchesAvailable`. The test
  now drives the injected `PulseObservers`, the same seam the daemon wires.
- Same-package test-only helpers moved into `_test.go` files rather than allowlisted:
  `channelwire.Kinds` → `registeredKinds`, `toolreg.Lookup` → `lookupSpec`, and the governor
  `budgetScopeNamed`/`budgetExceeded`/`taskBudgetExceeded` probes.
- One symbol genuinely could not move: `toolreg.Names`, whose consumer is the ratchet in
  `plugins/builtintools` (toolreg cannot import the package that registers its specs). It is
  pinned in a new `testOnlyCrossPackageSeams` allowlist keyed `file|symbol` — deliberately not
  a directory prefix, so the next dead function in the same file is still reported. Covered by
  its own table test.

**Finding 3.** The guard walks `kernel/`, `plugins/`, `internal/`, `cmd/agezt` with a one-entry
exclusion list and a vacuity check. Widening the matcher past `os.Getenv(...)` surfaced **31**
uninventoried vars, not the 13 found by hand — the narrow pattern had been hiding
`const TokenSecretEnv = "AGEZT_…"` and `envLookup(lookup, "AGEZT_…")` forms, i.e. the whole
provider `compat` surface and the execution-profile backends. All 31 are in `configEnvVars`,
and all 31 now have Config Center fields (18 already did; 13 were added across `provider`,
`interfaces`, `security` and two new sections, `files` and `run-health`).

One judgement call inside "full Settings coverage": `AGEZT_CHATGPT_OAUTH` is not an operator
knob — it is the vault key holding the OAuth token blob that `Sign in with ChatGPT` writes. It
gets a `ReadOnly` + `Secret` field, so the console shows whether a sign-in exists but refuses a
pasted value. Everything else is editable.

Verified after the changes: `go build ./...`, `go vet ./...`, `go test ./...`, `staticcheck ./...`
(0 findings), gofmt on every changed file, and all seven `make check` gates.

## Open questions — resolved 2026-08-12

1. **`NetguardGaps` at boot** → **warn and continue.** Consistent with the boot-resilience law:
   an unjournaled refusal is a visibility loss, not an open door — the guard still blocks — so
   it must not take the daemon down.
2. **Deadcode policy** → **Option A**, per-item cleanup keeping the strict gate. `-test` was
   rejected because it would have hidden the `NetguardGaps` class permanently.
3. **Settings UI** → **full coverage.** All 31 vars get fields, with `pw()` for
   `AGEZT_AGENTGW_TOKEN_SECRET` and a `ReadOnly` exception for `AGEZT_CHATGPT_OAUTH`, which is
   a vault-written token blob rather than a knob (see the execution record above).

## Proposed execution order

Each step is independently green and shippable; nothing here is a big-bang branch.

| Step | Work | Status |
|---|---|---|
| 1 | Finding 1 — fix `extractRoutes` + non-empty guard test | ✅ done |
| 2 | Finding 3 — invert the guard scan, add the uninventoried vars | ✅ done (31, not 13) |
| 3 | H4 — doc header + commit SHAs | ✅ done (H1 withdrawn, H7 restated) |
| 4 | Finding 2 classes (a) and (c) — arm the alarm, delete duplicates, retire the shim | ✅ done |
| 5 | Finding 2 class (b) — move same-package helpers, allowlist the one that can't move | ✅ done |
| 6 | Finding 3 — Settings fields | ✅ done (all 31) |
| 7 | H2, H3, H5 — docs index, security re-verification, E2E breadth | ✅ done (surfaced Finding 4) |

**Step 7 detail.**

*H2.* `docs/index.md` gains two sections — "Engineering program" (refactoring index, the current
master scan, this plan, the dead-code audit) and "Point-in-time status reports" (Jarvis vision,
NEXT, spec-status, system audit, missing-parts), the latter carrying an explicit warning that each
is a snapshot stamped with the commit it was measured at. The individual `REFACTOR-*-PLAN.md` files
stay unlinked on purpose: `REFACTORING-INDEX.md` is their entry point, and listing all fifteen would
bury the docs a newcomer actually needs. The checkable-artifacts table gains `deadcodecheck`,
`changelog-lint`, and the contract-codegen diff.

*H3.* The report was **not** regenerated — that needs a full scan run, not a repair pass. Instead
the four cheap machine checks were re-run against current source and the result stamped into
`security-report/SECURITY-REPORT.md` as a dated partial re-verification: `govulncheck` clean,
`staticcheck` 0 findings, `depscheck` 24 justified, and `gitleaks` — which was **not** clean, giving
Finding 4 above. The stamp says plainly that the remaining findings are still `ef7b412d`-era and
must be re-checked before being acted on.

*H5.* New `frontend/e2e/views.spec.ts`, a breadth companion to the existing depth spec. It walks
every section and mounts every view the nav offers, asserting each renders a non-empty `<main>` and
logs no console error, with errors attributed to the view that produced them. The item list is read
from the DOM, so a view added to `src/nav.tsx` is covered the day it lands; only the eight section
labels are pinned, because the rail and the item list are both plain buttons in the same `<nav>`
with no accessible distinction to filter on. It exists for a specific past regression: a design
sweep left `ui/tab-nav.tsx` uncontrolled, so Dashboard, Runs and Status Overview shipped a blank
panel while every unit test passed.

**66 views mounted, 0 blank, 0 console errors, 3.9s.** The count is printed on success and guarded
by a floor assertion — a spec that quietly stops covering things is worse than one that fails. My
first attempt derived the section labels from the DOM too, conflated rail and item buttons, and hung
for ten minutes clicking a "group" named Jarvis; the vacuity guards were added after that.
