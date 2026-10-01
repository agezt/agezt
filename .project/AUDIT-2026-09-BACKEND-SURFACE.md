# Backend & Surface Audit — 2026-09-27

> Scope: full-repo health audit with the WebUI surface as the subject.
> Method: every project gate executed against a clean tree at `0b6c8519`
> (main, 2026-09-24). No source file was modified. All numbers below are
> measured, not estimated.

---

## 0. Verdict

**The kernel was sound; the verification layer was not.** Every gate the
project already had was either red, unwired, or measuring the wrong string,
and the WebUI's "dismantled but still documented" surface was a documentation
drift that no gate could see. Nothing in the build, the tests or the typecheck
noticed any of it.

That was the finding as the audit began. It turned out to be the *third* most
important one, and the honest reading of the whole exercise is:

| | |
|---|---|
| **The finding that outranks the rest** | **20 of ci.yml's 22 checks could not run.** No runner was registered for this repo; the pool they named lives on another host and was not reporting. A queued job is neither pass nor fail, so nothing turned red and a dashboard reading "no failing checks" was indistinguishable from a healthy pipeline — and because `main` requires the `CI` check, **no PR could merge at all**, including the one fixing everything else. (Finding I) |
| **The one the instinct got wrong** | The WebUI. Measured across all 38 live views: empty states 38/38, error 37/38, loading 33/38, 0 unresolved imports, and a shared 17-module design system the views already use. "Looks like a mess" was mostly not there. What was there was narrow and worse: a three-tab row where all three tabs rendered one component, two rail entries opening one page, and two screens that reported a healthy system when the daemon was unreachable. (Finding J) |
| **The two real product bugs** | The shell tool enforced its output budget on the wrong string; the Conductor store grew without bound. Both sat behind gates that existed and were green. The shell test meant to catch it decided its verdict on machine speed, so it passed or failed with the load. |

Turning the CI pool back on also exposed **six more defects the dead pool had
been hiding**, and two coverage ratchets that were red on `main` and not
introduced here (`kernel/tunnel` at 99.0% on Linux; the voice/Jarvis ratchet,
which had been pointing at source files that no longer existed and so had never
run at all).

Everything found is fixed and committed on `audit/2026-09-surface`
(pushed; the commit count is deliberately not written here — a live number in
a document about its own branch is stale the moment the next commit lands, and
`REVIEW-MAP.md` carries the ordered list). `main` is untouched at `0b6c8519`.

The full CI history for the branch is three consecutive 22-of-22 green runs.

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

