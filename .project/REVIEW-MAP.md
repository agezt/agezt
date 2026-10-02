# Review map — 2026-09-27/30 surface audit

**This is what the branch delivers, not a proposal.** `audit/2026-09-surface`
takes `0b6c8519` to here.

> **No branch-wide file totals are quoted here, and that is a correction.**
> Earlier revisions stated "N files changed · N modified · N added" and called
> them stable. They are not, and saying so was the mistake: every commit that
> touches a file changes them, so the `docclaimscheck` gate fired on this
> document five times during the work, each time correctly — a document written
> while the work is in progress drifts. Correcting the number only moved the
> expiry date to the next commit, which is exactly the same problem the commit
> count had and why that one was removed rather than restated.
>
> Reproduce them yourself instead; they are one command:
> `git diff --shortstat 0b6c8519..HEAD` and
> `git diff --name-status 0b6c8519..HEAD`.
>
> What a review actually navigates by is the **per-slice counts below** — "38
> files, 2,890 lines" — and those are stable, because each slice is a fixed set
> of commits that is already landed. The same goes for the branch's shape:
> 17 job definitions in `ci.yml`, 36 live views across 8 nav sections and 27
> rows, 8 sections × 17 jobs of CI, all of which are properties of the code
> rather than of how many commits it took to get there.

`main` is untouched, locally and on `origin`.

The commits are grouped below into the review order they were made in, each
standing on its own and individually revertible. The two product bugs are at
the front, in two-file commits, so they are not buried.

> **A note on the counts, because they were confusingly two numbers at one
> point.** While the work was still uncommitted, `git status` reported **1562**
> while `git diff --name-only` reported **1068**. Both were true: the extra
> ~490 were files whose line endings had been normalised, which git absorbs on
> comparison, so they never appeared in the diff but stayed in the status
> listing behind a stale stat cache. What counts is the **1064** the branch
> actually changes against `0b6c8519` — that is what this map describes and
> what a review should look at. The working tree is now clean.

Every slice rests on the same evidence: the ten gates in `make check` all
pass, `tsc --noEmit` is clean, `go test ./...` is 192 packages / 0 failures
and the frontend 162 files / 1454 tests. The full findings, and the mistakes
made along the way, are in `.project/AUDIT-2026-09-BACKEND-SURFACE.md`.

## Reproduce every claim yourself

Nothing above needs trusting. From a clean tree:

```sh
# the nine project gates
make check

# CI's lint job — gofmt and staticcheck are called zero-tolerance
gofmt -l cmd internal kernel plugins sdk tools contract examples
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

> The `gofmt` list is scoped to the Go roots on purpose. `gofmt -l .` over the
> whole checkout also walks `.temp_files/`, where a dozen deliberate
> `prove_F1_*` / `verify_F1_*` scratch programs live and are not gofmt-clean by
> design. A formatting gate that is red before anyone touches anything is a
> gate people learn to ignore — the same failure this audit found four times.

```sh
# the two product bugs this audit found have their own regression tests
go test ./plugins/tools/shell/ -run TestRenderResult_RespectsBudgetOnFinalOutput -v
cd frontend && npx vitest run src/lib/conductorStore.retention.test.ts

# the surface-drift gate that keeps README / CONSOLE-IA honest
cd frontend && npx vitest run src/nav-docs.test.ts src/consoledoc.test.ts

# the running system, not just its tests
go build -o .temp_files/agezt.exe ./cmd/agezt
go build -o .temp_files/agt.exe   ./cmd/agt
bash scripts/e2e-smoke.sh  .temp_files/agezt.exe .temp_files/agt.exe
bash scripts/webui-e2e.sh  .temp_files/agezt.exe .temp_files/agt.exe

# cross-build, the six targets CI compiles
for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 freebsd/amd64; do
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 \
    go build -trimpath -ldflags='-s -w' -o .temp_files/xout/${t}/ ./cmd/...
done

