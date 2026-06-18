# Agezt — Council Hardening Integration Plan

> Status: Planned overlay · Scope: post-MVP hardening of the existing Agezt architecture
> Depends on: DECISIONS.md, SPEC-02, SPEC-10, SPEC-14, SPEC-15, SPEC-16

This plan folds the council audit into the build without resetting the roadmap. The base
kernel still ships as the first-party single-agent tool loop plus the DAG/workflow layer
from DECISIONS B0d. The items below become explicit hardening workstreams with stable task
IDs, phase placement, and acceptance criteria.

---

## 1. Priority order

| ID | Workstream | Primary spec | Phase | Why first |
|---|---|---|---|---|
| CH-01 | Tool-call boundary validator | SPEC-15 | Phase 1 | Reject malformed or hallucinated tool args before policy/invoke. |
| CH-02 | Provider constrained generation | SPEC-15 | Phase 1-2 | Prefer invalid-call prevention where provider/local backend supports it. |
| CH-03 | Semantic tool discovery | SPEC-10 | Phase 2 | Keep large tool catalogs out of context; retrieve the relevant tools. |
| CH-04 | Differential observation layer | SPEC-10 | Phase 2-3 | Feed agents state deltas, not noisy dumps. |
| CH-05 | Deterministic pipeline primitive | SPEC-02 | Phase 5-6 | Run well-typed tool chains without LLM round-trips between steps. |
| CH-06 | Plan invariant monitor | SPEC-02 | Phase 5-6 | Invalidate committed plans when assumptions/world/resource constraints break. |
| CH-07 | Effect model + HITL bundles | SPEC-14 | Phase 6 | Route reversible, compensable, and irreversible actions correctly. |
| CH-08 | Stochastic eval replay | SPEC-14/16 | Phase 5-8 | Test agent behavior with expectation bands, not brittle point equality. |
| CH-09 | Heuristic bypass + tool memoization | SPEC-02/10 | Phase 2-5 | Avoid LLM calls for known-safe deterministic subproblems. |

---

## 2. Architecture integration

### CH-01 Tool-call boundary validator

Every tool invocation crosses one mandatory validation boundary:

1. Validate tool name exists in the effective run tool map.
2. Validate `input_json` against the registered JSON Schema.
3. Enforce strict unknown-field behavior unless the schema explicitly permits extras.
4. Normalize validated input before Edict classification and tool invocation.
5. Journal validation failures as structured tool-call rejection events.

Registration also lints schemas so broken plugin/tool schemas fail early instead of
failing during a user run.

### CH-02 Provider constrained generation

Provider/model capabilities gain a constrained-generation flag family:

- `json_mode`
- `tool_use_native`
- `schema_constrained_decoding`
- `grammar_constrained_decoding`
- `strict_tool_args`

Routing prefers providers that can prevent invalid tool calls for requests that require
strictness. Where APIs do not support constrained decoding, CH-01 is the required fallback
and the degradation is journaled.

### CH-03 Semantic tool discovery

Tool descriptions, schemas, capability labels, examples, and observed success metadata are
indexed in the same spirit as memory retrieval. The planner and loop receive a relevant
tool subset instead of the whole catalog when the catalog is large.

Acceptance requires deterministic fallback: if embeddings are unavailable, keyword and
capability filtering still produce a bounded, auditable candidate set.

### CH-04 Differential observation layer

Tool results and observer outputs should provide:

- `before_ref` when previous state exists
- `after_ref` or compact value reference
- `delta` for file/API/world/model changes
- `summary` for context insertion
- `raw_ref` for full recoverability outside the prompt

The loop context gets the summary/delta; the journal keeps recoverable raw artifacts.

### CH-05 Deterministic pipeline primitive

A pipeline is a validated subgraph that executes without LLM round-trips between internal
steps. It is useful for solved chains such as fetch -> parse -> extract -> summarize-input
where every edge has a schema contract.

Pipeline validation must prove:

- every step has a registered tool or deterministic transform
- each output edge satisfies the next input schema
- failure, retry, timeout, and compensation policy are declared
- Edict still evaluates every effectful step

### CH-06 Plan invariant monitor

The monitor is separate from the planner. It watches committed assumptions during plan or
workflow execution, including:

- resource budgets and deadlines
- world-model facts used by the plan
- file/resource versions
- user approval scope and trust level
- external state freshness for APIs or repos

If an invariant breaks, the monitor emits a plan invalidation event and routes to
minimal re-plan, compensation, HITL, or fail-fast according to policy.

### CH-07 Effect model + HITL bundles

Actions gain explicit effect metadata:

- `read_only`
- `reversible`
- `compensable`
- `irreversible`

Compensability is declared by the tool/action and verified by the planner where possible.
Irreversible or high-blast-radius bundles must present the human with related actions,
predicted effects, confidence, and rollback/compensation notes.

### CH-08 Stochastic eval replay

Replay has two modes:

- event replay: recorded outputs are replayed exactly to rebuild state
- behavioral re-run: the same scenario is re-executed with model, seed when supported,
  context snapshot, tool mocks, and expectation bands

Temperature greater than zero must not use byte equality as the behavioral oracle. Eval
assertions should combine schema validity, semantic checks, cost/latency bounds, and
task-specific scoring.

### CH-09 Heuristic bypass + tool memoization

Before invoking the LLM, a fast-path layer may route known-safe intents to deterministic
handlers. Examples: current time, simple file reads, exact run status, cached provider
catalog lookups, and repeated deterministic tool calls with identical inputs.

Bypass rules must be explicit, testable, policy-aware, and journaled. Memoization must
respect tool effect class, TTL, user/tenant scope, and secret redaction.

---

## 3. Delivery rules

- Do not block the MVP core on all nine items.
- CH-01 and CH-02 are the only Phase 1 hardening items because tool-call integrity is a
  boundary concern.
- CH-03, CH-04, and CH-09 can land incrementally as context/tool catalogs grow.
- CH-05, CH-06, and CH-07 should land with Flow Studio, workflow reliability, and
  saga/compensation work.
- CH-08 starts as a deterministic mock-provider harness, then grows into stochastic
  expectation-band evaluation.

---

## 4. Non-goals for this overlay

- Do not replace the first-party loop with a third-party agent framework.
- Do not make every task a large workflow; fast-path and single-loop execution remain
  valid.
- Do not require provider-level constrained decoding where the provider API cannot offer
  it; enforce strict validation fallback instead.
- Do not treat replayed recorded outputs and behavioral re-runs as the same test class.