The split blocks were not the only ones. **98 packages declared several package
doc comments** — invalid Go, spread across kernel, internal, cmd, plugins,
sdk and tools. `go/doc` concatenates them, so the summary a developer, an
IDE, or `go doc` showed first was:

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
8 sections · 27 rows · 36 views
Talk       Jarvis · Chat · Voice
Observe    Monitor(mission, feed) · Runs
Automate   Workflows · Triggers(schedules, standing) · Autonomy
Govern     Approvals · Policy · Oversight(overseer, council)
Agents     Agents · Roster · Skills · Capabilities(market, execution-profiles) · Sandbox
Knowledge  Memory · World · Data & Files(data, artifacts) · Thinking Partners(research, analyst, reflect)
Connect    Providers & Models · Routing · Channels · Integrations(mcp, acp, connections)
Admin      Setup · Config Center · Backups
```

*(Retimed 2026-10-01: Runs lost its Activity and Replay facets and the Identity
row went away — see Finding J. Both rendered a component another entry already
rendered.)*

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

**The audit is closed.** Everything below is landed on
`audit/2026-09-surface` — pushed, `main` untouched at `0b6c8519`
both locally and on `origin`.

| # | Item | Result |
|---|---|---|
| 1 | **Decide the console-surface question** | **The retirement stands.** Restoring would have meant authoring ~12 UI surfaces whose implementations were deleted — inventing product, not recovering it. `docs/CONSOLE-IA.md` records the dated decision with its reasoning and how to reverse it, and the capabilities' continued CLI availability is stated. No kernel handler was touched. **The one decision made without asking.** |
| 2 | Surface counts never hand-written | ✅ `nav-docs.test.ts`, green |
| 3 | Delete the orphaned frontend surface | ✅ 51 dead files, 53 dead exports/types. `chat/legacy/` deliberately **kept** when linters called it a duplicate — it is the only copy of the Chat implementation — then renamed to `impl/` |
| 4 | Restore the prose package comments | ✅ 59 recovered from `52234e77`; regeneration is byte-identical |
| 5 | Demote the split / file-scope blocks | ✅ 98 → 0 packages declaring more than one package comment |
| 6 | Wire the gate that had no CI step | ✅ `structure-md -check` added to the `deps-check` job |
| 7 | `gofmt` gate | ✅ 636 violations cleared; `.gitattributes` pins Go line endings; `fmt` target live in `check`, negative-tested |
| 8 | knip's `lazyNamed` blind spot | ✅ the 38 lazy-loaded views declared as entries — acting on the report would have broken the console |
| 9 | `e2e-smoke.ps1` | ✅ broken since 2026-07-06 (the shell twin got the token-file fix, the PowerShell twin never did); now matches `.sh` |
| 10 | `nav:audit` portability | ✅ the POSIX `tail` pipe removed |
| 11 | The three packages holding two package docs | ✅ `kernel/event` and `kernel/runtime/types` merged; `kernel/governor`'s contradiction **resolved against the code** |
| 12 | `.gitleaks.toml` rationale | ✅ stale "16 hits" replaced with measured numbers, scan re-verified green afterwards |
| 13 | **The CI runner pool** (Finding I) | ✅ 20 of 22 checks could not run; all 17 jobs moved to `ubuntu-latest`, `fetch-depth: 0` where the merge base is needed, and the runner pool documented DORMANT |
| 14 | **What the dead pool was hiding** (Finding I) | ✅ six defects fixed, incl. `setup-go-safe` testing the wrong env var, `ci-go-retry.sh` deleting the cache it then retried against, and a lockfile that made `npm ci` impossible |
| 15 | **Two coverage ratchets red on `main`** (Finding I) | ✅ `kernel/tunnel` 99.0% → 100% on Linux (one uncovered statement, now a test); voice/Jarvis ratchet repointed and taken to 100% on all four metrics |
| 16 | **The WebUI itself** (Finding J) | ✅ three nav entries retired for opening a page already one click away; `setActive` now resolves `VIEW_ALIASES`; two false all-clears fixed; views with a test 35/38 → 38/38 |

| Metric | At `0b6c8519` | Now |
|---|---|---|
| `structure-md-check` | ❌ exit 1 | ✅ exit 0 |
| `frontend-deadcode` | ❌ exit 1 | ✅ `{"issues":[]}` |
| Packages with >1 package comment | 98 | **0** |
| `.go` files failing `gofmt` | 636 | **0** |
| Packages with no doc comment | 59 lost | **0** |
| `staticcheck` findings | 3 | **0** |
| Tests whose verdict depends on machine speed | 3 | **0 fixed** |

`go test -race` needed no work: `race-breadth` and `race-depth` already run
it in CI.

### Which CI jobs were actually run here

**This subsection is obsolete and is kept only to show how the answer changed.**
When this report was first written, twelve of the seventeen jobs had been run
or verified by hand on this machine and five could not run here at all. The
report said plainly that those five were *not* verified.

They have now all run, in CI, on GitHub-hosted runners — see Finding I for why
that needed a change rather than patience. The five are no longer
"unverified":

| Was unverifiable here | Now measured in CI |
|---|---|
| `race-breadth`, `race-depth` | both green (linux, cgo) |
| `frontend-dist-in-sync` | green — the committed `dist` is byte-identical to a Linux rebuild |
| `frontend-dist-rebuild` | green (no-op on a pull request, as designed) |
| `rust-sdk` | green |
| `typescript-sdk` | green — but only after Finding I's lockfile fix; `npm ci` could not have succeeded before it |

The jobs this report had verified by hand — `deps-check`, `lint`, `secrets`,
`changelog`, `codegen-in-sync`, `multi-arch`, `test`, `e2e`, `webui-e2e` and
`python-sdk` — now all run in CI as well, so nothing above rests on a local
substitution any more.

`ci.yml` itself was parsed: 17 job definitions, 0 malformed steps, all 17 on
`ubuntu-latest`, every fork-guard intact, and the `structure-md -check` step
this audit added sits correctly in `deps-check`.

### The one judgement call left to the reader

`kernel/governor`'s `doc.go` described the provider chain as
"subscription-first → quality → cost → latency" in the present tense while
`governor.go` said that policy only lands with the model-catalog sync. The
code settles it: `routeChain` + `authModePriority` is purely a cost ranking,
so subscription-first and cost have shipped and quality and latency have not.
Both files now say that. Whether quality and latency *should* become ordering
dimensions is a roadmap question, not a documentation one.

## 12. Finding H — the frontend could not be installed at all — **FIXED**

**Resolved on evidence, not judgement.** The previous revision of this finding
declined to touch the pin because the advisory it responded to was not
determinable *from the repository*. It is determinable from the public
advisories, so it was looked up before deciding.

### The overrides block was two pins, and both were wrong

```json
"overrides": { "dompurify": "3.4.14", "undici": "8.19.4" }
```

| Pin | What it was | Verified against advisories | Now |
|---|---|---|---|
| `undici 8.19.4` | deliberate security pin from `76e666dd`, 2026-06-21, *"pins patched undici"* | the version does not exist (registry: `version not found`); the highest patched 8.x floor across the undici advisories is **8.10.2** (CVE-2026-84890, CVE-2026-84933, CVE-2026-84961); newest published 8.x is **8.11.2** | `8.11.2` — above every floor |
| `dompurify 3.4.14` | added in the v1.1.0 release commit | `npm audit` reports DOMPurify **3.4.13 – 3.4.15** vulnerable (*IN_PLACE afterSanitize hook leaves a detached subtree event handle*, reaching the build through monaco-editor); `latest` is **3.4.16** | `3.4.16` |

So the second pin was not merely stale: it sat *inside* a vulnerable range,
and `npm audit` said so. The first was unsatisfiable. Both replacements are
above every patched floor their advisories name.

### Verified after the change

| Check | Result |
|---|---|
| `npm ci --ignore-scripts` (CI's install step) | **exit 0** — was `ETARGET: No matching version found for undici@8.19.4` |
| `npm install` | exit 0 — 7 added, 4 removed, 116 changed |
| `npm audit` | **found 0 vulnerabilities** — was 2 low (dompurify, monaco-editor) |
| `tsc --noEmit` | exit 0 |
| vitest | 166 files / 1495 tests, 0 failures |
| `knip` | exit 0, `{"issues":[]}` |
| `make check` composition, 9 steps | **9/9 green**, `go test` 192 packages / 0 FAIL |

`kernel/webui/dist` was rebuilt from the new tree, since the dependency change
moves the bundle: 79 chunks renamed, 80 modified, 79 deleted. As with the
earlier rebuild, the bundle is checked for absolute-path leakage (none) and
committed LF, but byte-equality with a Linux build still cannot be proven from
this machine.

### What stays uncertain

Which advisory each pin was originally added for is still not recorded in the
repo — the changelog never mentions undici or dompurify, and no security report
references either. The replacements satisfy every advisory those packages
currently have, which is the strongest claim available without that history.
If the original pins were for something outside the public advisories, that is
not visible from here.

---


## 13. Checked and explicitly *not* a problem

- `contract/gen/types.gen.go` is untracked — **intentional**, `.gitignore:23-24`
  documents it as "regenerated by `make gen`". Not a bug.
- `docs/index.md`'s "make gen then verify no diff" is imprecise (the file is
  gitignored so `git diff` can't compare it) — a doc nit, not a defect.
- `chat/legacy/` looks like dead code and is not. Checked before deleting.
- No route orphans, no type errors, no test failures, no unexpected dead Go code.

---

## 14. Finding I — 20 of 22 CI gates could never run, and had not for months — **FIXED**

**This is the finding that outranks the rest of this report, and it was found
while trying to merge it.** Every other finding here is a defect in code or
prose. This one is that the apparatus meant to catch defects was not attached
to anything.

### The measurement

```
$ gh api repos/agezt/agezt/actions/runners
{"total_count":0,"runners":[]}
```

No self-hosted runner is registered for this repository. Meanwhile fifteen job
definitions in `ci.yml` carried:

```yaml
runs-on: [self-hosted, Linux, X64]
```

`multi-arch` is a six-leg matrix, so those fifteen `runs-on` lines account for
**20 of the file's 22 checks**. The runners they name are documented in
`ops/wsl-runners/README.md` as living on a different host (`WHITE`) — three
WSL-Ubuntu runners, `wsl-runner-1..3`. None of them is reporting.

### What it looked like from the outside

On PR #594, run `36774797606`:

- `updatedAt` never advanced past `createdAt`.
- The only two checks that reached a conclusion were the two already on
  `ubuntu-latest` (`race-depth`, `changelog gates`).
- The other twenty sat in `queued` for over an hour and would have sat there
  indefinitely. `check_suite.status` was `queued`, not `in_progress`.

The failure mode is the reason this went unnoticed for so long. **A queued job
is neither pass nor fail.** Nothing goes red, no notification fires, and a
dashboard showing "no failing checks" is indistinguishable from a dashboard
showing a healthy pipeline. The two jobs that *did* run were green — and
`ci.yml`'s own header had already noted, on 2026-09-06, that
`race-depth` succeeded in 6 of the 12 most recent runs *while every self-hosted
job in those same runs was cancelled because the pool had zero runners
registered*. The author had diagnosed it and written the rule down:

> Cheap new gates therefore belong on `ubuntu-latest`: a job that never gets a
> runner enforces nothing.

The rule was applied to new gates only. The fifteen jobs that already existed
were left pointing at a pool that had stopped existing, and the header's own
framing — *"Most jobs run on self-hosted WSL-Ubuntu runners, which cost
nothing"* — kept reading as a statement about how CI worked.

### The cost, concretely

`main` is protected and requires the `CI` check. With twenty checks unable to
conclude, **no PR could be merged at all** — including the one that was fixing
all of this. And twenty gates enforced nothing, among them
`frontend-dist-in-sync`, the single check that can answer whether the committed
`kernel/webui/dist` matches a rebuild (see §11: it does).

### The fix

All seventeen jobs now run on `ubuntu-latest`. The repository is public, so
hosted Linux minutes are free — the self-hosted design's only advantage was
cost, and it had cost the entire gate. The header was rewritten to record the
measurement, the rule, and a warning that re-pinning any job to
`[self-hosted, Linux, X64]` re-opens the hole. `ops/wsl-runners/README.md` is
marked DORMANT, with the explicit caveat that its setup table records the last
known configuration on `WHITE`, not a verified live state.

Nothing else had to change. `setup-go-safe` already branched on the runner
environment, and the two hosted jobs were the evidence it worked.

### Six defects the working pool had been hiding

Bringing the jobs up surfaced real breakage that no one had seen, because the
jobs meant to report it had no runner. Four were fixed in the same commit
rather than discovered by a red run; the prior art was branch
`ci-hosted-runners` (local, never merged, based on `1de3a1f5`), which made the
same migration and fixed the same failures in `d721c0bf`.

1. **`setup-go-safe` tested the wrong variable to detect a hosted runner.** It
   guarded on `[ -z "${RUNNER_NAME:-}" ]` under a comment asserting that
   `RUNNER_NAME` "is only set on self-hosted runners". GitHub's variables
   reference gives `RUNNER_NAME` the example `Hosted Agent` and lists no
   self-hosted-only restriction, so the guard **never fired**: every hosted job
   staged ~600MB of `GOROOT` onto tmpfs under a path containing a space
   (`goroot-Hosted Agent 2`). Confirmed from CI's own logs, where the runner
   names are `GitHub Actions 1000017247`. The correct discriminator is
   `RUNNER_ENVIRONMENT`, documented as exactly the `github-hosted` |
   `self-hosted` split.

2. **`scripts/ci-go-retry.sh` deleted the staged `GOCACHE`/`GOTMPDIR` and
   re-staged only `GOROOT`**, so every retry died with `creating work dir: no
   such file or directory` before the command ran, masking the real error
   behind five useless attempts. Both directories are now recreated per attempt.

3. **`sdk/typescript/package-lock.json` was out of sync with `package.json`** —
   `typescript 6.0.3` locked against `^7.0.2` required — so `npm ci` could not
   succeed in `typescript-sdk`. Regenerated; `npm ci` now exits 0.

4. **`kernel/creds` `credential_process` fixtures wrote unquoted helper paths.**
   GitHub's `TMPDIR` contains spaces (`/dev/shm/gotmp-GitHub Actions 1234`), so
   the tokeniser split the path into several argv tokens and every helper exec
   failed. Paths are now quoted, and `TestAWS_CredentialProcess_QuotedSpacedPath`
   pins a helper inside a directory literally named
   `GitHub Actions 1000016278`.

5. **Two symlink-TOCTOU proof tests in `plugins/tools/file` asserted the wrong
   channel for a refusal.** The tool signals a containment refusal as
   `agent.Result{IsError: true}`, not as a Go `error`, so a correctly-refused
   read fell through to `t.Fatalf("BUG: read returned unexpected output")`. The
   companion search test's substring assertion self-tripped, because the hits
   envelope embeds the search *pattern* — which in that test is the secret
   itself. That file is `//go:build !windows`, so it can only run on the Linux
   runners, which is precisely why a dead pool hid it.

