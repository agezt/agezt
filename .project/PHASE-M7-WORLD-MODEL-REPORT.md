# Phase Report — Milestone M7 (World Model v1)

> Status: **shipped** · Date: 2026-05-30
> Phase 2 (ROADMAP product-M2 "Memory & self-improvement"), slice 1 of 3.
> Adds the journaled, content-addressed entity/relation graph of the
> operator's world — the substrate the retrieval pipeline resolves
> references against (SPEC-05 §7) and the relevance signal Pulse's
> Salience was built to consume (the hole `salience.go:21` left for
> "the full world-model relevance signals land with Memory").

## Scope

v0.1.0 shipped memory-lite — a flat fact store. But the system still
couldn't answer "what does *'the portfolio'* mean?" or judge whether a
CI failure was about a project Ersin actually cares about. SPEC-05 §7
fixes the order: the retrieval pipeline **resolves entities via the
world model first**, then retrieves skills. So Phase 2 starts here — the
graph the later slices (Forge activation, reflection decay) stand on.

**MVP cut (DECISIONS D2: "adjacency lists … graph-DB driver optional
later"):** a **file-backed** graph store mirroring the memory-lite
`FileStore` cut (single `worldmodel.json`, atomic write) — *not* the
CobaltDB adjacency backend yet, consistent with how M3 deferred the
spec's embedded KV. Entities + weighted relations, content-addressed,
soft update/forget, resolve + neighborhood queries.

Three consumers, all wired:
- **Operator** — `agt world add|relate|resolve|neighbors|list|show`.
- **Agent** — an in-process `world` tool (add/relate/resolve/neighbors)
  and per-run **entity injection** into the system prompt.
- **Pulse** — a **salience relevance boost** so a delta about a known
  active entity is nudged up a band (SPEC-05 §3.4).

## What shipped

### New `kernel/worldmodel`
Mirrors kernel/memory's two-layer split seam-for-seam:
- **Store** (`worldmodel.go`) — pure, file-backed. `Entity` (content-
  addressed `BLAKE3("entity"\0kind\0name)`, kind/name/aliases/attrs/
  weight, soft `SupersededBy`/`Tombstoned`) and `Relation`
  (`BLAKE3("rel"\0from\0verb\0to)`, directed/weighted). `Kind`/`Verb`
  are open vocabularies — a well-known set validated, unknown values
  kept verbatim (the graph records the operator's words, never refuses
  to learn). Single `worldmodel.json`, atomic write + deterministic
  sort.
- **Resolve** (`resolve.go`) — pure ranking: exact name > exact alias >
  token overlap, weighted by entity weight × recency, tombstone-filtered
  (mirrors `memory.Search`). Plus `Neighbors` (incident active edges +
  the adjacent entity, deterministically ordered).
- **Graph** (`manager.go`) — the journaling boundary: wraps the Store
  with the kernel bus so every mutation is durable-before-publish under
  the run's correlation. `Upsert` (content-address dedupe → reinforce/
  revive, merge aliases/attrs, nudge weight), `Relate` (auto-creates
  unknown endpoints as topics so an edge never dangles), `Resolve`
  (journals `worldmodel.retrieved` for `agt why`), `ResolveQuiet`,
  `Neighbors`, `Forget` (tombstone entity *or* relation), the `world`
  agent tool, and `IsActiveSubject` — the pulse relevance adapter.

### Event kinds (append-only)
`worldmodel.entity.upserted`, `worldmodel.relation.upserted`,
`worldmodel.retrieved`, `worldmodel.forgotten`, `worldmodel.superseded`
— added to both the const block and `knownKinds`.

### Runtime wiring (`kernel/runtime/runtime.go`)
The graph is a **first-class kernel resident**, opened at
`BaseDir/worldmodel` in `Open` exactly like memory (not daemon-built).
Config knobs `WorldInject`/`WorldTopK`/`WorldTool` (all default OFF);
`World()` accessor; `injectWorld` prepends a compact "Known entities"
block to the system prompt next to `injectMemory`, journaling
`worldmodel.retrieved` so resolved references are explainable.

### Control plane + CLI
`kernel/controlplane/world.go` handlers (`CmdWorld{Add,Relate,Resolve,
Neighbors,List,Get}`) + `cmd/agt/world.go` (`--json` throughout,
reusing `dial`/`encodeJSON`). `agt status` now reports `world_entities`.

