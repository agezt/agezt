# Agezt — LLM, Context & Routing Specification (SPEC-10)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-02 (Governor), SPEC-04 (Provider), SPEC-05 (Memory)
> Defines how Agezt extracts maximum value from LLMs: model routing, leveraging modern capabilities, and — critically — intelligent context management. "Use the LLM's power fully" means *put the LLM where it's strongest and let the deterministic skeleton carry it where it's weak* — never hand it blind control.

---

## 0. Principle

LLMs are extraordinary at reasoning, language, planning, synthesis, judgment — and weak at consistency, determinism, numeric precision, and not-repeating-themselves. Agezt's hybrid DAG + LLM-loop exists to exploit the strengths and contain the weaknesses. "Full leverage" is three things: **right model for the job (routing)**, **modern capabilities used to the hilt**, and **the right context, not all the context**.

---

## 1. Model routing (Governor extension)

Not all LLM power comes from one model. Routing picks the best model per call:

- **By task type:** cheap/fast model (e.g. local or small) for salience scoring, classification, simple summarization, routing decisions; strong model for hard planning, complex reasoning, code; vision model when images are involved. (Mirrors the cheap-router-LLM pattern from the football work.)
- **By cost/quality trade-off:** Governor orders candidates subscription-first → cost → latency (SPEC-02 §6.2), now with a **quality floor per task type** so it never routes a hard task to a too-weak model just because it's cheaper.
- **By capability requirement:** the call declares needs (modalities, context size, tool-use, JSON mode); only providers advertising those are eligible.
- **Escalation on uncertainty:** a borderline result (low confidence, failed validation) can re-route to a stronger model — bounded, journaled.
- **Local floor:** a local provider (Ollama/vLLM) is always an eligible fallback so the system degrades gracefully rather than stalling when paid quota is exhausted (SPEC-04 §2.4).

Every routing decision is journaled (which model, why, cost) → visible in Live Monitor and `agt why`.

---

## 2. Leveraging modern LLM capabilities (first-class, not afterthoughts)

Provider plugins advertise capability attributes; the kernel uses each to the fullest:

- **Tool/function calling** — the core of agentic action; structured tool-call deltas stream through `loop-node`s.
- **Vision** — browser screenshots, document/image understanding, chart reading. Routed to vision-capable models automatically.
- **Structured output / JSON mode** — used wherever the system must reliably parse the LLM (plan generation, salience scores, classifications). Reliability over free-form parsing.
- **Prompt caching** — cache stable prefixes (system prompt, active skills, retrieved facts) on supporting providers; large cost/latency win. Governor accounts for cache hits.
- **Extended thinking / reasoning modes** — engaged for hard planning/complex problems where the deeper reasoning budget pays off; not wasted on trivial calls.
- **Parallel tool calls** — multiple independent tool calls in one turn map to the scheduler's bounded-parallel execution.
- **Streaming** — token-by-token everywhere (CLI, channels, UI) for responsiveness.

Capabilities are negotiated, not assumed: if a provider lacks one (e.g. no JSON mode), the kernel falls back to a robust parsing strategy and records the degradation.

---

## 3. Intelligent context management (the heart of cost & quality)

The context window is scarce and expensive; filling it blindly inflates cost and *degrades* quality ("lost in the middle"). For every LLM call, Agezt assembles the highest value-per-token context for that specific task.

### 3.1 Context budgeting
Each call gets a token budget tied to the routed model and Governor's cost ceiling. Over budget → compress; under budget → enrich. The budget split across context sections is explicit.

### 3.2 Tiered assembly
Decide *what* goes in, by priority and budget share:
1. System prompt / identity.
2. Active, relevant skills (retrieved, SPEC-05 §4).
3. Retrieved memory: semantic + keyword + graph neighborhood, ranked by relevance × confidence × recency.
4. Recent N turns (conversation continuity).
5. Task-specific artifacts/inputs.
Each section has a priority and a budget cap; low-value items are dropped before high-value ones.

### 3.3 Compression / summarization
When near the limit: protect the first few and last few turns, summarize the middle via a cheap model call, and sanitize orphaned tool-call/result pairs. **Differentiator:** every compression is journaled — what was dropped or summarized is recorded and recoverable, unlike Hermes's in-place compressor. You can ask "what did it forget here?" and get it back.

### 3.4 Relevance scoring
World model + embeddings score "how relevant is this to the current task"; low-relevance context is excluded even if it fits. Prevents noise from crowding out signal.

### 3.5 Context observability (ties to the conversation/debug UI)
Every LLM call records **exactly what was sent**: which tokens came from which source (system/skill/memory/turn/artifact), what was compressed, the total token count, and the cost. This is journaled and surfaced in the UI's **context inspector** (SPEC-07). Most "why did it answer that?" questions are really "what was in its context?" — now answerable.

---

## 4. The conversation/reasoning record

- Every LLM request/response is journaled (`EVT_LLM_REQUEST/RESPONSE`, tokens via `EVT_LLM_TOKEN`), including the assembled context reference and the routing/cost decision.
- This makes reasoning **reproducible** (replay uses recorded outputs), **explainable** (`agt why`), and **analyzable** (reflection loop studies which context/model choices led to good vs bad outcomes).

---

## 5. Self-improvement leverage (LLM improves the system itself)

The highest form of "using the LLM fully": the LLM doesn't just do tasks, it improves Agezt.
- **Planner** detects missing capabilities and proposes plugins/skills.
- **Forge** authors and patches skills (governed, shadow-tested, reversible — SPEC-05 §5).
- **Reflection** recalibrates judgment (SPEC-05 §6).
- **Coding delegation:** the system can have a coding-agent *write a new plugin's code* for a missing capability, shadow-test, and enlist it (SPEC-04 §4, SPEC-13). A system that grows its own army.

All within governance: self-generated capabilities enter via the same trust ladder, shadow-test, and approval paths — power without loss of control.

---

## 6. Cost & FinOps observability

Beyond Governor's budgeting:
- Per-task, per-agent, per-standing-order cost attribution (events carry cost).
- Trends over time (this week vs last), most-expensive agents/skills, projected monthly burn for standing orders.
- Surfaced in Live Monitor (SPEC-07); anomalies (spend spike) feed the auto-halt detectors (SPEC-06).
An autonomous system can quietly burn money; this makes spend visible, attributable, and bounded.

---

## 7. Phase placement

- Governor routing v1 + local fallback: **Phase 1**.
- Context budgeting + tiered assembly + compression: **Phase 1–2** (needed as soon as loops run).
- Capability leverage (vision/JSON/caching/reasoning): **Phase 1–3** as providers land.
- Context observability UI: **Phase 5** (with the conversation surface).
- Self-improvement leverage: **Phase 2 (Forge), 6 (coding), 8 (reflection)**.
- FinOps views: **Phase 5**.

---

## 8. Open questions

1. Borderline-escalation policy: confidence threshold + which validator decides a re-route.
2. Summarization model choice and how to measure summary fidelity loss.
3. Context budget defaults per task type — tune empirically.
4. Embedding routing/budget: local-default vs provider embeddings, accounted separately from completions.
5. How aggressively prompt-cache across tasks without leaking stale context.

---

*Next: SPEC-11 (Deployment & Runtime Environments — Docker, sandbox images, GHCR CI/CD, multi-arch).*