### Two coverage ratchets, both red on `main`, neither a regression

The first run after the migration finished **20/22**. Both reds were pre-existing
rot on `main`, not anything this branch introduced — `git diff origin/main..HEAD`
is empty for `kernel/tunnel`, and the voice config's staleness predates the
audit.

- **`test (linux)` — `kernel/tunnel` at 99.0% statement coverage.** The
  100% ratchet on ten named packages runs only on the Linux runner, and the
  package had exactly one uncovered block: `proc_unix.go:28.72,30.3`, the
  `_ = cmd.Process.Kill()` arm of `killProcessTree`, which runs only when the
  process-group signal fails. No test made it fail.
  `TestKillProcessTree_FallsBackWhenGroupMissing` does — deterministically, by
  starting the child *without* `setProcessGroup`, so no process group carries
  its pid and `syscall.Kill` returns ESRCH on every run. That deliberately
  avoids the two ways this test would otherwise be a flaky way to `SIGKILL` a
  stranger's process group: pid reuse, and racing a child that has already been
  reaped. Re-measured in a `golang:1.26` container: **100.0%**, no function below
  100%, stable over `-count=5`.

- **`frontend-test` — the voice/Jarvis ratchet had never run.**
  `vitest.voice-coverage.config.ts` still listed `src/lib/voice*.ts` and
  `src/views/{Jarvis,Voice,VoiceSetup}.tsx`, but those surfaces had moved into
  feature slices and taken their tests with them: all nine configured test files
  and all eight configured sources were gone, and vitest exited 1 with
  *"No test files found"*. It failed `frontend-test` only by accident, on the
  wrong evidence, after the 1,454-test main suite had already passed and knip
  had already reported clean. Repaired, it immediately reported what it had been
  hiding: statements 99.67%, branches 94.83%, lines 99.81% against a 100%
  threshold. Ten tests later all four metrics are 100% and the step exits 0.
  The threshold was **not** relaxed — see the changelog for the states those
  tests cover.

