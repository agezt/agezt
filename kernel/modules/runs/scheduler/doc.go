// SPDX-License-Identifier: MIT

// Package scheduler is the DAG layer that sits *above* the first-party
// single-agent tool-loop (DECISIONS B0d, SPEC-02 §4). A Plan is a
// directed-acyclic graph of named Nodes; the Executor walks the graph
// topologically, runs independent branches in parallel under a bounded
// worker pool, and publishes node.* + plan.* events on the bus.
//
// The package's HOME is the runs module: it lived at kernel/scheduler until
// W3.78, and the architecture gate had already placed it in this module long
// before that (tools/archcheck/layers.json). The path was the last thing out
// of place; the body, the names and the callers did not move. A plan is the
// runs module's own work unit (kernel/modules/runs/plan), and this executor
// is the half of it that walks the graph.
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
