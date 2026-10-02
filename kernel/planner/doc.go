// SPDX-License-Identifier: MIT

// Package planner generates `scheduler.Plan`-shaped JSON from a
// natural-language intent by asking the configured Provider to
// emit a DAG. The output is the same JSON shape `agt plan <file>`
// already executes (handlePlan in kernel/controlplane), so
// running a generated plan is identical to running a hand-authored
// one — same node types, same scheduler, same audit trail.
//
// **Scope (M1.v).** Two node kinds: `loop` and `gate`. The
// scheduler supports both; the planner can emit either. Future
// node kinds (llm/tool/agent — SPEC-02 §4.2) will need both a
// scheduler implementation and a planner-prompt update.
//
// **No agentic-meta nonsense.** The planner is a *single*
// LLM call that returns a static DAG. It does NOT recurse into
// sub-planners, it does NOT re-plan mid-execution, and it does
// NOT call tools during planning. Those are real capabilities,
// but they're deliberately out of scope for v1 — they invite
// runaway behaviour the audit story can't keep up with.
//
// **Output validation.** We don't trust the model. After parsing
// the JSON the planner verifies:
//
//   - at least one node
//   - every node id is unique and non-empty
//   - every `deps` reference resolves
//   - no cycles (handled by the scheduler's existing topological
//     sort, but we duplicate the check here so the operator sees
//     "your planner emitted a bad DAG" rather than "scheduler
//     refused")
//   - every node kind ∈ {loop, gate}
//
// Failures return a descriptive error; the prompt asks for JSON
// in a fenced code block, and we strip the fence before parsing.
package planner