### What this changes about the rest of this report

Three claims in earlier sections were true when written and are now known to be
optimistic:

- §10 asserted that "the gates this audit fixed are the gates CI already ran".
  True of the *definitions*; the runner pool meant some of them had not run in
  months.
- §11 originally listed five jobs as unverifiable here and said so plainly. That
  honesty was the right call, and the answer has since improved rather than
  degraded — all five are now measured in CI.
- Every "verified locally" in this report was verification *substituting* for a
  gate that was not running. That is a weaker guarantee than it looks like, and
  it is the reason this finding is filed at the top rather than the bottom.

### Still not proven

- The WSL runners' actual state on `WHITE` is unknown from here. Re-pinning a
  job to them without re-registering reproduces this finding exactly.
- The two ratchet fixes were measured locally — in a Linux container for the Go
  one, in the same vitest config for the frontend one — and then confirmed by CI.
  `plugins/tools/file`'s tests remain verifiable only on the Linux runners.

---

## 15. Finding J — the WebUI, audited for itself — **FIXED**

The audit above set out to check everything *outside* the web, because the WebUI
looked like a mess and the instinct was that the problem was there. The web was
then audited for itself, because that instinct was never actually tested.

**The instinct was wrong, and that is the finding.** Measured across all 38 live
views:

| | |
|---|---|
| views that handle an empty state | **38 / 38 (100%)** |
| views that handle an error state | 37 / 38 |
| views that handle a loading state | 33 / 38 |
| views with a test | 38 / 38 (100%) |
| unresolved imports | 0 |
| live ids still listed as retired | 0 |

