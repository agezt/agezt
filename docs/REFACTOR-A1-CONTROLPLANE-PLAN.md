# Refactor A1 — `kernel/controlplane` → domain packages (file-by-file plan)

> Companion to `docs/REFACTORING-SCAN.md` finding **A1**.
> **Generated:** 2026-07-03 · **Branch analyzed at generation time:** `feat/cursor-pagination-agents` (historical snapshot; not the current branch).
> Grounded in a live inventory of `kernel/controlplane/*.go` (not from memory).

## Evidence

The god-package contains a god-file. Live inventory (non-test `.go`, by size):

| File | Size | Handlers | Nature |
|---|---|---|---|
| `roster.go` | **159 KB** | 68 `*Server` methods | ~11 `handleAgentX` route handlers + ~57 `agentXViews` / `agentXImpact` domain folds |
| `protocol.go` | 86 KB | — | wire protocol / envelope types (leave as-is) |
| `server.go` | 81 KB | conn loop + `Set*` wiring | genuine control-plane glue |
| `workboard.go` | 27 KB | 23 handlers | workboard state machine + warden dispatch inline |
| `runs.go` | 34 KB | `collectRuns`, `handleRunsList`, `handleRunsStats` | journal fold inline |
| `memory.go` | 18 KB | 15 handlers | memory list/search/audit fold inline |
| `board.go` | 11 KB | 9 handlers | board reader/writer + reply-thread fold |
| `inbox.go` | 5 KB | 1 handler | mailbox view (dup of board fold) |

**Structural facts shaping the plan:**
1. Handlers are `func (s *Server) handleX(args map[string]any) (...)` — decode args → return envelope.
2. State is reached through **`s.k *runtime.Kernel`**, not injected stores. The only injected domain
   store is `boardStore` (`SetBoard`). Moving "logic to the domain package" = extract a **pure
   function** into the domain package; the handler stays and calls it.
3. `roster.go`'s `agentXImpact` methods compute **cross-domain cascade-delete impact**
   (schedule/memory/skill/workflow/mailbox/standing + all `agentSubagent*Impact`) — the biggest
   offenders: cross-domain reach buried in a handler file.
4. Target domain packages all exist: `journal`, `board`, `memory`, `roster`, `workboard`, `skill`,
   `workflow`, `scheduler`, `standing`.

**Design principle:** controlplane stays transport + dispatch. Move pure, testable domain
functions down; the handler becomes decode → call → encode. Do not move `handleX` methods
themselves. Do not inject new stores unless the moved function needs one; prefer an interface
supplied at wiring time (mirrors existing `Set*` injectors).

---

## Phase 0 — Guardrails (no code moves)

- **0.1** Add `kernel/controlplane/doc.go` stating the transport-only boundary rule.
- **0.2** Baseline gate (record pass counts; every phase keeps it green):
  `go build ./... && go vet ./kernel/controlplane/... && go test ./kernel/controlplane/... ./kernel/journal/... ./kernel/board/... ./kernel/memory/... ./kernel/roster/... ./kernel/workboard/...`
- **0.3** Coordinate via mailbox: the cursor-pagination work recently touched `runs.go`,
  `board.go`, `inbox.go`, `memory.go`, `roster.go`. Rebase on that branch before Phases 3/5.

## Phase 1 — `runs.go` → `kernel/journal` (template, lowest risk)

- **1.1** New `kernel/journal/runs.go`: move `runEntry` + `collectRuns` fold as
  `func CollectRuns(entries []event.Event, opts RunsQuery) (RunsPage, error)` (pure, no `*Server`).
- **1.2** New `kernel/journal/cursor.go`: move the `ms:seq` cursor encode/decode (generic, reused by A2 log endpoints).
- **1.3** `controlplane/runs.go`: `collectRuns` becomes a call to `journal.CollectRuns`; handlers keep arg-parse + encode. 34 KB → ~8 KB.
- **1.4** Move fold tests → `kernel/journal/runs_test.go`; keep `run_argvalidation_test.go` in controlplane.
- **1.5** Gate.

## Phase 2 — `board.go` + `inbox.go` → `kernel/board`

- **2.1** New `kernel/board/view.go`: reply-thread/sort/clamp/cursor-page as pure funcs over `[]board.Message`.
- **2.2** `handleBoardRead/Inbox/Get/Replies` call `board.BuildInbox` / `board.Thread`; keep
  `boardReader`/`boardWriter` (transport-lifecycle concern) in controlplane.
