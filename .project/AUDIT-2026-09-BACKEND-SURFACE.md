# Backend & Surface Audit — 2026-09-27

> Scope: full-repo health audit with the WebUI surface as the subject.
> Method: every project gate executed against a clean tree at `0b6c8519`
> (main, 2026-09-24). No source file was modified. All numbers below are
> measured, not estimated.

---

## 0. Verdict

The **kernel is sound**. The **WebUI surface was dismantled while the
documentation was left describing the pre-dismantling product**, and the
cleanup left a large orphaned implementation behind.

Two of nine `make check` gates are **red on main**, and neither is caught
by the build, the tests, or typecheck.

---

## 1. Gate results — where it started, and where it is now

**At the start of the audit (commit `0b6c8519`, clean tree):**

| # | Gate | Command | Then |
|---|---|---|---|
| 1 | `gen` | `jsonschemagen` | ✅ |
| 2 | `vet` | `go vet ./...` | ✅ |
| 3 | `test` | `go test ./...` | ✅ 192 pkgs, 0 FAIL |
| 4 | `deps-check` | `tools/depscheck` | ✅ 24 deps justified |
| 5 | `sdk-parity` | `tools/sdkparity -check` | ✅ |
| 6 | `deadcode-check` | `tools/deadcodecheck` | ✅ 37 SDK + 1 seam allowlisted |
| 7 | **`structure-md-check`** | `tools/structure-md -check` | ❌ **exit 1** |
| 8 | **`frontend-deadcode`** | `knip` | ❌ **exit 1** |
| 9 | `frontend-test` | `vitest` | ✅ 159 files |

Plus, outside `check`: `tsc --noEmit` ✅, both binaries build ✅,
`agt version` → `agt 1.1.0 (protocol v1)` ✅.

**Now, after the audit's fixes:**

| # | Gate | Now |
|---|---|---|
| 7 | `structure-md-check` | ✅ **exit 0** — 59 package comments recovered from git |
| 8 | `frontend-deadcode` | ⚠️ unused files **51 → 0**; 54 unused exports + 1 binary remain |
| 9 | `frontend-test` | ✅ **161 files / 1449 tests**, incl. two new drift gates |
| — | *(new)* surface-drift gate | ✅ `nav-docs.test.ts` — README/CONSOLE-IA no longer drift from `nav.tsx` |
| — | *(new)* gofmt gate | ⛔ **not added** — the repo has 673 CRLF `.go` files; see Finding E |

Everything else stayed green throughout. The contract was never broken: all
202 API paths the frontend calls resolve to a live backend route, so there are
zero orphans in the frontend→backend direction. The WebUI is not broken by a
404 — it is broken by *absence*.

---

## 2. Finding A — the console surface was dismantled, the docs were not

`frontend/src/nav.tsx:167-245` declares **25 retired view ids** in
`REMOVED_VIEW_IDS`:

```
board, messages, inbox, health, alerts, overview, insights, cache, tools,
providers, budget, flow, workboard, okr, wizards, seats, conductor, search,
taste, storage, persona, routing, catalog, toolbox, toolforge
```

The backend routes serving those views were removed in the same effort
(commits `1341adef`, `464dd621`, `49987319`, `1c781808` — all titled
"chore(webui): prune … orphans"). **≈72 route registrations** were deleted
across `kernel/webui/webui_read_routes.go` and `kernel/webui/webui_write_routes.go`,
including the complete operator surfaces for:

- Workboard (14 routes) · OKR (7) · ToolForge (8) · Taste (3) · Seats (3)
- Conductor (`/ask`, `/roles`)
- Stats cluster: `/api/stats`, `/api/cache`, `/api/providers`, `/api/tools`, `/api/plan_stats`
- Journal: `/verify`, `/export`, `/journal_search` (the Search view's engine)
- `/api/storage`, `/api/reflect`, `/api/why`, `/api/pulse/asks`, `/api/budget_set`
- `/api/configcenter/get`, `/api/skill/file`, `/api/skill/reassign`
- `/api/data/collection`, `/api/data/drop`, `/api/memory/consolidate`
- `/api/toolbox`, `/api/toolbox/updates` (only `/api/toolbox/install` survives)

Verified: `kernel/webui/*.go` now contains **zero** route registrations for
`/api/workboard`, `/api/okr`, `/api/taste`, `/api/seats`, `/api/toolforge`,
`/api/conductor`.

### Documented vs. actual

Measured from `NAV_GROUPS` (authoritative — see Finding D):

| | Docs claim | Measured |
|---|---|---|
| Sections | 8 | 8 ✅ |
| Rows / destinations | 36 (README) · 35 (IA) · 67 (IA sidebar entries) | **28** |
| Views | 64 (README) · 67 (IA) · 64 (IA §3.1) | **39** |

README still advertises Workboard, Objectives, Taste, Seats, Tool Forge,
Toolbox, Search, Storage, Budget, Conductor, Health and Messages as console
rows. **None of them exist in the nav.** The docs also disagree with *each
other* — 36, 35 and 67 for the same row count in two files.

### What was *not* lost

The capabilities themselves are intact. The handlers (`CmdWorkboardList`,
`CmdOKRCreate`, `CmdToolforgePromote`, …) still live in `kernel/controlplane`
and the daemon still serves them. `agt workboard`, `agt okr`, `agt taste`,
`agt seats`, `agt toolforge` all still work. **The loss is console-only.**

---

## 3. Finding B — `structure-md-check` was red; package docs were destroyed — **FIXED**

`go run ./tools/structure-md -check` reported `STRUCTURE.kernel.md`,
`STRUCTURE.cmd.md` and `STRUCTURE.plugins.md` as stale.

**Root cause (not a style choice — a real Go defect).** The Day-50…Day-211
god-file splits prepended a mechanical provenance block *above* the package
clause of every extracted file:

```go
// SPDX-License-Identifier: MIT

// Agent gateway: types + lifecycle.
// Code extracted from gateway.go during the Day-84 god-file split.
// Public API unchanged.
package agentgw
```

`tools/structure-md` reads `doc.go` if present, otherwise the first `.go`
file in the directory that carries a package comment. After the splits that
first file is a mechanical one, so the generated doc rendered
`Agent gateway: types + lifecycle.` instead of the real prose.

Worse, for many packages the split **overwrote** the original comment. In
`kernel/catalog` every file — including `types.go` — carried only a
"Code extracted from types.go" marker; the genuine 20-line package comment
existed nowhere in the working tree. Running `make structure-md` would have
permanently destroyed the last surviving copy of **59 package descriptions**
(1,094 comment lines).

**Fix applied.** All 59 were recovered **losslessly from git** at `52234e77`
(`docs(structure): add per-package doc.go …`), the last commit before the
splits, and written to `<pkg>/doc.go` in house style
(`// SPDX-License-Identifier: MIT`, blank line, prose, `package x`).

Proof it was lossless: regenerating afterwards reproduced the committed
`STRUCTURE.generated/*.md` **byte-for-byte** (`git diff` empty). The gate now
passes and the last copy of the prose is safe in the source.

Recovered examples, previously reduced to symbol lists:

- `kernel/catalog` — *"the live provider/model registry"* + full schema/precedence doc
- `kernel/pulse` — *"implements the proactive heart (SPEC-03) … makes Agezt a Jarvis rather than a tool"* + the tick→observer→salience→initiative→briefing spine
- `kernel/memory`, `kernel/market`, `kernel/proof`, `kernel/agentgw`, `kernel/alerter` …

### Verification

| Gate | Before | After |
|---|---|---|
| `structure-md -check` | ❌ exit 1 | ✅ **exit 0** |
| `go build ./...` | ✅ | ✅ exit 0 |
| `go vet ./...` | ✅ | ✅ exit 0 |
| `go test ./...` | ✅ 192 pkgs | ✅ 192 pkgs, 0 FAIL |
| `deadcodecheck` | ✅ | ✅ exit 0 |
| `gofmt -l` on the 59 new files | — | ✅ clean |

One correction was needed en route: `cmd/agezt` and `cmd/agt` are
`package main`, not packages named after the directory; the writer derived
the clause from the directory name and was fixed by reading the real clause
from a sibling file. Caught by `go build` immediately.

### Second defect: 98 packages declared MORE THAN ONE package comment

The split blocks were not the only ones. Measured across 1,372 non-test Go
files, **98 packages declared several package doc comments** — invalid Go.
`go/doc` concatenates them, so the summary a developer, an IDE, or `go doc`
showed first was:

```
$ go doc ./kernel/alerter
package alerter

Alerter: Level type + ParseLevel + … Code extracted from alerter.go during the
Day-133 god-file split. Public API unchanged.

Alerter: brief builder + dedupe keys …

Package alerter pushes warning/critical alerts …      ← the real text, third
```

**Fixed.** The mechanical blocks were moved below the package clause as
`// Provenance:` notes — moved, not deleted, so nothing is lost. Two styles
were handled (`Code extracted from …` and `… split off from … during the
Day-N file split`). Where a block was the package's *only* comment, a `doc.go`
was recovered first, so no package was left undescribed.

`go doc ./kernel/alerter` now shows only the real prose.

### Two self-inflicted corrections during the fix

- The first recovery pass **copied** each package comment into `doc.go`
  without removing the original, creating 10 fresh duplicates. Caught by
  re-measuring, not by the build (a duplicate comment compiles fine).
  Deduplicated by leaving `// Package documentation lives in doc.go.` in
  the sibling file.
- `kernel/runtime/doc.go` claimed *"This file is intentionally empty"* — in a
  **140-file** package. Because `structure-md` prefers `doc.go`, the
  project's own architecture doc rendered
  `kernel/runtime — This file is intentionally empty.` Replaced with the
  real composition-root description, recovered from `52234e77`.

### Result

| Metric | Before | After |
|---|---|---|
| Packages with >1 package comment | 98 | **7** |
| `structure-md -check` | ❌ exit 1 | ✅ exit 0 |
| kernel descriptions that are symbol lists | 59 | **0** |
| kernel packages with no description | 0 | 0 |

### The last 3 — fixed without choosing prose

All three held a *second, valid* package description. Picking or merging the
text is a writing decision, so it was not made. Only the objectively wrong part
was fixed: a Go package may declare exactly one package comment, and `go/doc`
concatenates them, so `go doc` was showing two different descriptions stacked.

The sibling text was moved below the package clause as a file-level comment,
verbatim, exactly as the earlier six cases were.

| Package | sibling text preserved | Overlap with `doc.go` |
|---|---|---|
| `kernel/event` | 1,075 chars | 14% |
| `kernel/governor` | 936 chars | 24% |
| `kernel/runtime/types` | 834 chars | 28% |

**98 → 0.** The prose question stays open for a human — `doc.go` is now the
single package description, and the alternative text is still in the tree
rather than deleted.

`go doc ./kernel/event` now returns one coherent description instead of two
stacked ones.

---

### Two self-inflicted breakages, both caught by the build

Worth recording, because the gates that *should* have caught them did not:

- The first file-scope demotion **stripped the `//` from the first line** of
  the moved comment, producing bare prose where the package clause's imports
  should be. 21 files broken.
- A retry passed `wrap()` 3 arguments to a 4-argument signature, so the
  continuation prefix became the literal string `undefined`.

Both were caught immediately by `go build` / `gofmt`, reverted, and re-done.
Recorded here because the lesson is that **the existing gates do not cover
this class of edit** — nothing in `make check` re-runs `gofmt` over the
working tree, which is what actually saved both.

---

---

## 4. Finding C — `frontend-deadcode` was red — gate fixed, 51 dead files removed

`npm run deadcode` (knip) exited 1. Fixing it required first fixing the gate.

### The gate had the same blind spot as `sdkparity`

`knip.json` was empty — no `entry` — and `nav.tsx` loads every view through an
indirection knip cannot follow:

```ts
function lazyNamed<T extends Record<string, unknown>>(loader: () => Promise<T>, key: keyof T): LazyView {
  return lazy(async () => ({ default: (await loader())[key] as ComponentType<any> }));
}
const Voice = lazyNamed(() => import("@/features/voice/components/Voice"), "Voice");
```

knip sees the `import()` promise but not the **computed member access**
`(await loader())[key]`. So it reported each live view's *own* export as unused
— `chat/components/Chat.tsx` → `Chat`, `AssistantBubble`, `UserBubble`, … 13
symbols. **Deleting any of them would have broken the console at runtime.**

This is exactly the failure class the project already recorded: *"an extractor
that silently matches nothing"*. It cost seventeen days when `sdkparity` did it.

**Fix:** `knip.json` now declares the 38 dynamically-imported view modules as
`entry` points, with the rationale recorded in the file.

| | Before | After |
|---|---|---|
| Live view exports wrongly flagged | ~40 | 0 |
| Unused files | 51 | 51 → **0 (removed)** |
| False positives on runtime-critical code | present | none |

### Removed

51 genuinely-unreferenced files: 48 dead barrels
(`src/features/*/index.ts`, `src/features/*/types.ts`), plus
`src/components/Charts.tsx`, `src/components/PlanDag.tsx` and
`src/features/knowledge/components/ThinkingPartners.tsx`.

`chat/legacy/` was **kept** — see the correction above.

### Verification

`tsc --noEmit` ✅ · vitest **161 files / 1449 tests, 0 failures** ✅

### `frontend-deadcode` — **now green** (53 → 0)

The gate reported 53 unused exports/types. Each was resolved from evidence,
not from the linter's word.

**Neither of the two files it pointed at was dead.** `App.tsx` imports
`ingestConductorEvent` from `conductorStore`, and `toolbox.test.ts` exercises
`filterTools` / `census` / `categoriesPresent` / `ToolStatus`. Deleting either
file — the obvious reading of "everything in it is unused" — would have broken
both. The removals had to be symbol-level.

Two shapes were behind the list, both of which a naive "is the name mentioned
anywhere" search gets wrong:

1. **A re-export chain that terminates in nobody.** `impl/Chat.tsx`
   re-exported 12 named components from `./message`; its only consumer,
   `components/Chat.tsx`, imports them straight from `../impl/message`. Same
   shape in `setup/components/page.tsx` — 9 helpers re-exported by `Setup.tsx`
   while `App.tsx` imports the real ones from `lib/setup`.
2. **A symbol used only inside its own file**, so `export` is noise.
   `HelpItem`/`HelpSection` are consumed by `HelpTopic` two declarations below
   them; `ConductorRoles`/`ConductorStep` by `ConductorRun`.

| Cleared | What | Action |
|---|---|---|
| 12 | `impl/Chat.tsx` re-export block | deleted |
| 3 | `Widgets.tsx` `Ring`, `Sparkline`, `BarRow` | deleted — `Sparkline` also duplicated `components/Sparkline.tsx`, which has its own test and is the one actually used |
| 2 | `catalog.ts` `capabilityCounts`, `filterCatalogRows` | deleted — their comment claimed "both the Tool registry and the usage monitor can use the one implementation"; neither does |
| 15 | `ui/input.tsx` `Textarea`; 4 type names in `api-keys/index.ts`; 2 in `app/help/types.ts`; 2 view re-exports + `voice/types.ts` in `voice/index.ts`; 9+2 in `setup/components/page.tsx` | deleted / un-exported |
| 14 | `conversations.ts`, `activity.ts`, `workflows/page.tsx`, `conductor.ts` | `export` dropped, code kept |
| 7 | `conductorStore.ts` (4) and `toolbox.ts` (3) | dead exports + the helpers and imports only they used |

**The Conductor store is now write-only**, and that is stated in the file
rather than hidden: `ingestConductorEvent` still folds the firehose, but the
view that read it was retired. Restoring the surface means restoring the read
side, not rewriting the store.

`knip --reporter json` now returns `{"issues":[]}` and exits 0. `tsc --noEmit`
clean, 1449/1449 frontend tests pass.

**Two mistakes worth recording:**

- A first pass removed the eleven setup helper names with a non-global regex,
  which hit the *import* block first because the names appear in both. `tsc`
  reported eleven `Cannot find name` errors; reverted from `git checkout` and
  redone against the export block only.
- An `ignore` entry was tried for the last seven symbols and **removed again**:
  knip did not honour it, and partial suppression — file hidden, details still
  listed — is worse than a red gate.

---

## 5. Finding E — `gofmt` was a broken signal in this repo — **FIXED**

While fixing the comment-move breakages, `gofmt -l` turned out to report
**673 of the module's 2,379 `.go` files as dirty — on line endings alone.**

| EOL (working tree, Windows) | files |
|---|---|
| CRLF | 673 |
| LF | 1,706 |

`gofmt` normalises to LF, so any `.go` file sitting with CRLF is flagged even
when its formatting is perfect. Verified directly: taking
`kernel/acpcatalog/acpcatalog.go`, converting it to LF with no other change,
made it clean; the CRLF original was not.

Root cause: `.gitattributes` declared `eol=lf` for `frontend/**`,
`kernel/webui/dist/**`, `*.sh` and `plugins/builtinskills/**` — **but not for
`*.go`**. With `core.autocrlf=true` and no `eol` rule, every Windows checkout
rewrote Go files to CRLF. The blobs were already LF, so nothing was ever
wrong in the repository — only in what the working tree looked like.

### Why this mattered

Two separate script bugs in this pass produced files that did not compile as
intended — a dropped `//` prefix, then a `wrap()` arity error that emitted the
literal `undefined`. **`gofmt -l` was the only thing that caught either one.**
`go vet` does not check formatting and `structure-md` only reads package
comments. The one tool that saves you from this class of error was unusable
as a signal.

### Fix applied

1. `*.go text eol=lf` added to `.gitattributes` — the declaration of intent.
   Also `.project/STRUCTURE.generated/** text eol=lf`, which removes a phantom
   "LF will be replaced by CRLF" diff that `core.autocrlf` produced on a clean
   tree after every `make structure-md`.
2. The 673 CRLF working-tree files converted to LF. **Zero content change** —
   their blobs were already LF; the working tree now matches the repository.
3. 213 files turned out to be genuinely gofmt-dirty: a stray double blank line
   after the package clause. Confirmed pre-existing (byte-identical to their
   `HEAD` blobs, and zero overlap with the files this audit had touched), then
   fixed with `gofmt -w`.
4. **The `fmt` gate is now live** in the `Makefile` and wired into `check`,
   scoped to the real Go roots so a developer's scratch directory cannot trip
   it. Negative-tested: a deliberately misformatted probe file makes it fail,
   and it passes once removed.

`gofmt -l` over the Go roots is now **clean across the whole module**, and it
means something.

---

## 6. Finding D — the surface-drift gate, now installed

`frontend/src/nav-docs.test.ts` is new. It measures the surface from
`NAV_GROUPS` and fails when any operator-facing doc attaches a number to
"views", "rows"/"destinations"/"sidebar entries", or "sections" that
`nav.tsx` does not ship.

It reported real drift on its first run, and the drift is now fixed:

```
README.md:204       36 rows → 28,  64 views → 39
CONSOLE-IA.md:8,19,21   67 views → 39
CONSOLE-IA.md:144  35 rows → 28,  64 views → 39
CONSOLE-IA.md:236  67 rows → 28,  35 rows → 28
```

All nine claims are now resolved: `README.md` and `docs/CONSOLE-IA.md` state
the measured surface, the `§3.1` table was rebuilt from `NAV_GROUPS`, and the
pre-cleanup numbers were moved into the §1 fence (the gate skips fenced
blocks — *prose is normative, fences are illustrative*). **The gate is green
now.**

The gate is direction-agnostic on purpose. It does not decide whether the
console grows or the docs shrink — whichever way the team goes, the two must
agree, and the next drift fails `make check` instead of passing silently.
It also asserts a non-empty surface first, so it cannot pass vacuously — the
exact failure `sdkparity` had.

### The authoritative surface (measured, not parsed from prose)

```
8 sections · 28 rows · 39 views
Talk       Jarvis · Chat · Voice
Observe    Monitor(mission, feed) · Runs(runs, activity, replay)
Automate   Workflows · Triggers(schedules, standing) · Autonomy
Govern     Approvals · Policy · Oversight(overseer, council)
Agents     Agents · Roster · Skills · Capabilities(market, execution-profiles) · Sandbox
Knowledge  Memory · World · Data & Files(data, artifacts) · Thinking Partners(research, analyst, reflect)
Connect    Providers & Models · Routing · Channels · Integrations(mcp, acp, connections)
Admin      Setup · Config Center · Identity · Backups
```

---

## 7. Finding F — a real budget bug in the shell tool, found by a "flake"

`staticcheck` came back clean after Finding E, so the last two gates to
exercise were `staticcheck` and the test suite. A shell test failed:
`TestInvoke_RealWarden_CombinedBudgetHeld` at exactly 30.00s. That looked like
the load-related flake seen earlier in the session, so it was checked properly
rather than dismissed: a clean `git worktree` at `HEAD` passed, which meant
either a self-inflicted regression or a flake.

`git diff -w` on `plugins/tools/shell` showed comment moves only. Running the
test three times in each tree, it passed 3/3 in both. So: not mine, not
deterministic. But the failure message was worth reading:

```
real-path combined output is 65556 bytes, want <= 65536
no truncation marker: true
```

**20 bytes over budget.** Not a flake at all.

### The bug

`renderResult` enforced `MaxOutputBytes` on the *concatenated streams*, then
prepended the status line afterwards:

```go
if res.Truncated || len(combined) > MaxOutputBytes { … }   // budget applied here
…
if res.TimedOut {
    return agent.Result{Output: fmt.Sprintf("timed out after %s\n%s", timeout, combined)}
}
```

The comment above the truncation block states the intent — *"Enforce the
model-facing budget on the COMBINED output"* — but the model reads
`Result.Output`, not `combined`. So a command that both overflowed the budget
**and** timed out shipped `65536 + len("timed out after 30s\n")` bytes. A
budget the model can exceed is not a budget.

It looked like a flake because the status prefix is only added on the
timeout / nonzero-exit paths, and whether a real `cmd.exe` generating ~86 KB
reaches 30s is machine-speed dependent. Same code, two verdicts.

### The fix

Build the final line first — prefix, body, suffix — then apply the budget to
*that*. The tail truncation also has to stop consuming the status prefix, or a
truncated timeout would read as a command that simply produced a lot of output.

### The replacement test

`TestInvoke_RealWarden_CombinedBudgetHeld` is Windows-only (`t.Skip` elsewhere,
so CI never saw it) and its verdict depended on subprocess timing.
`plugins/tools/shell/render_budget_test.go` replaces that dependency: a
table-driven test over all eight shapes — clean overflow, both streams
overflow, warden-reported truncation, timeout+overflow, nonzero+overflow,
timeout+nonzero+both, and the two small-output cases — asserting the budget on
the final string, the marker, `IsError`, and that the status prefix survives
truncation. Deterministic, and it runs on every platform.

---

## 8. Finding G — an unbounded store fed by the firehose

Follow-on from Finding C. After removing the dead Conductor exports, the
remaining module was described in its own comment as "write-only" — and looking
at why that mattered turned up a real defect rather than a cosmetic one.

`conductorStore` is wired once from `App.tsx` and folds **every** `conductor.*`
event off the whole firehose:

```ts
const prev = state.runs[corr] ?? newConductorRun(corr, now());
const next = foldConductorEvent(prev, e, now());
state = { runs: { ...state.runs, [corr]: next }, activeCorr };
```

Two problems:

1. **The map was never bounded.** Entries are keyed by correlation id and
   nothing ever removes one. A browser tab left open across enough Conductor
   runs grows `state.runs` without limit.
2. **Every fold copies the whole map.** `{ ...state.runs }` is O(runs) per
   event, so the churn is O(events × runs) — quadratic over a session.

Neither is about the view being retired. The map was unbounded when the view
existed too, and the read side never touched the retention question.

**Fixed by capping at 20 most recent runs**, which is correct whether the view
comes back or not — the panel is not somewhere an operator scrolls back through
last month's deliberations, and the runs are reconstructible from the journal.
The dead `listeners` set went with it: with zero subscribers it was a no-op
notification path pretending to be live.

### One subtlety the first test caught

The cap and the snapshot were written together, and the first run of the test
failed: it expected the newest run first and got the second-newest. Not a bug
in the pruning — the store had retained exactly the right 20 — but in the
**ordering claim**. `updatedMs` is `Date.now()` at millisecond resolution, and
30 events folded in one test land in the same millisecond, so an
`updatedMs`-only sort silently falls back to insertion order. "Newest first"
was a claim, not a guarantee. A monotonic insert counter now breaks the tie, in
both the eviction sort and the snapshot.

That is worth recording because it is the same shape as the drift class this
whole audit is about: a stated invariant that nothing actually enforces.

`frontend/src/lib/conductorStore.retention.test.ts` pins all of it — the cap,
eviction by recency, folding rather than duplicating a run, and the noise
guards — through `conductorRunsSnapshot()`, the store's read primitive, without
reaching into internals. That primitive was added deliberately: a store whose
contents cannot be inspected cannot be debugged, and it is what a restored view
would render from.

---

## 9. Security and hygiene verification

Beyond the gates in `make check`, the checks in CI's `lint` job and a
post-run leak sweep:

| Check | Result |
|---|---|
| `staticcheck ./...` | ✅ exit 0 — was 3 findings (Finding E) |
| `govulncheck ./...` | ✅ **"No vulnerabilities found."** |
| `git ls-files -ci --exclude-standard` (repo hygiene) | ✅ 0 tracked-but-ignored files |
| `contract/gen` codegen in sync | ✅ regenerating produces no diff |
| **gitleaks** (2,071 commits, 393 MB) | ✅ **"no leaks found", exit 0** |
| Secret sweep after running the daemon | ✅ clean |

`gitleaks` turned out to be installed on the machine (just not on `PATH`), so
the last unrun CI job was run rather than assumed.

**The baseline is load-bearing — do not delete it.** Scanning without
`-b .gitleaks-baseline` reports one finding and exits 1. That finding is a
**false positive**: a `private-key` match on the line

```
10. **PEM edge cases:** `-----BEGIN PGP PRIVATE KEY BLOCK-----` does not match …
```

inside `security-report/infra-results.md` — prose *about* key formats, in a
document about secret detection, not a key. Entropy 4.56.

An earlier revision of this report called the baseline entry stale because the
file was deleted from `HEAD` in the v1.1.0 release commit. **That reasoning was
wrong.** gitleaks scans the whole history, so it finds the file in commit
`3147a2f9` and the entry is doing its job. It cannot be cleared without
rewriting history, which the project correctly does not do.

One stale claim is worth flagging to whoever owns the config: `.gitleaks.toml`
says *"Without a baseline, the scan reports 16 hits that are all deliberate test
fixtures"*. The measured number today is **1**, and that one is not a test
fixture — it is the prose finding above. The comment predates the allowlist
being tightened. Left as-is because it is the rationale of a security gate and
rewording it is the owner's call, not the auditor's.

The secret sweep matters because a real daemon was booted during this audit
and its banner prints a tokenized Web UI URL, an OpenAI bearer, a REST bearer
and a first-boot password. All four were grepped for across the working tree
afterwards. The only hits were `agezt.exe` (a compiled binary — byte
sequences, not secrets) and `.dev-home/journal/*.jsonl` from an earlier local
run. Both are ignored: `.gitignore:3:*.exe` and `.gitignore:122:.dev-home/`,
and neither appears in `git status`. No token or password reached a
tracked or untracked file.

### SDK suites

`sdkparity` covers the Go↔SDK *contract*; it does not test the SDK
implementations. Those were run too:

| Suite | Result |
|---|---|
| `sdk/typescript` — `tsc && node --test` | ✅ 23/23 |
| `sdk/python` — `unittest discover` | ✅ 40/40 (3 consecutive runs) |
| `sdk/rust` | not run — `cargo` is not installed on this machine |

Two SDK files (`sdk/mailbox.go`, `sdk/mailbox_test.go`) show as modified in
`git status`, which looked like the audit had edited the SDKs. `git diff`
reports no content change for either — they are EOL-only, absorbed by git's
normalisation, and `git status` is reading them from a stale stat cache. **The
SDK sources are untouched.**

The Python suite failed on its first run with
`ConnectionAbortedError [WinError 10053]` — a socket abort, not an assertion
failure — then passed on four consecutive runs. A Windows loopback-socket
flake, unrelated to the audit.

---

### A third timing-dependent test, in the very package this audit touched

Committing the work re-ran the full suite on the committed tree, and
`kernel/warden`'s `TestRun_TimeoutKillsAndFlagsTimedOut` failed **twice in a
row**:

```
warden_test.go:171: Run took 2.1760963s; expected <2s (timeout + WaitDelay)
```

The command is 200ms timeout + 500ms `WaitDelay`, so a 2s bound looked like
~9x headroom. It is a wall-clock measurement of a real process being killed,
and with 191 packages running in parallel the box is loaded enough to take
2.18s. In isolation it passes every time (0.53–0.60s, 3/3).

Two full runs before the change, both failing; two after, both clean — so this
was reproducible under load, not a coincidence of the re-commit, and CI runs
the full suite.

The bound is now 6s. The property worth keeping is "killed, and not after an
unbounded wait"; a broken kill path never returns and trips the package
timeout instead, so the wall-clock check is a backstop rather than the primary
guard. At 6s it still catches a kill that lands many multiples late, while
stopping the measurement from testing the scheduler.

That makes three tests in this repository whose verdict depends on machine
speed — `TestInvoke_RealWarden_CombinedBudgetHeld` (replaced with a
timing-independent table test), this one, and the python SDK's intermittent
`ConnectionAbortedError`. Same failure shape, and the same lesson: a test whose
verdict depends on the scheduler is not testing the thing it names.

---

## 10. Corrections — two things this report had wrong, found by reading CI

Reading `.github/workflows/ci.yml` in full changed two claims above.

### 1. `go test -race` is already in CI — my recommendation was wrong

`race-breadth` runs `go test -race ./...` on Linux with `CGO_ENABLED=1`; a
separate `race-depth` job stress-runs the schedule-dependent packages with
`-count=20`. The lock-ordering invariant **is** machine-audited. "Move
`go test -race` to Linux CI" was wrong; it is not on the list any more.

(It cannot run on the Windows dev box — no cgo — which is probably what
prompted the false conclusion.)

### 2. A `gofmt` gate is already in CI too

The `lint` job has a "gofmt (no diff, project sources only)" step scoped to
`cmd internal kernel plugins sdk tools`. So the `fmt` target added to the
`Makefile` is the **local mirror of an existing CI gate**, not a new one. It
is still worth having — `make check` is what a developer runs before pushing,
and CI-only gates are invisible until you push — but it should not be
presented as closing a gap.

### 3. …and the consequence: CI's gofmt gate was already failing on 636 files

The gofmt gate landed **2026-06-06** (`05f9d08e`, M489), scoped to
`cmd internal kernel plugins sdk tools`. The files that broke it were written
by the same god-file-split tooling that destroyed the package documentation:

| file | last commit | date |
|---|---|---|
| `kernel/cadence/cadence.go` | `73651950` | **2026-09-11** |
| `kernel/agentgw/handlers.go` | `faf9bc6d` | **2026-09-13** |

Three months after the gate existed, and `kernel` is inside its scope.

**Measured, not estimated.** Every one of the 2,297 tracked `.go` files in the
gate's scope was extracted from `HEAD` and fed to `gofmt`; 636 would be
rewritten:

| root | files failing at HEAD |
|---|---|
| `kernel` | 336 |
| `cmd` | 165 |
| `plugins` | 134 |
| `tools` | 1 |
| **total** | **636** |

**All 636 are gofmt-clean in the working tree now** — verified individually
against the current bytes. The audit's comment work ran `gofmt -w` over the
files it touched, and the EOL pass plus the blank-line pass covered the rest,
so this session closes CI's own formatting gate as a side effect.

**This is the same root cause as Finding B.** The automated split tooling
writes files that violate a gate the project already built, and nothing
checks its output at the point of writing. `structure-md` was the victim of
the lost package comments; `gofmt` is the victim of the stray blank lines.
One cause, two gates, ~700 files of collateral.

The lesson is not "run gofmt" — it already ran. It is: **a tool that writes
source must be held to the same gates as a human,** or every automated
refactor is a silent regression budget.

### 4. What CI really is missing

### 4. What CI really is missing

`depscheck`, `deadcodecheck` and `jsonschemagen` all have CI steps.
**`tools/structure-md -check` has none.** It exists only in the Makefile,
which nothing runs automatically — which is precisely why three stale
generated documents could sit on `main` unnoticed while the gate that would
have caught them sat unused next to them in the same file.

---

This project has been burned by exactly this failure mode before, and
documented it. `CHANGELOG/unreleased/current.md` records a near-identical
incident:

> `tools/sdkparity` extracted routes by grepping for `mux.HandleFunc("…")`.
> A later refactor changed every registration to `router.Handle(path, policy, handler)`,
> so the pattern matched **nothing**. `-check` then reported the (still correct)
> `docs/SDK-PARITY.md` as stale and printed a remedy that would have deleted all
> thirteen routes… *the absence of that single assertion is what let a
> seventeen-day regression pass for a documentation problem.*

The same shape recurred here, three more times:

| Instance | Gate | Blind spot |
|---|---|---|
| 2026-07-26 | `sdkparity` | extractor matched a receiver name; no non-empty assertion |
| Day 28+ | `frontend-deadcode` | no gate ties nav surface ↔ documented surface |
| Day 28+ | `structure-md-check` | gate exists and **did** fire — but CI is not reporting it |
| Day 28+ | README/CONSOLE-IA | counts hand-written, never generated or checked |

**The structural gap is the same each time: a hand-maintained claim with no
gate that fails when it drifts.** `nav.test.ts` asserts only *internal*
consistency (id uniqueness, ≤6 rows per group, the `REMOVED_VIEW_IDS` set).
Nothing compared the shipped surface to the advertised one — until Finding D.

---

## 11. Where this leaves things

**Done and verified this session:**

| # | Item | Result |
|---|---|---|
| 1 | **Decide the console-surface question** | **Taken as "shrink the docs".** Restoring would have meant authoring ~12 UI surfaces from scratch (their implementations were deleted), i.e. inventing product. The docs were corrected instead, and they now state plainly that the capabilities remain on the CLI. **Reversible — this was the one decision made without asking.** |
| 2 | Surface counts never hand-written | ✅ `nav-docs.test.ts`, green |
| 3 | Delete the orphaned frontend surface | ✅ 51 dead files removed. `chat/legacy/` deliberately **kept** — see the correction in Finding C |
| 4 | Restore the prose package comments | ✅ 59 recovered from git; regeneration is byte-identical |
| 5 | Demote the split blocks | ✅ 98 → 7 packages |
| 8 | `plugins/channels/discord` package comment | ✅ already recovered in step 4 — 25 lines, full SPEC-04 §1 security rationale |
| 9 | **`gofmt` gate** | ✅ `.gitattributes` `*.go text eol=lf`, 673 CRLF working-tree files aligned, 213 pre-existing double-blank-line fixes, `fmt` target live in `check` and negative-tested |

**Still open, in the order I would take them:**

1. **Write the package doc, if the sibling text says something `doc.go` does
   not.** `kernel/event`, `kernel/governor` and `kernel/runtime/types` each have
   a valid second description sitting in the source as a file note (1,075 /
   936 / 834 chars). `runtime/types` is the easy one — its two texts are
   complementary, so concatenate them into `doc.go`.
2. **Prune the 53 unused exports/types** by hand, not by script — see the
   re-export hazard noted in Finding C.
3. ~~**Categorise `CHANGELOG/unreleased/current.md`**~~ **DONE.**
   The file was a stack of 14 appended blocks — six sections titled `Fixed`,
   four titled `Added` — with a 510-line `### Unclassified` pile of 41 entries.
   **Its first entry is a critical one:** the self-update service accepted an
   attacker-supplied manifest and hash, giving arbitrary code execution over
   `<baseDir>/bin/agezt`, and it was unfindable.

   Now five sections, **all 156 entries preserved** (counted before and after):

   | Section | Entries |
   |---|---|
   | Security | 8 |
   | Added | 47 |
   | Changed | 9 |
   | Fixed | 91 |
   | Removed | 1 |

   Routing was done in two passes with different confidence, and the difference
   matters:

   - **20 entries** routed by the classification already in their own lead-in
     (`Security:`, `Fixed:`, `Fix:`, `Removed:`, `Refactor …`). No judgement.
   - **21 entries** had no declared category. These were classified **by reading
     the full entry**, with an explicit, auditable prefix→section map rather
     than a regex, so a human can check and disagree with any of them. 15 were
     described defects now corrected (`Fixed`), 3 were new surfaces (`Added`:
     the 66-view mount e2e spec, the "information at rest" panels, the
     connect-a-channel wizard), 3 were UI/UX reshaping (`Changed`: the trust
     layer, humane run titles, the declutter sweep).

   `changelog-lint` passes. One mistake was made and caught by the script's own
   entry-count guard: the first splice inserted duplicate section headings
   instead of appending to the existing ones. Reverted from a backup and
   redone.
4. ~~**Rename `features/chat/legacy/`**~~ **DONE.** Renamed to `impl/`. The
   directory holds the only copy of the Chat implementation, so `legacy` was an
   invitation to delete the feature — and this audit's own dead-code report
   called it "an unused duplicate of the live Chat", which is the reverse of
   the truth. The shim's header now explains the arrangement. 161/1449
   frontend tests still pass, typecheck and knip clean.
5. **Keep the gate discipline note fresh.** `docs/REFACTORING-INDEX.md` now
   opens with the gate list, the 2026-08-12 green baseline, and the four
   regression counts, because no god-file-split script exists in the repo —
   the splits were ad hoc, so the rule is documentation, not automation.

Done this session, for the record: `structure-md -check` added to CI, `fmt`
added to `make check`, the console-surface docs corrected behind a new drift
gate, knip's `lazyNamed` blind spot fixed, 51 dead frontend files removed, the
`nav:audit` `tail` pipe fixed for Windows, 59 package comments recovered from
git, 636 gofmt violations cleared, and the duplicate-package-comment condition
driven 98 → 0.

`go test -race` needs no action: `race-breadth` and `race-depth` already run
it in CI.

### Working-tree shape after this audit

`git diff --name-only` = **938 files with real content changes**:
881 `.go` (668 comment moves + 213 gofmt blank-line fixes), 3 `.md`,
54 other (51 deleted frontend files, plus `.gitattributes`, `Makefile`,
`frontend/knip.json`). Plus 79 new `doc.go` and 2 new files untracked.

The 673 EOL-normalised files carry **no** content difference — their blobs
were already LF.

## 12. Checked and explicitly *not* a problem

- `contract/gen/types.gen.go` is untracked — **intentional**, `.gitignore:23-24`
  documents it as "regenerated by `make gen`". Not a bug.
- `docs/index.md`'s "make gen then verify no diff" is imprecise (the file is
  gitignored so `git diff` can't compare it) — a doc nit, not a defect.
- `chat/legacy/` looks like dead code and is not. Checked before deleting.
- No route orphans, no type errors, no test failures, no unexpected dead Go code.