Empty states are not hand-rolled per view. They are one shared component —
`<EmptyState icon title hint>` in `src/components/ui/`, a 17-module design system
the views already share. The "mess" was not a missing-states problem.

### What was actually broken, and it was small

Two things, both about the nav promising something it did not deliver:

**1. Three tabs that were one page.** `Observe › Runs` carried three tabs —
Runs, Activity, Replay — and all three rendered the `Runs` component. The
mechanism is one line in `App.tsx`:

```js
const current = NAV.find((n) => n.id === active) || NAV[0];
const View = current.render;      // instantiated with no props
```

The active view id never reaches the component, so a component cannot tell which
entry it was reached through. Activity and Replay were byte-identical to Runs.
A tab strip promises facets; these were three names on one screen.

**2. Two rail entries, one page.** `Admin › Identity` rendered the `Skills`
component — as did the `Agents › Skills` row beside it.

`nav.test.ts` caught neither, and the reason is structural rather than an
oversight: every test in it compares a row's **label** against the title its
render would show. "Activity" shared a root word with its render, "Identity"
was listed in `ROOT_WORDS`. The file has no test that asks whether two entries
open the same page.

**3. And a third, found on the way: a link to a view that does not exist.** The
Vitals bar's spend tile rendered `<button title="Go to today">` wired to
`onNavigate("budget")` — `budget` was retired with "no live equivalent" and is
not in `VIEW_ALIASES`, so the click fell through `viewFromHash`'s
`|| "mission"` and delivered Mission Control. `setActive` did not resolve
aliases either, so any in-app hop to a retired id first rendered `NAV[0]` (Chat)
until the hash round-trip corrected it — the exact failure the `VIEW_ALIASES`
comment warns about, already live.

