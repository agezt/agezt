# AGEZT — Deep Architecture Analysis

> **Scope:** This document is a measurement-based architectural map of the
> entire AGEZT repository. `docs/ARCHITECTURE.md` describes the product
> direction and the target architecture; this document describes **the actual
> state of the code today**.
>
> **Method:** All numbers are derived from the `go list` import graph, file/line
> counts, and `go build` / `go vet` / `go test` runs; none of them are
> estimates. Measurement date: 2026-08-04, branch `main`.

---

## 1. Executive Summary

| Dimension | Value | Assessment |
|---|---|---|
| Go package count | 173 | High but well organized |
| Go production code | 191,896 lines | Large monorepo |
| Go test code | 155,172 lines | test/code ratio **0.81** — very good |
| Frontend source | 77,149 lines (251 files) | Large SPA |
| Frontend tests | 29,680 lines (186 files) | ratio 0.38 — moderate |
| Cyclic dependencies | **0** | Excellent |
| Layer violations | **1** | Almost clean |
| `go build ./...` | ✅ passing | Verified |
| `go vet ./...` | ✅ clean | Verified |
| `go test ./... -short` | ❌ **1 package broken** | `tools/depscheck` |
| Package doc coverage | 159/173 (92%) | Very good |

**Overall report card:** Architectural discipline is above expectations.
Dependency direction is consistent, layers separate topologically, and the test
investment is serious. The main risks are **code volume concentrated in a few
huge packages** and a dependency test left broken on `main`.

---

## 2. Verified Health Status

Every line in this section was actually executed in this session.

| Check | Command | Result |
|---|---|---|
| Build | `go build ./...` | exit 0, no errors |
| Static analysis | `go vet ./...` | exit 0, no warnings |
| Test suite | `go test ./... -count=1 -short` | 1 FAIL / 170+ packages |
| Cycle scan | Tarjan SCC, import graph | 0 cycles |

### 2.1 Broken test — `tools/depscheck` (THE ONLY REAL BREAKAGE)

```
--- FAIL: TestGoModOnlyListsCompiledDeps
    golang.org/x/sys is in go.mod's require block
    — should be MVS-only transitives (UPD-002)
```

**Root cause (verified):** `go.mod:20` contains the line `golang.org/x/sys
v0.47.0 // indirect`. Meanwhile `tools/depscheck/main_test.go:64` keeps
`golang.org/x/sys` on a list of modules that "must never appear in the
`require` block".

The test's parser (`main_test.go:75-86`) detects the `require` block by tab
indentation and **does not distinguish the `// indirect` block from the direct
block**. So either:

- `golang.org/x/net` (v0.57.0, a direct dep, used for the browser tool's PSL)
  requires `golang.org/x/sys` somewhere, forcing `go mod tidy` to write it as
  indirect — in which case **the test rule is wrong**,
- Or a genuinely unnecessary dependency has leaked in — in which case
  **go.mod is wrong**.

Since `golang.org/x/net` is a direct dependency and `x/net` internally uses
`x/sys`, **the first possibility is far more likely**. The correct fix is
probably to make the test exempt indirect lines. This requires a decision and
should not be changed blindly.

**Impact:** The `go test ./...` step is red in CI. That is a source of noise
masking every other regression — the highest-priority fix.

---

## 3. Layer Architecture

Topological levels were computed from the `go list` import graph. A package's
level = the length of its longest dependency chain.

```
L9  webui                                        ← HTTP/SPA presentation
L8  controlplane                                 ← command protocol
L6  runtime                                      ← composition root
L5  agentgw, alerter, contextselect, market, reflect
L4  configcenter, delegation, executionprofile, governor, memory,
    planner, plugin, pulse, resume, scheduler, skill, toolexec, worldmodel
L3  agent, anomaly, approval, cadence, channel, openaiapi, restapi,
    standing, warden, webhook, workflow
L2  bus, chatgptauth, httpserver, workboard
L1  artifact, auth, board, catalog, creds, journal, mcp, okr, proof,
    roster, settings, state, taste, toolforge, update, acp, paths
L0  event, ulid, netguard, edict, intent, redact, convo, assure,
    tenant, tenantctx, seat, meshctx, streamlimit, stt, tunnel,
    envscrub, apperrors, atomicfile, brand, strutil
```

### 3.1 Dependency backbone

The 6 most depended-upon packages are the system's real core:

| Package | Fan-in | Fan-out | Role |
|---|---|---|---|
| `kernel/agent` | **57** | 3 | Canonical message/tool types + tool loop |
| `kernel/event` | **56** | 0 | Event type + BLAKE3 hash chain |
| `kernel/bus` | **50** | 2 | Event bus (durable-before-publish) |
| `kernel/ulid` | **45** | 0 | Sortable identifier generation |
| `kernel/channel` | **29** | 3 | Channel normalization types |
| `kernel/netguard` | **16** | 0 | Network egress protection |

This distribution is a **healthy core signature**: the most depended-upon
packages are simultaneously the least dependent ones (fan-out 0-3). That means
the core is genuinely leaf-positioned and stable.

### 3.2 Layer violations

The scan found **a single violation**:

```
kernel/controlplane  →  plugins/tools/overseertool
```

A `kernel/*` package depends on a `plugins/*` package. This inverts the
direction in which the plugin layer should depend on the kernel. Since it is
the only instance, the fix is cheap: the types overseertool needs from the
kernel should either move under `kernel/` or be injected through an interface.

### 3.3 Cyclic dependencies

**Zero.** A Tarjan SCC scan across 173 packages found no multi-node component.
For a Go monorepo of this size, that is a notable indicator of discipline.

---

## 4. Component Map

### 4.1 Entry points (`cmd/`)

| Package | Code | Tests | Fan-out | Role |
|---|---|---|---|---|
| `cmd/agezt` | 10,652 | 3,898 | **110** | Daemon; composition root |
| `cmd/agt` | 35,722 | 11,661 | 25 | CLI; all user commands |

`cmd/agezt` imports 110 packages — an expected number for a composition root;
it has to know every subsystem by design.

### 4.2 Core (`kernel/`) — 75 packages, 330 files

Functional groups:

**Event and persistence backbone**
`event` (hash chain) · `bus` (durable-before-publish) · `journal`
(append-only) · `state` (file store) · `artifact` (BLAKE3 content-addressed
blobs) · `datalake` · `ulid`

**Agent execution**
`agent` (canonical types + tool loop) · `runtime` (composition) ·
`toolexec` · `delegation` · `contextselect` · `resume` · `planner` ·
`workflow` · `workflowexec` · `scheduler` · `cadence`

**Security and policy**
`edict` (policy engine + trust ladder L0-L4) · `warden` (process isolation
profiles) · `netguard` (SSRF/internal network protection) · `approval`
(HITL) · `governor` (budget) · `auth` · `creds` · `redact` · `envscrub`

**Control and presentation**
`controlplane` · `webui` · `restapi` · `openaiapi` · `httpserver` ·
`agentgw` · `channel`

**Supporting subsystems**
`pulse` (proactive notification) · `alerter` · `anomaly` · `intervention` ·
`update` · `market` · `plugin` · `mcp` · `toolforge` · `roster` ·
`board` / `workboard` · `catalog` · `configcenter` · `settings` ·
`executionprofile` · `seat` · `tenant` / `tenantctx` · `webhook` ·
`tunnel` · `stt` · `proof` · `assure` · `intent` · `meshctx` ·
`streamlimit` · `chatgptauth`

### 4.3 Plugins (`plugins/`) — 81 packages, 127 files

- `plugins/providers/` — LLM provider adapters (bedrock, vertex, voice, …).
  Translation between canonical `agent.Message` and provider dialects.
- `plugins/tools/` — 30+ tools (browser, codeexec, file, shell, http, db,
  fetch, websearch, …)
- `plugins/builtinskills/` — bundled skill definitions
- `plugins/external/mcpbridge` — MCP bridge

### 4.4 Frontend

A React SPA of 251 source files / 77,149 lines. The largest files are at the
view level: `Schedules.tsx` (2,125), `Workflows.tsx` (1,685), `Roster.tsx`
(1,460). `lib/help.ts` is the single largest file at 2,848 lines.

---

## 5. Risk Analysis — God Packages

This is the system's most concrete technical debt.

### 5.1 `kernel/controlplane` — 30,125 lines / 87 files

| File | Lines | Functions | Types |
|---|---|---|---|
| `roster.go` | **4,925** | 133 | 15 |
| `server.go` | 2,294 | 41 | 7 |
| `protocol.go` | 1,646 | 0 | 2 |
| `schedule.go` | 1,103 | 18 | 2 |
| `runs.go` | 906 | 16 | 2 |

**Findings:**
- **319 command constants** (`protocol.go`), dispatched through a single
  `switch req.Cmd` statement at `server.go:543`.
- The `handleConn` function is **705 lines** — authentication, parsing,
  dispatch, and error handling all in one function.
- `roster.go` alone hosts 133 functions; that is not a file, it is a hidden
  subsystem.
- 46 distinct packages are imported.

**Risk:** Every new feature has to touch this package → merge conflicts,
regression surface, and cognitive load keep increasing.

**Positive note:** There are 22,737 lines of tests (ratio 0.75) and
`handleConn` performs per-connection panic isolation (`server.go:483`) — so
the existing complexity is not untested, merely concentrated.

### 5.2 `kernel/runtime` — 12,535 lines / 27 files

- `runtime.go` alone is **4,402 lines**
- The `Kernel` struct has **57 fields** (lines 526-610)
- The `Config` struct has **72 fields** (lines 98-523) — a 425-line definition
- **219 methods** on `*Kernel`
- Depends on 40 different kernel packages

The package documentation already acknowledges this state:

> *"runtime is the composition root and thin adapter layer… it may
> temporarily host orchestration helpers while boundaries are being
> extracted, but long-term feature-specific logic should live in
> narrower domain packages"*

So this is **deliberate, temporarily accepted** debt. The danger is
"temporary" becoming permanent.

### 5.3 `cmd/agezt/main.go` — 7,456 lines

- The `runDaemon` function is **2,038 lines** — the longest function in the
  repository
- `buildGovernor` is 257 lines, `buildCadence` is 180 lines

Its test ratio of 0.37 is markedly below the kernel average.

### 5.4 Longest functions

| Lines | Location |
|---|---|
| 2,038 | `cmd/agezt/main.go:197` `runDaemon` |
| 705 | `kernel/controlplane/server.go:481` `handleConn` |
| 371 | `cmd/agt/main.go:128` `cmdRun` |
| 342 | `kernel/controlplane/autonomy.go:342` `autonomyDoctorDetail` |
| 332 | `kernel/controlplane/schedule.go:391` `handleScheduleEdit` |
| 326 | `cmd/agt/schedule.go:126` `cmdScheduleAdd` |

---

## 6. Test Strategy

### 6.1 Overall

The total test/code ratio is **0.81** — top quartile in the Go ecosystem.

**Packages with a high ratio (well protected):**
`openaiapi` 1.62 · `governor` 1.44 · `agentgw` 1.37 · `restapi` 1.24 ·
`plugin` 1.24 · `agent` 1.14

**Packages with a low ratio (risk):**
`cmd/agt` 0.33 · `workflow` 0.36 · `cmd/agezt` 0.37 · `market` 0.43 ·
`acpcatalog` 0.43 · `settings` 0.42 · `executionprofile` 0.47 ·
`configcenter` 0.49

At 35,722 lines, `cmd/agt` is both the repository's largest package and the one
with the lowest test ratio — the highest absolute risk is here.

### 6.2 CI gates

`.github/workflows/ci.yml` (555 lines) enforces:

- `go vet` + `go test` + `go build` (self-hosted WSL runner)
- **100% line coverage ratchet** for 9 packages: `plugins/providers/voice`,
  `kernel/stt`, `kernel/board`, `tools/jsonschemagen`,
  `plugins/tools/runstool`, `plugins/tools/workboardtool`,
  `plugins/providers/internal/provopts`, `plugins/tools/standingtool`,
  `kernel/event`, `kernel/tunnel`
- Race detector: whole tree (breadth) + a depth pass under stress
  (`RACE_STRESS_COUNT=20`) — `channel`, `controlplane`, `chatgptauth`,
  `runtime`, `pulse`, `governor`, `bus`, `journal`
- E2E smoke (real daemon), codegen-in-sync, frontend dist-in-sync,
  Vitest + Playwright

**Gap:** The macOS and Windows test legs are disabled.

---

## 7. Quality Signals

| Signal | Value | Comment |
|---|---|---|
| TODO/FIXME/HACK | 23 | Very low; mostly `context.TODO()` |
| Missing package doc | 14/173 | Mostly tool/example packages |
| `go vet` warnings | 0 | Clean |
| Cyclic dependencies | 0 | Clean |

**`context.TODO()` usage:** 13 calls inside `cmd/agt/overseer.go`. These are a
real gap that breaks cancellability — CLI commands cannot be stopped cleanly
with Ctrl+C.

**Meaningful packages missing a package doc:** `kernel/configcenter` (fan-in 4,
7 files) and `cmd/agezt` (8 files). The rest are single-file tool packages.

---

## 8. Priority Improvement Areas

Ordered by impact/cost:

### P0 — The broken test on `main`
`tools/depscheck` FAILs. As long as CI is red, real regressions are invisible.
A decision is required: is the test rule wrong, or is go.mod?

### P1 — `cmd/agt` test gap
35,722 lines, ratio 0.33. The repository's largest and least protected
package. Command-by-command test additions can be done incrementally.

### P1 — Decomposing `kernel/controlplane`
Moving 319 commands from a single `switch` into a handler registry table is a
low-risk first step. `roster.go` (4,925 lines) can be extracted into its own
package.

### P2 — Splitting `runDaemon`
A 2,038-line function; the biggest obstacle to raising the `cmd/agezt` test
ratio. Breaking it into sub-builder functions directly improves testability.

### P2 — Boundary extraction in `kernel/runtime`
The `Kernel` (57 fields) and `Config` (72 fields) structs. The direction the
package doc already points to: move field-specific logic into narrow domain
packages.

### P2 — Layer violation
`kernel/controlplane → plugins/tools/overseertool`. A single instance, a cheap
fix, and a permanent rule gained.

### P3 — `context.TODO()` cleanup
13 calls in `cmd/agt/overseer.go` → real context propagation.

### P3 — Windows/macOS CI legs
Should be re-enabled once billing is resolved; Windows is the primary
development platform.

---

## 9. Measurement Method

For reproducibility:

```bash
# Packages + import graph
go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./...

# Health gates
go build ./...
go vet ./...
go test ./... -count=1 -short -timeout=25m
```

Cycle detection used Tarjan SCC; layer levels were derived via longest-path
computation over the import graph. Line counts are raw `\n` counts (comments
and blank lines included).

---

## 10. Conclusion

For its size, AGEZT is a **far more disciplined** codebase than expected: zero
cycles, a single layer violation, 92% documentation coverage, a 0.81 test
ratio, a clean `vet`, and serious CI gates (race, coverage ratchet, E2E,
codegen-sync).

The technical debt is not scattered but **concentrated**: three files
(`main.go`, `roster.go`, `runtime.go`) and two packages (`controlplane`,
`cmd/agt`) carry most of the risk. That is good news — targetable debt rather
than diffuse debt.

The only thing requiring urgent intervention is the broken `depscheck` test on
`main`.
