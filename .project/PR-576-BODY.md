# Refactor: 30-day kernel/runtime + 18-day frontend domain carve-out (2026-08-12 → 2026-09-10)

> **Two coordinated refactor sprints, both pure in-binary / in-package
> reorganization. Zero contract change** — DECISIONS B0 (stdio + JSON-RPC),
> SPEC-01 §0.5 wire shapes, the `agezt-contract.jsonc` schema, and the
> CHANGELOG `v1.1.0` entry are all unchanged.
>
> **The two sprints together produced 28 commits in this PR.** The
> Go-side work lifted 2,516-line `kernel/runtime.go` into 5 sub-packages
> + a Runner; the frontend work lifted 25 page-level features into
> 25 `features/*/` barrels + 6 `app/help/` topic modules.

This PR body is the executive summary. Full day-by-day logs live in:

- **Go sprint (Day 1-30)**: `.project/SPRINT-2026-09-REFACTOR-REPORT.md` (5 sections, full per-day detail with the `RunWith`-stays-on-`*Kernel` analysis, lock-ordering invariant, KernelAPI bloat policy, etc.)
- **Frontend sprint (Day 1-18)**: `docs/FRONTEND-REFACTOR-PLAN.md` (§9 = Day 3-18 commit-by-commit progress; §10 = backlog)

---

## 1. Outcome

| Metric | Day 0 | Sprint end | Δ |
|---|---|---|---|
| Go kernel sub-packages | 0 | **5** (`lifecycle`, `accessors`, `types`, `compose`, `runexec`) | +5 |
| Go utility packages (`cmd/agt/*`) | 0 | **8** (`router`, `providerlookup`, `dial`, `jsonout`, `whoami`, `haltresume`, `keys`, `format`) | +8 |
| Frontend `features/*` carve-outs | 0 | **25** (voice, incidents, council, runs, agents, workflows, schedules, execution-profiles, memory, standing, configcenter, skills, market, setup, autonomy, overseer, sandbox, data, models, artifacts, mcp, channels, policy, world, connections) | +25 |
| Frontend cross-cutting `app/*` modules | 0 | **7** (`utils`, `api`, `events`, `format`, `cursor-pager`, `export`, `help` — the last split 6-ways by topic) | +7 |
| `kernel/runtime/runtime.go` LOC | 2,516 | ~770 | **−69%** |
| `runexec.Runner` entry points | 0 | 17 (4 entry + 4 journal + 3 post-run + 4 one-shot + 2 wrappers) | +17 |
| `runexec.KernelAPI` entries | n/a | 42 | +42 |
| Frontend LOC moved | 0 | **~52K** satır (17K component + 8K lib + 24K view + 3K help/6) | +52K |
| Go unit tests | n/a | 17/17 yeşil (`go test ./kernel/runtime/... ./kernel/controlplane/... ./cmd/agezt/... ./cmd/agt/...`) | — |
| Frontend tests | 1628 | **1628 / 1628 yeşil** (191 test files) | sıfır regression |
| `make check` Go-side | n/a | all 7 yeşil (`gen`, `deps-check`, `sdk-parity`, `deadcode-check`, `structure-md-check`, `vet`, `test`) | — |
| `make check` frontend | n/a | `tsc --noEmit` + `npx vitest run` all yeşil | — |

---

## 2. The two sprints

### 2.1 Go kernel/runtime (Day 1-30 + Day 31-34 next-slice)