- **2.3** `inbox.go` `handleInbox` calls the same `board.BuildInbox`; kill the duplicated view.
- **2.4** Fold tests → `kernel/board/view_test.go`; keep write/ack tests in controlplane.
- **2.5** Gate.

## Phase 3 — `memory.go` → `kernel/memory` (coordinate: cursor just landed)

- **3.1** Classify the 15 handlers: `List/Search/FindRelated/Audit/Tidy` fold; `Add/Supersede/Forget/Promote/Prune` pass-through.
- **3.2** New `kernel/memory/query.go`: list-cursor-page, search ranking, related-graph, audit summary (pure).
- **3.3** Rewrite the 5 folding handlers to call the new funcs; leave 10 pass-through wrappers.
- **3.4** Fold tests → `kernel/memory/query_test.go`; keep transport/arg tests in `memory_test.go`.
- **3.5** Gate. Rebase on the `/api/memory` cursor work first so cursor logic moves as one unit.

## Phase 4 — `workboard.go` → `kernel/workboard`

- **4.1** New `kernel/workboard/transitions.go`: claim/heartbeat/block/unblock/complete/fail/reclaim/sweep as store methods, same invariants.
- **4.2** New `kernel/workboard/dispatch.go`: move `applyWardenExecutionProfile` + `runWorkboardDispatch`
  behind an injected `ExecutionProfileResolver` interface (avoid a hard `kernel/warden` import cycle).
- **4.3** Handlers shrink to arg-decode + call.
- **4.4** Split tests.
- **4.5** Gate.

## Phase 5 — `roster.go` → `kernel/roster` (159 KB; do LAST, in 5 slices)

| Slice | Moves | Target | Notes |
|---|---|---|---|
| **5a** | 11 `agentXViews` fold methods | `kernel/roster/views.go` | pure read-folds, no writes — lowest risk |
| **5b** | `agentRepairSummaries`, `agentEscalationRows`, activity/repair/escalations fold portions | `kernel/roster/audit.go` | cursor-paginated — coordinate |
| **5c** | ~18 `agentXImpact` cascade-delete methods (+ `agentSubagent*Impact`) | `kernel/roster/impact.go` via a `CascadeResolver` interface | highest value; interface avoids import cycle |
| **5d** | retire/revive/remove/tombstone/graveyard transitions + standing pause/count | `kernel/roster/lifecycle.go` | depends on 5c resolver |
| **5e** | wake/resolve/escalation-handling + hierarchy/delegate validation | `kernel/roster/wake.go` | arch doc says this belongs in roster |

After 5a–5e, controlplane `roster.go` = ~11 thin handlers. 159 KB → ~20 KB.

**Import-cycle guard (5c/5d):** `kernel/roster` must not import scheduler/memory/skill/workflow
directly (they import roster). Define `CascadeResolver` as an interface in `kernel/roster`; supply
the concrete impl at wiring time, like the existing `Set*` injectors in `server.go`.

## Phase 6 — route split (after handlers are thin)

Routes register in **`kernel/webui/webui.go`** (`apiRoutes` / `readArgsRoutes`), not `server.go`.

- **6.1** Split the route map into per-domain registrars in `kernel/webui`:
  `routes_{runs,roster,board,memory,workboard}.go`, each `registerXRoutes(mux, s)`.
- **6.2** `server.go` keeps listener/accept/shutdown/token/`Set*`. Leave `protocol.go` untouched.

---

## Sequencing

```
1 (runs→journal)      ← template, no coordination
2 (board+inbox→board) ← store already injected
4 (workboard)         ← independent
3 (memory)            ← coordinate: cursor just landed
5a → 5b → 5e → 5c → 5d ← roster; 5c/5d last (need resolver interface)
6 (route split)       ← after handlers are thin
```

## Per-PR gate

`go build ./... && go vet ./kernel/... && go test ./kernel/controlplane/... ./kernel/<domain>/...`
then `go run ./tools/deadcodecheck` to catch orphaned helpers left in controlplane.

## Impact

~200 KB of domain logic relocated to testable-in-isolation homes; controlplane reduced to
transport + dispatch. Handler-file sizes: roster 159→~20 KB, runs 34→~8 KB, workboard 27→~10 KB,
memory 18→~8 KB, board+inbox 16→~8 KB.
