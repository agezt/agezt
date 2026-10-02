// SPDX-License-Identifier: MIT

// Package scheduler is the DAG layer that sits *above* the first-party
// single-agent tool-loop (DECISIONS B0d, SPEC-02 §4). A Plan is a
// directed-acyclic graph of named Nodes; the Executor walks the graph
// topologically, runs independent branches in parallel under a bounded
// worker pool, and publishes node.* + plan.* events on the bus.
//
// Two Node types ship in M1.e:
//
//   - LoopNode  — wraps one agent.Run; the existing tool-loop becomes
//     a single node, so a 1-node plan is identical to
//     today's `agt run` end-to-end.
//   - GateNode  — synchronously submits an approval.Request and
//     blocks until the operator decides (or the configured
//     timeout). A deny aborts the plan; a grant releases
//     the downstream branch.
//
// Future node types (per SPEC-02 §4.2): llm, tool, agent (parallel
// sub-agent spawn), coding. The Node interface is intentionally narrow
// so they slot in without changing the executor.
//
// Determinism: given a fixed plan + journal prefix, re-execution is
// reproducible up to LLM nondeterminism (SPEC-02 §4.4 — exactly the
// same guarantee the bare tool-loop already gives). The scheduler
// adds nothing stochastic of its own.
package scheduler