### What changed

- The Runs row is one destination. `activity` and `replay` are retired and
  aliased to `runs`. The Identity row is retired; `prompts` is aliased to
  `skills`. The ids stay addressable, so bookmarks, help chips and ⌘K history
  land on a real surface.
- `setActive` resolves `VIEW_ALIASES`.
- The spend tile is a readout, not a button. A control that promises a
  destination it does not have is worse than one that never claimed to be.
- `nav.test.ts` gains a reference-identity guard: **no component may be rendered
  by two nav entries.** Negative-proven — pointing one row's `render` at
  another's component fails it with the two keys named.
- `nav-audit.spec.ts` gains a case asserting a retired id resolves to the
  surface its alias names, rather than merely to something with text on it.
- `AppNav.test.tsx`'s three Runs-specific tab tests were rewritten against
  Monitor. They had been asserting the defect.

Surface: 8 sections / 27 rows / 36 views, from 39. `nav-docs.test.ts` failed on
every document that quoted the old numbers until `README.md` and
`docs/CONSOLE-IA.md` were corrected — the gate working as intended.

### How the measurement went wrong first

Four separate scans produced four different answers before one was right, and
each wrong number was caught by opening a real file:

| Claimed | Actual | Why the scan lied |
|---|---|---|
| 19 views handle no states | 1 | Scanned `Market.tsx`, a re-export barrel whose code is in `page.tsx` — an empty shim |
| 21 views untested | 3 | Looked for `page.test.tsx`; tests are named after the nav destination, `Workflows.test.tsx` |
| Shared primitives barely used | consistent | Measured `Panel`/`DataView` — not `EmptyState`, the primitive states actually use |
| — | — | An alias chain measured per-line missed a multi-line `export { … } from` |

A heuristic scan compares files against each other, which keeps a *divergence*
meaningful even when a single verdict is not proof. None of these numbers was
reported until a real file confirmed it.