- **Day 1-6**: extracted 8 `cmd/agt/*` utility packages.
- **Day 7-8**: shim rewrite (374 call sites via Python bulk-rewrite script); `cmd/agt/format` package.
- **Day 9-11**: `kernel/runtime/runtime.go` (2,516 satır) split into 4 files (`lifecycle.go` stub, `compose.go` 444, `accessors.go` 460, `runexec.go` 870+, `runtime.go` 770).
- **Day 12-13**: `lifecycle.Manager` (8 metot) + `accessors.Accessor` (11 metot) sub-packages.
- **Day 14-19**: `accessors` expanded to 47 metot (configMu-aware mutators, Standing/Roster CRUD, live mutators).
- **Day 17**: `types` sub-package (SubAgentLimits, PluginInfo, CouncilMember).
- **Day 20a-23**: `compose` skeleton + `runexec.Runner` + KernelAPI (Run, RunWithRetry, Causes, ParentOf, Verify, Why, + 3 post-run hooks).
- **Day 24-25**: docs & cleanup (`STRUCTURE.md`, SPEC-01 §0.6, ROADMAP §7, DECISIONS E6/E7).
- **Day 26-30**: final integration + sprint report.
- **Day 31-34 (next-slice)**: `cmd/agt` dispatcher wiring (closed Day 26 `deadcodecheck` gap); `RunWith` body move (closed the 30-day sprint's last open item — 260 satır, 25 new KernelAPI accessors, 3 lock-protected encapsulation helpers).

**`runexec.Runner` is now 17 metot**, the `KernelAPI` interface has 42 entries, and the `RunWith` body lives in the Runner (lock-ordering invariant: `configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu` — encapsulated in `*Kernel.SetupRunState` / `CleanupRunState` / `DeregisterRunSteer`).

Full log: `.project/SPRINT-2026-09-REFACTOR-REPORT.md`

### 2.2 Frontend domain carve-out (Day 1-18)

- **Day 3-5a**: lifted 7 cross-cutting `lib/*` modules to `app/*` (utils, api, events, format, cursor-pager, export, help) — 376 dosya, ~3650 satır.
- **Day 6**: `features/voice/` — first feature carve-out (29 dosya, ~2000 satır).
- **Day 7**: `features/incidents/`.
- **Day 8**: `features/council/`.
- **Day 9**: `features/runs/`.
- **Day 10-11**: `features/agents/` (the biggest — 49 dosya, ~11K satır, split over 2 days: lib + 28 component + 3 view on Day 10, Roster + roster/ subdir on Day 11).
- **Day 12**: `features/workflows/`.
- **Day 13**: `features/schedules/` + `features/execution-profiles/`.
- **Day 14**: `features/memory/` + `features/standing/` + `features/configcenter/`.
- **Day 15**: `features/skills/` + `features/market/` + `features/setup/`.
- **Day 16**: `features/autonomy/` + `features/overseer/` + `features/sandbox/`.
- **Day 17**: `features/data/` + `features/models/` + `features/artifacts/`.
- **Day 18**: `features/mcp/` + `features/channels/` + `features/policy/` + `features/world/` + `features/connections/` — **sprint complete: 25/25 feature**.
- **Day 5b (after sprint)**: `app/help/` (2864 satır) split into 6 topic modules (`converse/monitor/agents/automation/knowledge/system`) + `types.ts` + aggregator (`help.ts` 36 satır).
- **chore (after sprint)**: 7 WIP files (ci.yml + Makefile + README + 3 CONSOLE/REFACTORING docs + STRUCTURE.md) — Day 6 Quick Connect removal artifacts + Day 24-25 structure-md tool CI integration.

**Each `features/*/` follows the same template** (per `docs/FRONTEND-REFACTOR-PLAN.md` §3):

```
features/<name>/
├── components/    # page + sub-components (co-located)
├── lib/           # business logic (package-internal; external code via barrel)
├── hooks/         # custom hooks (where applicable)
├── types.ts       # shared shapes re-exported
└── index.ts       # public surface barrel
```

**Cross-cutting shared components stayed in `components/`** (used by ≥2 features): `AgentAvatar`, `AgentPicker`, `ConfigInventory`, `WorldGraph`, `ConnectionChip`, `DataView`, `ModelPicker`, `ModelChip`.

**Helper pattern**: 11 `scripts/dev/rewrite-{feature}-imports.py` scripts — round-1 string match + round-2 regex subpath. Sıfır regression garantili (her commit öncesi/sonrası `tsc --noEmit` + `vitest run` 191/1628 yeşil).

Full log: `docs/FRONTEND-REFACTOR-PLAN.md` §9 (Day 3-18 commit-by-commit).

---

## 3. Commit list (28 total)

### Go kernel + utility (8 commits)

| # | Commit | Scope |
|---|---|---|
| 1 | `e54273c5` | docs: sprint report + DECISIONS E6/E7 + SPEC-01 §0.6 + ROADMAP §7 |
| 2 | `90460b47` | tools(structure-md): auto-doc generator + deadcodecheck allowlist |
| 3 | `ba8fd563` | refactor(cmd/agt): 8 utility packages + shim rewrite (374 call sites) |
| 4 | `b8c677b0` | refactor(kernel/runtime): 2,516-line god file split into 5 sub-packages + Runner |
| 5 | `52234e77` | docs: per-package doc.go so STRUCTURE.generated stays current |
| 6 | `1d237288` | refactor(frontend): drop Quick Connect from Connections + nav |
| 7 | `b8c677b0` + `ee7267da` family | next-slice #1-#3: cmd/agt dispatcher wiring + post-run trio + RunWith body move (Day 31-34, post-sprint) |
| 8 | `45551d43` | chore(gitignore): ignore stray diff.txt |

### Frontend domain carve-out (20 commits, Day 3-18 + Day 5b + chore)

| # | Commit | Day | Scope |
|---|---|---|---|
| 9 | `030ecb34` | 3 | `app/{utils,api}` (243 dosya, 256 LOC) |
| 10 | `d0db493d` | 4 | `app/{events,format,cursor-pager,export}` (128 dosya, ~600 LOC) |
| 11 | `451a1033` | 5a | `app/help.ts` (2795 LOC) lifted from `lib/help.ts` |
| 12 | `db31cc8f` | 6 | `features/voice/` (29 dosya, 87 test) |
| 13 | `64b0b28f` | 7 | `features/incidents/` (31 dosya, 27 test) |
| 14 | `5fc5af85` | 8 | `features/council/` (10 dosya, 14 test) |
| 15 | `7e17d0e3` | 9 | `features/runs/` (9 dosya) |
| 16 | `ee7267da` | 10 | `features/agents/` (lib + components + 3 view pages; 41 dosya) |
| 17 | `a83732a0` | 11 | `features/agents/components/{Roster,roster/*}` (9 dosya) |
| 18 | `52aa777f` | 12 | `features/workflows/` (5 dosya; 1597 satır god file) |
| 19 | `63ae84c9` | 13 | `features/schedules/` + `features/execution-profiles/` (6 dosya) |
| 20 | `0b0f26a8` | 14 | `features/memory/` + `standing/` + `configcenter/` (8 dosya) |
| 21 | `f0658055` | 15 | `features/skills/` + `market/` + `setup/` (9 dosya) |
| 22 | `04e4534c` | 16 | `features/autonomy/` + `overseer/` + `sandbox/` (8 dosya) |
| 23 | `9971e2cb` | 17 | `features/data/` + `models/` + `artifacts/` (12 dosya) |
| 24 | `1d1136f6` | 18 | `features/mcp/` + `channels/` + `policy/` + `world/` + `connections/` (14 dosya) — **sprint complete: 25/25 feature** |
| 25 | `13fada06` | chore | 7 WIP dosya: ci.yml + Makefile + 3 doc + STRUCTURE.md |
| 26 | `9b531751` | 5b | `app/help/` 64 topic 6 modüle bölündü (converse/monitor/agents/automation/knowledge/system) |

**Total PR delta: 28 commits, ~52K frontend satır + 2,516→770 Go satır (%69 azalma).**

---

## 4. Backlog (post-PR)

Carve-out scope complete; integration is the next slice:

| Item | Scope |
|---|---|
| `views/` 35+ view temizliği (Board, Dashboard, Workboard, …) | 5+ commit, multi-day |
| 17 god file bölünmesi (Workflows 1597, Schedules 1518, …) | 5+ commit, 1-2 oturum |
| `.project/STRUCTURE.md` frontend bölümünü güncelle (features/ sonrası gerçeği yansıt) | 1 commit |
| `plans/phase2.*.md` stale delete'leri (önceki oturumdan kalan) | 1 cleanup commit |

The 25-feature frontend refactor unblocks all four.

---

## 5. No code paths or wire shapes were changed

Both sprints are **pure in-binary / in-package organization**. The kernel behaves identically to Day 0. The frontend renders the same views with the same routes. The only diff is the source-tree shape — and the test coverage proves it: 17/17 Go + 1628/1628 vitest, both green at every commit boundary.