# SDKs — sdkparity covers the contract, these cover the implementations
cd sdk/typescript && npm test                                 # 23 tests
cd ../python    && python -m unittest discover -s tests        # 40 tests
```

So a failure is diagnosable rather than mysterious:

- `staticcheck` and `govulncheck` are not on `PATH`; they need
  `go run <module>@latest`.
- The e2e scripts boot a real keyless daemon and need `curl`. PowerShell twins
  exist for Windows — note `e2e-smoke.ps1` was broken until this audit fixed
  it (slice 4), which is why it is worth re-running rather than assuming.
- `sdk/rust` needs `cargo`, which was not installed on the machine this ran on.
- `sdk/typescript` reports **23** tests, not the 14 named in
  `.github/workflows/ci.yml` — the `mailbox` tests landed after that job was
  written. The CI comment is stale, not the suite.

## The commits

> The commit count is deliberately not stated here. Any number written into
> this file goes stale the moment the next commit lands -- including the commit
> that would fix it. `git log --oneline main..HEAD` is the authority, and
> `tools/docclaimscheck` verifies the numbers that *can* hold still.

Review order, oldest first — the sequence the slices below describe. The two
product bugs are deliberately near the front, in two-file commits, so they are
not buried under an 878-file diff:

`1bf10afa` **fix(shell)** — the output budget, enforced on the wrong string
`5b913061` **fix(frontend)** — the Conductor store, unbounded
`f868e431` **test(warden)** — the third timing-dependent test
`3d9af9be` **fix(docs)** — `kernel/governor`'s chain order, documented two ways

Newest first:

- `7cd51b1d` — docs: the review map's counts were pre-commit estimates, not measurements
- `c1d6df3c` — docs: the PR body's bisect claim was measured on 13 commits, not all of them
- `91059240` — docs: correct the CI job accounting, and restore the closing section
- `ec79821c` — docs: a ready-to-paste PR description for this branch
- `3fb0e825` — docs: bring the audit record up to the closed state
- `bfa2b325` — docs(gitleaks): the config's rationale cited a stale hit count
- `3d9af9be` — fix(docs): kernel/governor's chain order was documented two ways, and one was wrong
- `69bb2887` — docs(kernel): finish the last two packages holding two package docs
- `42bd5a4a` — docs: record the third timing-dependent test and the commit map
- `f868e431` — test(warden): give the timeout bound room for a loaded machine
- `70859382` — docs: the surface audit record -- seven findings and their measurements
- `5351b317` — refactor(frontend): remove 53 dead exports and rename chat/legacy to chat/impl
- `96dc3138` — chore(frontend): delete 51 unreferenced files
- `9c430bc1` — docs(changelog): 14 appended blocks into 5 sections, 156 entries preserved
- `feed2ace` — docs(console): state the surface that actually ships, and pin it
- `a48cf1ba` — ci: wire the one gate that had no CI step, and fix the Windows e2e harness
- `4bcf3c66` — style(kernel): gofmt -- blank-line fixes and LF normalisation
- `74ca24f5` — refactor: move 98 packages' second comment below the package clause
- `5b913061` — fix(frontend): bound the Conductor store the firehose feeds
- `1bf10afa` — fix(shell): enforce the output budget on what the model actually reads
- `86b6b3e7` — docs(kernel): restore 80 package comments destroyed by the god-file splits
## Slices

> The nine below describe the **content** changes, grouped by what they fix.
> The **commit** order interleaves them with the two single-file product-bug
> fixes (`1bf10afa` shell, `5b913061` conductor) and the follow-up commits, and
> is listed verbatim in the commit list above. Read the commit list for the
> order to review in; read these sections for what each group of changes is for.

## 1. Restore 79 package comments

**79 files, 37 lines.** The Day-50…Day-211 god-file splits overwrote each package's doc comment with a mechanical “Code extracted from …” header. 59 were recovered verbatim from `52234e77`, the last commit before the splits; 3 more (approval, toolbox, toolforge) after a comment move left them with none; 17 from the same commit; and `kernel/runtime/types` rewritten by hand to merge two complementary texts. Regenerating STRUCTURE.generated over the damaged source would have destroyed the last surviving copy — this order is the point.

- `plugins/channels/` — 14 files
- `plugins/tools/` — 13 files
- `plugins/providers/` — 6 files
- `kernel/runtime/` — 2 files
- `cmd/agezt/` — 2 files
- `kernel/creds/` — 2 files
- `cmd/agt/` — 1 file
- `kernel/acp/` — 1 file
- `kernel/agentgw/` — 1 file
- `kernel/alerter/` — 1 file
- `kernel/approval/` — 1 file
- `kernel/cadence/` — 1 file
- `kernel/catalog/` — 1 file
- `kernel/chatgptauth/` — 1 file
- `kernel/contextselect/` — 1 file
- `kernel/controlplane/` — 1 file
- `kernel/datalake/` — 1 file
- `kernel/delegation/` — 1 file
- `kernel/executionprofile/` — 1 file
- `kernel/market/` — 1 file
- `kernel/mcp/` — 1 file
- `kernel/memory/` — 1 file
- `kernel/okr/` — 1 file
- `kernel/openaiapi/` — 1 file
- `kernel/planner/` — 1 file
- `kernel/plugin/` — 1 file
- `kernel/pulse/` — 1 file
- `kernel/redact/` — 1 file
- `kernel/restapi/` — 1 file
- `kernel/resume/` — 1 file
- `kernel/roster/` — 1 file
- `kernel/scheduler/` — 1 file
- `kernel/selfrepair/` — 1 file
- `kernel/settings/` — 1 file
- `kernel/skill/` — 1 file
- `kernel/standing/` — 1 file
- `kernel/toolbox/` — 1 file
- `kernel/toolforge/` — 1 file
- `kernel/update/` — 1 file
- `kernel/warden/` — 1 file
- `kernel/webui/` — 1 file
- `kernel/workflow/` — 1 file
- `kernel/worldmodel/` — 1 file
- `plugins/builtinchannels/` — 1 file
- `plugins/builtintools/` — 1 file
- `plugins/providerboot/` — 1 file
- `plugins/sdk/` — 1 file

## 2. Move 98 packages' second comment below the package clause

**697 files, 7.557 lines.** 98 packages declared more than one package comment — invalid Go, and `go doc` concatenated them so the summary shown first was the split provenance. Moved below the clause, verbatim; no prose was chosen, merged or deleted. `kernel/runtime/types` was then merged by hand because its two texts were complementary.

- `cmd/agt/` — 142 files
- `kernel/controlplane/` — 121 files
- `kernel/runtime/` — 66 files
- `plugins/tools/` — 41 files
- `plugins/channels/` — 32 files
- `plugins/providers/` — 24 files
- `cmd/agezt/` — 23 files
- `kernel/creds/` — 17 files
- `kernel/webui/` — 14 files
- `plugins/builtinchannels/` — 12 files
- `kernel/cadence/` — 10 files
- `kernel/openaiapi/` — 10 files
- `kernel/skill/` — 10 files
- `kernel/memory/` — 9 files
- `kernel/agent/` — 8 files
- `kernel/configcenter/` — 8 files
- `kernel/selfrepair/` — 8 files
- `kernel/governor/` — 7 files
- `plugins/builtintools/` — 7 files
- `plugins/providerboot/` — 7 files
- `kernel/roster/` — 6 files
- `kernel/edict/` — 5 files
- `kernel/market/` — 5 files
- `kernel/mcp/` — 5 files
- `kernel/pulse/` — 5 files
- `kernel/worldmodel/` — 5 files
- `kernel/acpcatalog/` — 4 files
- `kernel/catalog/` — 4 files
- `kernel/executionprofile/` — 4 files
- `kernel/journal/` — 4 files
- `kernel/settings/` — 4 files
- `kernel/toolbox/` — 4 files
- `kernel/warden/` — 4 files
- `kernel/agentgw/` — 3 files
- `kernel/bus/` — 3 files
- `kernel/chatgptauth/` — 3 files
- `kernel/contextselect/` — 3 files
- `kernel/datalake/` — 3 files
- `kernel/delegation/` — 3 files
- `kernel/okr/` — 3 files
- `kernel/plugin/` — 3 files
- `kernel/restapi/` — 3 files
- `kernel/resume/` — 3 files
- `kernel/scheduler/` — 3 files
- `kernel/standing/` — 3 files
- `kernel/update/` — 3 files
- `kernel/workflow/` — 3 files
- `kernel/acp/` — 2 files
- `kernel/alerter/` — 2 files
- `kernel/approval/` — 2 files
- `kernel/planner/` — 2 files
- `kernel/redact/` — 2 files
- `kernel/toolforge/` — 2 files
- `kernel/workboard/` — 2 files
- `plugins/external/` — 2 files
- `plugins/sdk/` — 2 files
- `kernel/event/` — 1 file
- `tools/structure-md/` — 1 file

## 3. gofmt: 213 pre-existing blank-line fixes + CRLF→LF

**188 files, 362 lines.** CI's gofmt gate (landed 2026-06-06) was already failing on 636 committed files. Verified against the blobs, not the working tree. 673 files were also CRLF in a Windows checkout with no `*.go` eol rule, which made `gofmt -l` meaningless locally. Purely mechanical; verified per-file against the committed blob.

- `cmd/agt/` — 76 files
- `plugins/channels/` — 19 files
- `plugins/providers/` — 17 files
- `kernel/controlplane/` — 14 files
- `plugins/tools/` — 11 files
- `kernel/runtime/` — 7 files
- `kernel/selfrepair/` — 6 files
- `kernel/governor/` — 5 files
- `kernel/creds/` — 3 files
- `kernel/workboard/` — 3 files
- `cmd/agezt/` — 2 files
- `kernel/agentgw/` — 2 files
- `kernel/cadence/` — 2 files
- `kernel/memory/` — 2 files
- `kernel/restapi/` — 2 files
- `kernel/toolreg/` — 2 files
- `kernel/webhook/` — 2 files
- `kernel/webui/` — 2 files
- `plugins/builtinguardians/` — 2 files
- `plugins/external/` — 2 files
- `plugins/providerboot/` — 2 files
- `kernel/agent/` — 1 file
- `kernel/approval/` — 1 file
- `kernel/configcenter/` — 1 file
- `kernel/mcp/` — 1 file
- `kernel/workflow/` — 1 file

## 4. Wire the missing gates

**5 files, 115 lines.** `tools/structure-md -check` was the only project gate with no CI step — which is why three stale generated documents could sit on main. Added it. `fmt` was added to `make check` as the local mirror of CI's existing gofmt gate. `knip.json` gained the 38 dynamically-imported view modules: knip cannot follow `lazyNamed(loader, key)`, so it reported live view exports as unused.

- `.gitattributes`
- `.github/workflows/ci.yml`
- `Makefile`
- `frontend/knip.json`
- `frontend/package.json`

## 5. Correct the documented console surface

**5 files, 140 lines.** README and docs/CONSOLE-IA.md described 64 views / 36 rows; nav.tsx ships 39 / 28, and the docs disagreed with each other (36, 35 and 67 for the same count). Corrected to the measured surface, with the retired list and the note that the capabilities remain on the CLI. `docs/REFACTORING-INDEX.md` gained the gate-discipline rule. `frontend/src/nav-docs.test.ts` is the new gate that keeps the two in sync.

- `.project/STRUCTURE.generated/STRUCTURE.kernel.md`
- `README.md`
- `docs/CONSOLE-IA.md`
- `docs/REFACTORING-INDEX.md`
- `frontend/src/nav-docs.test.ts`

## 6. Changelog: 14 appended blocks → 5 sections

**1 file, 1.997 lines.** Six sections were titled `Fixed` and four `Added`, with a 510-line Unclassified pile of 41 entries whose first was a critical self-update finding (attacker-supplied manifest and hash → arbitrary code execution over <baseDir>/bin/agezt), unfindable. **187 entries**, none dropped: 156 were carried over from the original 14 blocks and 31 were added since — the two product-bug fixes this audit landed, the remainder of the audit's own work, the CI runner migration with the defects it exposed, and the WebUI audit that retired three nav entries opening a page already one click away. 20 were routed by the classification already in their own lead-in; 21 were classified by reading the full entry.

- `CHANGELOG/unreleased/current.md`

## 7. Delete 51 dead frontend files

**52 files, 1.451 lines.** 48 unreferenced barrels (`features/*/index.ts`, `features/*/types.ts`) plus Charts.tsx, PlanDag.tsx and ThinkingPartners.tsx. `features/chat/legacy/` was deliberately KEPT — see the correction in slice 8.

- `frontend/src/` — 52 files

## 8. Remove 53 dead exports/types + rename chat/legacy → chat/impl

**38 files, 2.890 lines.** Neither file knip pointed at was dead: App.tsx imports ingestConductorEvent, toolbox.test.ts exercises the toolbox helpers. Removals had to be symbol-level. The two shapes behind the list were re-export chains that terminate in nobody, and symbols used only in their own file (so `export` is noise). `chat/legacy` held the only copy of the Chat implementation, so the name was a hazard — a linter report in this very audit called it “an unused duplicate of the live Chat”, which is the reverse of the truth.

- `frontend/src/` — 38 files

## 9. The audit report

The findings, the measurements, the two self-inflicted mistakes and the open product decision. Read this first; the other eight slices are its remediation.

- `.project/AUDIT-2026-09-BACKEND-SURFACE.md`
- `.project/REVIEW-MAP.md`

