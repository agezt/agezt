# Agezt — Memory, World Model, Skills & Forge Specification (SPEC-05)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT · Language: English
> Depends on: SPEC-01, SPEC-02, SPEC-04 (Memory interface)
> Defines the knowledge substrate: memory tiers, the world model (context graph), the skill system, the Forge (self-improvement), and the reflection loop. This is what makes Agezt *yours* and what beats Hermes's Curator.

---

## 0. Why this is a competitive moat

Hermes's edge is its closed learning loop: `MEMORY.md` + FTS5 session search + Honcho user-modeling + a Curator that grades/consolidates skills on a 7-day cron. The weaknesses: it's markdown-based, audit is weak, mutations aren't truly versioned or revertible, and the "model of you" is shallow.

Agezt matches the loop and beats it on **auditability and reversibility**: every memory and skill mutation is a journaled, hash-chained, content-addressed event. You can ask *why* it knows something, *when* it learned it, and *undo* it. Plus a real **world model** (graph), not just a user model.

---

## 1. Memory tiers

Four tiers, each with a clear role and retention:

| Tier | Holds | Lifetime | Backend |
|---|---|---|---|
| **Working** | current task context, scratchpad | task lifetime | in-memory projection (journal-backed) |
| **Episodic** | session transcripts, what happened when | long, compacted | journal + FTS index |
| **Semantic** | distilled facts, summaries, embeddings | durable | Memory plugin (embedded / Flint Vector) |
| **World model** | entities, relations, preferences (graph) | durable, evolving | graph store + vectors |

- **Working** is reconstructed by folding the task's events; never the source of truth on its own.
- **Episodic** is the raw history (full-text searchable, the basis for "what did we decide about X").
- **Semantic** is the compressed, retrievable knowledge layer (RAG).
- **World model** is the structured "who/what/relations" graph (below).

Promotion flows upward: working → episodic (always) → semantic (distilled on completion or by reflection) → world model (when a durable entity/relation/preference is recognized).

---

## 2. Memory record model

```proto
message MemoryRecord {
  string id            = 1;   // content-addressed (BLAKE3 of canonical content)
  MemoryType type      = 2;   // FACT | SUMMARY | RELATION | PREFERENCE | SKILL_REF | OBSERVATION
  string subject       = 3;   // entity/topic this is about
  string content       = 4;   // the text
  repeated float embedding = 5; // optional vector
  repeated string tags = 6;
  string source_event  = 7;   // provenance: the journal event that produced it
  float  confidence    = 8;   // 0..1; decays/strengthens over time
  int64  created_ms    = 9;
  int64  last_seen_ms  = 10;  // recency for ranking/decay
  string superseded_by = 11;  // if updated, points to newer record (no destructive edit)
  bool   tombstoned    = 12;  // forgotten (soft)
}
```

Key properties:
- **Content-addressed** → identical knowledge dedupes; versions are explicit via `superseded_by`.
- **Provenance** (`source_event`) → `agt why` explains every belief.
- **Confidence + recency** → ranking and decay; stale/contradicted facts fade rather than lie.
- **Soft updates/forgets** → no destructive mutation; history preserved.

---

## 3. The world model (context graph)

### 3.1 Purpose
A structured, evolving graph of Ersin's world so the system understands references ("the portfolio", "Trabzonspor", "the repos", "my Claude plan") and so **Salience** can judge relevance/novelty *to you specifically*.

### 3.2 Graph shape
- **Nodes (entities):** projects, repositories, people, organizations, accounts/subscriptions, devices, channels, topics, recurring tasks.
- **Edges (relations):** `owns`, `depends_on`, `member_of`, `prefers`, `relates_to`, `assigned_to`, `derived_from`, with weights and timestamps.
- **Attributes:** preferences ("brief me in the morning, terse"), habits (learned), constraints (budgets, quiet hours).

### 3.3 Construction & maintenance
- **Bootstrapped** from explicit config + an onboarding interview (optional) + ingestion of existing memory.
- **Grown** continuously: observer deltas (`Before`/`After`) and task outcomes add/strengthen nodes/edges.
- **Pruned/decayed** by the reflection loop (unused entities lose weight).
- Every change is a journaled event → the graph is reconstructable and revertible; you can diff the world model across time.

### 3.4 Queries the rest of the system asks it
- Planner: "what does 'the portfolio' resolve to?" → list of repo entities.
- Salience: "is this delta about an active project? have we surfaced it before? does Ersin care about this category?"
- Briefing: "what tone/cadence does Ersin prefer?"
- Initiative: "is acting here within learned preferences?"

### 3.5 Beyond Honcho
Honcho models *the user*. The world model models *the user's entire operational world* — entities, their relations, and the user's stance toward them — which is what proactive judgment actually requires.

---

## 4. Skills system

### 4.1 What a skill is
A reusable, named procedure the agent can load to handle a class of task — analogous to OpenClaw SOUL.md skills and Hermes skill docs, but versioned and journaled.

```
skill:
  id: content-addressed
  name: "diagnose-failing-ci"
  description: "..."           # used for retrieval/activation matching
  triggers: [tags, conditions] # when it's relevant
  body: |                      # instructions / steps / sub-DAG template
    ...
  tools_required: [shell, http]
  version: semver
  lineage: [parent_skill_ids]  # what it evolved from
  status: draft|shadow|active|quarantined|archived
  metrics: {uses, successes, failures, last_used}
```

### 4.2 Activation
On a task, relevant skills are retrieved (by description/trigger match against the intent + world-model context) and injected into the planning/loop context — exactly the OpenClaw/Hermes pattern, but retrieval is hybrid (semantic + tag + condition) and every activation is logged.

