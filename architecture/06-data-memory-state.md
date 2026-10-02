# 06 — Data, Memory & State

**Scope:** `kernel/event`, `kernel/journal`, `kernel/bus`, `kernel/ulid`, `kernel/platform/filestore`, `kernel/memory`, `kernel/worldmodel`, `kernel/datalake`, `kernel/artifact`, `kernel/board`, `kernel/workboard`, `kernel/okr`, `kernel/taste`, `kernel/anomaly`, `kernel/alerter`.

Sibling docs: [00-README.md](00-README.md) (overview) · [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md) (where these stores are opened/wired) · [02-cli-cmd-agt.md](02-cli-cmd-agt.md) (`agt journal|why|memory|world|board|workboard|okr|taste`) · [03-control-plane-and-http.md](03-control-plane-and-http.md) (handlers, SSE, webhooks, REST mailbox) · [04-agent-runtime.md](04-agent-runtime.md) (agent loop, context injection, proof/assure) · [05-governance-routing-security.md](05-governance-routing-security.md) (redact, state, edict capabilities) · [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md) (pulse, standing orders, workflow, selfrepair, cadence systemtasks) · [10-tools.md](10-tools.md) (`boardtool`, `workboardtool`, `artifacts`, `db` tools) · [11-frontend-console.md](11-frontend-console.md).

---

## Responsibilities at a glance

| Layer | Package | One-line role |
|---|---|---|
| Identity | `ulid` | Kernel-owned 26-char Crockford ULIDs (48-bit ms + 80-bit crypto random). |
| Event spec | `event` | The one `Event` struct, its deterministic JSON + BLAKE3 hash chain, and the append-only `Kind` catalog (164 constants). |
| Durable log | `journal` | Append-only, fsync'd, hash-chained JSONL segments under `$AGEZT_HOME/journal/`; recovery, verify, restore, tail, provenance (`Why`/`Causes`/`ParentOf`), cursor pagination helpers. |
| Fan-out | `bus` | In-process NATS-style pub/sub; **durable-before-publish** (journal append + fsync before any subscriber sees the event); secret redaction; ephemeral streaming path. |
| Store plumbing | `filestore` | Tolerant `Load` + atomic `Save` shared by every single-file JSON store. |
| Knowledge | `memory` | Content-addressed, journaled fact store with private-by-default scopes, hybrid keyword+embedding recall, distillation, consolidation, operator profile, retention/hygiene. |
| Knowledge graph | `worldmodel` | Content-addressed, journaled entity/relation graph with alias resolve, neighbors, decay. |
| Structured data | `datalake` | "Personal Data Lake": schema'd collections, one JSON file per record, 7 built-in system collections. Not journaled. |
| Blobs | `artifact` | BLAKE3 content-addressed blob store + per-arrival metadata index; offload target for oversized tool outputs. |
| Messaging | `board` | Shared persistent message board / agent mailbox (topics, DMs, broadcast, help, ack). Single shared instance invariant. |
| Work | `workboard` | Restart-safe typed task state machine (claims, heartbeats, retries, deps, acceptance criteria + proof gate). |
| Goals | `okr` | Objectives → Key Results → linked workboard tasks; pure rollup. |
| Quality | `taste` | Curated exemplars injected into run system prompts. |
| Safety | `anomaly` | Global tool-call-rate circuit breaker → `system.anomaly` + kernel halt. |
| Notify | `alerter` | Classifies warning/critical bus events into Pulse briefs delivered to channels. |

Key architectural facts:

1. **Two persistence tiers.** The *journal* is the immutable audit/replay truth. Every other store in this area is a **mutable projection/cache** written by its own package (memory.json, worldmodel.json, workboard.json …). The memory and worldmodel *managers* journal every mutation (event first, then store write); workboard/okr/taste/board events are published by the **runtime/daemon wrappers**, not by the store packages; datalake and artifact are **not journaled at all**.
2. **Single serialization point.** `bus.Publish` holds `Bus.mu` across `journal.Append` (which itself holds `Journal.mu` and fsyncs). All durable publishes in the daemon are therefore globally serialized, one fsync each.
3. **Stores are "load whole file, save whole file".** Every filestore-based store keeps the full dataset in memory under one mutex and rewrites the entire file atomically on every mutation. Correct and crash-safe, O(n) per write; and **two instances of the same store in one process silently clobber each other** (hence the board invariant).

---

## 1. Import graph (in-scope)

From `internal-deps.txt`:

| Package | Depends on (internal) | Depended on by |
|---|---|---|
| `event` | — (leaf; only `lukechampine.com/blake3`) | ~60 packages: virtually all of `kernel/*`, every `plugins/channels/*`, `cmd/agezt`, `cmd/agt`, `sdk` |
| `ulid` | — (leaf, stdlib only) | journal, artifact, board, datalake, okr, taste, workboard, approval, cadence, controlplane, mcp, openaiapi, pulse, roster, runtime(+lifecycle), scheduler, standing, toolforge, workflow, agentgw, every channel plugin, cmd/agezt |
| `filestore` | `internal/atomicfile` | board, cadence, mcp, memory, okr, roster, skill, standing, taste, toolforge, workboard, workflow, worldmodel |
| `journal` | event, ulid | bus, agentgw, controlplane, reflect, runtime(+accessors, runexec), toolreg, cmd/agt |
| `bus` | event, journal | ~45 packages (all event producers/consumers, every channel plugin, codeexec) |
| `memory` | **agent**, bus, **edict**, event, filestore | cmd/agezt, agentgw, contextselect, controlplane, runtime(+accessors, runexec) |
| `worldmodel` | **agent**, bus, **edict**, event, filestore | contextselect, controlplane, reflect, runtime(+accessors, runexec) |
| `datalake` | atomicfile, ulid | controlplane, runtime(+accessors, runexec), toolreg, plugins/tools/db |
| `artifact` | atomicfile, ulid | cmd/agezt, controlplane, runtime(+accessors, runexec), toolreg, plugins/tools/{artifacts,browser,codeexec,fetch} |
| `board` | filestore, ulid | cmd/agezt, controlplane, restapi, selfrepair, toolreg, plugins/tools/{boardtool,overseertool} |
| `workboard` | filestore, **proof**, ulid | controlplane, runtime, plugins/tools/workboardtool |
| `okr` | filestore, ulid | controlplane, runtime |
| `taste` | filestore, ulid | controlplane, runtime |
| `anomaly` | bus, event | cmd/agezt only |
| `alerter` | bus, event, **pulse** | cmd/agezt only |

Layering notes:
- `memory` and `worldmodel` import `kernel/agent` (for `agent.Tool`, `agent.Provider`, `agent.GenerateObject`, `agent.AgentFromContext`) and `kernel/edict` (capability names `CapMemory`/`CapWorld`). The "pure store" half of each package is clean; the tool + LLM-distillation half drags the agent loop's types into the data layer.
- `workboard` → `proof` → `assure`: the task store embeds the verifier's `assure.Verdict` type in its on-disk shape.
- `alerter` → `pulse` only for `pulse.BriefSink`, `pulse.Brief`, `pulse.DispAlert`, `pulse.QuietHours`.

---

## 2. `kernel/ulid`

**Purpose.** The kernel's ID source (DECISIONS B2: "Kernel assigns IDs; plugins never do").

| File | What it does |
|---|---|
| `ulid.go` | `Generator{mu, nowFunc, randSrc}`, package `Default` generator (time.Now + crypto/rand), `New()` / `(*Generator).New()`, and the unrolled 16-byte → 26-char Crockford base32 `encode`. Panics if crypto/rand fails ("the supervisor sees it"). |

- Constants: `Size = 16`, `EncodedSize = 26`, alphabet `0123456789ABCDEFGHJKMNPQRSTVWXYZ`.
- Concurrency: generator mutex; safe for concurrent use.
- **Gotcha:** not monotonic within a millisecond — the 80 random bits are fresh each call, so two IDs minted in the same ms sort randomly. Anything needing a total order uses the journal `Seq` (see cursor tie-break in §4.6).
- ID prefixes layered on top by callers: datalake `rec-<ulid>`, artifact index `art-<ulid>`; board/workboard/okr/taste/journal use the bare ULID.
- Tests: `ulid_test.go`.

---

## 3. `kernel/event`

**Purpose.** The canonical event shape shared by the journal (durable) and the bus (fan-out), the hash-chain contract, and the append-only kind enum.

| File | What it does |
|---|---|
| `doc.go` | Package contract: "everything is an event", immutability (corrections are inverse events), hash = `BLAKE3-256(prev_hash_bytes ‖ canonical_json_bytes)`, JSON (not protobuf) wire format per DECISIONS B0. |
| `event.go` | `Spec`, `Event`, `New`, `NewEphemeral`, `IsEphemeral`, `Canonical`, `computeHash`, `VerifyHash`, `Decode`, `marshalPayload`; constants `HashSize=32`, `HashHexLen=64`, `GenesisHash` (64 zeros). |
| `kinds.go` | `type Kind string` and all 164 `Kind*` constants (catalog in §3.3). |

Tests: `event_test.go` (required fields, determinism, hash chaining, ephemeral, RawMessage copy M482), `coverage_boost_test.go`.

### 3.1 The `Event` struct (wire contract)

```go
type Event struct {
    ID            string            `json:"id"`              // ULID, journal-assigned
    Seq           int64             `json:"seq"`             // 0-based, contiguous, journal-assigned
    TSUnixMS      int64             `json:"ts_unix_ms"`
    PrevHash      string            `json:"prev_hash"`       // hex; GenesisHash for seq 0
    Hash          string            `json:"hash,omitempty"`  // hex BLAKE3-256; "" = ephemeral
    Subject       string            `json:"subject"`         // dot-separated routing key
    Actor         string            `json:"actor"`
    Kind          Kind              `json:"kind"`
    CorrelationID string            `json:"correlation_id,omitempty"`
    CausationID   string            `json:"causation_id,omitempty"`
    Payload       json.RawMessage   `json:"payload,omitempty"`
    Tags          map[string]string `json:"tags,omitempty"`
}
```