### Pulse salience relevance (`kernel/pulse/salience.go`)
A `Relevance` **interface** (`IsActiveSubject(text) (name, bool)`) —
defined *inside* pulse so the package keeps no dependency on
kernel/worldmodel (same decoupling as `PulseController`/`SinkFunc`). A
bounded `+0.15` boost lifts a delta about a known active entity a band
and re-derives its disposition; nil `Relevance` = v1 behaviour
unchanged. The daemon passes `k.World()` (which satisfies the interface)
into the Pulse `Config`.

### Daemon wiring (`cmd/agezt/main.go`)
`worldOn` env toggle (`AGEZT_WORLDMODEL=off` disables per-run inject/
tool; store + CLI stay live), `cfg.World*` set alongside `memOn`,
`Relevance: k.World()` threaded into `buildPulse`, and a `knowledge`
banner line.

| Env var | Meaning |
|---|---|
| `AGEZT_WORLDMODEL` | `off` disables per-run entity injection + the `world` tool (store/CLI unaffected) |

## Design rules followed

- **No new external dependency** — stdlib + the existing BLAKE3; `go.mod`
  unchanged (POLICY).
- **Mirrors memory-lite seam-for-seam** — Store/Manager split, atomic
  write, content-addressing, journaling boundary, config-knob gating,
  control-plane↔CLI pairing. Nothing novel where a pattern existed.
- **Decoupling by interface** — controlplane has no compile dependency
  on worldmodel internals beyond the kernel accessor; pulse depends on
  its own `Relevance` interface, not the graph package.
- **Everything journaled & reversible** — entity/relation mutations are
  content-addressed events; updates are soft (`SupersededBy`), forgets
  are soft (`Tombstoned`); `agt why` explains every belief.

## Test coverage

~30 new tests; `go test ./...` green on host (windows) + `GOOS=linux`
cross-compile; `go vet` clean. Package count 42 → 44 (added
`kernel/worldmodel`; the rest extend existing packages).

- `kernel/worldmodel`: id normalization/disjointness; store round-trip +
  persistence + validation + deterministic sort; resolve exact/alias/
  token ranking, weight×recency, tombstone exclusion, empty phrase;
  neighbors (outgoing/incoming, tombstone-skip); manager dedupe/
  reinforce/revive, alias merge, relate auto-endpoint + journaling,
  resolve-journals-only-on-hit, `IsActiveSubject`, nil-bus store-only,
  `world` tool round-trip.
- `kernel/runtime`: entity injection into the system prompt + provenance
  event; injection off by default; `world` tool registered only when
  enabled.
- `kernel/controlplane`: add→resolve→relate→neighbors cycle; empty-name
  rejection; empty-list shape.
- `kernel/pulse`: relevance boost lifts a band; no boost when irrelevant;
  issue-key match; bounded/clamped + nil-safe (fake `Relevance`).
- `cmd/agt`: world help / arg-validation exit codes.

### Manual end-to-end (mock provider)
- `agt world add Lictor --kind project --alias portfolio --alias "the
  repos"` → `worldmodel.entity.upserted` journaled (content-addressed id
  + `source_event` provenance). `agt world resolve "the portfolio"` →
  resolves to Lictor (score 3.0, alias hit). `agt world relate Lictor
  depends_on go-stdlib` auto-created the `go-stdlib` topic; `agt world
  neighbors Lictor` listed the edge.
- A run "give me a status update on the portfolio" emitted
  `worldmodel.retrieved` (phrase→Lictor) under the run's correlation;
  `agt why` reconstructed `worldmodel.retrieved → task.received → llm ×2
  → tool → task.completed` as one arc.
- On-disk `worldmodel.json` confirms content-addressed ids, preserved
  aliases, provenance, the auto-topic, and the relation.

The Pulse salience boost is proven by unit tests + the one-line
`Relevance: k.World()` wiring (the daemon banner confirms "world model
on"); a live Pulse-probe boost run exercises the identical
`Score()`/`IsActiveSubject` path and is documented for an operator.

## Deferred (named for later → future Phase 2 slices)

- **Embedding/vector resolve** (v1 is keyword/alias/token only).
- **Automatic graph growth** from every observer delta & task outcome
  (v1 grows via the `world` tool + explicit ops).
- **Reflection-driven decay/pruning** of unused entities (slice 3).
- **Onboarding bootstrap**, world-model **diffing across time**,
  **per-entity sensitivity** for Edict, and the **CobaltDB adjacency
  backend** (DECISIONS D2 at scale).

## Closes / next

The world model now grounds references and feeds relevance into Pulse —
the foundation SPEC-05 §7 puts beneath skill retrieval. The four
in-process residents (agent loop, memory, pulse, channel) plus the world
model and the operator CLI are the stable base for the next slices:
**Forge v1** (skill lifecycle, shadow-test, revert — now able to resolve
entities for activation) then **Reflection v1**.