### 4.3 Storage
Skills are memory records (`SKILL_REF`) + content-addressed bodies in Storage. Versions are explicit; nothing is overwritten.

---

## 5. Forge — auditable self-improvement (Curator-killer)

### 5.1 Trigger
After a complex task (heuristic: ≥N tool calls, or a novel solved problem, or an explicit correction from Ersin), Forge proposes creating or patching a skill — same instinct as Hermes, but the result goes through a **state machine**, not straight to "active markdown."

### 5.2 Skill lifecycle state machine
```
draft ──(shadow-test passes)──▶ shadow ──(N successful real uses, gated)──▶ active
  │                                │                                          │
  │                                └──(regression)──▶ quarantined ◀───────────┘
  └──(fails review)──▶ archived         │
                                        └──(revert)──▶ previous active version
```
- **draft** — newly authored by Forge; not used in production.
- **shadow** — runs alongside real execution without affecting outcomes; its proposed actions are compared to what actually happened. Only promoted if it would have helped.
- **active** — in the retrieval pool.
- **quarantined** — a regression or repeated failure pulls it out automatically (Pulse surfaces this).
- **archived** — retired; retained for lineage/audit.
Every transition emits an event (`EVT_SKILL_CREATED|PATCHED|PROMOTED|QUARANTINED|REVERTED`).

### 5.3 Why it beats the Curator
- **Shadow-testing before promotion** → bad skills never reach production silently.
- **Every mutation journaled + content-addressed** → full version history; `agt skill history <id>`.
- **Reversible** → `agt skill revert <id>` appends a reversal; never edits history.
- **Justified** → each create/patch carries the reasoning (an `llm-node` output) and the metrics that justified it.
Hermes's Curator consolidates/prunes on a cron over markdown with weak audit; Forge is a governed, observable, reversible pipeline.

### 5.4 Consolidation & pruning
A periodic Forge pass (configurable, not a fixed 7 days) reviews overlapping skills, merges duplicates (creating a new version with lineage), and archives stale ones — all as journaled transitions. Pinned skills are protected.

---

## 6. Reflection loop (meta-cognition)

### 6.1 Purpose
Periodically (daily/weekly/after notable events) the system evaluates **its own behavior** and recalibrates — a level above Forge (which improves *skills*; reflection improves *judgment*).

### 6.2 What it examines (all from the journal)
- Task outcomes: success/failure rates by type; where it got stuck or looped.
- Predictions vs reality: did flagged-as-important things turn out important?
- User feedback: what Ersin approved, dismissed, snoozed, corrected, deleted.
- Cost/efficiency: budget patterns, redundant work.

### 6.3 What it adjusts
- **Salience thresholds & category weights** ("morning briefs keep getting deleted → lower their salience / change cadence").
- **Initiative aggressiveness** (within the trust ladder — it can become more cautious on its own, never more permissive than the ladder allows).
- **World-model weights** (decay unused entities, strengthen active ones).
- Proposes (never silently applies) trust-ladder changes for Ersin to approve.

### 6.4 Output
A reflection report (journaled) and a set of proposed adjustments. Auto-applies only what's within safe bounds (e.g. cadence tuning); anything affecting autonomy is proposed for approval. `agt reflect show` views the latest.

---

## 7. Retrieval pipeline (how memory is actually used at runtime)

When the Planner or a `loop-node` needs context:
```
1. Parse intent → extract entities/topics (resolve via world model).
2. Hybrid query Memory: semantic (vector) + keyword (FTS) + graph neighborhood.
3. Rank by relevance × confidence × recency; apply tombstone filter.
4. Retrieve matching active skills.
5. Assemble context within token budget (ContextCompressor, SPEC-02): protect recent + injected facts/skills; summarize the rest.
6. Record what was retrieved (provenance) so the result is explainable.
```

---

## 8. Privacy & data handling

- **PII redaction** before sending context to external providers (configurable). Local providers may receive unredacted per policy.
- **Local-first option:** with a local provider + embedded storage + local embeddings, no data leaves the machine.
- **Per-entity sensitivity:** world-model nodes can be marked sensitive; Edict restricts which providers/tools may see them.
- **Right to forget:** tombstone + optional hard-prune (operator-initiated, journaled) for compliance.

---

## 9. Storage mapping

| Concept | Default (embedded) | Pluggable |
|---|---|---|
| Episodic transcripts | JSONL journal + embedded FTS | Postgres |
| Semantic facts/embeddings | embedded vector index | Flint Vector |
| World-model graph | embedded graph (adjacency in KV) | graph DB / Postgres |
| Skill bodies | content-addressed blobs | object store |
| Cache | in-memory | Redis |

All accessed through the Memory/Storage plugin contracts (SPEC-04 §5–6); the kernel doesn't hardcode any backend.

---

## 10. Open questions

1. Graph store: embed a minimal property-graph in CobaltDB, or require a graph driver for the world model at scale?
2. Embedding model routing: local (fast, private) default with optional provider embeddings — how does Governor budget embeddings vs completions?
3. Shadow-test fidelity: how to fairly compare a shadow skill's hypothetical actions to reality without side effects?
4. Confidence decay function: linear, exponential, or evidence-based Bayesian update?
5. World-model bootstrap: how much onboarding is worth it vs purely emergent growth?

---

*Next: SPEC-06 (Security, Sandbox & Warden) — isolation profiles, Edict in depth, threat model, secret handling. Then SPEC-07 (UI & Surfaces) — Flow Studio, Inbox, Monitor, Memory Explorer, ambient surfaces.*