Invariants:
- **Field order is part of the hash contract.** `Canonical()` is plain `json.Marshal` of a clone with `Hash=""` (dropped by `omitempty`). Go's encoder follows struct declaration order and sorts map keys, which gives determinism "for free". New fields must be appended at the end with `omitempty` (DECISIONS B1).
- `Spec` is the caller part (Subject, Kind, Actor, CorrelationID, CausationID, Payload `any`, Tags). `New(spec, id, seq, ts, prevHash)` validates subject/kind/actor non-empty and `prevHash` = 64 hex chars, marshals the payload, computes the hash.
- `marshalPayload` **copies** a `json.RawMessage` payload (M482): otherwise a later mutation of the caller's slice would diverge `Payload` from an already-computed `Hash`.
- **Ephemeral events**: `NewEphemeral(spec)` leaves `Seq=0, PrevHash="", Hash=""` and sets `TSUnixMS=time.Now()`; `IsEphemeral()` ⇔ `Hash==""` (explicitly *not* `Seq==0`, because the journal's first durable event also has seq 0). Used for `llm.token` / `llm.reasoning` deltas and the synthetic `agezt.pulse.dropped` notice.
- `Decode(line)` parses one JSONL line.

### 3.2 Correlation and causation

- **`CorrelationID`** groups a multi-event causal chain — in practice one run (the run's correlation id, e.g. `run-…`, `brain-distill-<ulid>`, `profile-distill-<ulid>`). `journal.Why` groups by it.
- **`CausationID`** points at the single event that caused this one; it can cross correlation boundaries (pulse tick → delta/salience/initiative under different correlations; channel reply → inbound message). `journal.Causes` walks it.
- **Sub-agent links** are not carried by either field: `subagent.spawned` is journaled under the *parent* correlation with payload `{child_correlation, parent}`; `journal.ParentOf(childCorr)` scans for it.
- Run subjects: `runtime.Kernel.SubjectForRun(corr)` (delegates to the lifecycle package) is the pattern used by every per-run live stream (controlplane run handler, openaiapi, restapi runs). Agent-loop streaming uses `agent.<actor>.llm`; runtime overlays use `agent.agent-<corr>.<facet>` (e.g. `.taste`, `.assure`).

### 3.3 Event kind catalog

All constants live in `kernel/event/kinds.go` (append-only; "never renumber or rename"). Grouped by domain, wire string in parentheses where not obvious:

| Domain | Kinds |
|---|---|
| Agent lifecycle | `KindAgentSpawned` (agent.spawned), `KindAgentSuspended`, `KindAgentResumed`, `KindAgentDied`, `KindAgentCrashed`, `KindAgentRetry` |
| Task / run | `KindTaskReceived`, `KindTaskCompleted`, `KindTaskAbandoned` (boot orphan reconciliation, M28), `KindTaskFailed` (payload `{error, reason∈error|max_iters|canceled|timeout}`), `KindTaskContinued` (auto-continue past MaxIter, M833) |
| Multi-agent | `KindSubAgentSpawned`, `KindSubAgentCompleted` (async delegate, M881), `KindMeshLoopRefused` (mesh.loop_refused) |
| Tools | `KindToolInvoked`, `KindToolResult`, `KindCodeExecuted` (code.executed) |
| LLM | `KindLLMRequest`, `KindLLMResponse`, `KindLLMToken` (ephemeral), `KindLLMReasoning` (ephemeral) |
| Control plane | `KindHalt` (halt), `KindResume` (resume), `KindInfo` (info — generic notices, e.g. update.available) |
| Run steering | `KindRunPaused`, `KindRunResumed`, `KindRunStepped`, `KindRunSteered`, `KindRunIntervention` |
| Anomaly | `KindAnomalyDetected` (**system.anomaly**) |
| Policy / Edict | `KindPolicyDecision`, `KindPolicyChanged`, `KindPolicyCompacted` |
| Governor / budget | `KindRoutingDecision`, `KindBudgetConsumed`, `KindProviderFallback`, `KindProviderRetry`, `KindProviderBreakerOpen`, `KindProviderBreakerClosed`, `KindBudgetExceeded`, `KindRateLimited` (rate.limited), `KindBudgetCapInert`, `KindBudgetUnpriced`, `KindBudgetCeilingSet` |
| Capability routing | `KindCapabilityRejected`, `KindCapabilityRerouted`, `KindCapabilityDegraded` |
| Context / intent | `KindContextCompacted`, `KindContextSelection`, `KindIntentInterpreted`, `KindIntentConfirmationRequired` |
| Council (M837) | `KindCouncilConvened`, `KindCouncilBrief`, `KindCouncilStarted` (council.member.started), `KindCouncilOpinion`, `KindCouncilConsensus` |
| Conductor (M997) | `KindConductorStarted`, `KindConductorStep`, `KindConductorDone` |
| Assure | `KindAssureVerdict` (assure.verdict) |
| Netguard | `KindNetguardBlocked` |
| Warden | `KindWardenExecuted`, `KindWardenProfileDowngraded`, `KindWardenLimitExceeded` |
| Approval / HITL | `KindApprovalRequested`, `KindApprovalGranted`, `KindApprovalDenied`, `KindApprovalTimeout` |
| Config Center | `KindConfigAccess` (config.access) |
| Scheduler DAG | `KindPlanStarted`, `KindPlanCompleted`, `KindPlanFailed`, `KindNodeStarted`, `KindNodeCompleted`, `KindNodeFailed` |
| Catalog | `KindCatalogSynced`, `KindCatalogSyncFailed`, `KindCatalogDiscoveryCompleted`, `KindCatalogDiscoveryFailed` |
| Pulse | `KindPulseTick`, `KindObserverDelta`, `KindSalienceScored`, `KindInitiativeTaken`, `KindInitiativeAct` (initiative.act, M999), `KindBriefingSent`, `KindPulsePaused`, `KindPulseResumed` |
| Self-repair | `KindSelfRepairPanic` |
| Channels | `KindChannelInbound`, `KindChannelOutbound`, `KindChannelError` |
| Memory | `KindMemoryWritten`, `KindMemoryRetrieved`, `KindMemoryForgotten`, `KindMemoryPruned`, `KindMemorySuperseded`, `KindMemoryConsolidated`, `KindMemoryProfiled`, `KindMemoryPromoted`, `KindMemorySuspended`, `KindMemoryCleaned` |
| World model | `KindWorldEntityUpserted` (worldmodel.entity.upserted), `KindWorldRelationUpserted`, `KindWorldRetrieved`, `KindWorldForgotten`, `KindWorldSuperseded` |
| Skills / Forge | `KindSkillCreated`, `KindSkillPromoted`, `KindSkillQuarantined`, `KindSkillReverted`, `KindSkillRestored`, `KindSkillActivated`, `KindSkillShadowEval` (skill.shadow_evaluated), `KindSkillShared`, `KindSkillReassigned` |
| Standing orders | `KindStandingCreated`, `KindStandingUpdated`, `KindStandingRemoved`, `KindStandingFired`, `KindStandingError` |
| Roster | `KindRosterCreated`, `KindRosterUpdated`, `KindRosterRemoved` |
| Reflection | `KindReflectionCompleted` |
| Journal | `KindJournalSegmentRotated` (journal.segment_rotated) |
| Webhooks | `KindWebhookDelivered`, `KindWebhookFailed` |
| Cadence | `KindScheduleFired` |
| Board | `KindBoardPosted` (board.posted) |
| Script tools (M794) | `KindScriptToolCreated`, `…Updated`, `…Tested`, `…Promoted`, `…Quarantined`, `…Removed` |
| MCP (M796) | `KindMCPAdded`, `KindMCPUpdated`, `KindMCPAttached`, `KindMCPDetached`, `KindMCPRemoved` |
| Workflow (M798) | `KindWorkflowSaved`, `…Updated`, `…Removed`, `…Restored`, `…Started`, `KindWorkflowNode` (workflow.node), `…Completed`, `…Failed`, `KindWorkflowPanic`, `KindWorkflowDrafted` |
| Workboard | `KindWorkboardTaskCreated`, `…Updated`, `…Claimed`, `…Heartbeat`, `…Commented`, `…Linked`, `…Dependency`, `…Dispatched`, `KindWorkboardTaskProved`, `KindWorkboardTaskUnproven` (all `workboard.task.*`) |
| OKR | `KindOKRObjectiveCreated`, `KindOKRObjectiveUpdated`, `KindOKRObjectiveAchieved` (okr.objective.*) |
| Taste | `KindTasteInjected` (taste.injected) |

**Ad-hoc kinds outside `kinds.go`** (string-literal `event.Kind(...)`, bypassing the "only place new kinds are introduced" rule):

| Wire kind | Where | Durable? |
|---|---|---|
| `market.install.progress`, `market.uninstall.progress` | `kernel/controlplane/market.go` | published on the bus |
| `toolbox.progress` (and other `event.Kind(kind)` values) | `kernel/controlplane/toolbox.go` | published on the bus |
| `policy.auto_approved`, `prompt_injection.warned` | `kernel/runtime/intent.go` | journaled |
| `agezt.pulse.dropped` | `kernel/controlplane/pulse.go` | synthetic, written only to the stream client (Hash="") |
| `webhook.test` (`webhook.TestEventKind`) | `kernel/webhook/webhook_helpers.go` | test delivery |

**Defined but never referenced in non-test code** (computed by diffing `kinds.go` constants against all `event.Kind*` references in `cmd/ kernel/ plugins/ sdk/`): `KindAgentSpawned`, `KindAgentSuspended`, `KindAgentResumed`, `KindAgentDied`, `KindAgentCrashed`, `KindConfigAccess`, `KindJournalSegmentRotated`, `KindWorldSuperseded`. In particular the journal never emits `journal.segment_rotated`, and the world model has no supersede operation at all. They are kept because the enum is append-only/contract-pinned (`.project/agezt-contract.jsonc`).

**Kind misuse to know about:** `runexec.Runner.MaybeDistill` reports a distillation failure as `Kind: memory.written` with subject `memory.distill_failed` and payload `{action:"distill_failed"}` — consumers filtering on `KindMemoryWritten` see failures too.

---

## 4. `kernel/journal`

**Purpose.** The append-only, BLAKE3-chained, fsync'd event log; source of truth for audit, replay, provenance and every journal-folded view (runs list, stats, agent activity).

| File | What it does |
|---|---|
| `doc.go` | Package doc. **Stale claim:** it says "a sidecar index records the (offset, seq, hash) of every entry so tail/grep/head reads in O(log N)" — no index exists in code; all reads are sequential scans. |
| `journal.go` | `Options{SegmentBytes, Now, IDGen}`, `Journal` struct, perms `journalDirPerm=0o700` / `journalSegmentPerm=0o600` (EXPOSE-001), `Close`, `Head`, `Append`, `Range`; constants `DefaultSegmentBytes=64 MiB`, `segmentExt=".jsonl"`, `segmentDigits=8`. Doc comment for `Tail` sits here; the body is in `journal_io.go`. |
| `journal_open.go` | `Open(dir, opt)`: mkdir 0700, best-effort chmod of dir and every segment, recovery scan of all segments, torn-tail truncation, choose append target; `lastCompleteOffset`. |
| `journal_seg.go` | `openCurrent` (O_APPEND for resume, O_EXCL for new), `scanSegment` (recovery chain check), `segmentPath`, `listSegments` (numeric `NNNNNNNN.jsonl` only, sorted), `rangeCompleteLines` (bufio reader with no token ceiling; discards an unterminated final line). |
| `journal_io.go` | `Tail(n)`, `readSegment`, errors `ErrChainBreak`/`ErrNotEmpty`/`ErrNotFullExport`, `Restore(dir, events)`, `Verify()`, indirected `fsync`/`syncDir` (test hooks), `writeAndSync`, `rotate`. |
| `provenance.go` | `Why(eventID)` (correlation grouping), `Causes(eventID)` (causation ancestry, cycle-guarded), `ParentOf(childCorr)` (delegation backlink). Moved here from `kernel/runtime` in Phase 3.1. |
| `cursor.go` | Shared `<ms>:<seq>` cursor pagination: `EncodeCursor`, `DecodeCursor(any)`, `KeepBeforeCursor`, `NextCursor`. Used by every journal-backed list endpoint (runs, agents, inbox/board/memory, *_log). |
| `runs.go` | `RunEntry` (folded per-run state shared by runs listing/stats) and `RunEntryStatus` (precedence completed > failed > abandoned > running). |

Tests: `journal_test.go`, `provenance_test.go`, `cursor_test.go`, `mutation_internal_test.go` (fsync-failure truncation, rotation failure), `fuzz_test.go` (1 fuzz target).

### 4.1 On-disk format

```
$AGEZT_HOME/journal/            (0700)
  00000001.jsonl                (0600)  one compact JSON Event per line, '\n'-terminated
  00000002.jsonl
  ...
```
- Each line is `json.Marshal(event)` — includes `hash`. Seq is contiguous from 0 across segments; the chain continues across segment boundaries (segment N+1's first `prev_hash` = segment N's last `hash`).
- Non-numeric `.jsonl` files and subdirectories in the dir are ignored as foreign.
- Tenants get their own journal under `$AGEZT_HOME/tenants/<id>/journal/` (tenant kernels are opened via `kernel/tenant` with their own `BaseDir`, and get the same redactor installed — see `cmd/agezt/main.go`).

### 4.2 Append path (durable-before-publish)

```
bus.Publish(spec)                       [Bus.mu held]
  └─ redactSpecLocked(spec)             payload re-marshaled w/o HTML escaping, scrubbed
  └─ journal.Append(spec)               [Journal.mu held]
       id=idGen(); seq=nextSeq; ts=now(); prev=head
       e = event.New(spec,id,seq,ts,prev)   → BLAKE3(prev‖canonical)
       line = json.Marshal(e)+'\n'
       writeAndSync(line)
         write → fsync
           fsync fails → Truncate(curBytes)+Seek(curBytes) ; return error (fail-closed)
         curBytes += len
         if curBytes >= segBytes → rotate() (errors ignored; retried next append)
       head = e.Hash; nextSeq++
  └─ for each matching subscription: non-blocking send, else Dropped++
```
- `rotate()` opens `idx+1` with `O_EXCL` **before** swapping handles, so a failed open leaves the (oversized) current segment live; a successful rotate fsyncs the directory (best-effort; Windows may fail).
- New segments: `openCurrent(false)` fsyncs the directory entry (power-loss safety on Linux).

### 4.3 Recovery (`Open`)

1. List segments; `scanSegment` each in order, checking `ev.Seq == nextSeq`, `ev.PrevHash == head`, and `VerifyHash()` — any failure → `ErrChainBreak` and **Open fails** (daemon boot fails in `runtime.Open`).
2. Append target: if the last segment `>= segBytes` start `idx+1`; else compute `lastCompleteOffset` and **truncate a torn tail** (crash mid-write) so the next O_APPEND write starts exactly after the last committed line.
3. Cost: Open reads and hash-verifies the entire journal on every boot (O(total bytes)).

### 4.4 Verification, tail, restore, export

- `Verify()` = `Range` + seq/prev/hash checks from genesis. Exposed as control-plane `handleVerify` (`agt journal verify`).
- `Range(fn)` streams every event in seq order, lock-free (reads only complete fsync'd lines, safe alongside Append).
- `Tail(n)` reads newest segments backwards until it has ≥n events (cost ≈ last segment).
- `Restore(dir, events)` (M102) seeds an **empty** journal from a full, genesis-anchored, chain-verified slice: validates everything before touching disk, writes all events as **one** segment `00000001.jsonl` (O_EXCL, fsync, dir fsync), removes the file on any write error. Errors: `ErrNotEmpty`, `ErrNotFullExport` (windowed `--since` export or seq≠0), `ErrChainBreak`. Used by `agt journal import` (which re-Opens and compares Head) and `agt backup restore`.
- Export lives in the CLI (`cmd/agt/journal_export.go`): a `journalBundle{Manifest, Events []json.RawMessage}` with manifest `{tool, product, format_version, exported_at_unix_ms, count, first_seq, last_seq, head_seq, head_hash, truncated, since_ms, scope}`. Events stay raw bytes. A `--scope task:<corr>` cut is intentionally non-contiguous; its verify checks per-event hashes + scope membership instead of prev-hash continuity. Other scope prefixes (`agent:`, `tenant:`, `skill:`, `memory:`, `order:`) are rejected as unsupported. See [02-cli-cmd-agt.md](02-cli-cmd-agt.md).

### 4.5 Provenance — `agt why`

Control-plane `handleWhy` (`kernel/controlplane/server_commands.go`), tenant-scoped via `kernelFor(tenantOf(req))`:

1. `Why(id)` — full `Range`, keep all events, find the target; return every event sharing its `CorrelationID` (or the target alone when uncorrelated).
2. `ParentOf(corr)` — another full `Range` looking for `subagent.spawned` whose payload `child_correlation == corr`; last match wins → `parent_correlation`.
3. `Causes(id)` — a third full `Range` building `byID`, then walk `CausationID` back to the root (stops on `""`, missing parent, or a cycle) → `causation_chain` (oldest first; only included if length > 1).

So one `agt why` costs three complete journal scans and holds the entire journal in memory for `Why`/`Causes`.

### 4.6 Cursor pagination contract

- Lists are sorted DESC by `(ms, seq)`; token `"<ms>:<seq>"`; `(0,0)` encodes to `""` = no next page.
- `DecodeCursor` returns `ok=false` for absent/malformed/negative tokens → callers must serve the newest page, never error.
- `KeepBeforeCursor` drops rows **equal** to the cursor (the cursor is the oldest row already emitted).
- `NextCursor` encodes the *last* (oldest) emitted row, and only when the page is full.

### 4.7 Concurrency

`Journal.mu` guards `nextSeq/head/curFile/curBytes/curIndex`. Readers (`Range`, `Tail`, `Why`…) open segment files independently with no lock.

---

## 5. `kernel/bus`

**Purpose.** In-process pub/sub that enforces durable-before-publish, NATS-style subject matching, secret redaction, and an ephemeral fast path.

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `bus.go` | `Redactor` interface (`Redact`, `RedactBytes`), `Bus{j, mu, subs, nextSubID, closed, redactor}`, `New(j)`, `SetRedactor`/`Redactor`, `redactSpecLocked`, `subscription`/`Subscription{C, Dropped, cancel}`, `Subscribe(pattern, buf)` (default buf `DefaultSubBuffer=256`), `Close`, `ErrPattern`, `ErrClosed`. |
| `bus_publish.go` | `Publish` (journal append → fan-out) and `PublishStreaming` (ephemeral `event.NewEphemeral` → fan-out, never journaled). Both redact first. |
| `bus_match.go` | `MatchSubject` (exported for journal-replay filtering), `matches`, `ValidatePattern` (used by webhook config parsing), `parsePattern`. |

Tests: `bus_test.go`, `coverage_test.go`, `publish_error_test.go` (journal error ⇒ no subscriber sees the event).

**Semantics.**
- Patterns: `.`-separated tokens; `*` = exactly one token; `>` = one-or-more remaining tokens, must be last; `>` alone = everything. Empty tokens rejected.
- Delivery: `select { case ch <- e: default: dropped++ }` — **never blocks publishers**; a slow subscriber loses events and must watch `Subscription.Dropped`.
- Publish holds `Bus.mu` during the journal fsync **and** the fan-out loop, so (a) all durable publishes are totally ordered (subscriber order == seq order), (b) a slow disk stalls every publisher in the process.
- Redaction (M170/M418): payload is JSON-encoded with `SetEscapeHTML(false)` so literal secrets containing `& < >` are matched; tags are copied and scrubbed. Applied to ephemeral events too because they reach webhooks/SSE/OpenAI relay. Installed once at boot by `cmd/agezt/main.go` (`AGEZT_REDACT`, `AGEZT_REDACT_EXTRA` → [05-governance-routing-security.md](05-governance-routing-security.md)), and on each tenant bus.
- `Cancel` is `sync.Once`-guarded; `Close` closes every subscriber channel and makes later Publish/Subscribe return `ErrClosed`.

### 5.1 Event flow: bus → journal → subscribers

```mermaid
flowchart LR
  P[producers: agent loop, governor, edict, memory/world managers,<br/>runtime workboard/okr/taste wrappers, channels, pulse,<br/>standing, workflow, daemon boardNotify, anomaly ...] -->|Publish| R[redact]
  R --> J[(journal segment<br/>append + fsync)]
  J --> F{fan-out<br/>non-blocking}
  P -->|PublishStreaming<br/>llm.token / llm.reasoning| R2[redact] --> F
  F --> S1[controlplane pulse stream<br/>'agt pulse' / replay, buf 4096]
  F --> S2[webui /api/events SSE, '>' buf 256]
  F --> S3[run streams: controlplane run, restapi runs,<br/>openaiapi chat+responses — SubjectForRun(corr), buf 1024]
  F --> S4[webhook.Dispatcher — one sub per sink subject, buf 256]
  F --> S5[standing.Runner '>' / workflow.Runner '>']
  F --> S6[anomaly.Start '>' / alerter.Start '>']
  F --> S7[restapi mailbox watch 'board.>' / agentgw / selfrepair]
  F --> S8[cmd/agezt: artifact indexer, pulse-artifacts, misc '>']
```

Subscriber inventory (`grep "\.Subscribe("`, non-test): `cmd/agezt/main.go` (`>`), `cmd/agezt/main_pulse_artifacts.go` (`>`), `kernel/agentgw/handlers.go` (caller pattern), `kernel/alerter` (`>`), `kernel/anomaly` (`>`), `kernel/controlplane/pulse.go` (pattern, 4096), `kernel/controlplane/server_handlers_plan.go` (`<prefix>.>`, 1024), `kernel/controlplane/server_handle_run.go`, `kernel/openaiapi/openaiapi_run.go` ×2, `kernel/openaiapi/responses_stream.go`, `kernel/restapi/restapi_routes_runs.go` (all `SubjectForRun`, 1024), `kernel/restapi/mailbox_watch.go` (`board.>`), `kernel/selfrepair/selfrepair.go` (pulse subject, 64), `kernel/standing/runner.go` (`>`), `kernel/webhook/webhook.go` (per sink), `kernel/webui/webui_proxy.go` (`>`, SSE), `kernel/workflow/runner.go` (`>`).

- The controlplane "pulse" stream (`agt pulse`) subscribes **before** walking the journal for historical replay (`since`/`since_ts`/`until`/`replay_only`, optional `replay_rate`), then skips live events with seq ≤ `lastReplayed`; it emits a synthetic ephemeral `agezt.pulse.dropped` notice when `sub.Dropped` grows.
- The webhook dispatcher never re-delivers `webhook.*` events (no feedback loop); each delivery outcome is journaled as `webhook.delivered` / `webhook.failed` ([03-control-plane-and-http.md](03-control-plane-and-http.md)).
- Pulse itself is a producer here (tick→delta→salience→initiative→brief); standing orders bind to subjects such as `pulse.initiative.act` and `board.*` ([07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md)).

---

## 6. `kernel/platform/filestore`

| File | What it does |
|---|---|
| `store.go` | `Load(path, out)`: missing / empty / whitespace-only file ⇒ nil (first boot), strips a UTF-8 BOM, otherwise wraps read/parse errors with the path. `LoadFrom(dir, name, out)`: `EnsureDir(dir)` then Load, returns the joined path. `Save(path, v)`: `json.MarshalIndent` + `atomicfile.WriteFile(path, b, FilePerm)`. `EnsureDir`: `MkdirAll(dir, 0o700)` + best-effort `Chmod` so pre-existing 0755 installs tighten (same posture and best-effort rule as the journal). `FilePerm`=0600, `DirPerm`=0700. |
| `lock.go` + `lock_{unix,windows,other}.go` | `Lock(path) (unlock, error)`: exclusive OS lock on the sidecar `path+".lock"` (flock / LockFileEx via `golang.org/x/sys`), blocking, released by the kernel if the holder dies. Conflicts between two opens even inside one process, so it serialises goroutines and processes alike. Creates a missing directory but never re-permissions an existing one (the vault sits directly in the base dir). Used by `creds` and `settings` for merge-on-save. |

- Deliberately **not** a generic `Store[T]`; each store keeps its own mutex and domain methods (Phase 1.1 of `docs/REFACTORING-SCAN-2026-08.md`).
- `internal/atomicfile.WriteFile`: unique temp `.<base>.*.tmp` in the target dir → write → fsync → close → chmod → rename; on Windows a failed rename falls back to remove+rename, then a direct non-atomic write as last resort.
- Files are written 0600 in 0700 dirs (W1.3). Until then they were 0644 in 0755 dirs — memory.json, worldmodel.json, board.json, workboard.json were world-readable while the journal holding the same material was 0600/0700.
- **Gotcha:** a corrupt JSON file is a hard `Open` error, which propagates to `runtime.Open` failure for memory/worldmodel/workboard/okr/taste (boot fails), while board's failure only disables board commands.
- Tests: `store_test.go` (load/save), `lock_test.go` (privacy modes; goroutine exclusion; **two-process** exclusion via a re-exec'd helper process).

---

## 7. `kernel/memory`

**Purpose.** The agent's long-term knowledge: content-addressed records, journaled mutations, hybrid retrieval, scope privacy, and LLM maintenance passes (per-run distill, brain consolidation, operator profile).

### 7.1 Files

| File | What it does |
|---|---|
| `doc.go` | Package doc: two layers (pure `Store` vs journaling `Manager`), content addressing, soft updates/forgets, hybrid retrieval (M803). |
| `memory.go` | `Type` (FACT, SUMMARY, RELATION, PREFERENCE, OBSERVATION; `DefaultType=FACT`), `Evidence` ("", observed, inferred, curated, constraint), `ValidType`, `Record`, `Active`/`Suspended`/`Expired`/`Usable`, `ContentID(t,subject,content)`, `ScopedID(…, scope)` (M915). |
| `memory_store.go` | `Store` interface (Put/Get/Delete/All/Count/Close), `ErrEmptyContent`, `FileStore` (`<dir>/memory.json` map id→Record, RWMutex, whole-file snapshot per mutation), `sortRecords` (CreatedMS, ID). |
| `memory_search.go` | `Scored`, keyword `Search` (overlap × (0.5+conf) × recency), `keywordOverlap`, `recencyFactor` (1/(1+ageDays)), `tokenize` (lowercase, split on non letter/digit, drop ≤1-char). |
| `vector.go` | Local embedder: `EmbedDim=256` signed feature hashing (FNV-64a) of word tokens + rune 3-grams with `^`/`$` markers, L2-normalized; `Cosine`; `searchText` (subject+content+tags); `SearchSemantic` (max of whole-query cosine and 0.85-damped per-token cosine for tokens ≥4 runes; floor `minSemanticCosine=0.2`); `SearchHybrid`; `mergeScored` (sum scores by id); `sortScored`. |
| `embedder.go` | Provider-embedding opt-in (M884): `Embedder` interface (`EmbedBatch`), `embedTimeout=10s`, `SetEmbedder` (hot-swappable, resets cache), `semanticProvider` (one batch: query + cache misses; in-memory cache keyed by record ID). |
| `manager.go` | `Manager{store, bus, now, mu, embMu, embedder, embCache}`, `NewManager`, `RememberSpec`, `Remember` (create/reinforce/revive). |
| `manager_recall.go` | `Recall`/`RecallScoped` (journals `memory.retrieved`), `Forget`, `Promote` (M915), `Supersede` (doc comment split across `manager_hygiene.go`'s tail), `Get`, `Active`, `All`, `Count`, `Search`, `SearchScoped` (quiet). |
| `manager_hygiene.go` | `HygieneStats`, `Hygiene(olderThanMs)`, `Prune(corr, olderThanMs, dryRun)` (M857 — only destructive op on soft-deleted rows). |
| `manager_ops.go` | `Suspend`, `AuditReport` + `Audit` (contradiction groups), `CleanReport`/`CleanDecisionRow`/`ContradictionGroup`, `CleanLowValue` (hard-delete never-should-have-been-memory rows), `publish` helper, `clampConf`, ctx key type. |
| `manager_distill.go` | `Distill` (per-run, M993 subject dedupe), `distillKey`, `normalizeEvidence`, `defaultHalfLifeMS`, contradiction helpers, `DedupeDistilled`, `strongerNote`, `parseDistill`. (Its doc comment lives at the tail of `manager_tool_helpers.go`.) |
| `manager_tool.go` | The in-process `memory` agent tool: `toolInput`, `memoryTool`, `(*Manager).Tool()`, `Definition` (capability `edict.CapMemory`, `EffectReversible`), `Invoke` actions `remember|recall|forget|find_related|bulk_forget` (≤500 ids). |
| `manager_tool_ctx.go` | `WithCorrelation`/`CorrelationFrom`, `WithScope`/`ScopeFrom` (M786), `toolInputSchema`. |
| `manager_tool_helpers.go` | `toolActor` (agent slug or "agent"), `toolTags` (`source=agent`, optional `scope`), `scopeOf`, `filterScope`, `renderHits`, `plural`, `distillSystem` prompt, `distillResult`. |
| `consolidate.go` | Brain consolidation (M804): `Clusters` (scope-partitioned seed-cosine clustering), constants `clusterCosine=0.55`, `minClusterSize=3`, `maxClustersPerPass=4`, `maxClusterRecords=12`; `BrainDistillReport`; `DistillBrain`; `consolidateCluster` (via `agent.GenerateObject`); `supersedeExisting`. |
| `profile.go` | Operator profile (M1000): fixed facets `preferences`, `communication style`, `expertise`, `people and projects`, `current focus`; subject prefix `"operator profile: "`; `maxProfileInput=60`; `DistillProfile`; `ProfileText`. |
| `retention.go` | Rule-based retention filter: `RetentionDecision`, `AssessRetention`, `assessSpec`, `shouldFilterSpec`, system-agent detection (`guardian-*`, `*-sentinel`, …), needle lists for execution logs / system logs / transient text / durable predicates. |

Tests: `manager_test.go`, `memory_test.go`, `concurrency_test.go`, `consolidate_test.go`, `embedder_test.go`, `epistemic_test.go`, `profile_test.go`, `provenance_test.go`, `prune_test.go`, `scope_test.go`, `scope_ctx_test.go`, `supersede_bug_test.go`, `vector_test.go`.

### 7.2 Record model

| Field | Meaning |
|---|---|
| `ID` | `hex(BLAKE3(type\0subject\0content))`, or with `\0scope` appended when the record is scoped (`ScopedID`); empty scope hashes identically to `ContentID` so pre-M915 shared ids are stable. |
| `Type`, `Subject`, `Content`, `Tags` | Classification, retrieval subject, injected text, labels. Reserved tags: `scope` (private namespace; absent/empty = shared), `source` (`agent`, `distill`, `brain-distill`, `operator`, `profile`). |
| `SourceEvent` | Journal event id of the first `memory.written` (provenance for `agt why`). |
| `AddedBy` / `UpdatedBy` | First / latest writer (agent slug, `operator`, `distill`, `profile`) (M851). |
| `Confidence` | 0..1, default 1.0; +0.1 on each reinforce. |
| `Evidence` | Epistemic class; defaulted by `normalizeEvidence` (operator→curated, distill/agent→inferred, OBSERVATION→observed). |
| `CreatedMS`, `LastSeenMS` | Recency/decay inputs. |
| `HalfLifeMS` | Expiry budget; defaults: constraint 3650d, curated 180d, observed 30d; else PREFERENCE 180d, SUMMARY 90d, OBSERVATION 30d, other 60d. Expired = retained but excluded from recall until reinforced. |
| `SuspendedMS`/`SuspendedReason` | Epistemic suspension (excluded from recall; reinforce clears it). |
| `SupersededBy` | Soft update link. Preserved on reinforce so restating superseded content does not resurrect it. |
| `Tombstoned` | Soft forget; a fresh `Remember` revives it. |

`Usable(now)` = not tombstoned, not superseded, not suspended, not expired.

### 7.3 Write path: `Remember(corr, spec)`

1. Validate content/type; clamp confidence.
2. Retention filter: unless `spec.Force`, writes whose `source` is agent/distill/brain-distill (or actor agent/distill) go through `assessSpec`; rejected writes return `memory: low-value record rejected (<reason>)` (reasons: `empty`, `too_short`, `execution_log`, `transient_without_expiry`, `no_subject_or_durable_predicate`, `system_agent_log_type`, `system_agent_log_output`). Curated/constraint/PREFERENCE/operator writes always pass.
3. `id = ScopedID(type, subject, content, scope)`; take `Manager.mu` (serializes Get→Put, M421).
4. Existing ⇒ reinforce (keep CreatedMS/SourceEvent/AddedBy/SupersededBy, conf+0.1, clear suspension; `revive` if tombstoned).
5. **Publish `memory.written` first** (subject `memory.written`, actor `memory`, payload `{action, id, type, subject, chars, confidence, evidence?, half_life_ms?, actor?}`), record its id as SourceEvent, then `store.Put`. A crash between the two leaves an orphan audit event (accepted).

All Manager events use subject `memory.<suffix-of-kind>` and actor `memory`.

### 7.4 Private-by-default vs shared (M652/M786/M915)

- The runtime puts the acting agent's slug on the run ctx with `memory.WithScope`.
- Tool `remember`: explicit `scope` param wins; `shared=true` or `scope="shared"` ⇒ shared (`""`); otherwise the ctx scope ⇒ **private to the agent**. Unscoped runs (operator chat) write shared.
- Tool `recall` and the runtime's pre-run injection use `RecallScoped(scope)`: visible = shared records + records whose scope equals the requested scope. An empty scope sees shared only — a run never inherits another agent's private notes.
- Scope participates in identity, so the same fact noted privately by two agents is two records.
- `Promote(id)` deletes the `scope` tag (id unchanged) and publishes `memory.promoted {from_scope}` — the only path from private to shared.
- Per-run `Distill` tags facts with the run's scope; consolidation never mixes scopes (scope is a hard wall in `Clusters`) and the merged record inherits the cluster's scope; the operator profile reads **shared** records only.

### 7.5 Retrieval

- `SearchHybrid` = keyword `Search` ∪ `SearchSemantic` (local feature-hash), scores summed per id. Pure functions of `(records, query, limit, now)`; ties → LastSeenMS desc → ID asc.
- With an `Embedder` installed (`AGEZT_EMBED_URL` + `AGEZT_EMBED_MODEL` [+ `AGEZT_EMBED_KEY`] → `runtime.Config.MemoryEmbedder`), `RecallScoped` replaces the local semantic half with provider cosine (`mergeScored(Search, semanticProvider)`); any embedder error silently falls back to the local hybrid. The `memory.retrieved` payload records `embedder: "local"|"provider"`.
- `Recall*` journals `memory.retrieved {query, matched, ids, embedder}`; `Search`/`SearchScoped` are quiet (operator ad-hoc search; context-selection candidate lists).
- Runtime injection order in `kernel/runtime/prompt_run.go` (non-system agents only): operator profile (`ProfileText`, `cfg.ProfileInject`) → taste exemplars → memory recall (`MemoryTopK` default 5, plus a `context.selection` manifest of chosen/rejected candidates) → world-model resolve → skills. See [04-agent-runtime.md](04-agent-runtime.md).
- Cost: every recall/search loads `store.All()` (copy of the whole map, sorted) and embeds every record locally — O(n) per run.

### 7.6 Distillation, dedupe, consolidation, profile, tidy

| Pass | Entry point | Trigger | Effect / events |
|---|---|---|---|
| Per-run distill | `Manager.Distill(ctx, corr, provider, model, intent, transcript)` | `runexec.Runner.MaybeDistill` after a run whose folded tool count ≥ `MemoryDistillMinTools` (`AGEZT_MEMORY_DISTILL_MIN_TOOLS`, daemonconfig default 6, runner fallback 4) | One `TaskType:"distill"` completion; ≤3 facts JSON; **subject-level dedupe (M993)**: if an active `source=distill` record exists for `(type, normalized subject, scope)` it is reinforced instead of adding a near-duplicate. Facts tagged `source=distill` (+scope). Non-JSON answer = no-op. |
| Retro dedupe | `DedupeDistilled(corr, dryRun)` | `agt memory tidy`/controlplane, cadence systemtask | Groups active distill notes by `distillKey`, keeps the strongest (confidence, then recency), **Forgets** (tombstones) the rest. |
| Brain consolidation | `DistillBrain(ctx, corr, provider, model)` | `AGEZT_BRAIN_DISTILL_EVERY` ticker (`cmd/agezt/main_pulse.go`, corr `brain-distill-<ulid>`), `agt memory consolidate` | Clusters (≥3 records, cosine ≥0.55, same scope), merges ≤4 clusters/pass (≤12 records each) through `agent.GenerateObject`, writes a `source=brain-distill` record, supersedes originals (`memory.superseded`), then one `memory.consolidated`. Provider error aborts the pass; unusable answer skips the cluster. |
| Operator profile | `DistillProfile(ctx, corr, provider, model)` | `AGEZT_USER_PROFILE` (default on, requires memory) daily ticker (`AGEZT_USER_PROFILE_EVERY`, default 24h), cadence systemtask, `agt memory profile` | Reads ≤60 freshest shared non-profile records + current facets; writes each facet as a `TypePreference` record, subject `operator profile: <facet>`, tags `{scope:"", source:"profile"}`, `Force:true`; publishes `memory.profiled`. |
| Hygiene / prune | `Hygiene`, `Prune(corr, olderThanMs, dryRun)` | `agt memory hygiene/prune`, controlplane tidy | Prune hard-deletes only tombstoned/superseded rows older than the cutoff → `memory.pruned {pruned}`. |
| Clean | `CleanLowValue(corr, dryRun)` | controlplane (dry-run default), cadence systemtask | Re-applies `AssessRetention` to everything except operator/curated/constraint/PREFERENCE rows; hard-deletes rejects → `memory.cleaned`. |
| Audit / suspend | `Audit()`, `Suspend(corr,id,reason)` | `agt memory audit` | Reports expired/suspended ids and contradiction groups (same type + normalized subject + scope, differing content); `memory.suspended`. |

### 7.7 Env vars (read in `cmd/agezt/internal/daemonconfig`)

`AGEZT_MEMORY` (≠off: inject/tool/distill), `AGEZT_MEMORY_DISTILL_MIN_TOOLS`, `AGEZT_USER_PROFILE`, `AGEZT_USER_PROFILE_EVERY` (read in `main_pulse.go`), `AGEZT_BRAIN_DISTILL_EVERY`, `AGEZT_EMBED_URL`/`_MODEL`/`_KEY`. All are in the controlplane `configEnvVars` guard list.

### 7.8 Concurrency

`FileStore.mu` (RWMutex) per call; `Manager.mu` around every read-modify-write (Remember, Forget, Promote, Supersede's old-record update, Suspend, supersedeExisting, the delete loops of Prune/CleanLowValue); `Manager.embMu` for embedder + cache (never held during the remote call). `Supersede` calls `Remember` (locks/unlocks) then re-locks for the old record — not atomic across the pair.

---

## 8. `kernel/worldmodel`

**Purpose.** "World Model v1" (SPEC-05 §3): a journaled, content-addressed graph of the operator's world, resolved *before* memory during context assembly and used by Pulse salience (`IsActiveSubject`).

| File | What it does |
|---|---|
| `doc.go` | Package doc (mirrors memory's Store/Manager split). |
| `worldmodel.go` | Open vocabularies `Kind` (project, repo, person, org, account, device, channel, topic, task; `DefaultKind=topic`) and `Verb` (owns, depends_on, member_of, prefers, relates_to, assigned_to, derived_from; `DefaultVerb=relates_to`) with permissive `NormalizeKind`/`NormalizeVerb`; `Entity`, `Relation`, `EntityID` (`BLAKE3("entity"\0kind\0lower(name))`), `RelationID` (`BLAKE3("rel"\0from\0verb\0to)`), `Store` interface, `graphData{Entities, Relations}`, `FileStore`, `Open` (`<dir>/worldmodel.json`), `snapshotLocked`, sort helpers. |
| `worldmodel_store.go` | `FileStore` methods: `PutEntity`, `GetEntity`, `AllEntities`, `PutRelation`, `GetRelation`, `AllRelations`, `Count` (entities), `Close` (no-op). |
| `manager.go` | `Graph{store, bus, now, mu}`, `NewGraph`, `UpsertSpec`, `Upsert` (create/reinforce/revive; weight +0.1, alias/attr merge), `EditEntity` (replace aliases/attrs, action `edit`), `Relate` (resolve-or-create endpoints as `topic`, reinforce edge). |
| `manager_query.go` | `resolveOrCreate`, `Resolve` (journals `worldmodel.retrieved`), `ResolveQuiet`, `IsActiveSubject` (score floor 1.0 — pulse relevance adapter), `Neighbors`, `Forget` (entity or relation), `Get`, `Entities`, `Relations`, `Count`, `publish` (subject `worldmodel.<suffix>`, actor `worldmodel`), weight/alias/attr helpers. |
| `manager_tool.go` | `WithCorrelation`/`CorrelationFrom`, `toolInputSchema`, the `world` agent tool (capability `edict.CapWorld`; actions `add|relate|resolve|neighbors`), renderers. |
| `resolve.go` | Pure ranking: `ScoredEntity`, `Resolve` (exact name 4.0 > exact alias 3.0 > token overlap over name+kind+aliases; × (0.5+weight) × recency), `Neighbor`/`Neighbors`, `recencyFactor`, `tokenize` (identical to memory's). |
| `decay.go` | `DecayOptions` (StaleAfter 14d, Factor 0.8, Floor 0.1) and `Graph.Decay` — lowers stale active entity weights, never raises, never tombstones, never refreshes LastSeen; journals `worldmodel.entity.upserted` with `action:"decay"`. Called by `kernel/reflect`. |

Tests: `worldmodel_test.go`, `manager_test.go`, `resolve_test.go`, `decay_test.go`, `provenance_test.go`, `concurrency_test.go`.

Notes / gotchas:
- Env: `AGEZT_WORLDMODEL` (≠off) enables in-run injection and the `world` tool.
- No supersede operation exists (`KindWorldSuperseded` is unused) even though entities/relations carry `SupersededBy`.
- `Relate` resolves endpoints **outside** `Graph.mu` (`resolveOrCreate` reads `AllEntities` unlocked, then may `Upsert`), so two concurrent `Relate`s naming the same new endpoint can race — benign because `EntityID` is content-addressed (both upsert the same id; one becomes a reinforce).
- `Count()` counts entities only.

---

## 9. `kernel/datalake`

**Purpose.** "Personal Data Lake" (M834): dependency-free structured collections agents and the console share (rendered as generic tables or bespoke views).

| File | What it does |
|---|---|
| `doc.go` | Layout doc. |
| `datalake.go` | Errors (`ErrNotFound`, `ErrExists`, `ErrSystem`), `Field`, `Schema` (name, title, lucide icon, view, desc, fields, Builtin, System, CreatedMs/By), `Record` (`rec-<ulid>` id, `Fields map[string]any`, created/updated ms + by), `Query` (Search substring, Equals, SortBy, Desc, Limit, Offset), `CollectionInfo`, `Lake{dir, mu, colls, now}`, `Open(baseDir, now)` (loads every collection dir; unreadable collections are skipped silently), `loadCollection`, `validName` (`[A-Za-z0-9_-]{1,64}`, not `_schema`). |
| `datalake_schema.go` | `CreateCollection`, `EnsureCollection` (idempotent), `writeSchema`, `DropCollection` (refuses `System`), `ListCollections`, `titleOf`, `Schema`, `canonicalDate`/`canonicalizeDateFields` (copy-on-write; only fields the schema declares `date`, only whole-date values → `YYYY-MM-DD`). |
| `datalake_records.go` | `Insert`, `Get`, `Update` (merge patch; `nil` value deletes key), `Delete`, `Query`, `Count`, match/sort helpers (`matchEquals` via JSON-normalized compare, `matchSearch`, `lessRecords`, `toFloat`, `jsonEqual`), `writeRecord`, `writeJSON` (atomicfile 0600). |
| `builtins.go` | `BuiltinSchemas()` — 7 `Builtin+System` collections: expenses (view expense), calendar, tasks, notes, habits, bookmarks, contacts; `SeedBuiltins(actor)` (idempotent, run at every boot from `runtime.Open` with actor `system`, errors ignored). |

Tests: `datalake_test.go`, `datewrite_test.go`, `json_equal_bug_test.go`, `coverage_boost_test.go`.

- Layout: `$AGEZT_HOME/datalake/<collection>/_schema.json`, `$AGEZT_HOME/datalake/<collection>/rec/<id>.json` (dirs 0700, files 0600).
- Concurrency: one `Lake.mu` for everything; `Query` copies records under the lock then filters/sorts outside it.
- **Not journaled, no events.** Provenance is only `CreatedBy/UpdatedBy` on records. Consumers: `plugins/tools/db` (agent tool), controlplane datalake handlers, runtime accessor.
- In-memory index is updated only after a successful file write (write-then-commit).

---

## 10. `kernel/artifact`

**Purpose.** Content-addressed blob store (SPEC-04 §3.6) + browsable metadata index (M822/M823).

| File | What it does |
|---|---|
| `artifact.go` | `Store{dir}`, `Open` (0700), `Ref(data)` (hex BLAKE3-256), `Put` (dedup: skip if exists; atomicfile 0600), `Get` (re-hashes and returns `ErrCorrupt` on mismatch), `Has`, `Size`, `pathFor` (`<dir>/<ref[:2]>/<ref>`), `validRef` (64 lowercase hex — path-traversal guard); errors `ErrNotFound`, `ErrCorrupt`, `ErrBadRef`. |
| `index.go` | `Entry` (per-arrival: `art-<ulid>` id, ref, name, mime, kind `image|tool-output|file…`, source `telegram|slack|run…`, sender, corr, size, created_ms, caption), `Filter{Kind, Source, Corr}`, `Index{store, dir, mu, entries}`, `OpenIndex` (`<artifacts>/index/*.json`), `PutEntry`, `IndexRef` (index an already-stored blob), `List` (newest first), `StaleEntries`, `Collect(olderThanMs)` (M845 collector), `Get`, `Bytes`, `Count`, `Delete` (removes meta; GCs the blob only if no other entry references the ref). |

Tests: `artifact_test.go`, `index_test.go`.

Flow:
1. Agent loop offloads tool outputs > `ArtifactThreshold` (`agent.DefaultArtifactThreshold = 8 KiB`, overridable by `AGEZT_ARTIFACT_THRESHOLD`) into the Store; the journaled `tool.result` carries a preview + `raw_ref` ([04-agent-runtime.md](04-agent-runtime.md)).
2. `cmd/agezt` `wireArtifactIndexer` watches `tool.result` events with a `raw_ref` and calls `IndexRef` (M827); channels persist inbound images via `PutEntry`.
3. `runtime.ProveTask` lists `Filter{Corr}` entries as proof evidence.

Gotchas:
- `Index.Delete`/`Collect` GC a blob when no *index entry* references it, but the journal's `tool.result.raw_ref` may still point at it → the journal can hold dangling refs after a collect. Conversely, offloaded blobs that were never indexed are never GC'd.
- `PutEntry` updates the in-memory map before writing the meta file; a meta write failure leaves an in-memory-only entry until restart.
- No events are emitted by this package.

---

## 11. `kernel/board` — shared message board / mailbox

**Purpose.** Persistent topic board + agent-to-agent mailbox (M647 topics, M788 addressed DMs + replies, M849 broadcast + help, M937 per-reader ack).

| File | What it does |
|---|---|
| `board.go` | `MaxMessages=1000` (oldest dropped), `Message{ID, Topic, From, To, ReplyTo, Text, Help, TSMS, AckedBy}`, `Everyone="*"`, `Store{mu, path, msgs}`, `Open(dir)` (`<dir>/board.json`, a JSON array), `Post`, `Send` (assigns ULID + ts, trims, caps, saves), `Broadcast` (topic `broadcast`, To `*`), `HelpRequest` (topic `help`, Help=true, To defaults to `*`), `OpenHelp` (help messages with no reply), `Get`, `Ack` (per-reader, idempotent), `Inbox(to, limit, includeAnswered)`, `Replies(id)` (oldest first), `Read(topic)`, `Topics`, `save`. |

Tests: `board_test.go`, `a2a_test.go`, `ack_test.go`, `coverage_test.go`.

**Inbox semantics:** a message is "for me" if `To` equals my slug (case-insensitive) or it is a broadcast I didn't send. Unless `includeAnswered`, drop: anything I acked; directed messages with any reply; help broadcasts with any reply; plain broadcasts that *I* replied to.

**Single shared `board.Store` invariant (M937).** Each `Store` holds the whole list in memory and saves it whole, so two writer instances silently drop each other's messages. `cmd/agezt/main.go` opens **one** store at `$AGEZT_HOME/board/board.json` and hands it to: the `board` tool (`boardtool.Tool.BindStore` via `plugins/builtintools/latebound.go`), the control plane (`boardWriter()` returns *only* the shared instance; `boardReader()` may fall back to a fresh read-only `Open`), the REST mailbox (`restapi.SetMailbox`), and self-repair's `Mailbox` interface. Read-only fresh opens exist in `controlplane.boardReader` (fallback) and `overseertool.kernelSource.OpenHelp`. `boardtool.Tool.Bind(dir)` (which opens its own store) is now exercised only by tests.

**Events.** The store emits nothing; the daemon's `boardNotify(m, corr)` closure publishes `board.posted` (actor `board`) for every write through any door, with subject routing:

| Message | Subject |
|---|---|
| Help, directed | `board.help.<slug>` |
| Help, broadcast | `board.help` |
| Broadcast (`To="*"`) | `board.broadcast` |
| Directed DM | `board.dm.<slug>` |
| Plain topic post | `board.<topic-slug>` |

Payload `{topic, chars, id?, from?, to?, reply_to?, help?}` (text is not journaled). Standing orders bind to these subjects to wake the addressed agent; `restapi/mailbox_watch.go` and the Go SDK (`sdk/mailbox.go`) stream them.

---

## 12. `kernel/workboard` — durable task state machine + proof gate

**Purpose.** Restart-safe typed task queue (distinct from the board: tasks, not messages) that agents/workflows coordinate on, with the "PROOF > VIBES" gate.

| File | What it does |
|---|---|
| `workboard.go` | `storeVersion=1`, `maxTasks=10000`; errors (`ErrNotFound`, `ErrInvalidStatus`, `ErrClaimConflict`, `ErrNotClaimed`, `ErrClaimFresh`, `ErrInvalidPolicy`, `ErrUnproven`); `Status` (triage, todo, ready, running, blocked, review, done, archived) + `statusOrder`, `ParseStatus`; `Task`, `Claim`, `Dependency`, `DependencyState`, `Attempt`, `Comment`, `Link`, `RetryPolicy{MaxAttempts, EscalateTo}`, `RetryDecision`, `CreateSpec` (incl. `AcceptanceCriteria`, `Seat`), `Filter`, `Store`, `diskState{Version, Tasks}`. |
| `workboard_create.go` | `OpenStore(dir)` (`<dir>/workboard.json`; invalid status in file = hard error), `Create` (idempotency key scoped by tenant; default status triage), `normalizeCreateSpec`, `criteriaFromText` (→ unmet `proof.Criterion`), `normalizeRetryPolicy`, `cleanStrings`. |
| `workboard_store_crud.go` | `Get`, `List` (non-archived first, priority desc, updated desc, status order, id), `Claim` (→ running, starts an Attempt, conflict on other agent), `Heartbeat`, `Comment`, `Block`, `Fail` (marks attempt failed + applies retry policy), `Unblock` (→ ready), `SetRetryPolicy`, `Complete` (**proof gate**), `Prove`. Header comment references a non-existent `workboard_helpers.go`. |
| `workboard_store_more.go` | `reconcileCriteria` (merge judged Met/Note onto declared criteria by case-insensitive text), `provenGapSummary`, `finishRunningAttempt`, `Review`, `SetSeat`, `Archive`, `Link`, `AddDependency` (cycle check via `dependsOnLocked`), `BlockingDependencies`, `ReclaimStale`, `SweepStaleClaims` (batch, all-or-nothing rollback). |
| `workboard_store_internals.go` | `mutate` (find → clone → fn → save; restore clone on fn or save error), `find`, `saveLocked`, `dependsOnLocked`, `dependencySatisfied` (done or CompletedMS>0). |
| `workboard_retry.go` | `FailedAttemptCount` (failed + stale attempts), `RetryDecisionFor`, `markFailedAttempt`, `applyRetryPolicyDecision` (retry → ready; exhausted → blocked "escalate to X"; no policy → blocked), `retryDecision`, `reclaimStaleTask` (attempt → `stale`, task → ready or blocked if exhausted), `cloneTask` (deep copy). |

Tests: `workboard_test.go`, `proof_gate_test.go`, `coverage_supp_test.go`.

**Proof gate.**
- Declaring `AcceptanceCriteria` opts a task in (default-allow: ungated tasks complete freely).
- `Complete` returns `ErrUnproven` for a criteria-bearing task unless `Proof != nil && Proof.Satisfied()` (verdict complete AND every criterion met).
- `Prove(id, actor, proof)` is the only path for gated work: reconciles criteria, attaches `Proof` (cloned), clears claim; satisfied ⇒ `done` (+`CompletedMS`, "proven complete" comment), else ⇒ `review` with `CompletedMS=0` and an `unproven: <gap>` comment.
- `runtime.Kernel.ProveTask(ctx, corr, id, answer)` (`kernel/runtime/workboard_proof.go`) builds the proof: runs the criteria judge (journals `assure.verdict` on `agent.agent-<corr>.assure`), gathers evidence (`proof.Evidence{Corr, Artifacts from artifact index, JournalFrom/JournalTo by a **full journal Range** over the correlation}`), calls `Store.Prove`, publishes `workboard.task.proved` or `workboard.task.unproven`, then `recomputeOKRForTask`. Correlation resolved by `corrOrClaim` (hint → claim run id → latest attempt run id). See [04-agent-runtime.md](04-agent-runtime.md) for `kernel/proof`/`kernel/assure`.

**Events.** Published by `runtime.Kernel.*WorkboardTask` wrappers (`kernel/runtime/workboard.go`) via `publishWorkboard`: subject `workboard.<taskID>`, actor `workboard`, payload `{id, title, status, priority, assignee, tenant, action, …extra}`. `workboard.task.dispatched` is emitted by `kernel/controlplane/workboard_dispatch_helpers.go`. Blocks, fails (action = retry/escalate/block), unblocks, retry-policy changes, completes, reviews, archives and reclaims are all `workboard.task.updated` with distinguishing `action`.

**Env.** `AGEZT_WORKBOARD_SWEEP_EVERY` (timer for `SweepStaleWorkboardClaims`; blank = on demand via `agt workboard sweep`), `AGEZT_WORKBOARD_STALE_AFTER` (default 10m).

---

## 13. `kernel/okr` — objectives & key results

| File | What it does |
|---|---|
| `doc.go` | Package doc: rollup is fed by DONE tasks, which for gated tasks means *proven*. |
| `okr.go` | Limits (`maxObjectives=5000`, `maxKeyResults=50`, `maxLinkedTasks=500`), errors, `Status` (active, achieved, archived), `KeyResult{ID, Title, Target, TaskIDs}`, `Objective`, `KeyResultProgress`, `ObjectiveProgress`, `Objective.Progress(doneOf)` (pure), `CreateSpec`, `Filter`, `Store`, `diskState`. |
| `okr_store.go` | `OpenStore` (`<dir>/okr.json`), `Create`, `Get`, `List`, `AddKeyResult`, `LinkTask` (idempotent), `UnlinkTask`, `SetStatus` (stamps/clears `AchievedMS`), `Archive`, `ObjectivesForTask` (skips archived). |
| `okr_store_internals.go` | `mutate`, `find`, `findKR`, `saveLocked`, `cloneObjective`. |

Tests: `okr_test.go`, `coverage_supp_test.go`.

**Roll-up math.** For each KR: `target = Target` or `len(TaskIDs)` when 0; `pct = min(100, done*100/target)`; achieved ⇔ `done ≥ target` (a KR with no tasks and no target is never achieved). Objective `Percent` = mean of KR percents; `Achieved` ⇔ ≥1 KR and all achieved. `doneOf` is supplied by `runtime.Kernel.taskDone` (`workboard.Get(id).Status == done`), so progress is always computed **live on read**.

**Achieved signal.** Only `Objective.Status` is cached. `runtime.recomputeOKRForTask` (called after `CompleteWorkboardTask`, `ProveTask`, and `LinkObjectiveTask`) flips active↔achieved and publishes `okr.objective.achieved` / `okr.objective.updated{action:"reopened"}` on subject `okr.<objectiveID>`. It is **not** called on archive/fail/block of a task, so the cached status can lag a regression until the next prove/complete/link touching that task.

---

## 14. `kernel/taste` — exemplar overlay

| File | What it does |
|---|---|
| `taste.go` | Limits (`maxExemplars=2000`, `maxBodyBytes=8000`), `ErrNotFound`, `Exemplar{ID, Title, Body, Scope, Tags, CreatedMS, UpdatedMS}`, `CreateSpec`, `Filter`, `Store`, `OpenStore` (`<dir>/taste.json`), `Create`, `Get`, `List`, `ForScope(scope, limit)` (global + matching-scope, scoped first, newest first), `Delete` (with explicit slice-restore rollback), helpers. |

Tests: `taste_test.go`, `taste_cov_test.go`.

- No update method (UpdatedMS == CreatedMS forever).
- Injection: `runtime/prompt_run.go` when `cfg.TasteInject` (`AGEZT_TASTE_INJECT` ≠ off) and not a system agent; `TasteTopK` default 3; scope = the run's agent slug; publishes `taste.injected {count, ids, scope}` on `agent.agent-<corr>.taste`.

---

## 15. `kernel/anomaly` — tool-call-rate circuit breaker

| File | What it does |
|---|---|
| `detector.go` | `Detector{max, window, stamps}` sliding-window counter; `NewDetector`, `Enabled`, `Observe(t) (tripped, count)` (trip when count > max). Not goroutine-safe (single consumer). |
| `monitor.go` | `Config{MaxToolCalls, Window}`; `Start(ctx, bus, cfg, onTrip)`: subscribes `>` (buf 256), feeds `tool.invoked` timestamps (journal `TSUnixMS`), on trip publishes `system.anomaly` (subject `system.anomaly`, actor `anomaly`, payload `{signal:"tool_call_rate", count, window_ms, ceiling, reason}`), calls `onTrip`, then **returns (latch)**. |

Tests: `detector_test.go`, `monitor_test.go`, `coverage_test.go`.

- Wired by `cmd/agezt/main_overlay_anomaly.go buildAnomaly`: defaults `120` calls / `10s`; `AGEZT_ANOMALY_MAX_TOOLCALLS` (0 disables), `AGEZT_ANOMALY_WINDOW`; `onTrip` → `k.HaltWith(reason)`.
- Gotchas: the watcher latches after one trip and is not re-armed after an operator `resume` (the breaker is off for the rest of the daemon's life); a `recover()` in the deferred func swallows panics and also ends the watcher; it shares the 256-slot `>` subscription with all traffic, so under heavy load dropped events undercount the rate.

---

## 16. `kernel/alerter` — channel push for warning/critical signals

| File | What it does |
|---|---|
| `doc.go` | Scope: run failures, egress blocks, budget/rate trips, halts, approvals (M922); Pulse-originated kinds deliberately excluded (Pulse delivers its own briefs). |
| `alerter.go` | `Level` (info/warning/critical) + `ParseLevel`; `Alert`; `Classify(ev)`; `doctorIncidentBits`, `intAny`; `Config{MinLevel, Cooldown 5m, MaxPerWindow 12, Window 10m, Mute pulse.QuietHours, MuteSources}` + `normalize`; `Notifier`; `New`; `Handle` (level → source mute → quiet-hours (criticals break through) → dedupe cooldown → flood cap → `sink.Deliver`); `ParseMuteSources`. |
| `alerter_brief.go` | `brief` (→ `pulse.Brief{Disposition: DispAlert, IssueKey, CorrelationID}` with ⚠/🚨 title), `dedupeKey` (kind/correlation, or doctor subject/agent/phase/fingerprint), `alertIssueKey`, `Start(ctx, bus, sink, cfg)` (subscribe `>` buf 256, recover-on-panic), payload helpers. |

Tests: `alerter_test.go`.

Classification table (mirrors the console's `frontend/src/lib/alerts.ts`):

| Event | Level | Source |
|---|---|---|
| `task.failed` | warning | run |
| `netguard.blocked` | warning | tool or egress |
| `budget.exceeded` | critical | budget |
| `rate.limited` | warning | provider |
| `halt` | critical | kernel |
| `approval.requested` | warning | approval |
| subject `doctor.auto_repair` with phase `*failed` or `routing_force_exhausted_detected` | warning | doctor |

Env (`buildAlertNotify` in `cmd/agezt/main_overlay_anomaly.go`): `AGEZT_ALERT_NOTIFY` (1/on/true/yes to enable; needs a channel sink), `_LEVEL`, `_COOLDOWN`, `_MAX`, `_MUTE` (`START-END` hours), `_MUTE_SOURCES`. Same panic-latch gotcha as anomaly: a panic in `Handle` silently ends alerting for the daemon's lifetime.

---

## 17. On-disk stores under `$AGEZT_HOME` owned by these packages

(`BaseDir` = `$AGEZT_HOME`; per-tenant copies live under `tenants/<id>/` with the same layout.)

| Path | Format | Perms | Writer | Readers |
|---|---|---|---|---|
| `journal/NNNNNNNN.jsonl` | JSONL, one hash-chained `event.Event` per line, 64 MiB segments | dir 0700 / file 0600 | `journal.Journal.Append` via `bus.Publish` only; `journal.Restore` (`agt journal import`, `agt backup restore`) | `journal.Open` (boot recovery), `Range/Tail/Verify/Why/Causes/ParentOf`, controlplane journal/runs/stats/pulse-replay folds, `reflect`, `runtime.ProveTask`, `cmd/agt` backup/export |
| `memory/memory.json` | JSON object `{id: Record}` (indented, keys sorted) | 0644 in 0755 | `memory.FileStore` via `memory.Manager` | Manager (recall/search/admin), controlplane memory handlers, contextselect |
| `worldmodel/worldmodel.json` | JSON `{entities:{id:Entity}, relations:{id:Relation}}` | 0644 / 0755 | `worldmodel.FileStore` via `Graph` | Graph, controlplane world handlers, reflect (decay), pulse relevance |
| `datalake/<coll>/_schema.json` | JSON `Schema` | 0600 / 0700 | `datalake.Lake.CreateCollection/EnsureCollection` (+ `SeedBuiltins` at boot) | `Lake` (in-memory index loaded at Open) |
| `datalake/<coll>/rec/<rec-ULID>.json` | JSON `Record` | 0600 / 0700 | `Lake.Insert/Update`; `Delete` removes | `Lake`; `plugins/tools/db`, controlplane |
| `artifacts/<aa>/<ref>` | raw bytes, name = BLAKE3 hex | 0600 / 0700 | `artifact.Store.Put` (agent offload, channels, browser/fetch/codeexec tools) | `Store.Get` (verifies hash), `Index.Bytes`, artifacts tool, console file manager |
| `artifacts/index/<art-ULID>.json` | JSON `Entry` | 0600 / 0700 | `artifact.Index.PutEntry/IndexRef`; `Delete/Collect` remove (+ orphan blob GC) | `Index` (loaded at Open), controlplane, `ProveTask` evidence |
| `board/board.json` | JSON array of `Message` (≤1000) | 0644 / 0755 | the single shared `board.Store` (daemon) | same instance; fresh read-only Opens in controlplane fallback and overseer tool |
| `workboard/workboard.json` | JSON `{version:1, tasks:[Task]}` | 0644 / 0755 | `workboard.Store` (via runtime wrappers / workboardtool / controlplane) | same; OKR rollup via `runtime.taskDone` |
| `okr/okr.json` | JSON `{version:1, objectives:[Objective]}` | 0644 / 0755 | `okr.Store` via runtime wrappers | same |
| `taste/taste.json` | JSON `{version:1, exemplars:[Exemplar]}` | 0644 / 0755 | `taste.Store` (controlplane CRUD) | runtime `ForScope` at run start |

Not persisted: bus subscriptions, memory embedding cache, anomaly/alerter windows (all in-memory). `event`, `ulid`, `filestore`, `anomaly`, `alerter` own no files.

---

## 18. Extension points

- **New event kind:** append a `Kind…` constant to `kernel/event/kinds.go` (never rename/renumber; add a doc comment with payload shape). Avoid ad-hoc `event.Kind("…")` literals. If the console or alerter should react, update `frontend/src/lib/alerts.ts` and `alerter.Classify` together; if it is a run-terminal signal, update the journal folds in controlplane.
- **New event field:** append to `Event` with `omitempty` at the end only — reordering breaks every existing hash.
- **New subscriber:** `bus.Subscribe(pattern, buf)`; drain fast, watch `Dropped`, `Cancel` on exit; wrap the loop in a panic firewall that *journals* the panic (the `KindStandingError`/`KindWorkflowPanic`/`KindSelfRepairPanic` pattern) rather than a bare `recover()`.
- **New journal-backed list endpoint:** sort DESC by `(ms, seq)` and use `journal.DecodeCursor/KeepBeforeCursor/NextCursor`.
- **New single-file store:** `filestore.LoadFrom(dir, "<name>.json", &state)` in `Open`, own `sync.Mutex`, `filestore.Save` under the lock, mutate-with-rollback like `workboard.mutate`; open it once in `runtime.Open` (`kernel/runtime/compose.go`) and pass that instance everywhere. Publish events from the runtime/daemon wrapper with a domain subject (`<domain>.<id>`).
- **Swap the memory/world backend:** implement `memory.Store` / `worldmodel.Store` (both interfaces exist for a CobaltDB-class engine, DECISIONS D2).
- **Provider embeddings:** implement `memory.Embedder` and set `runtime.Config.MemoryEmbedder` (or `Manager.SetEmbedder` live).
- **New memory maintenance pass:** reuse `Remember`/`Forget`/`supersedeExisting` so every mutation stays journaled and soft; only `Prune`/`CleanLowValue` may hard-delete.
- **New data-lake built-in:** add a `Schema` to `BuiltinSchemas()` (seeded idempotently at next boot).
- **New anomaly signal:** another `Detector` fed from the same subscription in `anomaly.Start`.

---

## 19. Gotchas / invariants

1. **Durable-before-publish:** no subscriber may ever see a durable event that is not in the chain; `publish_error_test.go` guards it. Ephemeral events (`Hash==""`) are the only exception and must never be needed for audit/replay.
2. **Global publish serialization:** `Bus.mu` is held across the fsync; any slow disk or very slow subscriber channel send (sends are non-blocking, so only the disk) throttles the entire daemon.
3. **Boot cost and boot failure:** `journal.Open` re-verifies every event hash on every start; a single corrupt line mid-journal is `ErrChainBreak` and aborts `runtime.Open`. Only a torn *final* line is auto-repaired.
4. **No purge path.** The journal is append-only with no redaction-after-the-fact; the bus redactor must be installed before anything sensitive is published (`cmd/agezt/main.go` calls `SetRedactor` after `runtime.Open` and tool configuration, "before any Run"; anything a boot step publishes earlier is journaled unscrubbed). Hence the 0600/0700 hardening (EXPOSE-001).
5. **`journal/doc.go` is stale** (claims a sidecar offset index). Every read is a sequential scan; `agt why` costs three full scans; `ProveTask` evidence gathering and `ParentOf` are full scans too.
6. **Single-writer per store file.** Board is the documented case (M937), but the same hazard applies to every filestore store; they are safe only because `runtime.Open` opens each once.
7. **Memory identity is content-addressed**, so "updating" a fact means a new record; the old one stays active unless explicitly superseded. Consequence for the operator profile: when a facet's synthesized content changes, `DistillProfile` creates a *second* active record on the same `operator profile: <facet>` subject (nothing supersedes the old one) and `ProfileText` injects both until consolidation/dedupe merges them. `TestDistillProfile_ReinforcesNotDuplicatesOnRerun` only covers identical content.
8. **Distill dedupe gate is opportunistic-only:** explicit `memory` tool writes and curated writes are never collapsed; only `source=distill` notes.
9. **Retention filter runs on agent/distill writes** — an agent `remember` can be refused with "low-value record rejected"; operator/control-plane writes set `Force`.
10. **Workboard events come from the runtime, not the store** — calling `workboard.Store` methods directly (e.g. from a tool holding the store) bypasses journaling and OKR recompute. `workboardtool` and controlplane go through the kernel wrappers.
11. **Artifact GC vs journal refs** can leave dangling `raw_ref`s (see §10).
12. **anomaly/alerter watchers die silently** on panic or after one anomaly trip (see §15/§16).
13. ✅ **Fixed (W1.3):** store files were 0644 in 0755 dirs, looser than the journal (0600/0700) though memory.json holds distilled conversation content. `filestore` now writes 0600/0700 and tightens existing installs.
14. **Doc-comment drift from god-file splits:** `Supersede`'s comment straddles `manager_hygiene.go`/`manager_recall.go`; `Distill`'s comment sits at the end of `manager_tool_helpers.go`; `datalake.Insert`'s comment starts mid-sentence; `workboard_store_crud.go` names a nonexistent `workboard_helpers.go`.
