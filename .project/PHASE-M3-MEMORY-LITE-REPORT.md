# Phase Report — Milestone M3 (Memory-lite)

> Status: **shipped** · Date: 2026-05-30
> The first slice of SPEC-05. Implements the smallest memory that
> makes Agezt *remember* across tasks — content-addressed, journaled,
> reversible — without yet building the world-model graph, the
> skill/Forge lifecycle, or vector retrieval. Follows the ROADMAP
> §2.3 MVP build order ("memory-lite — enough for context, full Forge
> later"), the step between the M2 Operator-CLI consolidation and
> Pulse.

## Scope

Through M2 the agent loop ran every task **context-free**: the system
prompt was static and nothing the agent learned survived the run.
M3 adds a knowledge substrate with four capabilities, all on by
default in the daemon:

1. **A content-addressed store.** Records are addressed by
   `BLAKE3(type \0 subject \0 content)`, so identical knowledge
   dedupes; updates are soft (`superseded_by`) and forgets are soft
   (`tombstoned`) — history is never destructively edited (SPEC-05 §2).
2. **Run-time context injection.** Before each run, relevant records
   are recalled (keyword × confidence × recency) and prepended to the
   system prompt, and the recall is journaled (`memory.retrieved`)
   under the run's correlation — so `agt why` shows exactly what
   knowledge was surfaced.
3. **Three write paths** (the operator-chosen broad scope):
   - operator CLI (`agt memory add/forget`),
   - an in-process **`memory` agent tool** (`remember`/`recall`/
     `forget`) the model can call mid-run,
   - **auto-distillation**: after a multi-tool run, one best-effort
     LLM call extracts durable facts and stores them tagged
     `source=distill`.
4. **Operator visibility.** `agt memory list/get/search` and a
   `memory_records` count on `agt status`.

Every mutation is a journaled, hash-chained event, so the whole store
is reconstructable, auditable (`agt why`), and reversible — the
SPEC-05 §0 competitive thesis (auditability + reversibility over
Hermes's markdown Curator), at memory-lite scale.

## What shipped

### New package `kernel/memory`
- `memory.go` — pure, file-backed `Store` (`FileStore`, single
  `memory.json`, atomic write-temp+rename, RWMutex), mirroring
  `kernel/state`'s shape so a CobaltDB-class engine (DECISIONS D2) can
  replace it behind the interface. Owns content-addressing
  (`ContentID`) and the retrieval ranker (`Search`), both pure
  functions.
- `manager.go` — `Manager{store, bus}`, the journaling boundary:
  `Remember`/`Recall`/`Forget`/`Supersede` wrap each store mutation as
  a durable-before-publish event carrying the run correlation. Also
  hosts the in-process `memory` agent tool and the best-effort
  `Distill` helper. Exports `WithCorrelation`/`CorrelationFrom` so the
  tool can journal under the originating run.

### Event kinds (append-only, DECISIONS B0b)
`memory.written`, `memory.retrieved`, `memory.forgotten`,
`memory.superseded` added to `kernel/event/kinds.go` (const block +
`knownKinds`).

### Runtime wiring (`kernel/runtime`)
- `Open` opens `<BaseDir>/memory` and builds the `Manager`; the
  agent's effective tool map (exposed via `Tools()`) is the configured
  tools plus the `memory` tool when enabled.
- New `Config` knobs, all **default-off in the runtime** so the daemon
  is the single enable point and existing callers/tests are
  unaffected: `MemoryInject`, `MemoryTopK` (5), `MemoryTool`,
  `MemoryDistill`, `MemoryDistillMinTools` (4).
- `RunWith` recalls → injects into the system prompt, stamps the run
  ctx with the correlation, and after the answer runs best-effort
  distillation gated on the tool-call threshold.
- `Memory()` accessor.

### Control plane + CLI
- `CmdMemoryAdd/List/Get/Search/Forget` (`protocol.go` + `server.go`
  dispatch + `memory.go` handlers), mirroring the `state` command pair
  one-to-one.
- `cmd/agt/memory.go` — `agt memory add|list|search|get|forget`, flags
  in any position, extra-positional rejection, `--json` on every read.
- `memory_records` surfaced on `agt status`.

### Daemon
`cmd/agezt` enables memory by default; `AGEZT_MEMORY=off` disables the
per-run behaviour (the store and `agt memory` CLI stay available).

## Design rules followed

- **No new external dependency.** stdlib + the already-justified
  `lukechampine.com/blake3`. `go.mod` unchanged; honors POLICY /
  `DEPENDENCIES.md`.
- **Pure store, journaling at the runtime layer.** The store has no
  bus dependency (like `kernel/state`); the `Manager` is the single
  journaling point (like `Kernel.HaltWith`/`ResumeWith`).
- **Content-addressed dedupe.** Re-remembering identical knowledge
  reinforces (refreshes recency, nudges confidence) rather than
  duplicating.
- **Deterministic output.** `Store.All` and `Search` sort stably; two
  consecutive calls produce identical output (snapshot-test safe).
- **Best-effort distillation.** A distillation error is journaled
  (`memory.distill_failed`) and swallowed — it never turns a
  successful task into a failed one, and the threshold keeps simple
  Q&A runs from paying for an extra round-trip.
- **Exit-code semantics.** `agt memory get` returns 3 for a
  truly-absent id (consistent with `agt state get`). A *tombstoned*
  record still resolves by id (exit 0, `tombstoned:true`) so forgotten
  knowledge remains auditable and recoverable — `list`/`search` are
  the "active only" views.

## Privacy & secrets

The memory store never holds credentials (that's the vault's job).
`agt memory` surfaces only operator/agent-supplied record fields
(subject, content, tags, confidence, provenance event id). Writes are
tagged by origin — `source=operator`, `source=agent`, `source=distill`
— so audit can attribute every belief. PII redaction before external
providers (SPEC-05 §8) is deferred (see below).

## Test coverage

~30 new tests, all green; `go test ./...` clean on `GOOS=windows`
(host) and `GOOS=linux` (cross-compile); `go vet` clean. Package count
36 → 37 (added `kernel/memory`).

- `kernel/memory`: content-address dedup/distinctness, persistence
  across reopen, ranking + tombstone/superseded filtering, limit,
  Remember dedup/reinforce, Forget tombstone, Recall journaling
  (and non-journaling on a miss), Supersede linkage, the `memory` tool
  Invoke shapes, and distillation (JSON extraction + non-JSON no-op).
- `kernel/runtime`: injection-into-system (+ off-by-default),
  memory-tool registration (+ absent-by-default), auto-distill above
  threshold (+ no-distill below).
- `kernel/controlplane`: add/list/get cycle, search ranking, forget
  excludes-from-list-but-still-gettable, and arg-validation errors.
- `cmd/agt`: subcommand dispatch, help, flag/positional validation,
  exit codes, line rendering.

### Manual end-to-end (mock provider, no API key)
`agt memory add` → `list`/`search` (ranked) → `agt run` produces a
`memory.retrieved` event proving injection → `agt memory forget` (gone
from `list`, still `get`-able, `tombstoned:true`) → absent `get`
exits 3 → `AGEZT_MEMORY=off` removes the tool. All verified.

## Deferred to the full Memory & Forge milestone

- **Vector / semantic retrieval** + `memory-flintvector` plugin
  (SPEC-05 §9). Today retrieval is keyword + confidence + recency
  only.
- **World-model graph** (entities/relations/preferences; SPEC-05 §3).
- **Skill system + Forge lifecycle** (draft→shadow→active→quarantine,
  shadow-testing; SPEC-05 §4–5).
- **Reflection loop** (SPEC-05 §6) and **consolidation/prune** (§5.4).
- **PII redaction** before external providers (§8).
- **Distillation richness:** today it folds a compact transcript
  (tools used + final answer); a fuller transcript and dedup against
  existing beliefs wait for demand.

## Closes / next

Closes the "every run starts from zero" gap that every M1/M2 demo
implied. **Next on the confirmed ROADMAP order: Phase 3 — Pulse**
(heartbeat → observers → salience → initiative → briefing), the last
proactive piece before the Telegram channel completes the v0.1.0
"Jarvis" loop.
